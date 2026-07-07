package ingestion

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/ricardoalt1515/vigia/internal/core"
	"github.com/ricardoalt1515/vigia/internal/detection"
	"github.com/ricardoalt1515/vigia/internal/evaluation"
	"github.com/ricardoalt1515/vigia/internal/judge"
	"github.com/ricardoalt1515/vigia/internal/transcriber"
)

type recordingEvaluator struct {
	calls int
	got   evaluation.EvaluateInteractionInput
	err   error
}

func (r *recordingEvaluator) EvaluateInteraction(ctx context.Context, in evaluation.EvaluateInteractionInput) (core.Evaluation, error) {
	r.calls++
	r.got = in
	return core.Evaluation{TenantID: core.ID(in.TenantID), InteractionEventID: core.ID(in.InteractionEventID), OverallOutcome: "pass"}, r.err
}

type recordingTranscriptStore struct {
	calls int
	got   SaveTranscriptInput
	err   error
}

func (r *recordingTranscriptStore) SaveTranscript(ctx context.Context, in SaveTranscriptInput) error {
	r.calls++
	r.got = in
	return r.err
}

func TestAudioEvaluationServiceTranscribesStoresThenEvaluates(t *testing.T) {
	utterances := []judge.Utterance{{Speaker: "agent", Text: "buenas tardes"}}
	evaluator := &recordingEvaluator{}
	store := &recordingTranscriptStore{}
	svc := AudioEvaluationService{Transcriber: transcriber.NewFakeTranscriber(nil, transcriber.Result{Utterances: utterances}), Evaluator: evaluator, Transcripts: store}

	res, err := svc.EvaluateAudio(context.Background(), EvaluateAudioInput{TenantID: "tenant-1", InteractionEventID: "interaction-1", Interaction: detection.Interaction{Channel: core.InteractionChannelCall}, Audio: transcriber.AudioInput{SourceURI: "fixture://call"}})
	if err != nil {
		t.Fatalf("EvaluateAudio returned unexpected error: %v", err)
	}
	if evaluator.calls != 1 {
		t.Fatalf("evaluator calls = %d, want 1", evaluator.calls)
	}
	if !reflect.DeepEqual(evaluator.got.Utterances, utterances) {
		t.Fatalf("evaluator utterances = %#v, want %#v", evaluator.got.Utterances, utterances)
	}
	if store.calls != 1 {
		t.Fatalf("transcript store calls = %d, want 1", store.calls)
	}
	if res.Transcription.Metadata.Provider != "fake" {
		t.Fatalf("provider = %q, want fake", res.Transcription.Metadata.Provider)
	}
}

func TestAudioEvaluationServiceFailsClosedOnTranscriptionError(t *testing.T) {
	evaluator := &recordingEvaluator{}
	store := &recordingTranscriptStore{}
	svc := AudioEvaluationService{Transcriber: transcriber.NewFailingFakeTranscriber(transcriber.ErrProviderUnavailable), Evaluator: evaluator, Transcripts: store}

	_, err := svc.EvaluateAudio(context.Background(), EvaluateAudioInput{TenantID: "tenant-1", InteractionEventID: "interaction-1", Audio: transcriber.AudioInput{SourceURI: "fixture://call"}})
	if !errors.Is(err, transcriber.ErrProviderUnavailable) {
		t.Fatalf("error = %v, want provider unavailable", err)
	}
	if evaluator.calls != 0 || store.calls != 0 {
		t.Fatalf("calls after failed transcription: evaluator=%d store=%d", evaluator.calls, store.calls)
	}
}

func TestAudioEvaluationServiceTranscriptStoreFailureBlocksEvaluation(t *testing.T) {
	evaluator := &recordingEvaluator{}
	storeErr := errors.New("store down")
	store := &recordingTranscriptStore{err: storeErr}
	svc := AudioEvaluationService{Transcriber: transcriber.NewFakeTranscriber(nil, transcriber.Result{Utterances: []judge.Utterance{{Text: "hola"}}}), Evaluator: evaluator, Transcripts: store}

	_, err := svc.EvaluateAudio(context.Background(), EvaluateAudioInput{TenantID: "tenant-1", InteractionEventID: "interaction-1", Audio: transcriber.AudioInput{SourceURI: "fixture://call"}})
	if !errors.Is(err, storeErr) {
		t.Fatalf("error = %v, want store error", err)
	}
	if evaluator.calls != 0 {
		t.Fatalf("evaluator calls = %d, want 0", evaluator.calls)
	}
}
