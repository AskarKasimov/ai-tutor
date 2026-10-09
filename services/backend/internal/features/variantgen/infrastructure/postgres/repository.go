package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/variant"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/variantgen/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
)

type Repository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

var _ variant.TaskReader = (*Repository)(nil)

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool, queries: db.New(pool)} }

func (r *Repository) Create(ctx context.Context, ownerID, subjectID, key string, build application.BuildFunc) (variant.Variant, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return variant.Variant{}, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	subjectInfo, err := q.LockSubjectForVariant(ctx, subjectID)
	if err == pgx.ErrNoRows {
		return variant.Variant{}, fault.New(fault.NotFound, "SUBJECT_NOT_FOUND", "Предмет не найден.")
	}
	if err != nil {
		return variant.Variant{}, err
	}
	if existing, err := q.FindVariantByRequestKey(ctx, db.FindVariantByRequestKeyParams{UserID: ownerID, CreateRequestKey: key}); err == nil {
		_ = tx.Rollback(ctx)
		if existing.SubjectID != subjectID {
			return variant.Variant{}, idempotencySubjectConflict()
		}
		return r.Get(ctx, ownerID, existing.ID)
	} else if err != pgx.ErrNoRows {
		return variant.Variant{}, err
	}
	revision := subjectInfo.ActiveRevision
	rows, err := q.ReadVariantCandidates(ctx, revision)
	if err != nil {
		return variant.Variant{}, err
	}
	candidates := make([]variant.CandidateOutcome, 0, len(rows))
	for _, row := range rows {
		var profile variant.TaskProfile
		if err := json.Unmarshal(row.ProfileJson, &profile); err != nil {
			return variant.Variant{}, err
		}
		candidates = append(candidates, variant.CandidateOutcome{Profile: profile, CompetencySourceOrder: row.CompetencySourceOrder, SourceOrder: row.OutcomeSourceOrder, HasTask: row.TaskID != nil})
	}
	result, err := build(subjectInfo.ID, subjectInfo.Name, revision, candidates)
	if err != nil {
		return variant.Variant{}, err
	}
	skipped, err := json.Marshal(result.SkippedCompetencies)
	if err != nil {
		return variant.Variant{}, err
	}
	createdID, err := q.InsertVariant(ctx, db.InsertVariantParams{
		ID: result.ID, UserID: ownerID, CreateRequestKey: key, MapRevision: result.MapRevision,
		AlgorithmVersion: result.AlgorithmVersion, IncludedCompetencyCount: int32(result.IncludedCompetencyCount),
		SkippedCompetencies: skipped, CreatedAt: result.CreatedAt, SubjectID: subjectInfo.ID,
		SubjectNameSnapshot: subjectInfo.Name,
	})
	if err == pgx.ErrNoRows {
		_ = tx.Rollback(ctx)
		existing, readErr := r.queries.FindVariantByRequestKey(ctx, db.FindVariantByRequestKeyParams{UserID: ownerID, CreateRequestKey: key})
		if readErr != nil {
			return variant.Variant{}, readErr
		}
		if existing.SubjectID != subjectID {
			return variant.Variant{}, idempotencySubjectConflict()
		}
		return r.Get(ctx, ownerID, existing.ID)
	}
	if err != nil {
		return variant.Variant{}, err
	}
	for _, selection := range result.Competencies {
		for slot, task := range selection.Tasks {
			taskJSON, profileJSON, err := splitSnapshots(task.Task)
			if err != nil {
				return variant.Variant{}, err
			}
			liveID := task.Task.ID
			if err = q.InsertVariantTask(ctx, db.InsertVariantTaskParams{
				ID: task.ID, VariantID: createdID, LiveTaskID: &liveID,
				SourceTaskIDSnapshot: task.Task.ID, AudioAssetID: task.Task.AudioAssetID,
				CompetencyPosition: int32(selection.Position), Slot: int16(slot), Role: task.Role,
				TaskSnapshot: taskJSON, ProfileSnapshot: profileJSON,
			}); err != nil {
				return variant.Variant{}, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return variant.Variant{}, err
	}
	result.ID, result.OwnerID, result.SubjectID, result.SubjectNameSnapshot = createdID, ownerID, subjectInfo.ID, subjectInfo.Name
	return result, nil
}

type taskSnapshot struct {
	ID                string   `json:"id"`
	Question          string   `json:"question"`
	Options           []string `json:"options"`
	VoiceInstruction  *string  `json:"voice_instruction"`
	ReferenceAnswer   *string  `json:"reference_answer"`
	Criteria          *string  `json:"criteria"`
	Origin            string   `json:"origin"`
	CreatedAt         int64    `json:"created_at"`
	SourceRowIndex    *int32   `json:"source_row_index"`
	SourceColumnIndex *int32   `json:"source_column_index"`
}
type profileSnapshot struct {
	Competency  variant.CompetencyProfile  `json:"competency"`
	Constituent variant.ConstituentProfile `json:"constituent"`
	Outcome     variant.OutcomeProfile     `json:"outcome"`
}

func splitSnapshots(task variant.TaskProfile) ([]byte, []byte, error) {
	taskJSON, err := json.Marshal(taskSnapshot{ID: task.ID, Question: task.Question, Options: task.Options, VoiceInstruction: task.VoiceInstruction, ReferenceAnswer: task.ReferenceAnswer, Criteria: task.Criteria, Origin: task.Origin, CreatedAt: task.CreatedAt, SourceRowIndex: task.SourceRowIndex, SourceColumnIndex: task.SourceColumnIndex})
	if err != nil {
		return nil, nil, err
	}
	profileJSON, err := json.Marshal(profileSnapshot{Competency: task.Competency, Constituent: task.Constituent, Outcome: task.Outcome})
	return taskJSON, profileJSON, err
}

func mergeSnapshots(taskJSON, profileJSON []byte, audioAssetID *string) (variant.TaskProfile, error) {
	var task taskSnapshot
	var profile profileSnapshot
	if err := json.Unmarshal(taskJSON, &task); err != nil {
		return variant.TaskProfile{}, err
	}
	if err := json.Unmarshal(profileJSON, &profile); err != nil {
		return variant.TaskProfile{}, err
	}
	if task.Options == nil {
		task.Options = []string{}
	}
	return variant.TaskProfile{ID: task.ID, Question: task.Question, Options: task.Options, VoiceInstruction: task.VoiceInstruction, AudioAssetID: audioAssetID, ReferenceAnswer: task.ReferenceAnswer, Criteria: task.Criteria, Origin: task.Origin, CreatedAt: task.CreatedAt, SourceRowIndex: task.SourceRowIndex, SourceColumnIndex: task.SourceColumnIndex, Competency: profile.Competency, Constituent: profile.Constituent, Outcome: profile.Outcome}, nil
}

func (r *Repository) Get(ctx context.Context, ownerID, id string) (variant.Variant, error) {
	header, err := r.queries.ReadVariantHeaderByOwner(ctx, db.ReadVariantHeaderByOwnerParams{ID: id, UserID: ownerID})
	if err == pgx.ErrNoRows {
		return variant.Variant{}, variantNotFound()
	}
	if err != nil {
		return variant.Variant{}, err
	}
	result := variant.Variant{ID: header.ID, OwnerID: header.UserID, SubjectID: header.SubjectID, SubjectNameSnapshot: header.SubjectNameSnapshot,
		MapRevision: header.MapRevision, AlgorithmVersion: header.AlgorithmVersion, IncludedCompetencyCount: int(header.IncludedCompetencyCount),
		CreatedAt: header.CreatedAt, Competencies: []variant.CompetencySelection{}}
	if err := json.Unmarshal(header.SkippedCompetencies, &result.SkippedCompetencies); err != nil {
		return variant.Variant{}, err
	}
	rows, err := r.queries.ReadVariantTasks(ctx, id)
	if err != nil {
		return variant.Variant{}, err
	}
	result.TaskCount = len(rows)
	for _, row := range rows {
		task, err := mergeSnapshots(row.TaskSnapshot, row.ProfileSnapshot, row.AudioAssetID)
		if err != nil {
			return variant.Variant{}, err
		}
		position := int(row.CompetencyPosition) - 1
		for len(result.Competencies) <= position {
			result.Competencies = append(result.Competencies, variant.CompetencySelection{Position: len(result.Competencies) + 1, Tasks: []variant.VariantTask{}})
		}
		selection := &result.Competencies[position]
		if selection.Competency.ID == "" {
			selection.Competency = task.Competency
		}
		selection.Tasks = append(selection.Tasks, variant.VariantTask{ID: row.ID, Role: row.Role, Task: task})
	}
	return result, nil
}

func (r *Repository) Task(ctx context.Context, ownerID, variantID, taskID string) (variant.VariantTask, error) {
	row, err := r.queries.ReadVariantTaskByOwner(ctx, db.ReadVariantTaskByOwnerParams{UserID: ownerID, ID: variantID, ID_2: taskID})
	if err == pgx.ErrNoRows {
		return variant.VariantTask{}, variantNotFound()
	}
	if err != nil {
		return variant.VariantTask{}, err
	}
	task, err := mergeSnapshots(row.TaskSnapshot, row.ProfileSnapshot, row.AudioAssetID)
	if err != nil {
		return variant.VariantTask{}, err
	}
	return variant.VariantTask{ID: row.ID, Role: row.Role, Task: task}, nil
}

func (r *Repository) TaskForGrading(ctx context.Context, ownerID, variantID, taskID string) (variant.VariantTask, error) {
	return r.Task(ctx, ownerID, variantID, taskID)
}

func (r *Repository) List(ctx context.Context, ownerID, subjectID string, limit int, cursor *application.Cursor) ([]variant.Variant, *application.Cursor, error) {
	if _, err := r.queries.FindSubjectForVariantList(ctx, subjectID); err == pgx.ErrNoRows {
		return nil, nil, fault.New(fault.NotFound, "SUBJECT_NOT_FOUND", "Предмет не найден.")
	} else if err != nil {
		return nil, nil, err
	}
	params := db.ListVariantsByOwnerParams{UserID: ownerID, SubjectID: subjectID, PageSize: int32(limit + 1)}
	if cursor != nil {
		params.CursorCreatedAt, params.CursorID = &cursor.CreatedAt, &cursor.ID
	}
	rows, err := r.queries.ListVariantsByOwner(ctx, params)
	if err != nil {
		return nil, nil, err
	}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	items := make([]variant.Variant, 0, len(rows))
	for _, row := range rows {
		item := variant.Variant{ID: row.ID, OwnerID: ownerID, SubjectID: row.SubjectID, SubjectNameSnapshot: row.SubjectNameSnapshot,
			MapRevision: row.MapRevision, AlgorithmVersion: row.AlgorithmVersion, IncludedCompetencyCount: int(row.IncludedCompetencyCount),
			TaskCount: int(row.TaskCount), CreatedAt: row.CreatedAt, Competencies: []variant.CompetencySelection{}}
		if err := json.Unmarshal(row.SkippedCompetencies, &item.SkippedCompetencies); err != nil {
			return nil, nil, err
		}
		items = append(items, item)
	}
	if !more || len(items) == 0 {
		return items, nil, nil
	}
	last := items[len(items)-1]
	return items, &application.Cursor{CreatedAt: last.CreatedAt, ID: last.ID}, nil
}

func variantNotFound() *fault.Error {
	return fault.New(fault.NotFound, "VARIANT_NOT_FOUND", "Вариант или задание не найдено.")
}

func idempotencySubjectConflict() *fault.Error {
	return fault.New(fault.Conflict, "IDEMPOTENCY_KEY_SUBJECT_CONFLICT", "Ключ повтора уже использован для другого предмета.")
}
