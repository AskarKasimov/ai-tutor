package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskgen/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

// Repository reads grounding samples for task generation from the imported
// competency map. The map itself is replaced atomically by the competency slice.
type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) SourceByOutcome(ctx context.Context, outcomeID string) (application.Source, error) {
	var source application.Source
	err := r.pool.QueryRow(ctx, `SELECT o.id, o.name, s.name, c.name
		FROM outcomes o
		JOIN constituents s ON s.id = o.constituent_id
		JOIN competencies c ON c.id = s.competency_id
		WHERE o.id = $1`, outcomeID).Scan(&source.OutcomeID, &source.OutcomeName, &source.ConstituentName, &source.CompetencyName)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.Source{}, fault.New(fault.NotFound, "OUTCOME_NOT_FOUND", "Тема не найдена.")
	}
	if err != nil {
		return application.Source{}, err
	}
	rows, err := r.pool.Query(ctx, `SELECT question, criteria FROM tasks WHERE outcome_id = $1 ORDER BY id`, outcomeID)
	if err != nil {
		return application.Source{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var task application.Task
		if err = rows.Scan(&task.Question, &task.Criteria); err != nil {
			return application.Source{}, err
		}
		source.Tasks = append(source.Tasks, task)
	}
	return source, rows.Err()
}

func (r *Repository) ListOutcomes(ctx context.Context) ([]application.Outcome, error) {
	rows, err := r.pool.Query(ctx, `SELECT o.id, o.name, s.name, c.name, count(t.id)
		FROM outcomes o
		JOIN constituents s ON s.id = o.constituent_id
		JOIN competencies c ON c.id = s.competency_id
		LEFT JOIN tasks t ON t.outcome_id = o.id
		GROUP BY o.id, o.name, s.name, c.name
		ORDER BY c.name, s.name, o.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var outcomes []application.Outcome
	for rows.Next() {
		var outcome application.Outcome
		if err = rows.Scan(&outcome.ID, &outcome.Name, &outcome.ConstituentName, &outcome.CompetencyName, &outcome.TaskCount); err != nil {
			return nil, err
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, rows.Err()
}
