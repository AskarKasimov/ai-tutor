package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/competencymap"
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/security"
)

type Repository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, queries: db.New(pool)}
}

func (r *Repository) Replace(ctx context.Context, actorID string, parsed competencymap.Map, importedAt int64) (competencymap.ImportResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return competencymap.ImportResult{}, err
	}
	defer tx.Rollback(ctx)
	queries := r.queries.WithTx(tx)

	previousRevision, err := queries.LockCompetencyMapRevision(ctx)
	if err != nil {
		return competencymap.ImportResult{}, err
	}
	result := competencymap.ImportResult{
		Revision: previousRevision + 1, ImportedAt: importedAt,
		CompetencyCount: len(parsed.Competencies), ConstituentCount: len(parsed.Constituents),
		OutcomeCount: len(parsed.Outcomes), TaskCount: len(parsed.Tasks),
		UnparsedTaskCells: parsed.UnparsedTaskCells, Warnings: parsed.Warnings,
	}

	if err = queries.DeleteCompetencies(ctx); err != nil {
		return competencymap.ImportResult{}, err
	}
	if err = queries.DeleteCompetencyMapSourceRows(ctx); err != nil {
		return competencymap.ImportResult{}, err
	}
	if err = queries.DeleteCompetencyMapImports(ctx); err != nil {
		return competencymap.ImportResult{}, err
	}

	headers, err := json.Marshal(parsed.SourceHeaders)
	if err != nil {
		return competencymap.ImportResult{}, err
	}
	if err = queries.InsertCompetencyMapImport(ctx, db.InsertCompetencyMapImportParams{
		Revision: result.Revision, ImportedAt: result.ImportedAt, ImportedBy: actorID,
		CompetencyCount: int32(result.CompetencyCount), ConstituentCount: int32(result.ConstituentCount),
		OutcomeCount: int32(result.OutcomeCount), TaskCount: int32(result.TaskCount),
		SourceFormat: parsed.SourceFormat, SourceHeaders: headers,
		UnparsedTaskCellCount: int32(result.UnparsedTaskCells),
	}); err != nil {
		return competencymap.ImportResult{}, err
	}

	sourceRows := make(map[int]int, len(parsed.SourceRows))
	sourceCells := make(map[int][]string, len(parsed.SourceRows))
	for index, source := range parsed.SourceRows {
		rowIndex := source.Index
		if rowIndex == 0 {
			rowIndex = index + 1
		}
		cells, err := json.Marshal(source.Cells)
		if err != nil {
			return competencymap.ImportResult{}, err
		}
		if err = queries.InsertCompetencyMapSourceRow(ctx, db.InsertCompetencyMapSourceRowParams{
			Revision: result.Revision, RowIndex: int32(rowIndex), SourceLine: int32(source.Line), Cells: cells,
		}); err != nil {
			return competencymap.ImportResult{}, err
		}
		sourceRows[rowIndex] = source.Line
		sourceCells[rowIndex] = source.Cells
	}

	competencyIDs := make(map[string]string, len(parsed.Competencies))
	for _, competency := range parsed.Competencies {
		id, err := security.ID("competency")
		if err != nil {
			return competencymap.ImportResult{}, err
		}
		if err = queries.InsertCompetency(ctx, db.InsertCompetencyParams{ID: id, Name: competency.Name, Revision: result.Revision}); err != nil {
			return competencymap.ImportResult{}, err
		}
		competencyIDs[competency.Key] = id
	}

	constituentIDs := make(map[string]string, len(parsed.Constituents))
	for _, constituent := range parsed.Constituents {
		id, err := security.ID("constituent")
		if err != nil {
			return competencymap.ImportResult{}, err
		}
		topicLevelID := ""
		if constituent.TopicLevelCode != "" {
			topicLevelID = "topic:" + constituent.TopicLevelCode
		}
		if err = queries.InsertConstituent(ctx, db.InsertConstituentParams{
			ID: id, CompetencyID: competencyIDs[constituent.CompetencyKey], Name: constituent.Name, TopicLevelID: topicLevelID,
		}); err != nil {
			return competencymap.ImportResult{}, err
		}
		constituentIDs[constituent.Key] = id
	}

	sectionIDs := map[string]string{}
	sectionTitles := map[string]string{}
	curriculumCompetencyIDs := map[string]string{}
	constituentSectionIDs := map[string]string{}
	for _, constituent := range parsed.Constituents {
		constituentID := constituentIDs[constituent.Key]
		for _, section := range constituent.Sections {
			sectionID := sectionIDs[section.Code]
			if sectionID == "" {
				sectionID, err = security.ID("curriculum-section")
				if err != nil {
					return competencymap.ImportResult{}, err
				}
				if err = queries.InsertCurriculumSection(ctx, db.InsertCurriculumSectionParams{
					ID: sectionID, Revision: result.Revision, Code: section.Code, Title: section.Title,
				}); err != nil {
					return competencymap.ImportResult{}, err
				}
				sectionIDs[section.Code], sectionTitles[section.Code] = sectionID, section.Title
			} else if sectionTitles[section.Code] != section.Title {
				return competencymap.ImportResult{}, errCurriculumSectionConflict(section.Code)
			}

			linkKey := constituent.Key + "\x00" + section.Code
			linkID := constituentSectionIDs[linkKey]
			if linkID == "" {
				linkID, err = security.ID("constituent-section")
				if err != nil {
					return competencymap.ImportResult{}, err
				}
				if err = queries.InsertConstituentSection(ctx, db.InsertConstituentSectionParams{
					ID: linkID, Revision: result.Revision, ConstituentID: constituentID, SectionID: sectionID,
				}); err != nil {
					return competencymap.ImportResult{}, err
				}
				constituentSectionIDs[linkKey] = linkID
			}
			for _, code := range section.CompetencyCodes {
				id := curriculumCompetencyIDs[code]
				if id == "" {
					id, err = security.ID("curriculum-competency")
					if err != nil {
						return competencymap.ImportResult{}, err
					}
					if err = queries.InsertCurriculumCompetency(ctx, db.InsertCurriculumCompetencyParams{ID: id, Revision: result.Revision, Code: code}); err != nil {
						return competencymap.ImportResult{}, err
					}
					curriculumCompetencyIDs[code] = id
				}
				if err = queries.InsertConstituentSectionCompetency(ctx, db.InsertConstituentSectionCompetencyParams{
					Revision: result.Revision, ConstituentSectionID: linkID, CurriculumCompetencyID: id,
				}); err != nil {
					return competencymap.ImportResult{}, err
				}
			}
		}
	}

	outcomeIDs := make(map[string]string, len(parsed.Outcomes))
	for _, outcome := range parsed.Outcomes {
		id, err := security.ID("outcome")
		if err != nil {
			return competencymap.ImportResult{}, err
		}
		var importance int16
		if outcome.Importance != nil {
			value := int16(*outcome.Importance)
			importance = value
		}
		if err = queries.InsertOutcome(ctx, db.InsertOutcomeParams{
			ID: id, ConstituentID: constituentIDs[outcome.ConstituentKey], Name: outcome.Name,
			IncludeInTest: outcome.IncludeInTest,
			TaxonomyCode:  outcome.TaxonomyCode, AldLevelCode: outcome.ALDLevelCode,
			Importance: importance, EducationalContent: stringValue(outcome.EducationalContent),
		}); err != nil {
			return competencymap.ImportResult{}, err
		}
		outcomeIDs[outcome.Key] = id
	}

	for _, outcome := range parsed.Outcomes {
		for _, rowIndex := range outcome.SourceRowIndexes {
			if _, exists := sourceRows[rowIndex]; !exists {
				return competencymap.ImportResult{}, errMissingSourceRow(rowIndex)
			}
			if err = queries.InsertOutcomeSourceRow(ctx, db.InsertOutcomeSourceRowParams{
				OutcomeID: outcomeIDs[outcome.Key], SourceRevision: result.Revision, SourceRowIndex: int32(rowIndex),
			}); err != nil {
				return competencymap.ImportResult{}, err
			}
		}
	}

	for _, task := range parsed.Tasks {
		if task.SourceRowIndex <= 0 || task.SourceRowIndex > len(parsed.SourceRows) || task.SourceColumnIndex <= 0 || task.SourceColumnIndex > len(parsed.SourceHeaders) || parsed.SourceHeaders[task.SourceColumnIndex-1] != task.Column {
			return competencymap.ImportResult{}, fmt.Errorf("task %q has invalid source coordinates", task.Question)
		}
		if _, exists := sourceCells[task.SourceRowIndex]; !exists || task.SourceColumnIndex > len(sourceCells[task.SourceRowIndex]) {
			return competencymap.ImportResult{}, errMissingSourceRow(task.SourceRowIndex)
		}
		id, err := security.ID("task")
		if err != nil {
			return competencymap.ImportResult{}, err
		}
		options := task.Options
		if options == nil {
			options = []string{}
		}
		encodedOptions, err := json.Marshal(options)
		if err != nil {
			return competencymap.ImportResult{}, err
		}
		sourceRow, sourceColumn := int32(task.Row), task.Column
		if err = queries.InsertTask(ctx, db.InsertTaskParams{
			ID: id, OutcomeID: outcomeIDs[task.OutcomeKey], Question: task.Question,
			Criteria: nullableText(task.Criteria), SourceRow: &sourceRow, SourceColumn: &sourceColumn,
			Options: encodedOptions, VoiceInstruction: nullableText(task.VoiceInstruction),
			ReferenceAnswer: nullableText(task.ReferenceAnswer), SourceRevision: result.Revision,
			SourceRowIndex: int32(task.SourceRowIndex), SourceColumnIndex: int32(task.SourceColumnIndex), CreatedAt: importedAt,
		}); err != nil {
			return competencymap.ImportResult{}, err
		}
	}

	if err = queries.UpdateCompetencyMapRevision(ctx, result.Revision); err != nil {
		return competencymap.ImportResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return competencymap.ImportResult{}, err
	}
	return result, nil
}

func nullableText(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func errCurriculumSectionConflict(code string) error {
	return fmt.Errorf("conflicting title for curriculum section %q", code)
}

func errMissingSourceRow(index int) error {
	return fmt.Errorf("source row index %d is missing", index)
}
