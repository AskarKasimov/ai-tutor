package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/competencymap"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/security"
)

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) Replace(ctx context.Context, actorID string, parsed competencymap.Map, importedAt int64) (competencymap.ImportResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return competencymap.ImportResult{}, err
	}
	defer tx.Rollback(ctx)
	var revision int64
	if err = tx.QueryRow(ctx, "SELECT revision FROM competency_map_state WHERE singleton=true FOR UPDATE").Scan(&revision); err != nil {
		return competencymap.ImportResult{}, err
	}
	result := competencymap.ImportResult{Revision: revision + 1, ImportedAt: importedAt, CompetencyCount: len(parsed.Competencies), ConstituentCount: len(parsed.Constituents), OutcomeCount: len(parsed.Outcomes), TaskCount: len(parsed.Tasks)}
	if _, err = tx.Exec(ctx, `INSERT INTO competency_map_imports(revision,imported_at,imported_by,competency_count,constituent_count,outcome_count,task_count) VALUES($1,$2,$3,$4,$5,$6,$7)`, result.Revision, result.ImportedAt, actorID, result.CompetencyCount, result.ConstituentCount, result.OutcomeCount, result.TaskCount); err != nil {
		return competencymap.ImportResult{}, err
	}
	if _, err = tx.Exec(ctx, "DELETE FROM competencies"); err != nil {
		return competencymap.ImportResult{}, err
	}
	cIDs, sIDs, oIDs := map[string]string{}, map[string]string{}, map[string]string{}
	for _, c := range parsed.Competencies {
		id, err := security.ID("competency")
		if err != nil {
			return competencymap.ImportResult{}, err
		}
		cIDs[c.Key] = id
		if _, err = tx.Exec(ctx, "INSERT INTO competencies(id,name,revision) VALUES($1,$2,$3)", id, c.Name, result.Revision); err != nil {
			return competencymap.ImportResult{}, err
		}
	}
	for _, s := range parsed.Constituents {
		id, err := security.ID("constituent")
		if err != nil {
			return competencymap.ImportResult{}, err
		}
		sIDs[s.Key] = id
		if _, err = tx.Exec(ctx, "INSERT INTO constituents(id,competency_id,name) VALUES($1,$2,$3)", id, cIDs[s.CompetencyKey], s.Name); err != nil {
			return competencymap.ImportResult{}, err
		}
	}
	for _, o := range parsed.Outcomes {
		id, err := security.ID("outcome")
		if err != nil {
			return competencymap.ImportResult{}, err
		}
		oIDs[o.Key] = id
		attrs, err := json.Marshal(o.Attributes)
		if err != nil {
			return competencymap.ImportResult{}, err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO outcomes(id,constituent_id,name,attributes) VALUES($1,$2,$3,$4)", id, sIDs[o.ConstituentKey], o.Name, attrs); err != nil {
			return competencymap.ImportResult{}, err
		}
	}
	for _, t := range parsed.Tasks {
		id, err := security.ID("task")
		if err != nil {
			return competencymap.ImportResult{}, err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO tasks(id,outcome_id,question,criteria,source_row,source_column) VALUES($1,$2,$3,$4,$5,$6)", id, oIDs[t.OutcomeKey], t.Question, t.Criteria, t.Row, t.Column); err != nil {
			return competencymap.ImportResult{}, err
		}
	}
	if _, err = tx.Exec(ctx, "UPDATE competency_map_state SET revision=$1 WHERE singleton=true", result.Revision); err != nil {
		return competencymap.ImportResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return competencymap.ImportResult{}, err
	}
	return result, nil
}
