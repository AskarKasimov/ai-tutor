package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/competencymap"
	sharedpostgres "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupImportRepository(t *testing.T) (*Repository, context.Context, string) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL repository integration tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	schema := "competency_import_test_" + hex.EncodeToString(suffix)
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	if err := sharedpostgres.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	userID := "import-test-user"
	if _, err := pool.Exec(ctx, `INSERT INTO users(id,email,password_hash,created_at) VALUES ($1,$2,$3,$4)`, userID, userID+"@example.test", "hash", 1); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	return New(pool), ctx, userID
}

func importMap(name string) competencymap.Map {
	include := true
	importance := 4
	content := "Нужно уметь применить модель"
	return competencymap.Map{
		SourceFormat:  "ml-map",
		SourceHeaders: []string{"", "Компетенция", "Уровень темы", "Составляющая", "Образовательный результат", "Что должно войти в тест", "Таксономия", "Уровень ALDs", "Важность", "Раздел РПД · компетенции РПД", "Задание1"},
		Competencies:  []competencymap.Competency{{Key: "c", Name: "Компетенция " + name}},
		Constituents: []competencymap.Constituent{{
			Key: "s", CompetencyKey: "c", Name: "Составляющая " + name,
			TopicLevelCode: "basic",
			Sections:       []competencymap.CurriculumSection{{Code: "Р.1", Title: "Основы", CompetencyCodes: []string{"ОПК-2", "ПК-2"}}},
		}},
		Outcomes: []competencymap.Outcome{{
			Key: "o", ConstituentKey: "s", Name: "ОР " + name,
			IncludeInTest: &include, TaxonomyCode: "application", ALDLevelCode: "basic",
			Importance: &importance, EducationalContent: &content, SourceRowIndexes: []int{1},
		}},
		Tasks: []competencymap.Task{{
			OutcomeKey: "o", Question: "Вопрос " + name, Options: []string{"Первый", "Второй"},
			VoiceInstruction: "Объясните выбор", ReferenceAnswer: "Первый верен",
			Column: "Задание1", Row: 2, SourceRowIndex: 1, SourceColumnIndex: 11,
		}},
		SourceRows: []competencymap.SourceRow{{Index: 1, Line: 2, Cells: []string{
			"", "Компетенция " + name, "Базовый", "Составляющая " + name, "ОР " + name,
			"TRUE", "Применение", "базовый", "4", "Р.1 Основы", "Экран: тестовое задание",
		}}},
	}
}

func TestReplacePersistsTypedMapAndReplacesAllMapData(t *testing.T) {
	repository, ctx, userID := setupImportRepository(t)
	pool := repository.pool
	if _, err := pool.Exec(ctx, `INSERT INTO transcriptions(id,user_id,text,created_at) VALUES ('transcription-keep',$1,'сохранить',1)`, userID); err != nil {
		t.Fatal(err)
	}

	first, err := repository.Replace(ctx, userID, importMap("один"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != 1 || first.CompetencyCount != 1 || first.ConstituentCount != 1 || first.OutcomeCount != 1 || first.TaskCount != 1 {
		t.Fatalf("first import result = %#v", first)
	}
	assertImportedProfile(t, ctx, pool, "ОР один", "Вопрос один", 2)

	second, err := repository.Replace(ctx, userID, importMap("два"), 20)
	if err != nil {
		t.Fatal(err)
	}
	if second.Revision != 2 {
		t.Fatalf("replacement revision = %d, want 2", second.Revision)
	}
	var names []string
	if err := pool.QueryRow(ctx, `SELECT array_agg(name ORDER BY name) FROM outcomes`).Scan(&names); err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "ОР два" {
		t.Fatalf("active map after replacement = %v, want only new outcome", names)
	}
	var imports, sourceRows, oldTasks, transcriptions int
	for query, target := range map[string]*int{
		`SELECT count(*) FROM competency_map_imports`:                       &imports,
		`SELECT count(*) FROM competency_map_source_rows`:                   &sourceRows,
		`SELECT count(*) FROM tasks WHERE question='Вопрос один'`:           &oldTasks,
		`SELECT count(*) FROM transcriptions WHERE id='transcription-keep'`: &transcriptions,
	} {
		if err := pool.QueryRow(ctx, query).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	if imports != 1 || sourceRows != 1 || oldTasks != 0 || transcriptions != 1 {
		t.Fatalf("replacement cleanup/preservation imports=%d source_rows=%d old_tasks=%d transcriptions=%d", imports, sourceRows, oldTasks, transcriptions)
	}
}

func TestReplaceRollsBackInvalidMapWithoutLosingActiveMap(t *testing.T) {
	repository, ctx, userID := setupImportRepository(t)
	if _, err := repository.Replace(ctx, userID, importMap("сохранённая"), 10); err != nil {
		t.Fatal(err)
	}
	invalid := importMap("повреждённая")
	invalid.Outcomes[0].ConstituentKey = "missing-constituent"
	if _, err := repository.Replace(ctx, userID, invalid, 20); err == nil {
		t.Fatal("map with a missing constituent was imported")
	}
	var revision int64
	var outcomeName, question string
	if err := repository.pool.QueryRow(ctx, `SELECT revision FROM competency_map_state WHERE singleton=true`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if err := repository.pool.QueryRow(ctx, `SELECT outcome.name, task.question FROM outcomes outcome JOIN tasks task ON task.outcome_id=outcome.id`).Scan(&outcomeName, &question); err != nil {
		t.Fatal(err)
	}
	if revision != 1 || outcomeName != "ОР сохранённая" || question != "Вопрос сохранённая" {
		t.Fatalf("failed replacement changed active data: revision=%d outcome=%q task=%q", revision, outcomeName, question)
	}
}

func TestTaskSourceCoordinatesCannotBePartiallyNull(t *testing.T) {
	repository, ctx, userID := setupImportRepository(t)
	if _, err := repository.Replace(ctx, userID, importMap("координаты"), 10); err != nil {
		t.Fatal(err)
	}
	var outcomeID string
	var revision int64
	if err := repository.pool.QueryRow(ctx, `SELECT outcome_id, source_revision FROM tasks LIMIT 1`).Scan(&outcomeID, &revision); err != nil {
		t.Fatal(err)
	}
	_, err := repository.pool.Exec(ctx, `
INSERT INTO tasks(id,outcome_id,question,criteria,source_row,source_column,origin,
                  source_revision,source_row_index,source_column_index,created_at)
VALUES ('partial-coordinate-task',$1,'invalid',NULL,2,'Задание1','authored',$2,NULL,11,11)`, outcomeID, revision)
	if err == nil {
		t.Fatal("task with only source_revision and source_column_index was accepted")
	}
}

func assertImportedProfile(t *testing.T, ctx context.Context, pool *pgxpool.Pool, outcomeName, question string, rowLine int) {
	t.Helper()
	var gotOutcome, gotQuestion, taxonomy, ald, topic, section, codes, constituentName, sourceCell string
	var included bool
	var importance int16
	var rowIndex, columnIndex, sourceLine int
	var options []byte
	if err := pool.QueryRow(ctx, `
SELECT outcome.name, outcome.include_in_test, taxonomy.code, ald.code, outcome.importance,
       constituent.name, topic.code, section.code, source.cells->>10,
       task.question, task.options, task.source_row_index, task.source_column_index, source.source_line,
       codes.codes
FROM outcomes outcome
JOIN constituents constituent ON constituent.id=outcome.constituent_id
LEFT JOIN topic_levels topic ON topic.id=constituent.topic_level_id
LEFT JOIN taxonomies taxonomy ON taxonomy.id=outcome.taxonomy_id
LEFT JOIN ald_levels ald ON ald.id=outcome.ald_level_id
JOIN constituent_sections link ON link.constituent_id=constituent.id
JOIN curriculum_sections section ON section.id=link.section_id
JOIN tasks task ON task.outcome_id=outcome.id
JOIN competency_map_source_rows source ON source.revision=task.source_revision AND source.row_index=task.source_row_index
JOIN LATERAL (SELECT string_agg(curriculum.code, ',' ORDER BY curriculum.code) AS codes
              FROM constituent_section_competencies mapped
              JOIN curriculum_competencies curriculum ON curriculum.id=mapped.curriculum_competency_id
              WHERE mapped.constituent_section_id=link.id) codes ON true
WHERE outcome.name=$1`, outcomeName).Scan(&gotOutcome, &included, &taxonomy, &ald, &importance, &constituentName, &topic, &section, &sourceCell, &gotQuestion, &options, &rowIndex, &columnIndex, &sourceLine, &codes); err != nil {
		t.Fatal(err)
	}
	if gotOutcome != outcomeName || !included || taxonomy != "application" || ald != "basic" || importance != 4 || topic != "basic" || section != "Р.1" {
		t.Fatalf("stored profile fields mismatch for %q: taxonomy=%s ALD=%s importance=%d topic=%s section=%s", gotOutcome, taxonomy, ald, importance, topic, section)
	}
	if gotQuestion != question || rowIndex != 1 || columnIndex != 11 || sourceLine != rowLine {
		t.Fatalf("stored task/source mapping mismatch: question=%q row=%d col=%d physical=%d", gotQuestion, rowIndex, columnIndex, sourceLine)
	}
	if constituentName == "" || !strings.HasPrefix(sourceCell, "Экран:") {
		t.Fatalf("stored hierarchy/source cell are missing: constituent=%q cell=%q", constituentName, sourceCell)
	}
	var gotOptions []string
	if err := json.Unmarshal(options, &gotOptions); err != nil || len(gotOptions) != 2 || gotOptions[0] != "Первый" {
		t.Fatalf("stored options = %s, error=%v", options, err)
	}
	if codes != "ОПК-2,ПК-2" {
		t.Fatalf("stored curriculum competency codes = %q", codes)
	}
}
