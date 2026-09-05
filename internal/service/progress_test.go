package service

import "testing"

func TestProgressMethodsAreNilSafe(t *testing.T) {
	var up *UploadProgress
	up.expectedChunks()
	up.setTotalChunks(3)
	up.chunkUploaded(10)
	up.chunkVerified()

	var down *DownloadProgress
	down.setTotalsIfUnset(2, 20)
	down.chunkDownloaded(5)

	var del *DeleteProgress
	del.setTotalChunks(4)
	del.chunkDeleted()
}

func TestUploadProgressExpectedChunksRoundsUp(t *testing.T) {
	p := &UploadProgress{TotalBytes: UploadChunkSize*2 + 1}
	p.expectedChunks()
	if p.TotalChunks != 3 {
		t.Fatalf("expected 3 chunks, got %d", p.TotalChunks)
	}

	exact := &UploadProgress{TotalBytes: UploadChunkSize * 2}
	exact.expectedChunks()
	if exact.TotalChunks != 2 {
		t.Fatalf("expected 2 chunks, got %d", exact.TotalChunks)
	}

	unknown := &UploadProgress{}
	unknown.expectedChunks()
	if unknown.TotalChunks != 0 {
		t.Fatalf("unknown size must leave TotalChunks at 0, got %d", unknown.TotalChunks)
	}
}

func TestUploadProgressChunkUploadedCountsForVerification(t *testing.T) {
	p := &UploadProgress{}
	p.chunkUploaded(7)
	p.chunkUploaded(3)
	p.chunkVerified()
	if p.UploadedChunks.Load() != 2 || p.UploadedBytes.Load() != 10 {
		t.Fatalf("unexpected upload counters: chunks=%d bytes=%d", p.UploadedChunks.Load(), p.UploadedBytes.Load())
	}
	if p.VerificationTotalChunks.Load() != 2 || p.VerifiedChunks.Load() != 1 {
		t.Fatalf("unexpected verification counters: total=%d verified=%d", p.VerificationTotalChunks.Load(), p.VerifiedChunks.Load())
	}
}

func TestDownloadProgressSetTotalsIfUnsetKeepsArchiveTotals(t *testing.T) {
	p := &DownloadProgress{TotalChunks: 40, TotalBytes: 4000}
	p.setTotalsIfUnset(1, 10)
	if p.TotalChunks != 40 || p.TotalBytes != 4000 {
		t.Fatalf("pre-set totals must be kept, got chunks=%d bytes=%d", p.TotalChunks, p.TotalBytes)
	}

	fresh := &DownloadProgress{}
	fresh.setTotalsIfUnset(1, 10)
	fresh.chunkDownloaded(10)
	if fresh.TotalChunks != 1 || fresh.TotalBytes != 10 || fresh.DownloadedChunks.Load() != 1 || fresh.DownloadedBytes.Load() != 10 {
		t.Fatalf("unexpected fresh progress: %+v", fresh)
	}
}

func TestDeleteProgressCounters(t *testing.T) {
	p := &DeleteProgress{}
	p.setTotalChunks(2)
	p.chunkDeleted()
	if p.TotalChunks != 2 || p.DeletedChunks.Load() != 1 {
		t.Fatalf("unexpected delete progress: total=%d deleted=%d", p.TotalChunks, p.DeletedChunks.Load())
	}
}
