package handler

import "testing"

func TestHandlerConstructors(t *testing.T) {
	if NewAuthHandler(nil) == nil {
		t.Fatalf("NewAuthHandler returned nil")
	}
	if NewUsersHandler(nil) == nil {
		t.Fatalf("NewUsersHandler returned nil")
	}
	if NewStoragesHandler(nil) == nil {
		t.Fatalf("NewStoragesHandler returned nil")
	}
	if NewAccessHandler(nil) == nil {
		t.Fatalf("NewAccessHandler returned nil")
	}
	if NewStorageWorkersHandler(nil) == nil {
		t.Fatalf("NewStorageWorkersHandler returned nil")
	}
	if NewFilesHandler(nil) == nil {
		t.Fatalf("NewFilesHandler returned nil")
	}
}
