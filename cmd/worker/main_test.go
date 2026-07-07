package main

import (
	"testing"

	"github.com/ricardoalt1515/vigia/internal/config"
)

func TestPlanRuntimeRegistersLedgerCheckpointOnlyWhenTSAConfigured(t *testing.T) {
	withoutTSA := planRuntime(config.Config{})
	if withoutTSA.RegisterLedgerCheckpoint {
		t.Fatal("RegisterLedgerCheckpoint = true without RFC3161TSAURL, want false")
	}

	withTSA := planRuntime(config.Config{RFC3161TSAURL: "https://tsa.example.test"})
	if !withTSA.RegisterLedgerCheckpoint {
		t.Fatal("RegisterLedgerCheckpoint = false with RFC3161TSAURL, want true")
	}
}
