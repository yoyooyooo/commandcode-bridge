package catalog

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

//go:embed static-models.json
var staticJSON []byte

type Model struct {
	ID            string   `json:"id"`
	Name          string   `json:"name,omitempty"`
	OwnedBy       string   `json:"owned_by,omitempty"`
	Reasoning     bool     `json:"reasoning,omitempty"`
	Input         []string `json:"input,omitempty"`
	ContextWindow int      `json:"contextWindow,omitempty"`
	MaxTokens     int      `json:"maxTokens,omitempty"`
	Object        string   `json:"object,omitempty"`
	Created       int64    `json:"created,omitempty"`
}

type Snapshot struct {
	Models []Model
	Source string // live | last-known-good | static-fallback
}

type Catalog struct {
	mu     sync.RWMutex
	live   []Model
	lkg    []Model
	static []Model
	source string
}

type staticFile struct {
	CommandCodeVersion string  `json:"commandCodeVersion"`
	OwnedBy            string  `json:"ownedBy"`
	Models             []Model `json:"models"`
}

func New(staticModels []Model) *Catalog {
	models := normalize(staticModels, "commandcode")
	return &Catalog{static: models, source: "static-fallback"}
}

func MustStatic() []Model {
	models, err := parseStatic(staticJSON)
	if err != nil {
		panic(err)
	}
	return models
}

func LoadStatic(path string) ([]Model, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseStatic(raw)
}

func parseStatic(raw []byte) ([]Model, error) {
	var file staticFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, err
	}
	return normalize(file.Models, firstNonEmpty(file.OwnedBy, "commandcode")), nil
}

func DefaultStaticPath() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "protocol/catalog/static-models-v0.52.1.json"
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "protocol", "catalog", "static-models-v0.52.1.json"))
}

func (c *Catalog) Snapshot() Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	switch {
	case len(c.live) > 0:
		return Snapshot{Models: append([]Model(nil), c.live...), Source: "live"}
	case len(c.lkg) > 0:
		return Snapshot{Models: append([]Model(nil), c.lkg...), Source: "last-known-good"}
	default:
		return Snapshot{Models: append([]Model(nil), c.static...), Source: "static-fallback"}
	}
}

func (c *Catalog) ReadyState() string {
	if c.Snapshot().Source == "live" {
		return "ok"
	}
	return "degraded"
}

func (c *Catalog) Refresh(ctx context.Context, client *http.Client, baseURL, apiKey string) error {
	models, err := fetchLive(ctx, client, baseURL, apiKey)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil || len(models) == 0 {
		if len(c.live) > 0 {
			c.lkg = append([]Model(nil), c.live...)
			c.live = nil
			c.source = "last-known-good"
		} else if len(c.lkg) == 0 {
			c.source = "static-fallback"
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("live catalog was empty")
	}
	c.live = models
	c.lkg = append([]Model(nil), models...)
	c.source = "live"
	return nil
}

func fetchLive(ctx context.Context, client *http.Client, baseURL, apiKey string) ([]Model, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, stringsTrimRight(baseURL)+"/provider/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("catalog fetch status %d", resp.StatusCode)
	}
	return parseLive(body)
}

func parseLive(body []byte) ([]Model, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var payload map[string]any
	if err := dec.Decode(&payload); err != nil {
		return nil, err
	}
	var rawList []any
	if data, ok := payload["data"].([]any); ok {
		rawList = data
	} else if models, ok := payload["models"].([]any); ok {
		rawList = models
	}
	if len(rawList) == 0 {
		return nil, fmt.Errorf("live catalog was empty")
	}
	out := make([]Model, 0, len(rawList))
	for _, item := range rawList {
		obj, _ := item.(map[string]any)
		id, _ := obj["id"].(string)
		if id == "" {
			continue
		}
		name, _ := obj["name"].(string)
		ownedBy, _ := obj["owned_by"].(string)
		out = append(out, Model{
			ID: id, Name: name, OwnedBy: firstNonEmpty(ownedBy, "commandcode"),
			Object: "model",
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("live catalog was empty")
	}
	return out, nil
}

func normalize(models []Model, ownedBy string) []Model {
	out := make([]Model, 0, len(models))
	for _, model := range models {
		model.Object = "model"
		if model.OwnedBy == "" {
			model.OwnedBy = ownedBy
		}
		out = append(out, model)
	}
	return out
}

func stringsTrimRight(value string) string {
	for len(value) > 0 && value[len(value)-1] == '/' {
		value = value[:len(value)-1]
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
