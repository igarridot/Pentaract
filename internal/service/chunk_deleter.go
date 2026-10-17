package service

import (
	"context"
	"fmt"
	"log/slog"

	"golang.org/x/sync/errgroup"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/repository"
)

// DeleteFromTelegram deletes chunk messages from Telegram for the given chunks.
func (x *ChunkDeleter) DeleteFromTelegram(ctx context.Context, storage domain.Storage, chunks []domain.FileChunk, progress *DeleteProgress) error {
	slog.Info("removing chunks from telegram", "chunks", len(chunks), "chat", storage.Name)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(DeleteParallelism)

	var fallbackWorkers []repository.WorkerToken
	if x.workersRepo != nil {
		workers, err := x.workersRepo.ListTokensByStorage(ctx, storage.ID)
		if err != nil {
			slog.Warn("failed listing workers for delete, continuing with scheduler-selected workers only", "storage_id", storage.ID, "err", err)
		} else {
			fallbackWorkers = workers
		}
	}

	var total int64
	for _, c := range chunks {
		if c.TelegramMessageID != 0 {
			total++
		}
	}
	progress.setTotalChunks(total)

	for _, c := range chunks {
		if c.TelegramMessageID == 0 {
			continue
		}
		g.Go(func() error {
			if err := x.deleteMessage(gctx, storage, c.TelegramMessageID, fallbackWorkers); err != nil {
				return err
			}
			progress.chunkDeleted()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}
	slog.Info("finished removing chunks from telegram", "chat", storage.Name)
	return nil
}

// deleteMessage removes one chunk message, trying the scheduled worker first
// and every other worker of the storage after it.
func (x *ChunkDeleter) deleteMessage(ctx context.Context, storage domain.Storage, messageID int64, fallbackWorkers []repository.WorkerToken) error {
	wt, err := x.scheduler.GetToken(ctx, storage.ID)
	if err != nil {
		return fmt.Errorf("getting token for delete message %d: %w", messageID, err)
	}
	workers := appendUniqueWorker(make([]repository.WorkerToken, 0, len(fallbackWorkers)+1), *wt)
	for _, candidate := range fallbackWorkers {
		workers = appendUniqueWorker(workers, candidate)
	}
	if len(workers) == 0 {
		return fmt.Errorf("deleting message %d: no workers available", messageID)
	}

	var lastErr error
	for attempt, candidate := range workers {
		if attempt == 0 {
			slog.Info("deleting message", "message_id", messageID, "worker", candidate.Name, "chat", storage.Name)
		} else {
			slog.Warn("retrying message delete via fallback worker", "message_id", messageID, "worker", candidate.Name, "chat", storage.Name)
		}
		if lastErr = x.tgClient.DeleteMessage(ctx, candidate.Token, storage.ChatID, messageID); lastErr == nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}

	return fmt.Errorf("deleting message %d failed after trying %d workers: %w", messageID, len(workers), lastErr)
}
