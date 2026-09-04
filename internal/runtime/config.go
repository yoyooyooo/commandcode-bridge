package runtime

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/yoyooyooo/commandcode-bridge/internal/auth"
	"github.com/yoyooyooo/commandcode-bridge/internal/protocol"
	"github.com/yoyooyooo/commandcode-bridge/internal/secret"
)

const (
	DefaultListenAddr = "127.0.0.1:8788"
	DefaultBaseURL    = "https://api.commandcode.ai"
)

type Config struct {
	ListenAddr         string
	AllowExternal      bool
	BaseURL            string
	CommandCodeVersion string
	UpstreamKey        string
	ClientStore        *auth.Store
	ZDR                bool
	WorkingDir         string
}

func Load(ctx context.Context) (Config, error) {
	cfg := Config{
		ListenAddr:         envOrDefault("COMMANDCODE_PROXY_ADDR", DefaultListenAddr),
		AllowExternal:      os.Getenv("COMMANDCODE_PROXY_ALLOW_EXTERNAL") == "1",
		BaseURL:            strings.TrimRight(envOrDefault("COMMANDCODE_BASE_URL", DefaultBaseURL), "/"),
		CommandCodeVersion: envOrDefault("COMMANDCODE_VERSION", protocol.CommandCodeVersion()),
		ZDR:                os.Getenv("CMD_ZDR") == "1",
		WorkingDir:         envOrDefault("COMMANDCODE_PROXY_WORKING_DIR", ""),
	}
	if cfg.WorkingDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return Config{}, err
		}
		cfg.WorkingDir = wd
	}
	if err := ValidateListenAddr(cfg.ListenAddr, cfg.AllowExternal); err != nil {
		return Config{}, err
	}
	upstream, err := secret.Load(ctx, secret.Source{
		Env:     "COMMANDCODE_API_KEY",
		File:    os.Getenv("COMMANDCODE_API_KEY_FILE"),
		Command: os.Getenv("COMMANDCODE_API_KEY_COMMAND"),
	})
	if err != nil {
		return Config{}, fmt.Errorf("upstream command code key: %w", err)
	}
	cfg.UpstreamKey = upstream
	store, err := auth.LoadStore("COMMANDCODE_PROXY_CLIENT_KEYS", os.Getenv("COMMANDCODE_PROXY_CLIENT_KEYS_FILE"))
	if err != nil {
		return Config{}, fmt.Errorf("proxy client keys: %w", err)
	}
	cfg.ClientStore = store
	return cfg, nil
}

func ValidateListenAddr(addr string, allowExternal bool) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("listen address: %w", err)
	}
	if allowExternal {
		return nil
	}
	ip := net.ParseIP(host)
	if host == "localhost" || (ip != nil && ip.IsLoopback()) {
		return nil
	}
	return fmt.Errorf("refusing non-loopback listen %q; set COMMANDCODE_PROXY_ALLOW_EXTERNAL=1", addr)
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
