package awstranscribe

import (
	"context"
	"errors"
	"testing"
	"time"

	appconfig "github.com/ricardoalt1515/vigia/internal/config"
	"github.com/ricardoalt1515/vigia/internal/transcriber"
)

type fakeClient struct {
	started  bool
	calls    int
	statuses []JobStatus
}

func (f *fakeClient) StartJob(ctx context.Context, in StartJobInput) (StartJobOutput, error) {
	f.started = true
	return StartJobOutput{JobName: "job-1"}, nil
}
func (f *fakeClient) GetJob(ctx context.Context, jobName string) (JobStatus, error) {
	if len(f.statuses) == 0 {
		return JobStatus{State: StateInProgress}, nil
	}
	idx := f.calls
	if idx >= len(f.statuses) {
		idx = len(f.statuses) - 1
	}
	s := f.statuses[idx]
	f.calls++
	return s, nil
}

func TestAdapterPollsAndNormalizesCompletedJob(t *testing.T) {
	client := &fakeClient{statuses: []JobStatus{{State: StateCompleted, Transcript: Transcript{Items: []TranscriptItem{{Speaker: "spk_0", Text: "hola", Start: time.Second, End: 2 * time.Second, Confidence: ptr(0.9)}}}}}}
	adapter := New(client, Config{Region: "us-east-1", OutputBucket: "out", PollInterval: time.Nanosecond})

	res, err := adapter.Transcribe(context.Background(), transcriber.AudioInput{SourceURI: "s3://in/call.wav", MediaType: "audio/wav", LanguageCode: "es-MX"})
	if err != nil {
		t.Fatalf("Transcribe returned error: %v", err)
	}
	if !client.started {
		t.Fatal("StartJob was not called")
	}
	if got := res.Utterances[0].Text; got != "hola" {
		t.Fatalf("utterance text = %q, want hola", got)
	}
	if res.Metadata.Provider != "aws" || res.Metadata.Service != "transcribe" || res.Metadata.JobName != "job-1" {
		t.Fatalf("metadata = %#v", res.Metadata)
	}
}

func TestAdapterReturnsFailureWithoutUtterances(t *testing.T) {
	client := &fakeClient{statuses: []JobStatus{{State: StateFailed, FailureReason: "bad media"}}}
	adapter := New(client, Config{Region: "us-east-1", OutputBucket: "out", PollInterval: time.Nanosecond})
	res, err := adapter.Transcribe(context.Background(), transcriber.AudioInput{SourceURI: "s3://in/call.wav"})
	if err == nil {
		t.Fatal("Transcribe returned nil error, want failure")
	}
	if len(res.Utterances) != 0 {
		t.Fatalf("utterances = %#v, want none", res.Utterances)
	}
}

func TestAdapterHonorsContextTimeout(t *testing.T) {
	client := &fakeClient{statuses: []JobStatus{{State: StateInProgress}, {State: StateInProgress}}}
	adapter := New(client, Config{Region: "us-east-1", OutputBucket: "out", PollInterval: time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	_, err := adapter.Transcribe(ctx, transcriber.AudioInput{SourceURI: "s3://in/call.wav"})
	if !errors.Is(err, transcriber.ErrTranscriptionTimeout) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want timeout", err)
	}
}

func TestAdapterEnforcesConfiguredTimeoutWithoutCallerDeadline(t *testing.T) {
	client := &fakeClient{statuses: []JobStatus{{State: StateInProgress}}}
	adapter := New(client, Config{Region: "us-east-1", OutputBucket: "out", PollInterval: time.Nanosecond, Timeout: time.Millisecond})

	res, err := adapter.Transcribe(context.Background(), transcriber.AudioInput{SourceURI: "s3://in/call.wav"})
	if !errors.Is(err, transcriber.ErrTranscriptionTimeout) {
		t.Fatalf("error = %v, want transcription timeout", err)
	}
	if len(res.Utterances) != 0 {
		t.Fatalf("utterances = %#v, want none", res.Utterances)
	}
	if client.calls == 0 {
		t.Fatal("GetJob was not called before timeout")
	}
}

func TestNewFromConfigWiresConcreteSDKClientWithoutCredentials(t *testing.T) {
	adapter, err := NewFromConfig(context.Background(), Config{Region: "us-east-1", OutputBucket: "transcripts", LanguageCode: "es-MX", PollInterval: time.Nanosecond})
	if err != nil {
		t.Fatalf("NewFromConfig returned error: %v", err)
	}
	if _, ok := adapter.client.(*SDKClient); !ok {
		t.Fatalf("adapter client = %T, want *SDKClient", adapter.client)
	}
	if adapter.cfg.Region != "us-east-1" || adapter.cfg.OutputBucket != "transcripts" || adapter.cfg.LanguageCode != "es-MX" {
		t.Fatalf("adapter config = %#v", adapter.cfg)
	}
}

func TestNewFromTranscriberConfigMapsProductionConfig(t *testing.T) {
	adapter, err := NewFromTranscriberConfig(context.Background(), appconfig.TranscriberConfig{AWSRegion: "us-west-2", AWSOutputBucket: "vigia-transcripts", LanguageCode: "es-MX", AWSPollInterval: time.Nanosecond, AWSTimeout: 3 * time.Minute})
	if err != nil {
		t.Fatalf("NewFromTranscriberConfig returned error: %v", err)
	}
	if _, ok := adapter.client.(*SDKClient); !ok {
		t.Fatalf("adapter client = %T, want *SDKClient", adapter.client)
	}
	if adapter.cfg.Region != "us-west-2" || adapter.cfg.OutputBucket != "vigia-transcripts" || adapter.cfg.PollInterval != time.Nanosecond || adapter.cfg.Timeout != 3*time.Minute {
		t.Fatalf("adapter config = %#v", adapter.cfg)
	}
}

func ptr(v float64) *float64 { return &v }
