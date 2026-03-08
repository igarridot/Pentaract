package service

import (
	"errors"
	"testing"
)

func TestServiceConstructors(t *testing.T) {
	if NewUsersService(nil, "admin@example.com") == nil {
		t.Fatalf("NewUsersService returned nil")
	}
	if NewAuthService(nil, "secret", 3600) == nil {
		t.Fatalf("NewAuthService returned nil")
	}
	if NewAccessService(nil, nil) == nil {
		t.Fatalf("NewAccessService returned nil")
	}
	if NewStoragesService(nil, nil, nil, nil) == nil {
		t.Fatalf("NewStoragesService returned nil")
	}
	if NewStorageWorkersService(nil) == nil {
		t.Fatalf("NewStorageWorkersService returned nil")
	}
	if NewFilesService(nil, nil, &StorageManager{}) == nil {
		t.Fatalf("NewFilesService returned nil")
	}
	if NewWorkerScheduler(nil, 1) == nil {
		t.Fatalf("NewWorkerScheduler returned nil")
	}
	if NewStorageManager(nil, nil, nil, nil, nil, "secret") == nil {
		t.Fatalf("NewStorageManager returned nil")
	}
}

func TestStorageManagerHelpers(t *testing.T) {
	if isGetFileFailure(nil) {
		t.Fatalf("nil should not be getFile failure")
	}
	if !isGetFileFailure(errors.New("telegram getFile failed (status 400): bad request")) {
		t.Fatalf("expected getFile failure detection")
	}
	if isTelegramFileTooBig(nil) {
		t.Fatalf("nil should not be too-big failure")
	}
	if !isTelegramFileTooBig(errors.New("Bad Request: file is too big")) {
		t.Fatalf("expected too-big detection")
	}
}
