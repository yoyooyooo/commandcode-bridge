package secret

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Source loads a secret from environment, owner-only file, or command.
// The first non-empty source wins.
type Source struct {
	Env     string
	File    string
	Command string
}

func Load(ctx context.Context, src Source) (string, error) {
	if env := strings.TrimSpace(src.Env); env != "" {
		if value := strings.TrimSpace(os.Getenv(env)); value != "" {
			return value, nil
		}
	}
	if file := strings.TrimSpace(src.File); file != "" {
		value, err := ReadOwnerOnlyFile(file)
		if err != nil {
			return "", err
		}
		if value != "" {
			return value, nil
		}
	}
	if command := strings.TrimSpace(src.Command); command != "" {
		value, err := runCommand(ctx, command)
		if err != nil {
			return "", err
		}
		if value != "" {
			return value, nil
		}
	}
	return "", fmt.Errorf("secret source is empty")
}

func ReadOwnerOnlyFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("read secret file: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("secret file must be owner-only")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read secret file: %w", err)
	}
	return strings.TrimSpace(string(raw)), nil
}

func runCommand(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("secret command failed")
	}
	return strings.TrimSpace(stdout.String()), nil
}

func Redact(text string, secrets ...string) string {
	out := text
	for _, secret := range secrets {
		if strings.TrimSpace(secret) == "" {
			continue
		}
		out = strings.ReplaceAll(out, secret, "[redacted]")
	}
	return out
}
