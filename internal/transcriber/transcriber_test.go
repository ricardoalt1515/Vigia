package transcriber

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ricardoalt1515/vigia/internal/judge"
)

func TestFakeTranscriberReturnsStableScriptedResult(t *testing.T) {
	want := Result{
		Utterances: []judge.Utterance{{Speaker: "agent", Text: "buenas tardes"}},
		Segments:   []Segment{{Speaker: "agent", Text: "buenas tardes", Start: time.Second, End: 2 * time.Second}},
		Metadata:   Metadata{Adapter: "custom", Provider: "fake", Service: "fake", LanguageCode: "es-MX"},
	}
	fake := NewFakeTranscriber(map[string]Result{"fixture://call-1": want}, Result{})

	first, err := fake.Transcribe(context.Background(), AudioInput{SourceURI: "fixture://call-1", LanguageCode: "es-MX"})
	if err != nil {
		t.Fatalf("Transcribe returned unexpected error: %v", err)
	}
	second, err := fake.Transcribe(context.Background(), AudioInput{SourceURI: "fixture://call-1", LanguageCode: "es-MX"})
	if err != nil {
		t.Fatalf("Transcribe returned unexpected error on repeat: %v", err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("fake output not stable:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if first.Metadata.Provider != "fake" || first.Metadata.SchemaVersion != SchemaVersion {
		t.Fatalf("metadata = %#v, want stable fake metadata with schema", first.Metadata)
	}
}

func TestFakeTranscriberRejectsBlankUtterances(t *testing.T) {
	fake := NewFakeTranscriber(nil, Result{Utterances: []judge.Utterance{{Speaker: "agent", Text: "   "}}})

	_, err := fake.Transcribe(context.Background(), AudioInput{SourceURI: "fixture://blank"})
	if !errors.Is(err, ErrNoSpeech) {
		t.Fatalf("Transcribe error = %v, want ErrNoSpeech", err)
	}
}

func TestFakeTranscriberReturnsConfiguredFailureWithoutFabricatingTranscript(t *testing.T) {
	fake := NewFailingFakeTranscriber(ErrProviderUnavailable)

	res, err := fake.Transcribe(context.Background(), AudioInput{SourceURI: "fixture://call"})
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("Transcribe error = %v, want ErrProviderUnavailable", err)
	}
	if len(res.Utterances) != 0 {
		t.Fatalf("Utterances = %#v, want none on failure", res.Utterances)
	}
}
