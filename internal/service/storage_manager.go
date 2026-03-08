package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/errgroup"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/repository"
	"github.com/Dominux/Pentaract/internal/telegram"
)

const chunkSize = 20 * 1024 * 1024 // 20MB

type StorageManager struct {
	filesRepo    *repository.FilesRepo
	storagesRepo *repository.StoragesRepo
	scheduler    *WorkerScheduler
	tgClient     *telegram.Client
}

func NewStorageManager(
	filesRepo *repository.FilesRepo,
	storagesRepo *repository.StoragesRepo,
	scheduler *WorkerScheduler,
	tgClient *telegram.Client,
) *StorageManager {
	return &StorageManager{
		filesRepo:    filesRepo,
		storagesRepo: storagesRepo,
		scheduler:    scheduler,
		tgClient:     tgClient,
	}
}

// UploadProgress tracks chunk upload progress.
type UploadProgress struct {
	TotalChunks    int32
	UploadedChunks atomic.Int32
	TotalBytes     int64
	UploadedBytes  atomic.Int64
}

// DownloadProgress tracks chunk download progress.
type DownloadProgress struct {
	TotalChunks      int64
	DownloadedChunks atomic.Int64
	TotalBytes       int64
	DownloadedBytes  atomic.Int64
}

// Upload reads from reader chunk by chunk (streaming), uploads to Telegram in parallel,
// and records chunk metadata in the DB. Never holds the full file in memory.
func (m *StorageManager) Upload(ctx context.Context, file *domain.File, reader io.Reader, progress *UploadProgress) error {
	storage, err := m.storagesRepo.GetByID(ctx, file.StorageID)
	if err != nil {
		return fmt.Errorf("getting storage: %w", err)
	}

	log.Printf("[upload] starting file=%s storage=%s (chat=%s)", file.Path, storage.Name, storage.Name)

	// Calculate total chunks from file size if known
	if progress != nil && progress.TotalBytes > 0 {
		total := int32(progress.TotalBytes / chunkSize)
		if progress.TotalBytes%chunkSize != 0 {
			total++
		}
		progress.TotalChunks = total
	}

	type chunkResult struct {
		TelegramFileID    string
		TelegramMessageID int64
		Position          int16
	}

	var mu sync.Mutex
	var results []chunkResult

	// cleanupResults deletes any chunks already uploaded to Telegram.
	cleanupResults := func() {
		mu.Lock()
		uploaded := make([]chunkResult, len(results))
		copy(uploaded, results)
		mu.Unlock()

		if len(uploaded) > 0 {
			log.Printf("[upload] cancelled/failed file=%s, cleaning up %d chunks from telegram", file.Path, len(uploaded))
			cleanupChunks := make([]domain.FileChunk, len(uploaded))
			for i, r := range uploaded {
				cleanupChunks[i] = domain.FileChunk{
					TelegramMessageID: r.TelegramMessageID,
				}
			}
			go m.DeleteFromTelegram(context.Background(), *storage, cleanupChunks)
		}
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(5)

	// Close the reader when context is cancelled so io.ReadFull unblocks.
	cancelled := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			if rc, ok := reader.(io.Closer); ok {
				rc.Close()
			}
		case <-cancelled:
		}
	}()
	defer close(cancelled)

	position := int16(0)
	var readCancelled bool
	for {
		// Check context before blocking on read
		select {
		case <-ctx.Done():
			readCancelled = true
		default:
		}
		if readCancelled {
			break
		}

		buf := make([]byte, chunkSize)
		n, readErr := io.ReadFull(reader, buf)
		if n == 0 && readErr != nil {
			// Check if the read failed due to cancellation
			if ctx.Err() != nil {
				readCancelled = true
			}
			break
		}
		chunkData := buf[:n]
		pos := position
		position++

		g.Go(func() error {
			wt, err := m.scheduler.GetToken(gctx, storage.ID)
			if err != nil {
				return fmt.Errorf("getting token for chunk %d: %w", pos, err)
			}

			log.Printf("[upload] chunk %d of file=%s via worker=%s chat=%s", pos, file.Path, wt.Name, storage.Name)

			filename := telegram.GenerateChunkFilename(file.ID, int(pos))
			result, err := m.tgClient.Upload(wt.Token, storage.ChatID, chunkData, filename)
			if err != nil {
				return fmt.Errorf("uploading chunk %d: %w", pos, err)
			}

			mu.Lock()
			results = append(results, chunkResult{
				TelegramFileID:    result.FileID,
				TelegramMessageID: result.MessageID,
				Position:          pos,
			})
			mu.Unlock()

			if progress != nil {
				progress.UploadedChunks.Add(1)
				progress.UploadedBytes.Add(int64(len(chunkData)))
			}

			return nil
		})

		if readErr != nil {
			if ctx.Err() != nil {
				readCancelled = true
			}
			break
		}
	}

	if progress != nil {
		progress.TotalChunks = int32(position)
	}

	waitErr := g.Wait()

	// If cancelled or failed, clean up uploaded chunks from Telegram
	if readCancelled || waitErr != nil {
		cleanupResults()
		if waitErr != nil {
			return waitErr
		}
		return ctx.Err()
	}

	// Create chunk records
	fileChunks := make([]domain.FileChunk, len(results))
	for i, r := range results {
		fileChunks[i] = domain.FileChunk{
			FileID:            file.ID,
			TelegramFileID:    r.TelegramFileID,
			TelegramMessageID: r.TelegramMessageID,
			Position:          r.Position,
		}
	}

	if err := m.filesRepo.CreateChunks(ctx, fileChunks); err != nil {
		cleanupResults()
		return fmt.Errorf("saving chunks: %w", err)
	}

	if err := m.filesRepo.MarkUploaded(ctx, file.ID); err != nil {
		return fmt.Errorf("marking file as uploaded: %w", err)
	}

	log.Printf("[upload] completed file=%s chunks=%d storage=%s chat=%s", file.Path, len(results), storage.Name, storage.Name)

	return nil
}

// DeleteFromTelegram deletes chunk messages from Telegram for the given chunks.
func (m *StorageManager) DeleteFromTelegram(ctx context.Context, storage domain.Storage, chunks []domain.FileChunk) {
	log.Printf("[delete] removing %d chunks from telegram chat=%s", len(chunks), storage.Name)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(5)

	for _, chunk := range chunks {
		c := chunk
		if c.TelegramMessageID == 0 {
			continue
		}
		g.Go(func() error {
			wt, err := m.scheduler.GetToken(gctx, storage.ID)
			if err != nil {
				log.Printf("[delete] WARNING: could not get token to delete message %d from chat=%s: %v", c.TelegramMessageID, storage.Name, err)
				return nil
			}
			log.Printf("[delete] deleting message %d via worker=%s chat=%s", c.TelegramMessageID, wt.Name, storage.Name)
			if err := m.tgClient.DeleteMessage(wt.Token, storage.ChatID, c.TelegramMessageID); err != nil {
				log.Printf("[delete] WARNING: could not delete message %d via worker=%s chat=%s: %v", c.TelegramMessageID, wt.Name, storage.Name, err)
			}
			return nil
		})
	}

	g.Wait()
	log.Printf("[delete] finished removing chunks from chat=%s", storage.Name)
}

// Download retrieves all chunks from Telegram in parallel and reassembles the file.
// Download returns the full file contents in memory. Only suitable for smaller files.
func (m *StorageManager) Download(ctx context.Context, file *domain.File) ([]byte, error) {
	var buf bytes.Buffer
	if err := m.DownloadToWriter(ctx, file, &buf, nil); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DownloadToWriter streams a file's chunks sequentially to the given writer.
// Chunks are downloaded one at a time in order so only one chunk (~20MB) is in memory at once.
func (m *StorageManager) DownloadToWriter(ctx context.Context, file *domain.File, w io.Writer, progress *DownloadProgress) error {
	chunks, err := m.filesRepo.ListChunks(ctx, file.ID)
	if err != nil {
		return fmt.Errorf("listing chunks: %w", err)
	}

	if len(chunks) == 0 {
		return domain.ErrNotFound("file chunks")
	}

	if progress != nil && progress.TotalChunks == 0 && progress.TotalBytes == 0 {
		progress.TotalChunks = int64(len(chunks))
		progress.TotalBytes = file.Size
	}

	storage, err := m.storagesRepo.GetByID(ctx, file.StorageID)
	if err != nil {
		return fmt.Errorf("getting storage: %w", err)
	}

	log.Printf("[download] starting file=%s chunks=%d storage=%s chat=%s", file.Path, len(chunks), storage.Name, storage.Name)

	// Chunks are already sorted by position from ListChunks.
	// Download and write sequentially to keep memory usage constant (~20MB).
	for _, chunk := range chunks {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		wt, err := m.scheduler.GetToken(ctx, storage.ID)
		if err != nil {
			return fmt.Errorf("getting token for chunk %d: %w", chunk.Position, err)
		}

		log.Printf("[download] chunk %d of file=%s via worker=%s chat=%s", chunk.Position, file.Path, wt.Name, storage.Name)

		data, err := m.tgClient.Download(wt.Token, chunk.TelegramFileID)
		if err != nil {
			return fmt.Errorf("downloading chunk %d: %w", chunk.Position, err)
		}

		if _, err := w.Write(data); err != nil {
			return fmt.Errorf("writing chunk %d: %w", chunk.Position, err)
		}

		if progress != nil {
			progress.DownloadedChunks.Add(1)
			progress.DownloadedBytes.Add(int64(len(data)))
		}
	}

	log.Printf("[download] completed file=%s storage=%s chat=%s", file.Path, storage.Name, storage.Name)
	return nil
}
