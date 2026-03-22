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

	// Pipeline verification tuning
	PipelineVerifyParallelism = 5                // concurrent verifications during upload (runs alongside uploads)
	VerifyCBFailureThreshold  = 3                // consecutive transient failures to trip the circuit breaker
	VerifyCBCooldownDuration  = 30 * time.Second // pause duration before retrying after breaker trips
	VerifyCBMaxRetryRounds    = 3                // max cooldown+retry rounds for transiently failed chunks

	// Delete tuning
	DeleteParallelism = 5

	// Handler-level timing
	TrackerCleanupDelay      = 5 * time.Minute
	SSEPollingInterval       = 500 * time.Millisecond
	DownloadProgressWaitTime = 15 * time.Second
)
