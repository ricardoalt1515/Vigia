// Package ingestion coordinates voice ingestion into the existing evaluation path.
package ingestion

import (
	"context"
	"errors"
	"fmt"

	"github.com/ricardoalt1515/vigia/internal/core"
	"github.com/ricardoalt1515/vigia/internal/detection"
	"github.com/ricardoalt1515/vigia/internal/evaluation"
	"github.com/ricardoalt1515/vigia/internal/transcriber"
)

var ErrInvalidAudioEvaluationInput = errors.New("ingestion: invalid audio evaluation input")

type EvaluationRunner interface {
	EvaluateInteraction(ctx context.Context, in evaluation.EvaluateInteractionInput) (core.Evaluation, error)
}

type TranscriptStore interface {
	SaveTranscript(ctx context.Context, in SaveTranscriptInput) error
}

type SaveTranscriptInput struct {
	TenantID           string
	InteractionEventID string
	Utterances         any
	Transcription      transcriber.Result
}

type AudioEvaluationService struct {
	Transcriber transcriber.Transcriber
	Evaluator   EvaluationRunner
	Transcripts TranscriptStore
}

type EvaluateAudioInput struct {
	TenantID           string
	InteractionEventID string
	Interaction        detection.Interaction
	Audio              transcriber.AudioInput
}

type EvaluateAudioResult struct {
	Evaluation    core.Evaluation
	Transcription transcriber.Result
}

func (s AudioEvaluationService) EvaluateAudio(ctx context.Context, in EvaluateAudioInput) (EvaluateAudioResult, error) {
	if in.TenantID == "" || in.InteractionEventID == "" || in.Audio.SourceURI == "" {
		return EvaluateAudioResult{}, ErrInvalidAudioEvaluationInput
	}
	if s.Transcriber == nil {
		return EvaluateAudioResult{}, fmt.Errorf("%w: transcriber is required", ErrInvalidAudioEvaluationInput)
	}
	if s.Evaluator == nil {
		return EvaluateAudioResult{}, fmt.Errorf("%w: evaluator is required", ErrInvalidAudioEvaluationInput)
	}

	tr, err := s.Transcriber.Transcribe(ctx, in.Audio)
	if err != nil {
		return EvaluateAudioResult{}, err
	}
	tr, err = transcriber.NormalizeResult(tr)
	if err != nil {
		return EvaluateAudioResult{}, err
	}

	if s.Transcripts != nil {
		if err := s.Transcripts.SaveTranscript(ctx, SaveTranscriptInput{TenantID: in.TenantID, InteractionEventID: in.InteractionEventID, Utterances: tr.Utterances, Transcription: tr}); err != nil {
			return EvaluateAudioResult{}, err
		}
	}

	eval, err := s.Evaluator.EvaluateInteraction(ctx, evaluation.EvaluateInteractionInput{
		TenantID:           in.TenantID,
		InteractionEventID: in.InteractionEventID,
		Interaction:        in.Interaction,
		Utterances:         tr.Utterances,
	})
	if err != nil {
		return EvaluateAudioResult{}, err
	}
	return EvaluateAudioResult{Evaluation: eval, Transcription: tr}, nil
}
