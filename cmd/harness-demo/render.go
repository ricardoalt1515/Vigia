package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// disclaimer is repeated at the opening and closing of every rendered brief. It is a fixed,
// trusted constant — never built from untrusted content.
const disclaimer = "> **BORRADOR — requiere revisión del Supervisor de Cumplimiento**"

// spanishHandoffSection maps a caseflow.HandoffKind string (see brief.go's marshalHandoff) to its
// Spanish section header.
var spanishHandoffSection = map[string]string{
	"policy_explanation":      "Política aplicable",
	"case_investigation":      "Investigación",
	"evidence_manifest_draft": "Manifiesto de evidencia (borrador)",
	"supervisor_note_draft":   "Nota para el supervisor (borrador)",
}

// renderBriefMarkdown renders dto as a neutral professional Spanish Markdown document: an opening
// disclaimer, a case summary, one section per stage keyed by handoff kind, an optional Fallo
// section when the run is incomplete, and a closing disclaimer. All untrusted transcript/debtor/
// collector free-text fields are routed through renderUntrusted before being placed in the
// document.
func renderBriefMarkdown(dto briefDTO) string {
	var sb strings.Builder

	sb.WriteString(disclaimer)
	sb.WriteString("\n\n")

	sb.WriteString("## Resumen del caso\n\n")
	sb.WriteString(fmt.Sprintf("- **Identificador de caso**: %s\n", renderUntrusted(dto.CaseID)))
	sb.WriteString(fmt.Sprintf("- **Estado**: %s\n\n", spanishStatusLabel(dto.Status)))

	for _, stage := range dto.Stages {
		sb.WriteString(renderStageSection(stage))
	}

	if dto.Status == "incomplete" {
		sb.WriteString("## Fallo\n\n")
		sb.WriteString(fmt.Sprintf("- **Agente que falló**: %s\n", renderUntrusted(dto.FailedAgent)))
		sb.WriteString(fmt.Sprintf("- **Motivo del fallo**: %s\n\n", renderUntrusted(dto.FailureReason)))
	}

	sb.WriteString(disclaimer)
	sb.WriteString("\n")

	return sb.String()
}

// spanishStatusLabel translates the raw status enum value into neutral professional Spanish,
// never surfacing the raw English enum token as unlabeled prose.
func spanishStatusLabel(status string) string {
	switch status {
	case "complete":
		return "completo"
	case "incomplete":
		return "incompleto"
	default:
		return renderUntrusted(status)
	}
}

// renderStageSection renders one stage's Spanish section header followed by its untrusted
// free-text fields. It decodes the stage's handoff JSON structurally (never as a template) so
// every value routed into the document passes through renderUntrusted.
func renderStageSection(stage stageDTO) string {
	var sb strings.Builder

	header, ok := spanishHandoffSection[stage.Kind]
	if !ok {
		header = "Etapa"
	}
	sb.WriteString(fmt.Sprintf("## %s (%s)\n\n", header, renderUntrusted(stage.AgentName)))

	switch stage.Kind {
	case "policy_explanation":
		var v policyExplanationView
		if err := json.Unmarshal(stage.Handoff, &v); err == nil {
			for _, r := range v.Rules {
				sb.WriteString(fmt.Sprintf("- **%s** (%s): %s\n", renderUntrusted(r.Code), renderUntrusted(r.Severity), renderUntrusted(r.Title)))
				sb.WriteString(renderUntrustedBlock(r.PlainLanguage))
			}
		}
	case "case_investigation":
		var v caseInvestigationView
		if err := json.Unmarshal(stage.Handoff, &v); err == nil {
			for _, f := range v.Findings {
				sb.WriteString(fmt.Sprintf("- Regla **%s**\n", renderUntrusted(f.RuleCode)))
				sb.WriteString("  - Evidencia:\n")
				sb.WriteString(renderUntrustedBlock(f.Evidence))
				sb.WriteString("  - Análisis:\n")
				sb.WriteString(renderUntrustedBlock(f.Analysis))
			}
		}
	case "evidence_manifest_draft":
		var v evidenceManifestView
		if err := json.Unmarshal(stage.Handoff, &v); err == nil {
			sb.WriteString(renderUntrustedBlock(v.Findings))
		}
	case "supervisor_note_draft":
		var v supervisorNoteView
		if err := json.Unmarshal(stage.Handoff, &v); err == nil {
			sb.WriteString(renderUntrustedBlock(v.NoteBody))
		}
	}

	sb.WriteString("\n")
	return sb.String()
}

// policyExplanationView, caseInvestigationView, evidenceManifestView, and supervisorNoteView are
// renderer-local structural views of the four handoff kinds' JSON shapes. They exist only to pull
// untrusted free-text fields (PlainLanguage, Evidence, Analysis, Findings, NoteBody) out for
// rendering through renderUntrusted; they are never used to reconstruct a caseflow type.
type policyExplanationView struct {
	Rules []struct {
		Code          string `json:"code"`
		Title         string `json:"title"`
		Severity      string `json:"severity"`
		PlainLanguage string `json:"plain_language"`
	} `json:"rules"`
}

type caseInvestigationView struct {
	Findings []struct {
		RuleCode string `json:"rule_code"`
		Evidence string `json:"evidence"`
		Analysis string `json:"analysis"`
	} `json:"findings"`
}

type evidenceManifestView struct {
	Findings string `json:"findings"`
}

type supervisorNoteView struct {
	NoteBody string `json:"note_body"`
}

// renderUntrusted escapes markdown control characters in s so it cannot be interpreted as
// formatting directives or headings. It never interpolates s into a heading; callers only use it
// for inline label values or fenced blocks (see renderUntrustedBlock).
func renderUntrusted(s string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"*", "\\*",
		"_", "\\_",
		"#", "\\#",
		"[", "\\[",
		"]", "\\]",
		"<", "\\<",
		">", "\\>",
		"`", "'",
	)
	return replacer.Replace(s)
}

// renderUntrustedBlock wraps s in a fenced code block, neutralizing any internal triple-backtick
// sequence first so untrusted content cannot prematurely close the surrounding fence. The block is
// rendered as raw, non-interpreted display data — not formatting directives.
func renderUntrustedBlock(s string) string {
	neutralized := strings.ReplaceAll(s, "```", "'''")
	var sb strings.Builder
	sb.WriteString("```\n")
	sb.WriteString(neutralized)
	sb.WriteString("\n```\n")
	return sb.String()
}
