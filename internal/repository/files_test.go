package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v3"

	"github.com/Dominux/Pentaract/internal/domain"
)

func newFilesRepoMock(t *testing.T) (pgxmock.PgxPoolIface, *FilesRepo) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	return mock, NewFilesRepo(mock)
}

func TestFilesRepoCreateMarkGetDelete(t *testing.T) {
	mock, repo := newFilesRepoMock(t)
	defer mock.Close()
	ctx := context.Background()

	storageID := uuid.New()
	fileID := uuid.New()

	mock.ExpectQuery("WITH existing AS").
		WithArgs(storageID, "dir/", "video", ".mkv", int64(12)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "path", "size", "storage_id", "is_uploaded"}).
			AddRow(fileID, "dir/video.mkv", int64(12), storageID, false))
	f, err := repo.CreateFileAnyway(ctx, "dir/video.mkv", 12, storageID)
	if err != nil || f.ID != fileID {
		t.Fatalf("create file anyway failed: file=%+v err=%v", f, err)
	}

	mock.ExpectExec("UPDATE files SET is_uploaded = true WHERE id = \\$1").
		WithArgs(fileID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	if err := repo.MarkUploaded(ctx, fileID); err != nil {
		t.Fatalf("mark uploaded failed: %v", err)
	}

	mock.ExpectQuery("SELECT id, path, size, storage_id, is_uploaded FROM files WHERE storage_id = \\$1 AND path = \\$2").
		WithArgs(storageID, "dir/video.mkv").
		WillReturnRows(pgxmock.NewRows([]string{"id", "path", "size", "storage_id", "is_uploaded"}).
			AddRow(fileID, "dir/video.mkv", int64(12), storageID, true))
	got, err := repo.GetByPath(ctx, storageID, "dir/video.mkv")
	if err != nil || got.Path != "dir/video.mkv" {
		t.Fatalf("get by path failed: file=%+v err=%v", got, err)
	}

	mock.ExpectExec("DELETE FROM files WHERE storage_id = \\$1 AND \\(path = \\$2 OR path LIKE \\$3\\)").
		WithArgs(storageID, "dir/video.mkv", "dir/video.mkv/%").
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	if err := repo.Delete(ctx, storageID, "dir/video.mkv"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
}

func TestFilesRepoNotFoundCases(t *testing.T) {
	mock, repo := newFilesRepoMock(t)
	defer mock.Close()
	ctx := context.Background()

	storageID := uuid.New()

	mock.ExpectQuery("SELECT id, path, size, storage_id, is_uploaded FROM files WHERE storage_id = \\$1 AND path = \\$2").
		WithArgs(storageID, "missing").
		WillReturnError(pgx.ErrNoRows)
	if _, err := repo.GetByPath(ctx, storageID, "missing"); err == nil || err.Error() != "file not found" {
		t.Fatalf("expected file not found, got: %v", err)
	}

	mock.ExpectExec("DELETE FROM files WHERE storage_id = \\$1 AND \\(path = \\$2 OR path LIKE \\$3\\)").
		WithArgs(storageID, "missing", "missing/%").
		WillReturnResult(pgxmock.NewResult("DELETE", 0))
	if err := repo.Delete(ctx, storageID, "missing"); err == nil || err.Error() != "file not found" {
		t.Fatalf("expected file not found from delete, got: %v", err)
	}
}

func TestFilesRepoCreateFileIfNotExists(t *testing.T) {
	mock, repo := newFilesRepoMock(t)
	defer mock.Close()
	ctx := context.Background()
	storageID := uuid.New()
	fileID := uuid.New()

	mock.ExpectQuery("INSERT INTO files \\(path, size, storage_id, is_uploaded\\)").
		WithArgs("docs/a.txt", int64(4), storageID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "path", "size", "storage_id", "is_uploaded"}).
			AddRow(fileID, "docs/a.txt", int64(4), storageID, false))
	f, skipped, err := repo.CreateFileIfNotExists(ctx, "docs/a.txt", 4, storageID)
	if err != nil || skipped || f == nil || f.ID != fileID {
		t.Fatalf("expected created file, got file=%+v skipped=%v err=%v", f, skipped, err)
	}

	mock.ExpectQuery("INSERT INTO files \\(path, size, storage_id, is_uploaded\\)").
		WithArgs("docs/a.txt", int64(4), storageID).
		WillReturnError(pgx.ErrNoRows)
	f, skipped, err = repo.CreateFileIfNotExists(ctx, "docs/a.txt", 4, storageID)
	if err != nil || !skipped || f != nil {
		t.Fatalf("expected skipped duplicate, got file=%+v skipped=%v err=%v", f, skipped, err)
	}
}

func TestFilesRepoListDirAndSearch(t *testing.T) {
	mock, repo := newFilesRepoMock(t)
	defer mock.Close()
	ctx := context.Background()

	storageID := uuid.New()
	mock.ExpectQuery("SELECT DISTINCT ON \\(name\\)").
		WithArgs(storageID, "docs/").
		WillReturnRows(pgxmock.NewRows([]string{"path", "name", "size", "is_file"}).
			AddRow("docs/report.pdf", "report.pdf", int64(9), true).
			AddRow("docs/sub/child.txt", "sub", int64(0), false).
			AddRow("docs/.folder", ".folder", int64(0), true))
	list, err := repo.ListDir(ctx, storageID, "docs")
	if err != nil {
		t.Fatalf("list dir failed: %v", err)
	}
	if len(list) != 2 || list[1].Path != "docs/sub/" {
		t.Fatalf("unexpected list dir result: %+v", list)
	}

	mock.ExpectQuery("SELECT path, size").
		WithArgs(storageID, "docs/", "%report%").
		WillReturnRows(pgxmock.NewRows([]string{"path", "size"}).
			AddRow("docs/report.pdf", int64(9)).
			AddRow("docs/sub/report-2.pdf", int64(10)))
	search, err := repo.Search(ctx, storageID, "docs", "report")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(search) != 2 || search[1].Name != "report-2.pdf" {
		t.Fatalf("unexpected search result: %+v", search)
	}
}

func TestFilesRepoChunksFlow(t *testing.T) {
	mock, repo := newFilesRepoMock(t)
	defer mock.Close()
	ctx := context.Background()

	fileID := uuid.New()
	storageID := uuid.New()
	chunkID := uuid.New()

	if err := repo.CreateChunks(ctx, nil); err != nil {
		t.Fatalf("empty create chunks should be nil, got: %v", err)
	}

	mock.ExpectExec("INSERT INTO file_chunks").
		WithArgs(fileID, "tg-1", int64(111), int16(0), fileID, "tg-2", int64(112), int16(1)).
		WillReturnResult(pgxmock.NewResult("INSERT", 2))
	err := repo.CreateChunks(ctx, []domain.FileChunk{
		{FileID: fileID, TelegramFileID: "tg-1", TelegramMessageID: 111, Position: 0},
		{FileID: fileID, TelegramFileID: "tg-2", TelegramMessageID: 112, Position: 1},
	})
	if err != nil {
		t.Fatalf("create chunks failed: %v", err)
	}

	rows := pgxmock.NewRows([]string{"id", "file_id", "telegram_file_id", "telegram_message_id", "position"}).
		AddRow(chunkID, fileID, "tg-1", int64(111), int16(0))
	mock.ExpectQuery("SELECT id, file_id, telegram_file_id, telegram_message_id, position FROM file_chunks WHERE file_id = \\$1 ORDER BY position").
		WithArgs(fileID).
		WillReturnRows(rows)
	chunks, err := repo.ListChunks(ctx, fileID)
	if err != nil || len(chunks) != 1 {
		t.Fatalf("list chunks failed: chunks=%+v err=%v", chunks, err)
	}

	mock.ExpectQuery("SELECT fc.id, fc.file_id, fc.telegram_file_id, fc.telegram_message_id, fc.position").
		WithArgs(storageID, "docs", "docs/%").
		WillReturnRows(pgxmock.NewRows([]string{"id", "file_id", "telegram_file_id", "telegram_message_id", "position"}).
			AddRow(chunkID, fileID, "tg-1", int64(111), int16(0)))
	if _, err := repo.ListChunksByPath(ctx, storageID, "docs"); err != nil {
		t.Fatalf("list chunks by path failed: %v", err)
	}

	mock.ExpectQuery("SELECT fc.id, fc.file_id, fc.telegram_file_id, fc.telegram_message_id, fc.position").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "file_id", "telegram_file_id", "telegram_message_id", "position"}).
			AddRow(chunkID, fileID, "tg-1", int64(111), int16(0)))
	if _, err := repo.ListChunksByStorage(ctx, storageID); err != nil {
		t.Fatalf("list chunks by storage failed: %v", err)
	}

	mock.ExpectExec("UPDATE file_chunks SET telegram_file_id = \\$2 WHERE id = \\$1").
		WithArgs(chunkID, "tg-updated").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	if err := repo.UpdateChunkTelegramFileID(ctx, chunkID, "tg-updated"); err != nil {
		t.Fatalf("update chunk telegram id failed: %v", err)
	}
}

func TestFilesRepoCreateChunksAndMarkUploaded(t *testing.T) {
	mock, repo := newFilesRepoMock(t)
	defer mock.Close()
	ctx := context.Background()

	fileID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO file_chunks").
		WithArgs(fileID, "tg-1", int64(111), int16(0)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("UPDATE files SET is_uploaded = true WHERE id = \\$1").
		WithArgs(fileID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()

	err := repo.CreateChunksAndMarkUploaded(ctx, fileID, []domain.FileChunk{
		{FileID: fileID, TelegramFileID: "tg-1", TelegramMessageID: 111, Position: 0},
	})
	if err != nil {
		t.Fatalf("create chunks and mark uploaded failed: %v", err)
	}
}

func TestFilesRepoListUnderDirStatsMoveAndFolder(t *testing.T) {
	mock, repo := newFilesRepoMock(t)
	defer mock.Close()
	ctx := context.Background()

	storageID := uuid.New()
	fileID := uuid.New()

	mock.ExpectQuery("SELECT id, path, size, storage_id, is_uploaded FROM files").
		WithArgs(storageID, "docs/").
		WillReturnRows(pgxmock.NewRows([]string{"id", "path", "size", "storage_id", "is_uploaded"}).
			AddRow(fileID, "docs/report.pdf", int64(9), storageID, true))
	files, err := repo.ListFilesUnderPath(ctx, storageID, "docs")
	if err != nil || len(files) != 1 {
		t.Fatalf("list files under path failed: files=%+v err=%v", files, err)
	}

	mock.ExpectQuery("SELECT\\s+COALESCE\\(\\(SELECT SUM\\(f.size\\)").
		WithArgs(storageID, "docs/%").
		WillReturnRows(pgxmock.NewRows([]string{"total_bytes", "total_chunks"}).AddRow(int64(100), int64(4)))
	bytes, chunks, err := repo.DirStats(ctx, storageID, "docs")
	if err != nil || bytes != 100 || chunks != 4 {
		t.Fatalf("dir stats failed: bytes=%d chunks=%d err=%v", bytes, chunks, err)
	}

	mock.ExpectExec("UPDATE files SET path = \\$3 WHERE storage_id = \\$1 AND path = \\$2").
		WithArgs(storageID, "old.txt", "new.txt").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	if err := repo.Move(ctx, storageID, "old.txt", "new.txt"); err != nil {
		t.Fatalf("move file failed: %v", err)
	}

	mock.ExpectExec("UPDATE files SET path = \\$3 WHERE storage_id = \\$1 AND path = \\$2").
		WithArgs(storageID, "folder", "new-folder").
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	mock.ExpectExec("UPDATE files SET path = \\$3 \\|\\| substring\\(path from length\\(\\$2\\) \\+ 1\\)").
		WithArgs(storageID, "folder", "new-folder").
		WillReturnResult(pgxmock.NewResult("UPDATE", 2))
	if err := repo.Move(ctx, storageID, "folder", "new-folder"); err != nil {
		t.Fatalf("move folder failed: %v", err)
	}

	mock.ExpectExec("UPDATE files SET path = \\$3 WHERE storage_id = \\$1 AND path = \\$2").
		WithArgs(storageID, "absent", "target").
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	mock.ExpectExec("UPDATE files SET path = \\$3 \\|\\| substring\\(path from length\\(\\$2\\) \\+ 1\\)").
		WithArgs(storageID, "absent", "target").
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	if err := repo.Move(ctx, storageID, "absent", "target"); err == nil || err.Error() != "file not found" {
		t.Fatalf("expected move not found, got: %v", err)
	}

	mock.ExpectQuery("SELECT EXISTS\\(SELECT 1 FROM files WHERE storage_id = \\$1 AND path LIKE \\$2 \\|\\| '%'\\)").
		WithArgs(storageID, "existing/").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	if err := repo.CreateFolder(ctx, storageID, "existing"); err == nil || err.Error() != "folder already exists" {
		t.Fatalf("expected already exists, got: %v", err)
	}

	mock.ExpectQuery("SELECT EXISTS\\(SELECT 1 FROM files WHERE storage_id = \\$1 AND path LIKE \\$2 \\|\\| '%'\\)").
		WithArgs(storageID, "new-folder/").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec("INSERT INTO files \\(path, size, storage_id, is_uploaded\\) VALUES \\(\\$1, 0, \\$2, true\\)").
		WithArgs("new-folder/.folder", storageID).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := repo.CreateFolder(ctx, storageID, "new-folder"); err != nil {
		t.Fatalf("create folder failed: %v", err)
	}
}
