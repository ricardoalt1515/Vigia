// Package timestamping contains the network boundary for RFC 3161 trusted
// timestamp authorities. Ledger code depends on the small Client interface, so
// tests and local demos can stay deterministic without a TSA.
package timestamping

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/digitorus/timestamp"
)

// Token is the persisted RFC 3161 timestamp token plus the parsed digest it
// binds. Certificate-chain validation is deliberately a deployment concern:
// each installation chooses its trusted TSA roots and policy OIDs.
type Token struct {
	RawToken      []byte
	HashedMessage []byte
	Time          time.Time
}

type Client interface {
	Timestamp(ctx context.Context, payload []byte) (Token, error)
}

type TokenVerifier interface {
	VerifyTokenDigest(rawToken []byte, expectedDigest []byte) error
}

const (
	DefaultTSATimeout  = 10 * time.Second
	defaultMaxAttempts = 3
)

type RFC3161Client struct {
	URL        string
	HTTPClient *http.Client
}

func (c RFC3161Client) VerifyTokenDigest(rawToken []byte, expectedDigest []byte) error {
	parsed, err := timestamp.Parse(rawToken)
	if err != nil {
		return fmt.Errorf("timestamping: parse token: %w", err)
	}
	if !bytes.Equal(parsed.HashedMessage, expectedDigest) {
		return errors.New("timestamping: token digest does not match checkpoint")
	}
	return nil
}

func (c RFC3161Client) Timestamp(ctx context.Context, payload []byte) (Token, error) {
	if c.URL == "" {
		return Token{}, errors.New("timestamping: TSA URL is required")
	}

	reqBytes, err := timestamp.CreateRequest(bytes.NewReader(payload), &timestamp.RequestOptions{Hash: crypto.SHA256, Certificates: true})
	if err != nil {
		return Token{}, fmt.Errorf("timestamping: create request: %w", err)
	}

	hc := c.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: DefaultTSATimeout}
	}
	var lastErr error
	for attempt := 1; attempt <= defaultMaxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(reqBytes))
		if err != nil {
			return Token{}, fmt.Errorf("timestamping: create HTTP request: %w", err)
		}
		req.Header.Set("Content-Type", "application/timestamp-query")
		req.Header.Set("Accept", "application/timestamp-reply")

		resp, err := hc.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("timestamping: TSA request: %w", err)
		} else {
			body, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				lastErr = fmt.Errorf("timestamping: TSA returned %s", resp.Status)
			} else if readErr != nil {
				lastErr = fmt.Errorf("timestamping: read response: %w", readErr)
			} else if parsed, parseErr := timestamp.ParseResponse(body); parseErr != nil {
				lastErr = fmt.Errorf("timestamping: parse response: %w", parseErr)
			} else {
				return Token{RawToken: parsed.RawToken, HashedMessage: parsed.HashedMessage, Time: parsed.Time}, nil
			}
		}
		if attempt < defaultMaxAttempts {
			select {
			case <-ctx.Done():
				return Token{}, ctx.Err()
			case <-time.After(time.Duration(attempt) * 200 * time.Millisecond):
			}
		}
	}
	return Token{}, lastErr
}
