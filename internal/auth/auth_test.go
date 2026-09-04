package auth

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestLookupAndRevoke(t *testing.T) {
	store, err := NewStore([]Client{{Name: "scripts", Key: "sk_scripts"}, {Name: "apps", Key: "sk_apps"}})
	if err != nil {
		t.Fatal(err)
	}
	client, ok := store.Lookup("sk_apps")
	if !ok || client.Name != "apps" {
		t.Fatalf("lookup=%v ok=%v", client, ok)
	}
	if !store.Revoke("apps") {
		t.Fatal("revoke failed")
	}
	if _, ok := store.Lookup("sk_apps"); ok {
		t.Fatal("revoked key still accepted")
	}
	if _, ok := store.Lookup("sk_scripts"); !ok {
		t.Fatal("unrelated key was revoked")
	}
}

func TestAuthenticate(t *testing.T) {
	store, err := NewStore([]Client{{Name: "scripts", Key: "sk_scripts"}})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, "/v1/models", nil)
	if _, err := store.Authenticate(req); err == nil {
		t.Fatal("missing key accepted")
	}
	req.Header.Set("Authorization", "Bearer wrong")
	if _, err := store.Authenticate(req); err == nil {
		t.Fatal("wrong key accepted")
	}
	req.Header.Set("Authorization", "Bearer sk_scripts")
	client, err := store.Authenticate(req)
	if err != nil || client.Name != "scripts" {
		t.Fatalf("client=%v err=%v", client, err)
	}
}

func TestLoadStoreFromOwnerOnlyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clients.json")
	if err := os.WriteFile(path, []byte(`[{"name":"scripts","key":"sk_scripts"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := LoadStore("", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Lookup("sk_scripts"); !ok {
		t.Fatal("file key not loaded")
	}
}
