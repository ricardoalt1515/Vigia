package mcp

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestServeJSONLinesDoesNotRespondToNotifications(t *testing.T) {
	server := NewServer(Config{})
	input := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/unknown","params":{}}`,
		"",
	}, "\n"))
	var output bytes.Buffer

	if err := server.ServeJSONLines(context.Background(), input, &output); err != nil {
		t.Fatalf("ServeJSONLines: %v", err)
	}
	if got := output.String(); got != "" {
		t.Fatalf("notifications must not emit JSON-RPC responses, got %q", got)
	}
}

func TestServeJSONLinesStillRespondsToUnknownRequests(t *testing.T) {
	server := NewServer(Config{})
	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"unknown/request","params":{}}` + "\n")
	var output bytes.Buffer

	if err := server.ServeJSONLines(context.Background(), input, &output); err != nil {
		t.Fatalf("ServeJSONLines: %v", err)
	}
	got := output.String()
	if !strings.Contains(got, `"code":-32601`) {
		t.Fatalf("unknown request should emit method-not-found response, got %q", got)
	}
}
