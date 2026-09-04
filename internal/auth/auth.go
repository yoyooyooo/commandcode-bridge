package auth

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/yoyooyooo/commandcode-bridge/internal/secret"
)

type Client struct {
	Name string
	Key  string
}

type Store struct {
	mu      sync.RWMutex
	clients []Client
}

func NewStore(clients []Client) (*Store, error) {
	store := &Store{}
	for _, client := range clients {
		if err := store.Add(client); err != nil {
			return nil, err
		}
	}
	if store.Len() == 0 {
		return nil, fmt.Errorf("at least one proxy client key is required")
	}
	return store, nil
}

func LoadStore(keysEnv, keysFile string) (*Store, error) {
	var clients []Client
	if raw := strings.TrimSpace(os.Getenv(keysEnv)); raw != "" {
		parsed, err := parseEnvClients(raw)
		if err != nil {
			return nil, err
		}
		clients = append(clients, parsed...)
	}
	if strings.TrimSpace(keysFile) != "" {
		parsed, err := loadClientFile(keysFile)
		if err != nil {
			return nil, err
		}
		clients = append(clients, parsed...)
	}
	return NewStore(clients)
}

func (s *Store) Add(client Client) error {
	name := strings.TrimSpace(client.Name)
	key := strings.TrimSpace(client.Key)
	if name == "" || key == "" {
		return fmt.Errorf("client name and key are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.clients {
		if existing.Name == name {
			return fmt.Errorf("duplicate client name")
		}
	}
	s.clients = append(s.clients, Client{Name: name, Key: key})
	return nil
}

func (s *Store) Revoke(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.clients[:0]
	revoked := false
	for _, client := range s.clients {
		if client.Name == name {
			revoked = true
			continue
		}
		out = append(out, client)
	}
	s.clients = out
	return revoked
}

func (s *Store) Lookup(key string) (Client, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key = strings.TrimSpace(key)
	for _, client := range s.clients {
		if subtle.ConstantTimeCompare([]byte(client.Key), []byte(key)) == 1 {
			return client, true
		}
	}
	return Client{}, false
}

func (s *Store) Secrets() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.clients))
	for _, client := range s.clients {
		out = append(out, client.Key)
	}
	return out
}

func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}

func BearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	token := strings.TrimSpace(parts[1])
	return token, token != ""
}

func (s *Store) Authenticate(r *http.Request) (Client, error) {
	token, ok := BearerToken(r.Header.Get("Authorization"))
	if !ok {
		return Client{}, fmt.Errorf("missing client key")
	}
	client, ok := s.Lookup(token)
	if !ok {
		return Client{}, fmt.Errorf("invalid client key")
	}
	return client, nil
}

func parseEnvClients(raw string) ([]Client, error) {
	var clients []Client
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, key, ok := strings.Cut(part, ":")
		if !ok {
			return nil, fmt.Errorf("client key entries must be name:key")
		}
		clients = append(clients, Client{Name: strings.TrimSpace(name), Key: strings.TrimSpace(key)})
	}
	return clients, nil
}

func loadClientFile(path string) ([]Client, error) {
	raw, err := secret.ReadOwnerOnlyFile(path)
	if err != nil {
		return nil, err
	}
	var clients []Client
	if err := json.Unmarshal([]byte(raw), &clients); err == nil {
		return clients, nil
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, key, ok := strings.Cut(line, " ")
		if !ok {
			return nil, fmt.Errorf("client key file entries must be JSON or 'name key'")
		}
		clients = append(clients, Client{Name: strings.TrimSpace(name), Key: strings.TrimSpace(key)})
	}
	return clients, nil
}
