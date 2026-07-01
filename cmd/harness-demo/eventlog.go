package main

import (
	"bytes"
	"encoding/json"

	"github.com/ricardoalt1515/vigia/internal/harness"
)

// eventEntry pairs one harness.Event with the agent that produced it and its position in the
// run-wide monotonic sequence.
type eventEntry struct {
	agentName string
	seq       int
	event     harness.Event
}

// jsonEventLine is the JSONL wire shape for one eventEntry: the structured harness.Event fields
// plus agent_name and sequence annotations. It never carries raw model chain-of-thought — only
// the structured operational Event.
type jsonEventLine struct {
	Type      harness.EventType `json:"type"`
	Data      map[string]any    `json:"data,omitempty"`
	AgentName string            `json:"agent_name"`
	Sequence  int               `json:"sequence"`
}

// eventSink accumulates (agent_name, seq, event) triples produced by caseflow.EventObserver
// callbacks, in call order, and renders them as newline-delimited JSON.
type eventSink struct {
	entries []eventEntry
	nextSeq int
}

// newEventSink constructs an empty eventSink with its sequence counter starting at 0.
func newEventSink() *eventSink {
	return &eventSink{}
}

// observe implements caseflow.EventObserver's call shape: it appends one eventEntry per event in
// events, in order, assigning each a monotonically increasing sequence number across the whole
// run (spanning multiple agents and multiple observe calls).
func (s *eventSink) observe(agentName string, events []harness.Event) {
	for _, ev := range events {
		s.entries = append(s.entries, eventEntry{
			agentName: agentName,
			seq:       s.nextSeq,
			event:     ev,
		})
		s.nextSeq++
	}
}

// jsonl renders the accumulated entries as newline-delimited JSON, one harness.Event per line,
// each annotated with its agent_name and sequence.
func (s *eventSink) jsonl() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, e := range s.entries {
		line := jsonEventLine{
			Type:      e.event.Type,
			Data:      e.event.Data,
			AgentName: e.agentName,
			Sequence:  e.seq,
		}
		if err := enc.Encode(line); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}
