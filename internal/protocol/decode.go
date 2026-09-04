package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type FrameKind int

const (
	FrameJSON FrameKind = iota
	FrameDone
)

type Frame struct {
	Kind FrameKind
	Raw  json.RawMessage
}

type Event struct {
	Type string
	Raw  json.RawMessage
	Data map[string]any
}

func DecodeEvent(raw json.RawMessage) (Event, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var data map[string]any
	if err := dec.Decode(&data); err != nil {
		return Event{}, fmt.Errorf("malformed Command Code event: %w", err)
	}
	typ, _ := data["type"].(string)
	return Event{Type: typ, Raw: append(json.RawMessage(nil), raw...), Data: data}, nil
}

func ScanFrames(r io.Reader, fn func(Frame) error) error {
	reader := bufio.NewReaderSize(r, 64*1024)
	var sseData []string
	inSSE := false

	flushSSE := func() error {
		if !inSSE && len(sseData) == 0 {
			return nil
		}
		payload := strings.Join(sseData, "")
		sseData = nil
		inSSE = false
		payload = strings.TrimSpace(payload)
		if payload == "" {
			return nil
		}
		return emitPayload(payload, fn)
	}

	for {
		line, err := reader.ReadBytes('\n')
		hasNewline := err == nil
		if err != nil && err != io.EOF {
			return err
		}
		trimmed := bytes.TrimRight(line, "\r\n")
		text := string(trimmed)

		if inSSE || isSSEField(text) {
			inSSE = true
			if strings.TrimSpace(text) == "" {
				if err := flushSSE(); err != nil {
					return err
				}
			} else if payload, ok := sseDataPayload(text); ok {
				sseData = append(sseData, payload)
			}
			if err == io.EOF {
				if ferr := flushSSE(); ferr != nil {
					return ferr
				}
				if !hasNewline && len(bytes.TrimSpace(trimmed)) > 0 && !isSSEField(text) {
					return emitPayload(strings.TrimSpace(text), fn)
				}
				return nil
			}
			continue
		}

		if payload := strings.TrimSpace(text); payload != "" {
			if err := emitPayload(payload, fn); err != nil {
				return err
			}
		}
		if err == io.EOF {
			return nil
		}
	}
}

func ScanEvents(r io.Reader, fn func(Event) error) error {
	return ScanFrames(r, func(frame Frame) error {
		if frame.Kind == FrameDone {
			return nil
		}
		event, err := DecodeEvent(frame.Raw)
		if err != nil {
			return err
		}
		return fn(event)
	})
}

func emitPayload(payload string, fn func(Frame) error) error {
	if payload == "[DONE]" {
		return fn(Frame{Kind: FrameDone})
	}
	if !json.Valid([]byte(payload)) {
		return fmt.Errorf("malformed Command Code event")
	}
	return fn(Frame{Kind: FrameJSON, Raw: json.RawMessage(payload)})
}

func isSSEField(line string) bool {
	switch {
	case strings.HasPrefix(line, "data:"):
		return true
	case strings.HasPrefix(line, "event:"):
		return true
	case strings.HasPrefix(line, "id:"):
		return true
	case strings.HasPrefix(line, "retry:"):
		return true
	case strings.HasPrefix(line, ":"):
		return true
	default:
		return false
	}
}

func sseDataPayload(line string) (string, bool) {
	if !strings.HasPrefix(line, "data:") {
		return "", false
	}
	payload := strings.TrimPrefix(line, "data:")
	if strings.HasPrefix(payload, " ") {
		payload = payload[1:]
	}
	return payload, true
}
