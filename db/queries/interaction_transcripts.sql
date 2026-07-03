-- name: CreateInteractionTranscript :one
INSERT INTO interaction_transcripts (tenant_id, interaction_event_id, utterances)
VALUES ($1, $2, $3)
RETURNING id, tenant_id, interaction_event_id, utterances, created_at;

-- name: GetInteractionTranscriptByInteraction :one
SELECT id, tenant_id, interaction_event_id, utterances, created_at
FROM interaction_transcripts
WHERE tenant_id = $1 AND interaction_event_id = $2;
