package repository

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsUniqueViolation(t *testing.T) {
	if !isUniqueViolation(&pgconn.PgError{Code: "23505"}) {
		t.Fatalf("expected unique violation to be true")
	}
	if isUniqueViolation(&pgconn.PgError{Code: "22000"}) {
		t.Fatalf("expected non-unique pg error to be false")
	}
	if isUniqueViolation(errors.New("x")) {
		t.Fatalf("expected plain error to be false")
	}
}
