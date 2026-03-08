package jwt

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGenerateAndValidate(t *testing.T) {
	user := AuthUser{
		ID:    uuid.New(),
		Email: "user@example.com",
	}
	secret := "super-secret"

	token, err := Generate(user, time.Minute, secret)
	if err != nil {
		t.Fatalf("Generate() unexpected error: %v", err)
	}

	got, err := Validate(token, secret)
	if err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
	if got.ID != user.ID || got.Email != user.Email {
		t.Fatalf("Validate() mismatch: got=%+v want=%+v", got, user)
	}
}

func TestValidateRejectsWrongSecret(t *testing.T) {
	user := AuthUser{ID: uuid.New(), Email: "user@example.com"}
	token, err := Generate(user, time.Minute, "secret-a")
	if err != nil {
		t.Fatalf("Generate() unexpected error: %v", err)
	}
	if _, err := Validate(token, "secret-b"); err == nil {
		t.Fatalf("Validate() expected error for wrong secret")
	}
}

func TestValidateRejectsExpiredToken(t *testing.T) {
	user := AuthUser{ID: uuid.New(), Email: "user@example.com"}
	token, err := Generate(user, -time.Second, "secret")
	if err != nil {
		t.Fatalf("Generate() unexpected error: %v", err)
	}
	if _, err := Validate(token, "secret"); err == nil {
		t.Fatalf("Validate() expected error for expired token")
	}
}

func TestValidateRejectsMalformedToken(t *testing.T) {
	if _, err := Validate("not-a-token", "secret"); err == nil {
		t.Fatalf("Validate() expected error for malformed token")
	}
}
