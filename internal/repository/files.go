package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Dominux/Pentaract/internal/domain"
)

type FilesRepo struct {
	pool *pgxpool.Pool
}

func NewFilesRepo(pool *pgxpool.Pool) *FilesRepo {
	return &FilesRepo{pool: pool}
}

// CreateFileAnyway creates a file record, auto-resolving duplicate names with (1), (2), etc.
func (r *FilesRepo) CreateFileAnyway(ctx context.Context, path string, size int64, storageID uuid.UUID) (*domain.File, error) {
	// Split path into directory and filename parts
	lastSlash := strings.LastIndex(path, "/")
	var dir, filename, ext, nameWithoutExt string
	if lastSlash >= 0 {
		dir = path[:lastSlash+1]
		filename = path[lastSlash+1:]
	} else {
		dir = ""
		filename = path
	}

	// Split filename into name and extension
	dotIdx := strings.LastIndex(filename, ".")
	if dotIdx > 0 {
		nameWithoutExt = filename[:dotIdx]
		ext = filename[dotIdx:]
	} else {
		nameWithoutExt = filename
		ext = ""
	}

	f := &domain.File{}
	err := r.pool.QueryRow(ctx,
		`WITH existing AS (
			SELECT path FROM files
			WHERE storage_id = $1
			AND path ~ ('^' || regexp_quote($2) || regexp_quote($3) || '( \(\d+\))?' || regexp_quote($4) || '$')
		),
		next_num AS (
			SELECT COALESCE(
				(SELECT MIN(n) FROM generate_series(1, (SELECT COUNT(*) + 1 FROM existing)) AS n
				WHERE ($2 || $3 || ' (' || n || ')' || $4) NOT IN (SELECT path FROM existing)),
				0
			) AS num
		)
		INSERT INTO files (path, size, storage_id, is_uploaded)
		VALUES (
			CASE WHEN EXISTS(SELECT 1 FROM existing WHERE path = $2 || $3 || $4)
			THEN $2 || $3 || ' (' || (SELECT num FROM next_num) || ')' || $4
			ELSE $2 || $3 || $4
			END,
			$5, $1, false
		)
		RETURNING id, path, size, storage_id, is_uploaded`,
		storageID, dir, nameWithoutExt, ext, size,
	).Scan(&f.ID, &f.Path, &f.Size, &f.StorageID, &f.IsUploaded)

	if err != nil {
		return nil, err
	}
	return f, nil
}

func (r *FilesRepo) MarkUploaded(ctx context.Context, fileID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE files SET is_uploaded = true WHERE id = $1`,
		fileID,
	)
	return err
}

func (r *FilesRepo) GetByPath(ctx context.Context, storageID uuid.UUID, path string) (*domain.File, error) {
	f := &domain.File{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, path, size, storage_id, is_uploaded FROM files WHERE storage_id = $1 AND path = $2`,
		storageID, path,
	).Scan(&f.ID, &f.Path, &f.Size, &f.StorageID, &f.IsUploaded)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound("file")
		}
		return nil, err
	}
	return f, nil
}

// ListDir returns files and folders at the given path prefix.
func (r *FilesRepo) ListDir(ctx context.Context, storageID uuid.UUID, path string) ([]domain.FSElement, error) {
	// Ensure path ends with /
	if path != "" && !strings.HasSuffix(path, "/") {
		path += "/"
	}

	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT ON (name)
			f.path,
			CASE
				WHEN position('/' in substring(f.path from length($2) + 1)) > 0
				THEN substring(substring(f.path from length($2) + 1) from 1 for position('/' in substring(f.path from length($2) + 1)) - 1)
				ELSE substring(f.path from length($2) + 1)
			END AS name,
			CASE
				WHEN position('/' in substring(f.path from length($2) + 1)) > 0
				THEN 0
				ELSE f.size
			END AS size,
			CASE
				WHEN position('/' in substring(f.path from length($2) + 1)) > 0
				THEN false
				ELSE true
			END AS is_file
		FROM files f
		WHERE f.storage_id = $1
			AND f.path LIKE $2 || '%'
			AND f.is_uploaded = true
			AND length(f.path) > length($2)
		ORDER BY name`,
		storageID, path,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var elements []domain.FSElement
	for rows.Next() {
		var el domain.FSElement
		var fullPath string
		if err := rows.Scan(&fullPath, &el.Name, &el.Size, &el.IsFile); err != nil {
			return nil, err
		}
		// Hide .folder placeholder files
		if el.IsFile && el.Name == ".folder" {
			continue
		}
		if el.IsFile {
			el.Path = fullPath
		} else {
			el.Path = path + el.Name + "/"
		}
		elements = append(elements, el)
	}
	return elements, rows.Err()
}

func (r *FilesRepo) Search(ctx context.Context, storageID uuid.UUID, basePath, searchPath string) ([]domain.SearchFSElement, error) {
	if basePath != "" && !strings.HasSuffix(basePath, "/") {
		basePath += "/"
	}

	pattern := "%" + searchPath + "%"
	rows, err := r.pool.Query(ctx,
		`SELECT path, size
		FROM files
		WHERE storage_id = $1
			AND path LIKE $2 || '%'
			AND path LIKE $3
			AND is_uploaded = true
			AND path NOT LIKE '%/.folder'
		ORDER BY path`,
		storageID, basePath, pattern,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []domain.SearchFSElement
	for rows.Next() {
		var el domain.SearchFSElement
		if err := rows.Scan(&el.Path, &el.Size); err != nil {
			return nil, err
		}
		el.IsFile = true
		// Derive name from the last segment of the path
		if idx := strings.LastIndex(el.Path, "/"); idx >= 0 {
			el.Name = el.Path[idx+1:]
		} else {
			el.Name = el.Path
		}
		results = append(results, el)
	}
	return results, rows.Err()
}

// ListChunksByPath returns all chunks for files matching an exact path or folder prefix.
func (r *FilesRepo) ListChunksByPath(ctx context.Context, storageID uuid.UUID, path string) ([]domain.FileChunk, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT fc.id, fc.file_id, fc.telegram_file_id, fc.telegram_message_id, fc.position
		FROM file_chunks fc
		JOIN files f ON f.id = fc.file_id
		WHERE f.storage_id = $1 AND (f.path = $2 OR f.path LIKE $3)
		ORDER BY fc.file_id, fc.position`,
		storageID, path, path+"/%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []domain.FileChunk
	for rows.Next() {
		var c domain.FileChunk
		if err := rows.Scan(&c.ID, &c.FileID, &c.TelegramFileID, &c.TelegramMessageID, &c.Position); err != nil {
			return nil, err
		}
		chunks = append(chunks, c)
	}
	return chunks, rows.Err()
}

func (r *FilesRepo) Delete(ctx context.Context, storageID uuid.UUID, path string) error {
	// Delete exact file or all files under folder path
	ct, err := r.pool.Exec(ctx,
		`DELETE FROM files WHERE storage_id = $1 AND (path = $2 OR path LIKE $3)`,
		storageID, path, path+"/%",
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrNotFound("file")
	}
	return nil
}

func (r *FilesRepo) CreateChunks(ctx context.Context, chunks []domain.FileChunk) error {
	if len(chunks) == 0 {
		return nil
	}

	valueStrings := make([]string, 0, len(chunks))
	valueArgs := make([]interface{}, 0, len(chunks)*4)
	for i, c := range chunks {
		base := i * 4
		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d, $%d, $%d)", base+1, base+2, base+3, base+4))
		valueArgs = append(valueArgs, c.FileID, c.TelegramFileID, c.TelegramMessageID, c.Position)
	}

	query := `INSERT INTO file_chunks (file_id, telegram_file_id, telegram_message_id, position) VALUES ` + strings.Join(valueStrings, ", ")
	_, err := r.pool.Exec(ctx, query, valueArgs...)
	return err
}

func (r *FilesRepo) ListChunks(ctx context.Context, fileID uuid.UUID) ([]domain.FileChunk, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, file_id, telegram_file_id, telegram_message_id, position FROM file_chunks WHERE file_id = $1 ORDER BY position`,
		fileID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []domain.FileChunk
	for rows.Next() {
		var c domain.FileChunk
		if err := rows.Scan(&c.ID, &c.FileID, &c.TelegramFileID, &c.TelegramMessageID, &c.Position); err != nil {
			return nil, err
		}
		chunks = append(chunks, c)
	}
	return chunks, rows.Err()
}

// ListFilesUnderPath returns all uploaded files under a directory prefix.
func (r *FilesRepo) ListFilesUnderPath(ctx context.Context, storageID uuid.UUID, path string) ([]domain.File, error) {
	if path != "" && !strings.HasSuffix(path, "/") {
		path += "/"
	}

	rows, err := r.pool.Query(ctx,
		`SELECT id, path, size, storage_id, is_uploaded FROM files
		WHERE storage_id = $1 AND path LIKE $2 || '%' AND is_uploaded = true AND path NOT LIKE '%/.folder'
		ORDER BY path`,
		storageID, path,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []domain.File
	for rows.Next() {
		var f domain.File
		if err := rows.Scan(&f.ID, &f.Path, &f.Size, &f.StorageID, &f.IsUploaded); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// DirStats returns total uploaded file bytes and exact Telegram chunk count under a directory prefix.
func (r *FilesRepo) DirStats(ctx context.Context, storageID uuid.UUID, path string) (int64, int64, error) {
	if path != "" && !strings.HasSuffix(path, "/") {
		path += "/"
	}

	likePath := path + "%"
	var totalBytes int64
	var totalChunks int64
	err := r.pool.QueryRow(ctx,
		`SELECT
			COALESCE((SELECT SUM(f.size)
				FROM files f
				WHERE f.storage_id = $1
					AND f.path LIKE $2
					AND f.is_uploaded = true
					AND f.path NOT LIKE '%/.folder'), 0) AS total_bytes,
			COALESCE((SELECT COUNT(fc.id)
				FROM file_chunks fc
				JOIN files f ON f.id = fc.file_id
				WHERE f.storage_id = $1
					AND f.path LIKE $2
					AND f.is_uploaded = true
					AND f.path NOT LIKE '%/.folder'), 0) AS total_chunks`,
		storageID, likePath,
	).Scan(&totalBytes, &totalChunks)
	if err != nil {
		return 0, 0, err
	}

	return totalBytes, totalChunks, nil
}

// Move renames a file or moves all files under a folder prefix to a new path.
func (r *FilesRepo) Move(ctx context.Context, storageID uuid.UUID, oldPath, newPath string) error {
	// Move single file
	ct, err := r.pool.Exec(ctx,
		`UPDATE files SET path = $3 WHERE storage_id = $1 AND path = $2`,
		storageID, oldPath, newPath,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() > 0 {
		return nil
	}

	// Move folder: update all files under oldPath/
	ct, err = r.pool.Exec(ctx,
		`UPDATE files SET path = $3 || substring(path from length($2) + 1)
		WHERE storage_id = $1 AND path LIKE $2 || '/%'`,
		storageID, oldPath, newPath,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrNotFound("file")
	}
	return nil
}

// CreateFolder creates a placeholder file for a folder (size=0, is_uploaded=true).
func (r *FilesRepo) CreateFolder(ctx context.Context, storageID uuid.UUID, path string) error {
	// Check if folder already has content
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM files WHERE storage_id = $1 AND path LIKE $2 || '%')`,
		storageID, path+"/",
	).Scan(&exists)
	if err != nil {
		return err
	}
	if exists {
		return domain.ErrAlreadyExists("folder")
	}

	// Create a placeholder entry so the folder appears in listings
	_, err = r.pool.Exec(ctx,
		`INSERT INTO files (path, size, storage_id, is_uploaded) VALUES ($1, 0, $2, true)
		ON CONFLICT DO NOTHING`,
		path+"/.folder", storageID,
	)
	return err
}
