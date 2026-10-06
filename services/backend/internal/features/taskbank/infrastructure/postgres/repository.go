package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskbank/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
)

type Repository struct{ queries *db.Queries }

func New(pool *pgxpool.Pool) *Repository { return &Repository{queries: db.New(pool)} }

func (r *Repository) Search(ctx context.Context, filter application.SearchFilter) ([]application.TaskSummary, error) {
	includeInTest := int32(-1)
	if filter.IncludeInTest != nil {
		includeInTest = 0
		if *filter.IncludeInTest {
			includeInTest = 1
		}
	}
	rows, err := r.queries.SearchTaskProfiles(ctx, db.SearchTaskProfilesParams{
		OutcomeID: filter.OutcomeID, CompetencyID: filter.CompetencyID, ConstituentID: filter.ConstituentID,
		TaxonomyCode: filter.TaxonomyCode, AldLevelCode: filter.ALDLevelCode, TopicLevelCode: filter.TopicLevelCode,
		Importance: int16(filter.Importance), ImportanceMin: int16(filter.ImportanceMin), ImportanceMax: int16(filter.ImportanceMax), IncludeInTest: includeInTest, Origin: filter.Origin,
		SectionCode: filter.SectionCode, CurriculumCompetencyCode: filter.CurriculumCompetencyCode, TaskLimit: filter.Limit,
	})
	if err != nil {
		return nil, err
	}
	items := make([]application.TaskSummary, 0, len(rows))
	for _, row := range rows {
		options, err := decodeOptions(row.Options)
		if err != nil {
			return nil, err
		}
		items = append(items, application.TaskSummary{
			ID: row.ID, Question: row.Question, Origin: row.Origin, Options: options,
			VoiceInstruction: row.VoiceInstruction, OutcomeID: row.OutcomeID, OutcomeName: row.OutcomeName,
			CompetencyName: row.CompetencyName, ConstituentName: row.ConstituentName,
			TaxonomyCode: row.TaxonomyCode, ALDLevelCode: row.AldLevelCode, TopicLevelCode: row.TopicLevelCode,
			Importance: row.Importance, IncludeInTest: row.IncludeInTest,
		})
	}
	return items, nil
}

func (r *Repository) Profile(ctx context.Context, taskID string) (application.TaskProfile, error) {
	row, err := r.queries.GetTaskProfile(ctx, taskID)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.TaskProfile{}, fault.New(fault.NotFound, "TASK_NOT_FOUND", "Задание не найдено.")
	}
	if err != nil {
		return application.TaskProfile{}, err
	}
	options, err := decodeOptions(row.Options)
	if err != nil {
		return application.TaskProfile{}, err
	}
	var sections []application.CurriculumSection
	if err := json.Unmarshal([]byte(row.CurriculumSections), &sections); err != nil {
		return application.TaskProfile{}, err
	}
	if sections == nil {
		sections = []application.CurriculumSection{}
	}
	return application.TaskProfile{
		ID: row.TaskID, Question: row.Question, Origin: row.Origin, Options: options,
		VoiceInstruction: row.VoiceInstruction, OutcomeID: row.OutcomeID, OutcomeName: row.OutcomeName,
		IncludeInTest: row.IncludeInTest, TaxonomyCode: row.TaxonomyCode, ALDLevelCode: row.AldLevelCode,
		Importance: row.Importance, EducationalContent: row.EducationalContent,
		ConstituentID: row.ConstituentID, ConstituentName: row.ConstituentName,
		TopicLevelCode: row.TopicLevelCode, CompetencyID: row.CompetencyID, CompetencyName: row.CompetencyName,
		CurriculumSections: sections,
	}, nil
}

func decodeOptions(raw []byte) ([]string, error) {
	var options []string
	if err := json.Unmarshal(raw, &options); err != nil {
		return nil, err
	}
	if options == nil {
		options = []string{}
	}
	return options, nil
}
