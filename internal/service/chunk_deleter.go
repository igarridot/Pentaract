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
func (m *StorageManager) DeleteFromTelegram(ctx context.Context, storage domain.Storage, chunks []domain.FileChunk, progress *DeleteProgress) error {
	slog.Info("removing chunks from telegram", "chunks", len(chunks), "chat", storage.Name)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(DeleteParallelism)

	fallbackWorkers := make([]repository.WorkerToken, 0)
	if m.workersRepo != nil {
		workers, err := m.workersRepo.ListTokensByStorage(ctx, storage.ID)
		if err != nil {
			slog.Warn("failed listing workers for delete, continuing with scheduler-selected workers only", "storage_id", storage.ID, "err", err)
		} else {
			fallbackWorkers = workers
		}
	}

	if progress != nil {
		total := int64(0)
		for _, c := range chunks {
			if c.TelegramMessageID != 0 {
				total++
			}
		}
		progress.TotalChunks = total
	}

	for _, c := range chunks {
		if c.TelegramMessageID == 0 {
			continue
		}
		g.Go(func() error {
			wt, err := m.scheduler.GetToken(gctx, storage.ID)
			if err != nil {
				return fmt.Errorf("getting token for delete message %d: %w", c.TelegramMessageID, err)
			}
			workers := make([]repository.WorkerToken, 0, len(fallbackWorkers)+1)
			if wt != nil {
				workers = appendUniqueWorker(workers, *wt)
			}
			for _, candidate := range fallbackWorkers {
				workers = appendUniqueWorker(workers, candidate)
			}
			if len(workers) == 0 {
				return fmt.Errorf("deleting message %d: no workers available", c.TelegramMessageID)
			}

			var lastErr error
			for attempt, candidate := range workers {
				if attempt == 0 {
					slog.Info("deleting message", "message_id", c.TelegramMessageID, "worker", candidate.Name, "chat", storage.Name)
				} else {
					slog.Warn("retrying message delete via fallback worker", "message_id", c.TelegramMessageID, "worker", candidate.Name, "chat", storage.Name)
				}
				if err := m.tgClient.DeleteMessage(gctx, candidate.Token, storage.ChatID, c.TelegramMessageID); err == nil {
					if progress != nil {
						progress.DeletedChunks.Add(1)
					}
					return nil
				} else {
					lastErr = err
				}
				if gctx.Err() != nil {
					return gctx.Err()
				}
			}

			return fmt.Errorf("deleting message %d failed after trying %d workers: %w", c.TelegramMessageID, len(workers), lastErr)
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}
	slog.Info("finished removing chunks from telegram", "chat", storage.Name)
	return nil
}
