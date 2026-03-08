package password

import "testing"

func TestHashAndVerify(t *testing.T) {
	raw := "my-secret-pass"
	hash, err := Hash(raw)
	if err != nil {
		t.Fatalf("Hash() unexpected error: %v", err)
	}
	if hash == "" || hash == raw {
		t.Fatalf("expected non-empty hash different from raw password")
	}

	if err := Verify(raw, hash); err != nil {
		t.Fatalf("Verify() expected success, got: %v", err)
	}
	if err := Verify("wrong-pass", hash); err == nil {
		t.Fatalf("Verify() expected failure for wrong password")
	}
}
