package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
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
}

// Upload reads from reader chunk by chunk (streaming), uploads to Telegram in parallel,
// and records chunk metadata in the DB. Never holds the full file in memory.
func (m *StorageManager) Upload(ctx context.Context, file *domain.File, reader io.Reader, progress *UploadProgress) error {
	storage, err := m.storagesRepo.GetByID(ctx, file.StorageID)
	if err != nil {
		return fmt.Errorf("getting storage: %w", err)
	}

	type chunkResult struct {
		TelegramFileID string
		Position       int16
	}

	var mu sync.Mutex
	var results []chunkResult

	g, gctx := errgroup.WithContext(ctx)
	// Limit parallel uploads to avoid excessive memory (N chunks * 20MB in flight)
	g.SetLimit(5)

	position := int16(0)
	for {
		buf := make([]byte, chunkSize)
		n, readErr := io.ReadFull(reader, buf)
		if n == 0 && readErr != nil {
			break
		}
		chunkData := buf[:n]
		pos := position
		position++

		g.Go(func() error {
			token, err := m.scheduler.GetToken(gctx, storage.ID)
			if err != nil {
				return fmt.Errorf("getting token for chunk %d: %w", pos, err)
			}

			filename := telegram.GenerateChunkFilename(file.ID, int(pos))
			fileID, err := m.tgClient.Upload(token, storage.ChatID, chunkData, filename)
			if err != nil {
				return fmt.Errorf("uploading chunk %d: %w", pos, err)
			}

			mu.Lock()
			results = append(results, chunkResult{TelegramFileID: fileID, Position: pos})
			mu.Unlock()

			if progress != nil {
				progress.UploadedChunks.Add(1)
			}

			return nil
		})

		if readErr != nil {
			break
		}
	}

	if progress != nil {
		progress.TotalChunks = int32(position)
	}

	if err := g.Wait(); err != nil {
		return err
	}

	// Create chunk records
	fileChunks := make([]domain.FileChunk, len(results))
	for i, r := range results {
		fileChunks[i] = domain.FileChunk{
			FileID:         file.ID,
			TelegramFileID: r.TelegramFileID,
			Position:       r.Position,
		}
	}

	if err := m.filesRepo.CreateChunks(ctx, fileChunks); err != nil {
		return fmt.Errorf("saving chunks: %w", err)
	}

	if err := m.filesRepo.MarkUploaded(ctx, file.ID); err != nil {
		return fmt.Errorf("marking file as uploaded: %w", err)
	}

	return nil
}

// Download retrieves all chunks from Telegram in parallel and reassembles the file.
func (m *StorageManager) Download(ctx context.Context, file *domain.File) ([]byte, error) {
	chunks, err := m.filesRepo.ListChunks(ctx, file.ID)
	if err != nil {
		return nil, fmt.Errorf("listing chunks: %w", err)
	}

	if len(chunks) == 0 {
		return nil, domain.ErrNotFound("file chunks")
	}

	storage, err := m.storagesRepo.GetByID(ctx, file.StorageID)
	if err != nil {
		return nil, fmt.Errorf("getting storage: %w", err)
	}

	type downloadResult struct {
		Data     []byte
		Position int16
	}

	var mu sync.Mutex
	var results []downloadResult

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(5)

	for _, chunk := range chunks {
		c := chunk
		g.Go(func() error {
			token, err := m.scheduler.GetToken(gctx, storage.ID)
			if err != nil {
				return fmt.Errorf("getting token for download: %w", err)
			}

			data, err := m.tgClient.Download(token, c.TelegramFileID)
			if err != nil {
				return fmt.Errorf("downloading chunk %d: %w", c.Position, err)
			}

			mu.Lock()
			results = append(results, downloadResult{Data: data, Position: c.Position})
			mu.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Sort by position and concatenate
	sort.Slice(results, func(i, j int) bool {
		return results[i].Position < results[j].Position
	})

	var buf bytes.Buffer
	for _, r := range results {
		buf.Write(r.Data)
	}

	return buf.Bytes(), nil
}
