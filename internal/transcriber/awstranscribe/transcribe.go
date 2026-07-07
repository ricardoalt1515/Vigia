// Package awstranscribe implements the Amazon Transcribe adapter boundary.
// It is written around a small client interface so unit tests never call AWS.
package awstranscribe

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awstranscribesvc "github.com/aws/aws-sdk-go-v2/service/transcribe"
	awstranscribetypes "github.com/aws/aws-sdk-go-v2/service/transcribe/types"
	appconfig "github.com/ricardoalt1515/vigia/internal/config"
	"github.com/ricardoalt1515/vigia/internal/judge"
	"github.com/ricardoalt1515/vigia/internal/transcriber"
)

type Config struct {
	Region       string
	OutputBucket string
	LanguageCode string
	PollInterval time.Duration
	Timeout      time.Duration
}

type Client interface {
	StartJob(ctx context.Context, in StartJobInput) (StartJobOutput, error)
	GetJob(ctx context.Context, jobName string) (JobStatus, error)
}

type StartJobInput struct {
	MediaURI     string
	MediaFormat  string
	LanguageCode string
	OutputBucket string
}

type StartJobOutput struct {
	JobName   string
	RequestID string
}

type State string

const (
	StateInProgress State = "IN_PROGRESS"
	StateCompleted  State = "COMPLETED"
	StateFailed     State = "FAILED"
)

type JobStatus struct {
	State         State
	FailureReason string
	Transcript    Transcript
}

type Transcript struct{ Items []TranscriptItem }

type TranscriptItem struct {
	Speaker    string
	Text       string
	Start      time.Duration
	End        time.Duration
	Confidence *float64
}

type Adapter struct {
	client Client
	cfg    Config
}

// SDKClient is the concrete Amazon Transcribe-backed client used by production wiring.
// It satisfies Client while keeping AWS SDK details below the transcriber seam.
type SDKClient struct {
	client     *awstranscribesvc.Client
	httpClient *http.Client
}

// NewFromConfig constructs an Amazon Transcribe adapter from production AWS config.
// Loading AWS config is credential-free; credentials are resolved only when Transcribe
// requests are made, so unit tests can construct this adapter without live AWS access.
func NewFromConfig(ctx context.Context, cfg Config) (Adapter, error) {
	cfg = normalizeConfig(cfg)
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Region))
	if err != nil {
		return Adapter{}, fmt.Errorf("load aws transcribe config: %w", err)
	}
	return New(NewSDKClient(awstranscribesvc.NewFromConfig(awsCfg), http.DefaultClient), cfg), nil
}

func NewFromTranscriberConfig(ctx context.Context, cfg appconfig.TranscriberConfig) (Adapter, error) {
	return NewFromConfig(ctx, Config{Region: cfg.AWSRegion, OutputBucket: cfg.AWSOutputBucket, LanguageCode: cfg.LanguageCode, PollInterval: cfg.AWSPollInterval, Timeout: cfg.AWSTimeout})
}

func NewSDKClient(client *awstranscribesvc.Client, httpClient *http.Client) *SDKClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &SDKClient{client: client, httpClient: httpClient}
}

func New(client Client, cfg Config) Adapter {
	return Adapter{client: client, cfg: normalizeConfig(cfg)}
}

func normalizeConfig(cfg Config) Config {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.LanguageCode == "" {
		cfg.LanguageCode = "es-MX"
	}
	return cfg
}

func (c *SDKClient) StartJob(ctx context.Context, in StartJobInput) (StartJobOutput, error) {
	if c == nil || c.client == nil {
		return StartJobOutput{}, transcriber.ErrProviderUnavailable
	}
	jobName := "vigia-" + randomHex(12)
	showSpeakerLabels := true
	maxSpeakerLabels := int32(2)
	out, err := c.client.StartTranscriptionJob(ctx, &awstranscribesvc.StartTranscriptionJobInput{
		TranscriptionJobName: &jobName,
		LanguageCode:         awstranscribetypes.LanguageCode(in.LanguageCode),
		MediaFormat:          awstranscribetypes.MediaFormat(in.MediaFormat),
		Media:                &awstranscribetypes.Media{MediaFileUri: &in.MediaURI},
		OutputBucketName:     stringPtrOrNil(in.OutputBucket),
		Settings:             &awstranscribetypes.Settings{ShowSpeakerLabels: &showSpeakerLabels, MaxSpeakerLabels: &maxSpeakerLabels},
	})
	if err != nil {
		return StartJobOutput{}, err
	}
	requestID := ""
	if metadata, ok := awsmiddleware.GetRequestIDMetadata(out.ResultMetadata); ok {
		requestID = metadata
	}
	if out.TranscriptionJob != nil && out.TranscriptionJob.TranscriptionJobName != nil {
		jobName = *out.TranscriptionJob.TranscriptionJobName
	}
	return StartJobOutput{JobName: jobName, RequestID: requestID}, nil
}

func (c *SDKClient) GetJob(ctx context.Context, jobName string) (JobStatus, error) {
	if c == nil || c.client == nil {
		return JobStatus{}, transcriber.ErrProviderUnavailable
	}
	out, err := c.client.GetTranscriptionJob(ctx, &awstranscribesvc.GetTranscriptionJobInput{TranscriptionJobName: &jobName})
	if err != nil {
		return JobStatus{}, err
	}
	if out.TranscriptionJob == nil {
		return JobStatus{State: StateFailed, FailureReason: "missing transcription job"}, nil
	}
	job := out.TranscriptionJob
	switch job.TranscriptionJobStatus {
	case awstranscribetypes.TranscriptionJobStatusCompleted:
		if job.Transcript == nil || job.Transcript.TranscriptFileUri == nil || *job.Transcript.TranscriptFileUri == "" {
			return JobStatus{State: StateFailed, FailureReason: "missing transcript file URI"}, nil
		}
		transcript, err := c.fetchTranscript(ctx, *job.Transcript.TranscriptFileUri)
		if err != nil {
			return JobStatus{}, err
		}
		return JobStatus{State: StateCompleted, Transcript: transcript}, nil
	case awstranscribetypes.TranscriptionJobStatusFailed:
		reason := "transcription job failed"
		if job.FailureReason != nil && *job.FailureReason != "" {
			reason = *job.FailureReason
		}
		return JobStatus{State: StateFailed, FailureReason: reason}, nil
	default:
		return JobStatus{State: StateInProgress}, nil
	}
}

func (c *SDKClient) fetchTranscript(ctx context.Context, uri string) (Transcript, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return Transcript{}, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Transcript{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Transcript{}, fmt.Errorf("%w: transcript fetch status %d", transcriber.ErrProviderUnavailable, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Transcript{}, err
	}
	parsed, err := parseAWSResult(body)
	if err != nil {
		return Transcript{}, err
	}
	return parsed, nil
}

func (a Adapter) Transcribe(ctx context.Context, in transcriber.AudioInput) (transcriber.Result, error) {
	if a.client == nil {
		return transcriber.Result{}, transcriber.ErrProviderUnavailable
	}
	if a.cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.cfg.Timeout)
		defer cancel()
	}
	language := in.LanguageCode
	if language == "" {
		language = a.cfg.LanguageCode
	}
	started, err := a.client.StartJob(ctx, StartJobInput{MediaURI: in.SourceURI, MediaFormat: mediaFormat(in), LanguageCode: language, OutputBucket: a.cfg.OutputBucket})
	if err != nil {
		return transcriber.Result{}, fmt.Errorf("%w: %v", transcriber.ErrProviderUnavailable, err)
	}
	jobName := started.JobName
	if jobName == "" {
		jobName = "vigia-transcribe-job"
	}

	ticker := time.NewTicker(a.cfg.PollInterval)
	defer ticker.Stop()
	for {
		status, err := a.client.GetJob(ctx, jobName)
		if err != nil {
			return transcriber.Result{}, fmt.Errorf("%w: %v", transcriber.ErrProviderUnavailable, err)
		}
		switch status.State {
		case StateCompleted:
			return a.normalize(status.Transcript, jobName, started.RequestID, language, in)
		case StateFailed:
			return transcriber.Result{}, fmt.Errorf("%w: %s", transcriber.ErrProviderInvalidOutput, status.FailureReason)
		}
		select {
		case <-ctx.Done():
			return transcriber.Result{}, fmt.Errorf("%w: %v", transcriber.ErrTranscriptionTimeout, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (a Adapter) normalize(t Transcript, jobName, requestID, language string, in transcriber.AudioInput) (transcriber.Result, error) {
	segments := make([]transcriber.Segment, 0, len(t.Items))
	utterances := make([]judge.Utterance, 0, len(t.Items))
	for _, item := range t.Items {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		segments = append(segments, transcriber.Segment{Speaker: item.Speaker, Text: text, Start: item.Start, End: item.End, Confidence: item.Confidence})
		utterances = append(utterances, judge.Utterance{Speaker: item.Speaker, Text: text})
	}
	return transcriber.NormalizeResult(transcriber.Result{Utterances: utterances, Segments: segments, Metadata: transcriber.Metadata{Adapter: "aws-transcribe", Provider: "aws", Service: "transcribe", Region: a.cfg.Region, LanguageCode: language, ModelOrJobType: "batch-transcription", JobName: jobName, RequestID: requestID, MediaFormat: mediaFormat(in), Diarized: true, SchemaVersion: transcriber.SchemaVersion}})
}

func mediaFormat(in transcriber.AudioInput) string {
	switch in.MediaType {
	case "audio/wav":
		return "wav"
	case "audio/mpeg":
		return "mp3"
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(in.SourceURI)), ".")
	if ext == "" {
		return "wav"
	}
	return ext
}

type awsTranscriptFile struct {
	Results struct {
		Items []struct {
			Type         string `json:"type"`
			StartTime    string `json:"start_time"`
			EndTime      string `json:"end_time"`
			SpeakerLabel string `json:"speaker_label"`
			Alternatives []struct {
				Content    string `json:"content"`
				Confidence string `json:"confidence"`
			} `json:"alternatives"`
		} `json:"items"`
	} `json:"results"`
}

func parseAWSResult(body []byte) (Transcript, error) {
	var file awsTranscriptFile
	if err := json.Unmarshal(body, &file); err != nil {
		return Transcript{}, fmt.Errorf("%w: %v", transcriber.ErrProviderInvalidOutput, err)
	}
	items := make([]TranscriptItem, 0, len(file.Results.Items))
	for _, item := range file.Results.Items {
		if item.Type != "pronunciation" || len(item.Alternatives) == 0 {
			continue
		}
		alt := item.Alternatives[0]
		confidence, _ := strconv.ParseFloat(alt.Confidence, 64)
		items = append(items, TranscriptItem{Speaker: item.SpeakerLabel, Text: alt.Content, Start: secondsToDuration(item.StartTime), End: secondsToDuration(item.EndTime), Confidence: &confidence})
	}
	if len(items) == 0 {
		return Transcript{}, transcriber.ErrNoSpeech
	}
	return Transcript{Items: items}, nil
}

func secondsToDuration(value string) time.Duration {
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return time.Duration(seconds * float64(time.Second))
}

func randomHex(bytes int) string {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func stringPtrOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
