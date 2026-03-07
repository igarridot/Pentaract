package domain

import (
	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
}

type Storage struct {
	ID     uuid.UUID `json:"id"`
	Name   string    `json:"name"`
	ChatID int64     `json:"chat_id"`
}

type StorageWithInfo struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	ChatID      int64     `json:"chat_id"`
	FilesAmount int64     `json:"files_amount"`
	Size        int64     `json:"size"`
}

type AccessType string

const (
	AccessRead  AccessType = "r"
	AccessWrite AccessType = "w"
	AccessAdmin AccessType = "a"
)

type Access struct {
	ID         uuid.UUID  `json:"id"`
	UserID     uuid.UUID  `json:"user_id"`
	StorageID  uuid.UUID  `json:"storage_id"`
	AccessType AccessType `json:"access_type"`
}

type UserWithAccess struct {
	ID         uuid.UUID  `json:"id"`
	Email      string     `json:"email"`
	AccessType AccessType `json:"access_type"`
}

type StorageWorker struct {
	ID        uuid.UUID  `json:"id"`
	Name      string     `json:"name"`
	UserID    uuid.UUID  `json:"user_id"`
	Token     string     `json:"token"`
	StorageID *uuid.UUID `json:"storage_id"`
}

type File struct {
	ID         uuid.UUID `json:"id"`
	Path       string    `json:"path"`
	Size       int64     `json:"size"`
	StorageID  uuid.UUID `json:"storage_id"`
	IsUploaded bool      `json:"is_uploaded"`
}

type FileChunk struct {
	ID                uuid.UUID `json:"id"`
	FileID            uuid.UUID `json:"file_id"`
	TelegramFileID    string    `json:"telegram_file_id"`
	TelegramMessageID int64     `json:"telegram_message_id"`
	Position          int16     `json:"position"`
}

type FSElement struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	IsFile bool   `json:"is_file"`
}

type SearchFSElement struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	IsFile bool   `json:"is_file"`
}
