package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskgen/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskgen/infrastructure/profilejson"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/security"
)

type Repository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool, queries: db.New(pool)} }

func (r *Repository) GetByKey(ctx context.Context, outcomeID, requestKey string) (application.Task, error) {
	row, err := r.queries.GetGeneratedTaskByKey(ctx, db.GetGeneratedTaskByKeyParams{OutcomeID: outcomeID, RequestKey: requestKey})
	if errors.Is(err, pgx.ErrNoRows) {
		return application.Task{}, fault.New(fault.NotFound, "GENERATED_TASK_NOT_FOUND", "Сгенерированное задание не найдено.")
	}
	if err != nil {
		return application.Task{}, err
	}
	return taskFromRow(row.ID, row.OutcomeID, row.Question, row.Options, row.VoiceInstruction, row.ReferenceAnswer, row.Criteria, row.Origin)
}

func (r *Repository) Context(ctx context.Context, outcomeID string) (application.Context, error) {
	profile, err := r.queries.GetOutcomeForGeneration(ctx, outcomeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.Context{}, fault.New(fault.NotFound, "OUTCOME_NOT_FOUND", "Образовательный результат не найден.")
	}
	if err != nil {
		return application.Context{}, err
	}
	outcome := application.Outcome{
		ID: profile.ID, Name: profile.Name, CompetencyName: profile.CompetencyName,
		ConstituentName: profile.ConstituentName, IncludeInTest: profile.IncludeInTest,
		TaxonomyCode: profile.TaxonomyCode, ALDLevelCode: profile.AldLevelCode,
		TopicLevelCode: profile.TopicLevelCode, Importance: profile.Importance,
		EducationalContent: profile.EducationalContent,
	}
	var sections []profilejson.CurriculumSection
	if err := json.Unmarshal([]byte(profile.CurriculumSections), &sections); err != nil {
		return application.Context{}, err
	}
	outcome.CurriculumSections = profilejson.ToSections(sections)
	context := application.Context{Revision: profile.Revision, Outcome: outcome, Examples: []application.Example{}, Materials: []application.MaterialChunk{}}
	examples, err := r.queries.ListGenerationExamples(ctx, db.ListGenerationExamplesParams{OutcomeID: outcomeID, Limit: 6})
	if err != nil {
		return application.Context{}, err
	}
	for _, example := range examples {
		var options []string
		if err := json.Unmarshal(example.Options, &options); err != nil {
			return application.Context{}, err
		}
		context.Examples = append(context.Examples, application.Example{
			ID: example.ID, Question: example.Question, Options: options,
			VoiceInstruction: example.VoiceInstruction, ReferenceAnswer: example.ReferenceAnswer, Criteria: example.Criteria,
		})
	}
	query := outcome.Name
	if outcome.EducationalContent != nil {
		query += " " + *outcome.EducationalContent
	}
	chunks, err := r.queries.SearchMaterialChunks(ctx, db.SearchMaterialChunksParams{OutcomeID: outcomeID, PlaintoTsquery: query, Limit: 5})
	if err != nil {
		return application.Context{}, err
	}
	for _, chunk := range chunks {
		context.Materials = append(context.Materials, application.MaterialChunk{ID: chunk.ID, Name: chunk.MaterialName, Content: chunk.Content})
	}
	return context, nil
}

func (r *Repository) Persist(ctx context.Context, snapshot application.Context, requestKey, requestedBy, model string, draft application.Draft, createdAt int64) (application.Task, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return application.Task{}, err
	}
	defer tx.Rollback(ctx)
	queries := r.queries.WithTx(tx)
	revision, err := queries.LockCompetencyMapRevision(ctx)
	if err != nil {
		return application.Task{}, err
	}
	if revision != snapshot.Revision {
		return application.Task{}, fault.New(fault.Conflict, "COMPETENCY_MAP_CHANGED", "Карта компетенций изменилась во время генерации. Повторите запрос.")
	}
	if existing, getErr := queries.GetGeneratedTaskByKey(ctx, db.GetGeneratedTaskByKeyParams{OutcomeID: snapshot.Outcome.ID, RequestKey: requestKey}); getErr == nil {
		if err := tx.Commit(ctx); err != nil {
			return application.Task{}, err
		}
		return taskFromRow(existing.ID, existing.OutcomeID, existing.Question, existing.Options, existing.VoiceInstruction, existing.ReferenceAnswer, existing.Criteria, existing.Origin)
	} else if !errors.Is(getErr, pgx.ErrNoRows) {
		return application.Task{}, getErr
	}
	requestedProfile, err := json.Marshal(struct {
		Outcome     profilejson.Outcome `json:"outcome"`
		RequestedBy string              `json:"requested_by"`
	}{profilejson.FromOutcome(snapshot.Outcome), requestedBy})
	if err != nil {
		return application.Task{}, err
	}
	runID, err := security.ID("generation")
	if err != nil {
		return application.Task{}, err
	}
	runRows, err := queries.InsertGenerationRun(ctx, db.InsertGenerationRunParams{
		ID: runID, OutcomeID: snapshot.Outcome.ID, MapRevision: snapshot.Revision,
		Model: model, PromptVersion: application.PromptVersion, RequestKey: requestKey,
		RequestedProfile: requestedProfile, CreatedAt: createdAt,
	})
	if err != nil {
		return application.Task{}, err
	}
	if runRows == 0 {
		existing, err := queries.GetGeneratedTaskByKey(ctx, db.GetGeneratedTaskByKeyParams{OutcomeID: snapshot.Outcome.ID, RequestKey: requestKey})
		if err != nil {
			return application.Task{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return application.Task{}, err
		}
		return taskFromRow(existing.ID, existing.OutcomeID, existing.Question, existing.Options, existing.VoiceInstruction, existing.ReferenceAnswer, existing.Criteria, existing.Origin)
	}
	for _, example := range snapshot.Examples {
		if err := queries.InsertGenerationRunExample(ctx, db.InsertGenerationRunExampleParams{GenerationRunID: runID, TaskID: example.ID}); err != nil {
			return application.Task{}, err
		}
	}
	for _, chunk := range snapshot.Materials {
		if err := queries.InsertGenerationRunChunk(ctx, db.InsertGenerationRunChunkParams{GenerationRunID: runID, ChunkID: chunk.ID}); err != nil {
			return application.Task{}, err
		}
	}
	options, err := json.Marshal(draft.Options)
	if err != nil {
		return application.Task{}, err
	}
	taskID, err := security.ID("task")
	if err != nil {
		return application.Task{}, err
	}
	voice, answer, criteria := draft.VoiceInstruction, draft.ReferenceAnswer, draft.Criteria
	if err := queries.InsertGeneratedTask(ctx, db.InsertGeneratedTaskParams{
		ID: taskID, OutcomeID: snapshot.Outcome.ID, Question: draft.Question,
		Options: options, VoiceInstruction: &voice, ReferenceAnswer: &answer,
		Criteria: optional(criteria), GenerationRunID: &runID, CreatedAt: createdAt,
	}); err != nil {
		return application.Task{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return application.Task{}, err
	}
	return application.Task{ID: taskID, OutcomeID: snapshot.Outcome.ID, Question: draft.Question,
		Options: draft.Options, VoiceInstruction: &voice, ReferenceAnswer: &answer, Criteria: optional(criteria), Origin: "ai_generated"}, nil
}

func (r *Repository) ImportMaterial(ctx context.Context, name string, outcomeIDs, chunks []string, createdAt int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	queries := r.queries.WithTx(tx)
	if _, err := queries.LockCompetencyMapRevision(ctx); err != nil {
		return err
	}
	count, err := queries.CountExistingOutcomes(ctx, outcomeIDs)
	if err != nil {
		return err
	}
	if int(count) != len(outcomeIDs) {
		return fault.Validation("outcome_ids", "Один или несколько образовательных результатов не найдены.")
	}
	// Keep old chunks immutable: a generation may already hold their IDs in its snapshot.
	if err := queries.DeleteMaterialLinksByName(ctx, name); err != nil {
		return err
	}
	for index, content := range chunks {
		id, err := security.ID("material")
		if err != nil {
			return err
		}
		if err := queries.InsertMaterialChunk(ctx, db.InsertMaterialChunkParams{ID: id, MaterialName: name, Ordinal: int32(index + 1), Content: content, CreatedAt: createdAt}); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return fault.New(fault.Conflict, "MATERIAL_ALREADY_IMPORTED", "Материал с таким названием уже загружен.")
			}
			return err
		}
		for _, outcomeID := range outcomeIDs {
			if err := queries.LinkMaterialChunkToOutcome(ctx, db.LinkMaterialChunkToOutcomeParams{ChunkID: id, OutcomeID: outcomeID}); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

func taskFromRow(id, outcomeID, question string, rawOptions []byte, instruction, answer, criteria *string, origin string) (application.Task, error) {
	var options []string
	if err := json.Unmarshal(rawOptions, &options); err != nil {
		return application.Task{}, fmt.Errorf("decode task options: %w", err)
	}
	return application.Task{ID: id, OutcomeID: outcomeID, Question: question, Options: options,
		VoiceInstruction: instruction, ReferenceAnswer: answer, Criteria: criteria, Origin: origin}, nil
}

func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
