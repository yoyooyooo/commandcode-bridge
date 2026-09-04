package secret

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReadOwnerOnlyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "key")
	if err := os.WriteFile(path, []byte(" secret-value \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	value, err := ReadOwnerOnlyFile(path)
	if err != nil || value != "secret-value" {
		t.Fatalf("value=%q err=%v", value, err)
	}
}

func TestReadOwnerOnlyFileRejectsGroupReadable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "key")
	if err := os.WriteFile(path, []byte("secret-value\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadOwnerOnlyFile(path); err == nil {
		t.Fatal("expected owner-only error")
	}
}

func TestLoadPrefersEnvThenFileThenCommand(t *testing.T) {
	t.Setenv("TEST_SECRET_ENV", "from-env")
	dir := t.TempDir()
	path := filepath.Join(dir, "key")
	if err := os.WriteFile(path, []byte("from-file"), 0o600); err != nil {
		t.Fatal(err)
	}
	value, err := Load(context.Background(), Source{
		Env:     "TEST_SECRET_ENV",
		File:    path,
		Command: "printf from-command",
	})
	if err != nil || value != "from-env" {
		t.Fatalf("value=%q err=%v", value, err)
	}
}

func TestRedact(t *testing.T) {
	got := Redact("Bearer abc used abc", "abc")
	if got != "Bearer [redacted] used [redacted]" {
		t.Fatalf("got=%q", got)
	}
}
