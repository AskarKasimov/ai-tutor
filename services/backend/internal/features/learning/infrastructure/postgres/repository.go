// Package postgres stores learning plans and their lesson chats.
package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/learning"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/learning/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
)

type Repository struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool, q: db.New(pool)} }

var _ application.Repository = (*Repository)(nil)

func notFound() error {
	return fault.New(fault.NotFound, "LEARNING_PLAN_NOT_FOUND", "План обучения не найден.")
}

func decode(raw []byte, err error) (learning.Plan, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return learning.Plan{}, notFound()
	}
	if err != nil {
		return learning.Plan{}, err
	}
	var plan learning.Plan
	err = json.Unmarshal(raw, &plan)
	return plan, err
}

func (r *Repository) Outcomes(ctx context.Context, competencyIDs []string) ([]learning.Outcome, error) {
	rows, err := r.q.ListLearningOutcomes(ctx, competencyIDs)
	if err != nil {
		return nil, err
	}
	outcomes := make([]learning.Outcome, 0, len(rows))
	for _, row := range rows {
		outcomes = append(outcomes, learning.Outcome{
			ConstituentID: row.ConstituentID, MapRevision: row.MapRevision,
			CompetencyID: row.CompetencyID, CompetencyName: row.CompetencyName,
			OutcomeID: row.OutcomeID, OutcomeName: row.OutcomeName,
			TaxonomyCode: row.TaxonomyCode, Importance: int(row.Importance),
			EducationalContent: row.EducationalContent,
			TheoryMinutes:      minutes(row.TheoryMinutes), PracticeMinutes: minutes(row.PracticeMinutes),
		})
	}
	return outcomes, nil
}

func minutes(value *int16) *int {
	if value == nil {
		return nil
	}
	converted := int(*value)
	return &converted
}

func (r *Repository) ByDiagnostic(ctx context.Context, owner, diagnosticID string) (learning.Plan, error) {
	return decode(r.q.FindLearningPlanByDiagnostic(ctx, db.FindLearningPlanByDiagnosticParams{OwnerID: owner, DiagnosticID: diagnosticID}))
}

func (r *Repository) Get(ctx context.Context, owner, id string) (learning.Plan, error) {
	return decode(r.q.ReadLearningPlan(ctx, db.ReadLearningPlanParams{OwnerID: owner, ID: id}))
}

// Create stores the plan; when a concurrent request created one for the same
// diagnostic first, that plan is returned.
func (r *Repository) Create(ctx context.Context, plan learning.Plan) (learning.Plan, error) {
	data, err := json.Marshal(plan)
	if err != nil {
		return learning.Plan{}, err
	}
	inserted, err := r.q.InsertLearningPlan(ctx, db.InsertLearningPlanParams{ID: plan.ID, OwnerID: plan.OwnerID, DiagnosticID: plan.DiagnosticID, PlanData: data})
	if err != nil {
		return learning.Plan{}, err
	}
	if inserted == 0 {
		return r.ByDiagnostic(ctx, plan.OwnerID, plan.DiagnosticID)
	}
	return plan, nil
}

func (r *Repository) Messages(ctx context.Context, planID string, item int) ([]learning.Message, error) {
	rows, err := r.q.ListLearningMessages(ctx, db.ListLearningMessagesParams{PlanID: planID, ItemIndex: int32(item)})
	if err != nil {
		return nil, err
	}
	messages := make([]learning.Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, learning.Message{
			Sequence: row.Sequence, ItemIndex: int(row.ItemIndex),
			Role: learning.MessageRole(row.Role), Kind: learning.MessageKind(row.Kind),
			Text: row.Body, CreatedAt: row.CreatedAt,
		})
	}
	return messages, nil
}

func (r *Repository) Update(ctx context.Context, owner, id string, change func(*learning.Plan) ([]learning.Message, error)) (learning.Plan, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return learning.Plan{}, err
	}
	defer tx.Rollback(ctx)
	q := r.q.WithTx(tx)
	plan, err := decode(q.LockLearningPlan(ctx, db.LockLearningPlanParams{OwnerID: owner, ID: id}))
	if err != nil {
		return learning.Plan{}, err
	}
	messages, err := change(&plan)
	if err != nil {
		return learning.Plan{}, err
	}
	if len(messages) > 0 {
		sequence, err := q.NextLearningSequence(ctx, plan.ID)
		if err != nil {
			return learning.Plan{}, err
		}
		for _, message := range messages {
			if err = q.InsertLearningMessage(ctx, db.InsertLearningMessageParams{
				PlanID: plan.ID, Sequence: sequence, ItemIndex: int32(message.ItemIndex),
				Role: string(message.Role), Kind: string(message.Kind), Body: message.Text, CreatedAt: message.CreatedAt,
			}); err != nil {
				return learning.Plan{}, err
			}
			sequence++
		}
	}
	data, err := json.Marshal(plan)
	if err != nil {
		return learning.Plan{}, err
	}
	if err = q.SaveLearningPlan(ctx, db.SaveLearningPlanParams{OwnerID: owner, ID: id, PlanData: data}); err != nil {
		return learning.Plan{}, err
	}
	return plan, tx.Commit(ctx)
}
