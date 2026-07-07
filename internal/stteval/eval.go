// Package stteval compares transcriber adapters against synthetic references.
package stteval

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"regexp"
	"strings"

	"github.com/ricardoalt1515/vigia/internal/judge"
	"github.com/ricardoalt1515/vigia/internal/transcriber"
)

const NormalizerVersion = "stteval-normalizer.v1"

type Fixture struct {
	ID          string            `json:"id"`
	AudioURI    string            `json:"audio_uri"`
	Language    string            `json:"language"`
	Reference   []judge.Utterance `json:"reference"`
	Description string            `json:"description,omitempty"`
}

type Manifest struct {
	Fixtures []Fixture `json:"fixtures"`
}

type NamedTranscriber struct {
	Name        string
	Transcriber transcriber.Transcriber
}

type Metrics struct {
	WER float64 `json:"wer"`
	CER float64 `json:"cer"`
}

type AdapterResult struct {
	FixtureID         string               `json:"fixture_id"`
	Adapter           string               `json:"adapter"`
	Provider          string               `json:"provider"`
	Service           string               `json:"service"`
	WER               float64              `json:"wer"`
	CER               float64              `json:"cer"`
	NormalizerVersion string               `json:"normalizer_version"`
	Metadata          transcriber.Metadata `json:"metadata"`
	Error             string               `json:"error,omitempty"`
}

type Report struct {
	Results []AdapterResult `json:"results"`
}

func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func Run(ctx context.Context, fixtures []Fixture, adapters []NamedTranscriber) (Report, error) {
	report := Report{Results: make([]AdapterResult, 0, len(fixtures)*len(adapters))}
	for _, fixture := range fixtures {
		reference := flatten(fixture.Reference)
		for _, adapter := range adapters {
			res, err := adapter.Transcriber.Transcribe(ctx, transcriber.AudioInput{SourceURI: fixture.AudioURI, LanguageCode: fixture.Language})
			item := AdapterResult{FixtureID: fixture.ID, Adapter: adapter.Name, NormalizerVersion: NormalizerVersion, Metadata: res.Metadata, Provider: res.Metadata.Provider, Service: res.Metadata.Service}
			if item.Adapter == "" {
				item.Adapter = res.Metadata.Adapter
			}
			if err != nil {
				item.Error = err.Error()
				report.Results = append(report.Results, item)
				continue
			}
			metrics := CompareText(reference, flatten(res.Utterances))
			item.WER = metrics.WER
			item.CER = metrics.CER
			report.Results = append(report.Results, item)
		}
	}
	return report, nil
}

var punctuation = regexp.MustCompile(`[^\p{L}\p{N}\s]+`)

func NormalizeText(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = punctuation.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

func CompareText(reference, hypothesis string) Metrics {
	refNorm := NormalizeText(reference)
	hypNorm := NormalizeText(hypothesis)
	refWords := strings.Fields(refNorm)
	hypWords := strings.Fields(hypNorm)
	werDenom := math.Max(1, float64(len(refWords)))
	cerDenom := math.Max(1, float64(len([]rune(refNorm))))
	return Metrics{
		WER: float64(distanceStrings(refWords, hypWords)) / werDenom,
		CER: float64(distanceRunes([]rune(refNorm), []rune(hypNorm))) / cerDenom,
	}
}

func flatten(utterances []judge.Utterance) string {
	parts := make([]string, 0, len(utterances))
	for _, u := range utterances {
		parts = append(parts, u.Text)
	}
	return strings.Join(parts, " ")
}

func distanceStrings(a, b []string) int {
	return levenshtein(len(a), len(b), func(i, j int) bool { return a[i] == b[j] })
}
func distanceRunes(a, b []rune) int {
	return levenshtein(len(a), len(b), func(i, j int) bool { return a[i] == b[j] })
}

func levenshtein(n, m int, equal func(i, j int) bool) int {
	prev := make([]int, m+1)
	cur := make([]int, m+1)
	for j := 0; j <= m; j++ {
		prev[j] = j
	}
	for i := 1; i <= n; i++ {
		cur[0] = i
		for j := 1; j <= m; j++ {
			cost := 1
			if equal(i-1, j-1) {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[m]
}
