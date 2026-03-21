package service

import "time"

const (
	// Telegram Bot API limits
	MaxTelegramGetFileBytes = 20 * 1024 * 1024
	UploadChunkSafetyMargin = 64 * 1024 // keep encrypted chunk comfortably below 20MB
	UploadChunkSize         = MaxTelegramGetFileBytes - UploadChunkSafetyMargin

	// Upload tuning
	UploadChunkMaxAttempts    = 5
	UploadChunkParallelism    = 10
	TokenBatchSize            = 5 // S2: pre-fetch multiple worker tokens per DB query

	// Download tuning
	DownloadChunkMaxAttempts  = 3
	DownloadChunkParallelism  = 5
	VerifyChunkParallelism    = 10

	// Delete tuning
	DeleteParallelism = 5

	// Handler-level timing
	TrackerCleanupDelay      = 5 * time.Minute
	SSEPollingInterval       = 500 * time.Millisecond
	DownloadProgressWaitTime = 15 * time.Second
)
