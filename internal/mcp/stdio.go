package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// ServeJSONLines handles newline-delimited JSON-RPC requests. It is intentionally
// transport-small for the first slice: local stdio clients get one JSON-RPC response per line.
func (s *Server) ServeJSONLines(ctx context.Context, r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	encoder := json.NewEncoder(w)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			if encodeErr := encoder.Encode(errorResponse(nil, ErrCodeInvalidRequest, "invalid request")); encodeErr != nil {
				return fmt.Errorf("write invalid-request response: %w", encodeErr)
			}
			continue
		}
		if req.ID == nil {
			continue
		}
		if err := encoder.Encode(s.Handle(ctx, req)); err != nil {
			return fmt.Errorf("write response: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read json-rpc request: %w", err)
	}
	return nil
}
