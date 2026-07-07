// Package transcriber defines the audio-to-transcript seam used by voice ingestion.
package transcriber

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ricardoalt1515/vigia/internal/judge"
)

const SchemaVersion = "transcriber-result.v1"

var (
	ErrUnsupportedAudio      = errors.New("transcriber: unsupported audio")
	ErrNoSpeech              = errors.New("transcriber: no speech")
	ErrProviderUnavailable   = errors.New("transcriber: provider unavailable")
	ErrProviderUnauthorized  = errors.New("transcriber: provider unauthorized")
	ErrProviderInvalidOutput = errors.New("transcriber: provider invalid output")
	ErrTranscriptionTimeout  = errors.New("transcriber: transcription timeout")
)

type AudioInput struct {
	SourceURI    string
	MediaType    string
	LanguageCode string
	DurationHint time.Duration
	Metadata     map[string]string
}

type Result struct {
	Utterances []judge.Utterance
	Segments   []Segment
	Metadata   Metadata
	Raw        json.RawMessage
}

type Segment struct {
	Speaker    string
	Text       string
	Start      time.Duration
	End        time.Duration
	Confidence *float64
}

type Metadata struct {
	Adapter        string
	Provider       string
	Service        string
	Region         string
	LanguageCode   string
	ModelOrJobType string
	RequestID      string
	JobName        string
	MediaFormat    string
	Diarized       bool
	SchemaVersion  string
}

type Transcriber interface {
	Transcribe(ctx context.Context, in AudioInput) (Result, error)
}

func NormalizeResult(result Result) (Result, error) {
	if len(result.Utterances) == 0 {
		return Result{}, ErrNoSpeech
	}
	utterances := make([]judge.Utterance, 0, len(result.Utterances))
	for _, u := range result.Utterances {
		text := strings.TrimSpace(u.Text)
		if text == "" {
			continue
		}
		utterances = append(utterances, judge.Utterance{Speaker: strings.TrimSpace(u.Speaker), Text: text})
	}
	if len(utterances) == 0 {
		return Result{}, ErrNoSpeech
	}
	result.Utterances = utterances
	if result.Metadata.SchemaVersion == "" {
		result.Metadata.SchemaVersion = SchemaVersion
	}
	return result, nil
}

type FakeTranscriber struct {
	Scripts map[string]Result
	Default Result
	Err     error
}

func NewFakeTranscriber(scripts map[string]Result, defaultResult Result) FakeTranscriber {
	return FakeTranscriber{Scripts: scripts, Default: defaultResult}
}

func NewFailingFakeTranscriber(err error) FakeTranscriber {
	if err == nil {
		err = ErrProviderUnavailable
	}
	return FakeTranscriber{Err: err}
}

func (f FakeTranscriber) Transcribe(ctx context.Context, in AudioInput) (Result, error) {
	select {
	case <-ctx.Done():
		return Result{}, ctx.Err()
	default:
	}
	if f.Err != nil {
		return Result{}, f.Err
	}
	result := f.Default
	if f.Scripts != nil {
		if scripted, ok := f.Scripts[in.SourceURI]; ok {
			result = scripted
		}
	}
	if result.Metadata.Adapter == "" {
		result.Metadata.Adapter = "fake"
	}
	if result.Metadata.Provider == "" {
		result.Metadata.Provider = "fake"
	}
	if result.Metadata.Service == "" {
		result.Metadata.Service = "fake"
	}
	if result.Metadata.LanguageCode == "" {
		if in.LanguageCode != "" {
			result.Metadata.LanguageCode = in.LanguageCode
		} else {
			result.Metadata.LanguageCode = "es-MX"
		}
	}
	return NormalizeResult(result)
}
