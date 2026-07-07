-- name: CreateInteractionTranscript :one
INSERT INTO interaction_transcripts (
    tenant_id,
    interaction_event_id,
    utterances,
    provider,
    adapter,
    service,
    language_code,
    provider_job_id,
    provider_request_id,
    metadata
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, COALESCE(sqlc.arg(metadata)::jsonb, '{}'::jsonb))
RETURNING id, tenant_id, interaction_event_id, utterances, created_at, provider, adapter, service, language_code, provider_job_id, provider_request_id, metadata;

-- name: GetInteractionTranscriptByInteraction :one
SELECT id, tenant_id, interaction_event_id, utterances, created_at, provider, adapter, service, language_code, provider_job_id, provider_request_id, metadata
FROM interaction_transcripts
WHERE tenant_id = $1 AND interaction_event_id = $2;
