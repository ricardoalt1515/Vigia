package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ricardoalt1515/vigia/internal/auth"
	"github.com/ricardoalt1515/vigia/internal/core"
	vigiaDB "github.com/ricardoalt1515/vigia/internal/db"
	"github.com/ricardoalt1515/vigia/internal/evaluation"
	"github.com/ricardoalt1515/vigia/internal/httpapi"
	"github.com/ricardoalt1515/vigia/internal/ledger"
	"github.com/ricardoalt1515/vigia/internal/tenantdb"
)

const defaultInteractionLimit int32 = 50

type TenantAPIKeyStore struct {
	db tenantdb.Beginner
}

func NewTenantAPIKeyStore(db tenantdb.Beginner) *TenantAPIKeyStore {
	return &TenantAPIKeyStore{db: db}
}

func NewTenantAPIKeyStoreFromPool(pool *pgxpool.Pool) *TenantAPIKeyStore {
	return NewTenantAPIKeyStore(poolBeginner{pool: pool})
}

func (s *TenantAPIKeyStore) LookupTenantAPIKeyByHash(ctx context.Context, hash string) (auth.TenantAPIKey, error) {
	var key auth.TenantAPIKey
	err := tenantdb.WithAPIKeyHashTx(ctx, s.db, hash, func(ctx context.Context, tx tenantdb.Tx) error {
		record, err := vigiaDB.New(tx).GetTenantAPIKeyByHash(ctx, hash)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return auth.ErrAPIKeyNotFound
			}
			return err
		}

		var expiresAt *time.Time
		if record.ExpiresAt.Valid {
			expiresAt = &record.ExpiresAt.Time
		}
		key = auth.TenantAPIKey{
			ID:        uuidString(record.ID),
			TenantID:  uuidString(record.TenantID),
			KeyHash:   record.KeyHash,
			Status:    record.Status,
			ExpiresAt: expiresAt,
		}
		return nil
	})
	if err != nil {
		return auth.TenantAPIKey{}, err
	}
	return key, nil
}

type InteractionReader struct {
	db    tenantdb.Beginner
	limit int32
}

func NewInteractionReader(db tenantdb.Beginner) *InteractionReader {
	return &InteractionReader{db: db, limit: defaultInteractionLimit}
}

func NewInteractionReaderFromPool(pool *pgxpool.Pool) *InteractionReader {
	return NewInteractionReader(poolBeginner{pool: pool})
}

type poolBeginner struct {
	pool *pgxpool.Pool
}

func (b poolBeginner) Begin(ctx context.Context) (tenantdb.Tx, error) {
	return b.pool.Begin(ctx)
}

func (r *InteractionReader) ListInteractions(ctx context.Context, tenantID string) ([]httpapi.Interaction, error) {
	var items []httpapi.Interaction
	err := tenantdb.WithTenantTx(ctx, r.db, tenantID, func(ctx context.Context, tx tenantdb.Tx) error {
		rows, err := vigiaDB.New(tx).ListCurrentTenantInteractionsWithOutcome(ctx, r.limit)
		if err != nil {
			return err
		}
		items = make([]httpapi.Interaction, 0, len(rows))
		for _, row := range rows {
			items = append(items, httpapi.Interaction{
				ID:         uuidString(row.ID),
				OccurredAt: row.OccurredAt.Time,
				Channel:    row.Channel,
				Direction:  row.Direction,
				Outcome:    outcomeToAPI(row.OverallOutcome),
				Reason:     reasonToAPI(row.Reason),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

// outcomeToAPI is the single place that upper-cases the persisted
// overall_outcome for the JSON boundary: "pass" -> "PASS", "fail" -> "BLOCK".
// A missing (unevaluated) row stays nil — never a fabricated "PASS".
func outcomeToAPI(overallOutcome *string) *string {
	if overallOutcome == nil {
		return nil
	}
	var upper string
	switch *overallOutcome {
	case "pass":
		upper = "PASS"
	case "fail":
		upper = "BLOCK"
	default:
		log.Printf("postgres: outcomeToAPI: unexpected overall_outcome %q, returning nil", *overallOutcome)
		return nil
	}
	return &upper
}

// reasonToAPI narrows the sqlc-generated `interface{}` for the ->> jsonb
// text-extraction column (see db/queries/interaction_events.sql) to *string.
func reasonToAPI(reason any) *string {
	if reason == nil {
		return nil
	}
	switch v := reason.(type) {
	case string:
		return &v
	case *string:
		return v
	default:
		return nil
	}
}

func uuidString(id pgtype.UUID) string {
	return id.String()
}

func parseUUID(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		return pgtype.UUID{}, fmt.Errorf("parse uuid %q: %w", value, err)
	}
	return id, nil
}

// EvaluationStore persists an evaluations header row and its
// detector_result_rows children inside a single tenantdb.WithTenantTx call.
type EvaluationStore struct {
	db tenantdb.Beginner
}

func NewEvaluationStore(db tenantdb.Beginner) *EvaluationStore {
	return &EvaluationStore{db: db}
}

func NewEvaluationStoreFromPool(pool *pgxpool.Pool) *EvaluationStore {
	return NewEvaluationStore(poolBeginner{pool: pool})
}

// detectorResultPayload is the minimal JSON shape stored in
// detector_result_rows.result_payload for a detector's rationale.
type detectorResultPayload struct {
	Rationale string `json:"rationale"`
}

func (s *EvaluationStore) CreateEvaluation(ctx context.Context, in evaluation.CreateEvaluationInput) (core.Evaluation, error) {
	tenantUUID, err := parseUUID(in.TenantID)
	if err != nil {
		return core.Evaluation{}, err
	}
	interactionUUID, err := parseUUID(in.InteractionEventID)
	if err != nil {
		return core.Evaluation{}, err
	}

	var result core.Evaluation
	err = tenantdb.WithTenantTx(ctx, s.db, in.TenantID, func(ctx context.Context, tx tenantdb.Tx) error {
		q := vigiaDB.New(tx)

		header, err := q.CreateEvaluation(ctx, vigiaDB.CreateEvaluationParams{
			TenantID:           tenantUUID,
			InteractionEventID: interactionUUID,
			OverallOutcome:     in.OverallOutcome,
		})
		if err != nil {
			return err
		}

		detectorResults := make([]ledger.DetectorResult, 0, len(in.DetectorResults))
		for _, dr := range in.DetectorResults {
			payload, err := json.Marshal(detectorResultPayload{Rationale: dr.Rationale})
			if err != nil {
				return err
			}
			if _, err := q.CreateDetectorResultRow(ctx, vigiaDB.CreateDetectorResultRowParams{
				TenantID:           tenantUUID,
				InteractionEventID: interactionUUID,
				DetectorCode:       dr.DetectorCode,
				Outcome:            string(dr.Outcome),
				Severity:           string(dr.Severity),
				ResultPayload:      payload,
				EvaluationID:       pgtype.UUID{Bytes: header.ID.Bytes, Valid: true},
			}); err != nil {
				return err
			}
			detectorResults = append(detectorResults, ledger.DetectorResult{
				Code:      dr.DetectorCode,
				Outcome:   string(dr.Outcome),
				Severity:  string(dr.Severity),
				Rationale: dr.Rationale,
			})
		}

		// Evidence ledger append (issue #3): one more write inside this same
		// tenantdb.WithTenantTx call, after the header + detector rows. A
		// rollback anywhere above (or below) leaves no evaluations row, no
		// detector_result_rows row, no evidence_records row, and the
		// ledger_chain_heads row unchanged — the head lock and the evidence
		// insert commit atomically with everything else.
		head, err := q.LockChainHead(ctx, vigiaDB.LockChainHeadParams{
			TenantID: tenantUUID,
			LastHash: ledger.GenesisPrevHash,
		})
		if err != nil {
			return err
		}

		seq := head.LastSeq + 1
		prevHash := head.LastHash
		// created_at has no DB default: the ledger generates and inserts the
		// exact microsecond-truncated value it hashes, so a DB round-trip
		// never drifts the hash (Postgres timestamptz is microsecond).
		createdAt := time.Now().UTC().Truncate(time.Microsecond)

		body := ledger.Body{
			TenantID:            in.TenantID,
			InteractionEventID:  in.InteractionEventID,
			EvaluationID:        uuidString(header.ID),
			Seq:                 seq,
			OverallOutcome:      header.OverallOutcome,
			PolicyBundleVersion: header.PolicyBundleVersion,
			InputsDigest:        ledger.ComputeInputsDigest(detectorResults),
			CreatedAt:           createdAt,
		}

		// Judge sub-object (issue #4): populated ONLY when a judge ran
		// (in.JudgeModelID != ""), so judge-less bodies stay byte-identical
		// to their pre-#4 shape (Decision 6). The three evidence_records
		// columns are the hash-bearing copy and MUST be written together
		// with body.Judge — evidenceRowToRecord reconstructs Body.Judge
		// from these columns on every read, so without them every judged
		// record would fail re-verification (design's gate-fix CRITICAL).
		var judgeRubricVersion, judgeModelID, judgeConfidence *string
		if in.JudgeModelID != "" {
			confidenceStr := ""
			if in.JudgeConfidence != nil {
				confidenceStr = strconv.FormatFloat(*in.JudgeConfidence, 'f', 4, 64)
			}
			body.Judge = &ledger.JudgeEvidence{
				RubricVersion: in.RubricVersion,
				JudgeModelID:  in.JudgeModelID,
				Confidence:    confidenceStr,
			}
			rubricVersion := in.RubricVersion
			judgeModel := in.JudgeModelID
			judgeRubricVersion = &rubricVersion
			judgeModelID = &judgeModel
			judgeConfidence = &confidenceStr
		}

		hash := ledger.Hash(prevHash, body)

		if _, err := q.InsertEvidenceRecord(ctx, vigiaDB.InsertEvidenceRecordParams{
			TenantID:            tenantUUID,
			InteractionEventID:  interactionUUID,
			EvaluationID:        pgtype.UUID{Bytes: header.ID.Bytes, Valid: true},
			Seq:                 seq,
			PrevHash:            prevHash,
			Hash:                hash,
			OverallOutcome:      body.OverallOutcome,
			PolicyBundleVersion: body.PolicyBundleVersion,
			InputsDigest:        body.InputsDigest,
			CreatedAt:           pgtype.Timestamptz{Time: createdAt, Valid: true},
			JudgeRubricVersion:  judgeRubricVersion,
			JudgeModelID:        judgeModelID,
			JudgeConfidence:     judgeConfidence,
		}); err != nil {
			return err
		}

		if err := q.UpdateChainHead(ctx, vigiaDB.UpdateChainHeadParams{
			TenantID: tenantUUID,
			LastSeq:  seq,
			LastHash: hash,
		}); err != nil {
			return err
		}

		result = core.Evaluation{
			ID:                  core.ID(uuidString(header.ID)),
			TenantID:            core.ID(uuidString(header.TenantID)),
			InteractionEventID:  core.ID(uuidString(header.InteractionEventID)),
			OverallOutcome:      header.OverallOutcome,
			PolicyBundleVersion: header.PolicyBundleVersion,
			CreatedAt:           header.CreatedAt.Time,
		}
		return nil
	})
	if err != nil {
		return core.Evaluation{}, err
	}
	return result, nil
}

var _ evaluation.EvaluationStore = (*EvaluationStore)(nil)

// SummaryReader returns the tenant's out-of-hours (BLOCK) evaluation count
// via a SQL aggregate, computed inside the tenant-scoped transaction so RLS
// enforces isolation.
type SummaryReader struct {
	db tenantdb.Beginner
}

func NewSummaryReader(db tenantdb.Beginner) *SummaryReader {
	return &SummaryReader{db: db}
}

func NewSummaryReaderFromPool(pool *pgxpool.Pool) *SummaryReader {
	return NewSummaryReader(poolBeginner{pool: pool})
}

func (r *SummaryReader) CountOutOfHours(ctx context.Context, tenantID string) (int64, error) {
	var count int64
	err := tenantdb.WithTenantTx(ctx, r.db, tenantID, func(ctx context.Context, tx tenantdb.Tx) error {
		n, err := vigiaDB.New(tx).CountOutOfHoursEvaluations(ctx)
		if err != nil {
			return err
		}
		count = n
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

var _ httpapi.SummaryReader = (*SummaryReader)(nil)

// ChainVerifier is the store-backed adapter around ledger.VerifyChain: it
// loads a tenant's evidence records ordered by seq inside a tenant-scoped
// transaction, maps them to the pure ledger.EvidenceRecord shape, and
// delegates to ledger.VerifyChain. Used by cmd/ledger-verify.
type ChainVerifier struct {
	db tenantdb.Beginner
}

func NewChainVerifier(db tenantdb.Beginner) *ChainVerifier {
	return &ChainVerifier{db: db}
}

func NewChainVerifierFromPool(pool *pgxpool.Pool) *ChainVerifier {
	return NewChainVerifier(poolBeginner{pool: pool})
}

func (v *ChainVerifier) VerifyChain(ctx context.Context, tenantID string) (ledger.VerifyResult, error) {
	var result ledger.VerifyResult
	err := tenantdb.WithTenantTx(ctx, v.db, tenantID, func(ctx context.Context, tx tenantdb.Tx) error {
		tenantUUID, err := parseUUID(tenantID)
		if err != nil {
			return err
		}
		rows, err := vigiaDB.New(tx).ListEvidenceRecordsByTenant(ctx, tenantUUID)
		if err != nil {
			return err
		}
		records := make([]ledger.EvidenceRecord, 0, len(rows))
		for _, row := range rows {
			records = append(records, evidenceRowToRecord(row))
		}
		result = ledger.VerifyChain(records)
		return nil
	})
	if err != nil {
		return ledger.VerifyResult{}, err
	}
	return result, nil
}

// EvidenceReader assembles a self-contained evidence export package for one
// interaction, scoped to the caller's tenant. Any missing piece (no
// evaluation, no evidence record, unknown interaction) collapses into
// httpapi.ErrEvidenceNotFound — the same response regardless of which case
// occurred, so nothing leaks about other tenants' data.
type EvidenceReader struct {
	db tenantdb.Beginner
}

func NewEvidenceReader(db tenantdb.Beginner) *EvidenceReader {
	return &EvidenceReader{db: db}
}

func NewEvidenceReaderFromPool(pool *pgxpool.Pool) *EvidenceReader {
	return NewEvidenceReader(poolBeginner{pool: pool})
}

func (r *EvidenceReader) GetEvidencePackage(ctx context.Context, tenantID, interactionID string) (ledger.Package, error) {
	var pkg ledger.Package
	err := tenantdb.WithTenantTx(ctx, r.db, tenantID, func(ctx context.Context, tx tenantdb.Tx) error {
		q := vigiaDB.New(tx)

		tenantUUID, err := parseUUID(tenantID)
		if err != nil {
			return err
		}
		interactionUUID, err := parseUUID(interactionID)
		if err != nil {
			return httpapi.ErrEvidenceNotFound
		}

		record, err := q.GetEvidenceRecordByInteraction(ctx, vigiaDB.GetEvidenceRecordByInteractionParams{
			TenantID:           tenantUUID,
			InteractionEventID: interactionUUID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return httpapi.ErrEvidenceNotFound
			}
			return err
		}

		interactionRow, err := q.GetInteractionEventByID(ctx, vigiaDB.GetInteractionEventByIDParams{
			ID:       interactionUUID,
			TenantID: tenantUUID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return httpapi.ErrEvidenceNotFound
			}
			return err
		}

		evaluationRow, err := q.GetEvaluationByInteractionEventID(ctx, vigiaDB.GetEvaluationByInteractionEventIDParams{
			TenantID:           tenantUUID,
			InteractionEventID: interactionUUID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return httpapi.ErrEvidenceNotFound
			}
			return err
		}

		detectorRows, err := q.ListDetectorResultRowsByEvaluation(ctx, record.EvaluationID)
		if err != nil {
			return err
		}
		results := make([]ledger.DetectorResult, 0, len(detectorRows))
		for _, dr := range detectorRows {
			var payload detectorResultPayload
			if err := json.Unmarshal(dr.ResultPayload, &payload); err != nil {
				return err
			}
			results = append(results, ledger.DetectorResult{
				Code:      dr.DetectorCode,
				Outcome:   dr.Outcome,
				Severity:  dr.Severity,
				Rationale: payload.Rationale,
			})
		}

		rec := evidenceRowToRecord(record)
		pkg = ledger.BuildPackage(rec,
			ledger.PackageInteraction{
				ID:         uuidString(interactionRow.ID),
				TenantID:   uuidString(interactionRow.TenantID),
				Channel:    interactionRow.Channel,
				Direction:  interactionRow.Direction,
				OccurredAt: interactionRow.OccurredAt.Time,
			},
			ledger.PackageEvaluation{
				ID:                  uuidString(evaluationRow.ID),
				OverallOutcome:      evaluationRow.OverallOutcome,
				PolicyBundleVersion: evaluationRow.PolicyBundleVersion,
				CreatedAt:           evaluationRow.CreatedAt.Time,
			},
			results,
		)
		return nil
	})
	if err != nil {
		return ledger.Package{}, err
	}
	return pkg, nil
}

var _ httpapi.EvidenceReader = (*EvidenceReader)(nil)

// evidenceRowToRecord maps a generated evidence_records row to the pure
// ledger.EvidenceRecord shape VerifyChain/VerifyPackage operate on. Body.Judge
// is reconstructed from the three judge_* columns: nil when they are NULL
// (the judge-less shape, byte-identical to pre-#4 records), populated
// verbatim when set (judge_confidence is read back exactly as stored — no
// re-formatting — since it is already the canonical 4-decimal string that
// was hashed at write time).
func evidenceRowToRecord(row vigiaDB.EvidenceRecord) ledger.EvidenceRecord {
	var judge *ledger.JudgeEvidence
	if row.JudgeModelID != nil {
		var rubricVersion, confidence string
		if row.JudgeRubricVersion != nil {
			rubricVersion = *row.JudgeRubricVersion
		}
		if row.JudgeConfidence != nil {
			confidence = *row.JudgeConfidence
		}
		judge = &ledger.JudgeEvidence{
			RubricVersion: rubricVersion,
			JudgeModelID:  *row.JudgeModelID,
			Confidence:    confidence,
		}
	}

	return ledger.EvidenceRecord{
		ID: uuidString(row.ID),
		Body: ledger.Body{
			TenantID:            uuidString(row.TenantID),
			InteractionEventID:  uuidString(row.InteractionEventID),
			EvaluationID:        uuidString(row.EvaluationID),
			Seq:                 row.Seq,
			OverallOutcome:      row.OverallOutcome,
			PolicyBundleVersion: row.PolicyBundleVersion,
			InputsDigest:        row.InputsDigest,
			CreatedAt:           row.CreatedAt.Time,
			Judge:               judge,
		},
		PrevHash: row.PrevHash,
		Hash:     row.Hash,
	}
}
