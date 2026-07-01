package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"testing"

	"github.com/ricardoalt1515/vigia/internal/harness"
)

var knownEventTypes = map[string]bool{
	"agent_started":       true,
	"plan_created":        true,
	"tool_proposed":       true,
	"permission_decision": true,
	"tool_result":         true,
	"validation_failure":  true,
	"budget_exceeded":     true,
	"agent_completed":     true,
}

func TestEventSink_SequenceMonotonicAcrossAgents(t *testing.T) {
	sink := newEventSink()

	sink.observe("PolicyExplainer", []harness.Event{
		{Type: harness.EventAgentStarted, Data: map[string]any{}},
		{Type: harness.EventToolProposed, Data: map[string]any{}},
	})
	sink.observe("CaseInvestigator", []harness.Event{
		{Type: harness.EventAgentStarted, Data: map[string]any{}},
	})

	if len(sink.entries) != 3 {
		t.Fatalf("expected 3 accumulated entries, got %d", len(sink.entries))
	}

	prevSeq := -1
	for i, e := range sink.entries {
		if e.seq <= prevSeq {
			t.Errorf("entries[%d].seq: want > %d, got %d (not monotonically increasing)", i, prevSeq, e.seq)
		}
		prevSeq = e.seq
	}

	if sink.entries[0].agentName != "PolicyExplainer" || sink.entries[1].agentName != "PolicyExplainer" {
		t.Errorf("expected first two entries to be from PolicyExplainer, got %q and %q", sink.entries[0].agentName, sink.entries[1].agentName)
	}
	if sink.entries[2].agentName != "CaseInvestigator" {
		t.Errorf("expected third entry to be from CaseInvestigator, got %q", sink.entries[2].agentName)
	}
}

func TestEventSink_JSONL_OneKnownTypedObjectPerLine(t *testing.T) {
	sink := newEventSink()
	sink.observe("PolicyExplainer", []harness.Event{
		{Type: harness.EventAgentStarted, Data: map[string]any{}},
		{Type: harness.EventToolProposed, Data: map[string]any{"tool": "list_applicable_rules"}},
	})
	sink.observe("CaseInvestigator", []harness.Event{
		{Type: harness.EventAgentCompleted, Data: map[string]any{}},
	})

	out, err := sink.jsonl()
	if err != nil {
		t.Fatalf("jsonl: unexpected error: %v", err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	var lineCount int
	var lastSeq float64 = -1
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		lineCount++

		var decoded map[string]any
		if err := json.Unmarshal(line, &decoded); err != nil {
			t.Fatalf("line %d is not valid JSON: %v (line: %s)", lineCount, err, line)
		}

		typ, ok := decoded["type"].(string)
		if !ok || !knownEventTypes[typ] {
			t.Errorf("line %d: type %v is not a known harness.EventType", lineCount, decoded["type"])
		}

		agentName, ok := decoded["agent_name"].(string)
		if !ok || agentName == "" {
			t.Errorf("line %d: missing or empty agent_name", lineCount)
		}

		seq, ok := decoded["sequence"].(float64)
		if !ok {
			t.Errorf("line %d: missing sequence field", lineCount)
		}
		if seq <= lastSeq {
			t.Errorf("line %d: sequence %v not monotonically increasing after %v", lineCount, seq, lastSeq)
		}
		lastSeq = seq
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanning jsonl output: %v", err)
	}
	if lineCount != 3 {
		t.Fatalf("expected 3 JSONL lines, got %d", lineCount)
	}
}

func TestEventSink_IncompleteRun_StopsAtFailingAgent(t *testing.T) {
	sink := newEventSink()

	// Simulate the orchestrator emitting events only up to and including the failing agent's
	// terminal validation_failure — no events from downstream, un-invoked agents are ever fed
	// to the sink.
	sink.observe("PolicyExplainer", []harness.Event{
		{Type: harness.EventAgentStarted, Data: map[string]any{}},
		{Type: harness.EventAgentCompleted, Data: map[string]any{}},
	})
	sink.observe("CaseInvestigator", []harness.Event{
		{Type: harness.EventAgentStarted, Data: map[string]any{}},
		{Type: harness.EventValidationFailure, Data: map[string]any{"error": "bad shape"}},
	})

	for _, e := range sink.entries {
		if e.agentName == "EvidencePackager" || e.agentName == "SupervisorNoteDrafter" {
			t.Errorf("sink contains an entry for a downstream, never-invoked agent: %q", e.agentName)
		}
	}
	if len(sink.entries) != 4 {
		t.Fatalf("expected 4 entries (2 per agent that ran), got %d", len(sink.entries))
	}
}
