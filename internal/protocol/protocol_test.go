package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fixturesDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "protocol", "fixtures"))
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(fixturesDir(t), name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestCommandCodeVersionMatchesProtocolSSoT(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "protocol", "command-code-version"))
	if err != nil {
		t.Fatal(err)
	}
	if CommandCodeVersion() != strings.TrimSpace(string(raw)) {
		t.Fatalf("embedded version %q != protocol SSoT %q", CommandCodeVersion(), strings.TrimSpace(string(raw)))
	}
}

func TestReduceTextStopAndCachedUsage(t *testing.T) {
	out, err := Reduce(bytes.NewReader(readFixture(t, "text-stop.ndjson")))
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "PONG" || out.FinishReason != "stop" || out.TerminalKind != "finish" {
		t.Fatalf("%+v", out)
	}
	if out.Usage.PromptTokens != 7 || out.Usage.CompletionTokens != 3 || out.Usage.CachedTokens != 2 {
		t.Fatalf("usage=%+v", out.Usage)
	}
}

func TestReduceReasoning(t *testing.T) {
	out, err := Reduce(bytes.NewReader(readFixture(t, "reasoning-text-stop.ndjson")))
	if err != nil {
		t.Fatal(err)
	}
	if out.Reasoning != "think" || out.Text != "PONG" {
		t.Fatalf("%+v", out)
	}
}

func TestAuthoritativeToolCall(t *testing.T) {
	out, err := Reduce(bytes.NewReader(readFixture(t, "tool-call-authoritative.ndjson")))
	if err != nil {
		t.Fatal(err)
	}
	if out.FinishReason != "tool_calls" || len(out.Tools) != 1 || out.Tools[0].ID != "call_1" {
		t.Fatalf("%+v", out)
	}
	if string(out.Tools[0].Arguments) != `{"q":"x"}` {
		t.Fatalf("args=%s", out.Tools[0].Arguments)
	}
}

func TestProvisionalToolInputIsNotExecutable(t *testing.T) {
	_, err := Reduce(bytes.NewReader(readFixture(t, "tool-input-only.ndjson")))
	if !errors.Is(err, ErrUnconfirmedToolCalls) {
		t.Fatalf("err=%v", err)
	}
}

func TestIncompleteStreamFailsClosed(t *testing.T) {
	_, err := Reduce(bytes.NewReader(readFixture(t, "eof-no-terminal.ndjson")))
	if !errors.Is(err, ErrIncompleteStream) {
		t.Fatalf("err=%v", err)
	}
}

func TestAbortFailsClosed(t *testing.T) {
	_, err := Reduce(bytes.NewReader(readFixture(t, "abort.ndjson")))
	if !errors.Is(err, ErrAborted) {
		t.Fatalf("err=%v", err)
	}
}

func TestErrorEventFailsClosed(t *testing.T) {
	_, err := Reduce(bytes.NewReader(readFixture(t, "error-event.ndjson")))
	if err == nil || !strings.Contains(err.Error(), "upstream boom") {
		t.Fatalf("err=%v", err)
	}
}

func TestMalformedJSONFailsClosed(t *testing.T) {
	_, err := Reduce(bytes.NewReader(readFixture(t, "malformed.ndjson")))
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("err=%v", err)
	}
}

func TestFinishStepThenContinue(t *testing.T) {
	out, err := Reduce(bytes.NewReader(readFixture(t, "finish-step-then-continue.ndjson")))
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "AB" || out.TerminalKind != "finish" || out.Usage.CompletionTokens != 2 {
		t.Fatalf("%+v", out)
	}
}

func TestDuplicateToolIDKeepsFirst(t *testing.T) {
	out, err := Reduce(bytes.NewReader(readFixture(t, "duplicate-tool-id.ndjson")))
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Tools) != 1 || string(out.Tools[0].Arguments) != `{"q":"first"}` {
		t.Fatalf("%+v", out.Tools)
	}
}

func TestParallelTools(t *testing.T) {
	out, err := Reduce(bytes.NewReader(readFixture(t, "parallel-tools.ndjson")))
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Tools) != 2 || out.Tools[0].ID != "call_1" || out.Tools[1].ID != "call_2" {
		t.Fatalf("%+v", out.Tools)
	}
}

func TestInvalidToolArgumentsFailClosed(t *testing.T) {
	for _, name := range []string{"tool-args-scalar.ndjson", "tool-args-null.ndjson"} {
		_, err := Reduce(bytes.NewReader(readFixture(t, name)))
		if !errors.Is(err, ErrInvalidToolArgs) {
			t.Fatalf("%s err=%v", name, err)
		}
	}
}

func TestLargeIntegerToolArgumentsPreserved(t *testing.T) {
	out, err := Reduce(bytes.NewReader(readFixture(t, "tool-args-large-int.ndjson")))
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		N json.Number `json:"n"`
	}
	dec := json.NewDecoder(bytes.NewReader(out.Tools[0].Arguments))
	dec.UseNumber()
	if err := dec.Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.N.String() != "9007199254740993" {
		t.Fatalf("n=%s args=%s", payload.N.String(), out.Tools[0].Arguments)
	}
}

func TestSSEAndMultilineData(t *testing.T) {
	for _, name := range []string{"sse-wrapped.sse", "multiline-data.sse"} {
		out, err := Reduce(bytes.NewReader(readFixture(t, name)))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if out.Text != "Hi" {
			t.Fatalf("%s text=%q", name, out.Text)
		}
	}
}

func TestNoTrailingNewline(t *testing.T) {
	raw := bytes.TrimRight(readFixture(t, "no-trailing-newline.ndjson"), "\n")
	out, err := Reduce(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "Hi" {
		t.Fatalf("text=%q", out.Text)
	}
}
