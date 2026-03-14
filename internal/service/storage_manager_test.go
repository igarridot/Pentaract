package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v3"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/repository"
	"github.com/Dominux/Pentaract/internal/telegram"
)

type fakeManagerSchedulerRepo struct {
	getTokenFn        func(ctx context.Context, storageID uuid.UUID, rateLimit int) (*repository.WorkerToken, error)
	nextAvailableInFn func(ctx context.Context, storageID uuid.UUID, rateLimit int) (time.Duration, error)
}

func (f *fakeManagerSchedulerRepo) GetToken(ctx context.Context, storageID uuid.UUID, rateLimit int) (*repository.WorkerToken, error) {
	if f.getTokenFn == nil {
		return &repository.WorkerToken{Token: "TOKEN", Name: "w1"}, nil
	}
	return f.getTokenFn(ctx, storageID, rateLimit)
}

func (f *fakeManagerSchedulerRepo) NextAvailableIn(ctx context.Context, storageID uuid.UUID, rateLimit int) (time.Duration, error) {
	if f.nextAvailableInFn == nil {
		return 0, nil
	}
	return f.nextAvailableInFn(ctx, storageID, rateLimit)
}

func TestStorageManagerDownloadToWriter(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	fileID := uuid.New()
	storageID := uuid.New()
	chunkID := uuid.New()
	plain := []byte("hello")
	cipher := NewChunkCipher("secret")
	enc, err := cipher.EncryptChunk(fileID, 0, plain)
	if err != nil {
		t.Fatalf("encrypt chunk: %v", err)
	}

	mock.ExpectQuery("SELECT id, file_id, telegram_file_id, telegram_message_id, position FROM file_chunks WHERE file_id = \\$1 ORDER BY position").
		WithArgs(fileID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "file_id", "telegram_file_id", "telegram_message_id", "position"}).
			AddRow(chunkID, fileID, "TG_FILE_ID", int64(0), int16(0)))
	mock.ExpectQuery("SELECT id, name, chat_id FROM storages WHERE id = \\$1").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "chat_id"}).AddRow(storageID, "Main", int64(123)))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/chunk.bin"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(enc)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := &StorageManager{
		filesRepo:    repository.NewFilesRepoWithDB(mock),
		storagesRepo: repository.NewStoragesRepoWithDB(mock),
		scheduler:    NewWorkerSchedulerWithRepo(&fakeManagerSchedulerRepo{}, 1),
		tgClient:     telegram.NewClient(srv.URL),
		chunkCipher:  cipher,
	}

	var out bytes.Buffer
	progress := &DownloadProgress{}
	err = m.DownloadToWriter(context.Background(), &domain.File{ID: fileID, Path: "a.txt", Size: int64(len(plain)), StorageID: storageID}, &out, progress)
	if err != nil {
		t.Fatalf("download to writer failed: %v", err)
	}
	if out.String() != "hello" {
		t.Fatalf("unexpected download content: %q", out.String())
	}
	if progress.DownloadedChunks.Load() != 1 || progress.DownloadedBytes.Load() != int64(len(plain)) {
		t.Fatalf("unexpected progress: chunks=%d bytes=%d", progress.DownloadedChunks.Load(), progress.DownloadedBytes.Load())
	}
}

func TestStorageManagerExactFileSizeAndRange(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	fileID := uuid.New()
	storageID := uuid.New()
	chunkID := uuid.New()
	plain := []byte("hello world")
	cipher := NewChunkCipher("secret")
	enc, err := cipher.EncryptChunk(fileID, 0, plain)
	if err != nil {
		t.Fatalf("encrypt chunk: %v", err)
	}

	// ExactFileSize
	mock.ExpectQuery("SELECT id, file_id, telegram_file_id, telegram_message_id, position FROM file_chunks WHERE file_id = \\$1 ORDER BY position").
		WithArgs(fileID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "file_id", "telegram_file_id", "telegram_message_id", "position"}).
			AddRow(chunkID, fileID, "TG_FILE_ID", int64(0), int16(0)))
	mock.ExpectQuery("SELECT id, name, chat_id FROM storages WHERE id = \\$1").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "chat_id"}).AddRow(storageID, "Main", int64(123)))

	// DownloadRangeToWriter
	mock.ExpectQuery("SELECT id, file_id, telegram_file_id, telegram_message_id, position FROM file_chunks WHERE file_id = \\$1 ORDER BY position").
		WithArgs(fileID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "file_id", "telegram_file_id", "telegram_message_id", "position"}).
			AddRow(chunkID, fileID, "TG_FILE_ID", int64(0), int16(0)))
	mock.ExpectQuery("SELECT id, name, chat_id FROM storages WHERE id = \\$1").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "chat_id"}).AddRow(storageID, "Main", int64(123)))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/chunk.bin"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(enc)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := &StorageManager{
		filesRepo:    repository.NewFilesRepoWithDB(mock),
		storagesRepo: repository.NewStoragesRepoWithDB(mock),
		scheduler:    NewWorkerSchedulerWithRepo(&fakeManagerSchedulerRepo{}, 1),
		tgClient:     telegram.NewClient(srv.URL),
		chunkCipher:  cipher,
	}

	size, err := m.ExactFileSize(context.Background(), &domain.File{ID: fileID, StorageID: storageID})
	if err != nil || size != int64(len(plain)) {
		t.Fatalf("exact file size failed: size=%d err=%v", size, err)
	}

	var out bytes.Buffer
	progress := &DownloadProgress{}
	err = m.DownloadRangeToWriter(context.Background(), &domain.File{ID: fileID, StorageID: storageID}, &out, 6, 10, int64(len(plain)), progress)
	if err != nil {
		t.Fatalf("download range failed: %v", err)
	}
	if out.String() != "world" {
		t.Fatalf("unexpected range output: %q", out.String())
	}
}

func TestStorageManagerUploadAndDeleteFromTelegram(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	fileID := uuid.New()
	storageID := uuid.New()
	var uploadedChunk []byte

	mock.ExpectQuery("SELECT id, name, chat_id FROM storages WHERE id = \\$1").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "chat_id"}).AddRow(storageID, "Main", int64(123)))
	mock.ExpectExec("INSERT INTO file_chunks").
		WithArgs(fileID, "TG_FILE", int64(77), int16(0)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("UPDATE files SET is_uploaded = true WHERE id = \\$1").
		WithArgs(fileID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/sendDocument"):
			mr, err := r.MultipartReader()
			if err != nil {
				t.Fatalf("multipart reader: %v", err)
			}
			for {
				part, err := mr.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("next multipart part: %v", err)
				}
				if part.FormName() == "document" {
					uploadedChunk, err = io.ReadAll(part)
					if err != nil {
						t.Fatalf("read uploaded chunk: %v", err)
					}
				}
				part.Close()
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":77,"document":{"file_id":"TG_FILE"}}}`))
		case strings.Contains(r.URL.Path, "/deleteMessage"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := &StorageManager{
		filesRepo:    repository.NewFilesRepoWithDB(mock),
		storagesRepo: repository.NewStoragesRepoWithDB(mock),
		scheduler:    NewWorkerSchedulerWithRepo(&fakeManagerSchedulerRepo{}, 1),
		tgClient:     telegram.NewClient(srv.URL),
		chunkCipher:  NewChunkCipher("secret"),
	}

	progress := &UploadProgress{TotalBytes: 3}
	err = m.Upload(context.Background(), &domain.File{ID: fileID, Path: "a.txt", Size: 3, StorageID: storageID}, strings.NewReader("abc"), progress)
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	if progress.UploadedChunks.Load() != 1 {
		t.Fatalf("unexpected uploaded chunks: %d", progress.UploadedChunks.Load())
	}
	if bytes.Equal(uploadedChunk, []byte("abc")) {
		t.Fatalf("uploaded chunk should be encrypted")
	}
	if !bytes.HasPrefix(uploadedChunk, chunkCipherMagic) {
		t.Fatalf("uploaded chunk should include encrypted payload magic")
	}
	if len(uploadedChunk) > maxTelegramGetFileBytes {
		t.Fatalf("uploaded encrypted chunk exceeds getFile limit: %d", len(uploadedChunk))
	}
	decrypted, err := m.chunkCipher.DecryptChunk(fileID, 0, uploadedChunk)
	if err != nil {
		t.Fatalf("decrypt uploaded chunk failed: %v", err)
	}
	if string(decrypted) != "abc" {
		t.Fatalf("unexpected decrypted upload payload: %q", decrypted)
	}

	delProgress := &DeleteProgress{}
	err = m.DeleteFromTelegram(context.Background(), domain.Storage{ID: storageID, Name: "Main", ChatID: 123}, []domain.FileChunk{
		{TelegramMessageID: 0},
		{TelegramMessageID: 77},
	}, delProgress)
	if err != nil {
		t.Fatalf("delete from telegram failed: %v", err)
	}
	if delProgress.TotalChunks != 1 || delProgress.DeletedChunks.Load() != 1 {
		t.Fatalf("unexpected delete progress: total=%d deleted=%d", delProgress.TotalChunks, delProgress.DeletedChunks.Load())
	}
}

func TestStorageManagerRangeValidation(t *testing.T) {
	m := &StorageManager{}
	err := m.DownloadRangeToWriter(context.Background(), &domain.File{}, io.Discard, 10, 5, 20, nil)
	if err == nil {
		t.Fatalf("expected invalid range error")
	}
}

func TestValidateEncryptedChunkSize(t *testing.T) {
	cipher := NewChunkCipher("secret")
	fileID := uuid.New()
	plain := bytes.Repeat([]byte("a"), uploadChunkSize)

	enc, err := cipher.EncryptChunk(fileID, 0, plain)
	if err != nil {
		t.Fatalf("encrypt chunk: %v", err)
	}
	if len(enc) > maxTelegramGetFileBytes {
		t.Fatalf("encrypted chunk exceeds getFile limit: %d", len(enc))
	}
	if err := validateEncryptedChunkSize(enc); err != nil {
		t.Fatalf("expected encrypted chunk to be accepted, got: %v", err)
	}

	tooLarge := make([]byte, maxTelegramGetFileBytes+1)
	if err := validateEncryptedChunkSize(tooLarge); err == nil {
		t.Fatalf("expected oversized encrypted chunk to be rejected")
	}
}

func TestStorageManagerDownloadChunkWithWorkerRecoversByMessage(t *testing.T) {
	filesMock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer filesMock.Close()

	chunkID := uuid.New()
	storageID := uuid.New()
	filesMock.ExpectExec("UPDATE file_chunks SET telegram_file_id = \\$2 WHERE id = \\$1").
		WithArgs(chunkID, "NEW_FILE_ID").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/botTOKEN/getFile") && strings.Contains(r.URL.RawQuery, "file_id=OLD_FILE_ID"):
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: wrong file identifier/HTTP URL specified"}`))
		case strings.Contains(r.URL.Path, "/botTOKEN/forwardMessage"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":88,"document":{"file_id":"NEW_FILE_ID"}}}`))
		case strings.Contains(r.URL.Path, "/botTOKEN/deleteMessage"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		case strings.Contains(r.URL.Path, "/botTOKEN/getFile") && strings.Contains(r.URL.RawQuery, "file_id=NEW_FILE_ID"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/chunk.bin"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("raw-bytes"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := &StorageManager{
		filesRepo:   repository.NewFilesRepoWithDB(filesMock),
		tgClient:    telegram.NewClient(srv.URL),
		chunkCipher: NewChunkCipher("secret"),
	}

	data, err := m.downloadChunkWithWorker(context.Background(), domain.Storage{ID: storageID, ChatID: 123}, domain.FileChunk{
		ID:                chunkID,
		TelegramFileID:    "OLD_FILE_ID",
		TelegramMessageID: 77,
		Position:          0,
	}, repository.WorkerToken{Token: "TOKEN", Name: "w1"})
	if err != nil {
		t.Fatalf("download chunk with worker failed: %v", err)
	}
	if string(data) != "raw-bytes" {
		t.Fatalf("unexpected chunk bytes: %q", string(data))
	}
}

func TestStorageManagerDownloadChunkFallbackWorker(t *testing.T) {
	workersMock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer workersMock.Close()

	storageID := uuid.New()
	workersMock.ExpectQuery("SELECT token, name FROM storage_workers").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"token", "name"}).
			AddRow("TOKEN", "w1").
			AddRow("TOKEN2", "w2"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/botTOKEN/getFile"):
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: wrong file identifier/HTTP URL specified"}`))
		case strings.Contains(r.URL.Path, "/botTOKEN2/getFile"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN2/path/chunk.bin"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok-from-fallback"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := &StorageManager{
		workersRepo: repository.NewStorageWorkersRepoWithDB(workersMock),
		scheduler: NewWorkerSchedulerWithRepo(&fakeManagerSchedulerRepo{
			getTokenFn: func(ctx context.Context, storageID uuid.UUID, rateLimit int) (*repository.WorkerToken, error) {
				return &repository.WorkerToken{Token: "TOKEN", Name: "w1"}, nil
			},
		}, 1),
		tgClient:    telegram.NewClient(srv.URL),
		chunkCipher: NewChunkCipher("secret"),
	}

	data, err := m.downloadChunk(context.Background(), domain.Storage{ID: storageID, ChatID: 123}, domain.FileChunk{
		TelegramFileID:    "FILE_ID",
		TelegramMessageID: 0,
		Position:          0,
	})
	if err != nil {
		t.Fatalf("download chunk fallback failed: %v", err)
	}
	if string(data) != "ok-from-fallback" {
		t.Fatalf("unexpected fallback content: %q", string(data))
	}
}

func TestStorageManagerDownloadAndDecryptChunkTooBig(t *testing.T) {
	workersMock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer workersMock.Close()

	storageID := uuid.New()
	fileID := uuid.New()
	workersMock.ExpectQuery("SELECT token, name FROM storage_workers").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"token", "name"}).AddRow("TOKEN", "w1"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/getFile") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: file is too big"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	m := &StorageManager{
		workersRepo: repository.NewStorageWorkersRepoWithDB(workersMock),
		scheduler: NewWorkerSchedulerWithRepo(&fakeManagerSchedulerRepo{
			getTokenFn: func(ctx context.Context, storageID uuid.UUID, rateLimit int) (*repository.WorkerToken, error) {
				return &repository.WorkerToken{Token: "TOKEN", Name: "w1"}, nil
			},
		}, 1),
		tgClient:    telegram.NewClient(srv.URL),
		chunkCipher: NewChunkCipher("secret"),
	}

	_, err = m.downloadAndDecryptChunk(context.Background(), fileID, domain.Storage{ID: storageID, ChatID: 123}, domain.FileChunk{
		TelegramFileID: "FILE_ID",
		Position:       0,
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds Bot API download limit") {
		t.Fatalf("expected file-too-big error, got: %v", err)
	}
}
