package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ricardoalt1515/vigia/internal/auth"
	"github.com/ricardoalt1515/vigia/internal/config"
	"github.com/ricardoalt1515/vigia/internal/detection"
	"github.com/ricardoalt1515/vigia/internal/evaluation"
	"github.com/ricardoalt1515/vigia/internal/httpapi"
	"github.com/ricardoalt1515/vigia/internal/judge"
	"github.com/ricardoalt1515/vigia/internal/postgres"

	"github.com/anthropics/anthropic-sdk-go"
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

// buildJudge mirrors cmd/seed's buildJudge: "anthropic" builds the real
// Anthropic judge (requires AnthropicAPIKey/JudgeModelID/
// JudgeHITLConfidenceThreshold); anything else (including the zero value)
// degrades to judge.FakeJudge{} for local/dev use.
func buildJudge(cfg config.Config) judge.Judge {
	if cfg.JudgeMode == "anthropic" {
		return judge.NewAnthropicJudge(cfg.AnthropicAPIKey, anthropic.Model(cfg.JudgeModelID), cfg.JudgeHITLConfidenceThreshold)
	}
	return judge.FakeJudge{}
}

func run(ctx context.Context) error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	keyStore := postgres.NewTenantAPIKeyStoreFromPool(pool)
	reader := postgres.NewInteractionReaderFromPool(pool)
	summary := postgres.NewSummaryReaderFromPool(pool)
	evidence := postgres.NewEvidenceReaderFromPool(pool)

	// reevaluator reruns the same wired detectors/judge ReEvaluateInteraction
	// promises to be reproducible against (issue #6): it MUST mirror
	// cmd/seed's production wiring (contact-hours detector, MX-REDECO-05
	// judge) so a rerun genuinely proves reproducibility of the real
	// pipeline, not a stand-in.
	reevaluator := evaluation.Service{
		Detectors: []evaluation.NamedDetector{
			{Code: "contact-hours", Detector: detection.ContactHoursDetector{
				Window: detection.Window{StartHour: 8, EndHour: 21},
			}},
		},
		Judges: []evaluation.NamedJudge{
			{Code: "MX-REDECO-05", Judge: buildJudge(cfg)},
		},
		Rubric:       judge.LoadRubric(),
		Interactions: postgres.NewInteractionLookupAdapterFromPool(pool),
		Bundles:      postgres.NewBundleVersionResolverAdapterFromPool(pool),
	}

	server := httpapi.NewServer(auth.NewAuthenticator(keyStore, time.Now), reader, summary, evidence, reevaluator)

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	return http.ListenAndServe(addr, server)
}
