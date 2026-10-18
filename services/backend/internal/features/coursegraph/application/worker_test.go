package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/coursegraph"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/material"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
)

type requestRepo struct {
	snapshot  coursegraph.Snapshot
	queued    bool
	published []json.RawMessage
}

func (r requestRepo) Published(context.Context) ([]json.RawMessage, error) {
	return r.published, nil
}

func (r requestRepo) Claim(context.Context, string, time.Time) (coursegraph.Job, error) {
	return coursegraph.Job{}, nil
}
func (r requestRepo) Finish(context.Context, coursegraph.Job, json.RawMessage, time.Time) (bool, error) {
	return false, nil
}
func (r requestRepo) Fail(context.Context, coursegraph.Job, string, time.Time) error { return nil }
func (r requestRepo) Read(context.Context, string) (coursegraph.Snapshot, error) {
	return r.snapshot, nil
}
func (r requestRepo) Request(context.Context, string, time.Time) (bool, error) { return r.queued, nil }

type requestMaterials []material.Material

func (m requestMaterials) List(context.Context, string) ([]material.Material, error) { return m, nil }

type progressStorage struct{}

func (progressStorage) Get(context.Context, string) ([]byte, error) { return []byte(`{}`), nil }

// progressBuilder reports progress and checks the snapshot shows it while
// the build runs.
type progressBuilder struct {
	t      *testing.T
	worker *Worker
	id     string
}

func (b *progressBuilder) Build(ctx context.Context, buildID string, _ []coursegraph.Material, report func(coursegraph.BuildProgress)) (json.RawMessage, error) {
	b.id = buildID
	report(coursegraph.BuildProgress{Stage: "attach", StageIndex: 4, StageCount: 5, Done: 1, Total: 3})
	snapshot, err := b.worker.Read(ctx, user.User{Role: user.Admin}, "subject")
	if err != nil || snapshot.Progress == nil || snapshot.Progress.Stage != "attach" || snapshot.Progress.Done != 1 {
		b.t.Errorf("snapshot during the build = %+v, %v", snapshot.Progress, err)
	}
	return json.RawMessage(`{"version":"course-graph-v1","topics":[{}],"prerequisites":[],"quality":{"acyclic":true}}`), nil
}

func TestSnapshotShowsBuildProgressOnlyWhileTheBuildRuns(t *testing.T) {
	week := int32(1)
	kind := material.Kind("lecture")
	items := requestMaterials{{ID: "m", Week: &week, Kind: &kind, S3ParsedKey: "parsed", Cleaning: &material.Cleaning{Status: "ready", S3ReportKey: "report"}}}
	builder := &progressBuilder{t: t}
	w := New(requestRepo{snapshot: coursegraph.Snapshot{Status: "processing"}}, items, progressStorage{}, builder, nil, nil)
	builder.worker = w
	// Review of #135: a reclaimed lease keeps the attempt number while the
	// previous call may still run in course-graph, so two calls of the same
	// course and attempt must not share a build ID.
	ids := map[string]bool{}
	for _, token := range []string{"lease-1", "lease-2"} {
		if err := w.process(context.Background(), coursegraph.Job{SubjectID: "subject", Token: token, Attempts: 2}); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(builder.id, "subject#") || strings.Contains(builder.id, token) {
			t.Fatalf("build ID = %q: the lease token must not leave the API", builder.id)
		}
		ids[builder.id] = true
	}
	if len(ids) != 2 {
		t.Fatalf("build IDs = %v: each call needs its own", ids)
	}
	snapshot, err := w.Read(context.Background(), user.User{Role: user.Admin}, "subject")
	if err != nil || snapshot.Progress != nil {
		t.Fatalf("snapshot after the build = %+v, %v", snapshot.Progress, err)
	}
}

type reportStorage map[string]string

func (s reportStorage) Get(_ context.Context, key string) ([]byte, error) { return []byte(s[key]), nil }

type capturingBuilder struct{ materials []coursegraph.Material }

func (b *capturingBuilder) Build(_ context.Context, _ string, materials []coursegraph.Material, _ func(coursegraph.BuildProgress)) (json.RawMessage, error) {
	b.materials = materials
	return json.RawMessage(`{"version":"course-graph-v1","topics":[{}],"prerequisites":[],"quality":{"acyclic":true}}`), nil
}

func TestBuildReusesOnlyCardsOfTheCurrentQuestionPolicy(t *testing.T) {
	// Old cards may ask about all outcomes; rebuild them under the
	// single-concept question policy.
	items := requestMaterials{
		{ID: "old", S3ParsedKey: "parsed", Cleaning: &material.Cleaning{Status: "ready", S3ReportKey: "old"}},
		{ID: "new", S3ParsedKey: "parsed", Cleaning: &material.Cleaning{Status: "ready", S3ReportKey: "new"}},
	}
	storage := reportStorage{
		"old": `{"topics_version":"topics-v3","topics":[{"question":"Нарисуй график функции и вычисли производную."}]}`,
		"new": `{"topics_version":"topics-v4","topics":[{"question":"Объясни, зачем нужна производная."}]}`,
	}
	builder := &capturingBuilder{}
	w := New(requestRepo{}, items, storage, builder, nil, nil)
	if err := w.process(context.Background(), coursegraph.Job{SubjectID: "subject"}); err != nil {
		t.Fatal(err)
	}
	if len(builder.materials) != 2 || builder.materials[0].Topics != nil || builder.materials[1].Topics == nil {
		t.Fatalf("materials = %+v: only topics-v4 cards may be reused", builder.materials)
	}
}

func TestRequestReportsClassifiedMaterialIDsWithoutPollingAsProcessing(t *testing.T) {
	week := int32(1)
	items := requestMaterials{{ID: "needs-classification", Status: material.StatusParsed,
		Week: &week, Cleaning: &material.Cleaning{Status: "ready"}}}
	w := New(requestRepo{snapshot: coursegraph.Snapshot{Status: "ready"}}, items, nil, nil, nil, nil)
	_, err := w.Request(context.Background(), user.User{Role: user.Admin}, "subject")
	if err == nil {
		t.Fatal("expected incomplete metadata error")
	}
	failure, ok := err.(interface{ Error() string })
	if !ok || failure.Error() != "MATERIAL_METADATA_INCOMPLETE" {
		t.Fatalf("error = %v", err)
	}
}

func TestRequestKeepsConflictPrecedenceDuringActiveBuild(t *testing.T) {
	w := New(requestRepo{snapshot: coursegraph.Snapshot{Status: "processing"}}, nil, nil, nil, nil, nil)
	_, err := w.Request(context.Background(), user.User{Role: user.Admin}, "subject")
	if err == nil || err.Error() != "GRAPH_BUILD_IN_PROGRESS" {
		t.Fatalf("error = %v", err)
	}
}

func TestTopicQuestionsAreThePreparedQuestionsOfTheGraph(t *testing.T) {
	graph := json.RawMessage(`{"topics":[{"question":" Почему ACO находит короткий путь? ","rephrase":" Как муравьи находят путь? "},{"question":""},{"title":"без вопроса","rephrase":"Сирота?"}]}`)
	if got := TopicQuestions(graph); len(got) != 2 || got[0] != "Почему ACO находит короткий путь?" || got[1] != "Как муравьи находят путь?" {
		t.Fatalf("questions = %q", got)
	}
	if got := TopicQuestions(json.RawMessage(`not json`)); len(got) != 0 {
		t.Fatalf("questions of an invalid graph = %q", got)
	}
}

type snapshotRepo struct {
	Repository
	snapshot coursegraph.Snapshot
}

func (r snapshotRepo) Read(context.Context, string) (coursegraph.Snapshot, error) {
	return r.snapshot, nil
}

type defaultGraph struct {
	graph exam.Graph
	ok    bool
	err   error
}

func (d defaultGraph) Current(context.Context, string) (exam.Graph, bool, error) {
	return d.graph, d.ok, d.err
}

var admin = user.User{ID: "admin", Role: user.Admin}

func TestReadShowsTheFixtureUntilTheFirstBuild(t *testing.T) {
	fixture := defaultGraph{ok: true, graph: exam.Graph{Version: "fixture:1", Nodes: []exam.Node{
		{ID: "a", Title: "Агент", Content: "Теория", Outcomes: []string{"различать агентов"}, Questions: []string{"Что такое агент?"}},
		{ID: "b", Title: "Среда", Prerequisites: []string{"a"}},
	}}}
	w := New(snapshotRepo{snapshot: coursegraph.Snapshot{Status: "not_requested", Graph: json.RawMessage("null")}}, nil, nil, nil, nil, fixture)
	got, err := w.Read(context.Background(), admin, "subject")
	if err != nil || got.GraphSource != coursegraph.SourceFixture {
		t.Fatalf("snapshot = %+v, %v", got, err)
	}
	var shape struct {
		Topics []struct {
			ID, Title, Question, Text string
			Provides                  []string
		}
		Prerequisites []struct{ Source, Target string }
		Hierarchy     []struct{ Source, Target string }
	}
	if err := json.Unmarshal(got.Graph, &shape); err != nil {
		t.Fatal(err)
	}
	if len(shape.Topics) != 2 || shape.Topics[0].Question != "Что такое агент?" || shape.Topics[0].Text != "Теория" ||
		len(shape.Topics[0].Provides) != 1 || shape.Hierarchy == nil {
		t.Fatalf("topics = %+v", shape)
	}
	if len(shape.Prerequisites) != 1 || shape.Prerequisites[0].Source != "a" || shape.Prerequisites[0].Target != "b" {
		t.Fatalf("prerequisites = %+v", shape.Prerequisites)
	}
}

func TestReadShowsTheBuildOnceItIsPublished(t *testing.T) {
	built := json.RawMessage(`{"topics":[{"id":"x","title":"Собранная тема"}]}`)
	w := New(snapshotRepo{snapshot: coursegraph.Snapshot{Status: "ready", Graph: built}}, requestMaterials{{ID: "m"}}, nil, nil, nil,
		defaultGraph{err: errors.New("the fixture must not be read")})
	got, err := w.Read(context.Background(), admin, "subject")
	if err != nil || got.GraphSource != coursegraph.SourceBuilt || string(got.Graph) != string(built) {
		t.Fatalf("snapshot = %+v, %v", got, err)
	}
}

func TestReadWithoutAFixtureHasNoGraph(t *testing.T) {
	w := New(snapshotRepo{snapshot: coursegraph.Snapshot{Status: "not_requested", Graph: json.RawMessage("null")}}, nil, nil, nil, nil, defaultGraph{})
	got, err := w.Read(context.Background(), admin, "subject")
	if err != nil || got.GraphSource != coursegraph.SourceNone || string(got.Graph) != "null" {
		t.Fatalf("snapshot = %+v, %v", got, err)
	}
}

func TestReadShowsTheFixtureWhenTheBuiltCourseLostItsMaterials(t *testing.T) {
	// Geek ignores a published graph of a course without materials, so the
	// admin must see the fixture Geek examines by, not the stored build.
	built := json.RawMessage(`{"topics":[{"id":"x","title":"Собранная тема"}]}`)
	fixture := defaultGraph{ok: true, graph: exam.Graph{Version: "fixture:1", Nodes: []exam.Node{{ID: "a", Title: "Агент"}}}}
	w := New(snapshotRepo{snapshot: coursegraph.Snapshot{Status: "ready", Graph: built}}, requestMaterials{}, nil, nil, nil, fixture)
	got, err := w.Read(context.Background(), admin, "subject")
	if err != nil || got.GraphSource != coursegraph.SourceFixture || string(got.Graph) == string(built) {
		t.Fatalf("snapshot = %+v, %v", got, err)
	}
	w = New(snapshotRepo{snapshot: coursegraph.Snapshot{Status: "ready", Graph: built}}, requestMaterials{}, nil, nil, nil, nil)
	if got, err = w.Read(context.Background(), admin, "subject"); err != nil || got.GraphSource != coursegraph.SourceNone || got.Graph != nil {
		t.Fatalf("snapshot without a fixture = %+v, %v", got, err)
	}
}

func TestABrokenFixtureDoesNotFailAnAcceptedBuildRequest(t *testing.T) {
	week := int32(1)
	kind := material.Kind("lecture")
	items := requestMaterials{{ID: "m", Status: material.StatusParsed, Week: &week, Kind: &kind, Cleaning: &material.Cleaning{Status: "ready"}}}
	repo := requestRepo{snapshot: coursegraph.Snapshot{Status: "not_requested", Graph: json.RawMessage("null")}, queued: true}
	w := New(repo, items, nil, nil, nil, defaultGraph{err: errors.New("fixture unreadable")})
	got, err := w.Request(context.Background(), admin, "subject")
	if err != nil || got.GraphSource != coursegraph.SourceNone {
		t.Fatalf("accepted request = %+v, %v", got, err)
	}
	if got, err = w.Read(context.Background(), admin, "subject"); err != nil || got.GraphSource != coursegraph.SourceNone {
		t.Fatalf("read with a broken fixture = %+v, %v", got, err)
	}
}

type queuedVoices struct{ texts []string }

func (v *queuedVoices) Prepare(texts []string) int {
	v.texts = append(v.texts, texts...)
	return len(texts)
}

func TestPrepareVoicesQueuesTheQuestionsOfEveryPublishedGraph(t *testing.T) {
	voices := &queuedVoices{}
	repo := requestRepo{published: []json.RawMessage{
		json.RawMessage(`{"topics":[{"question":"Почему A?"},{"question":"Почему B?"}]}`),
		json.RawMessage(`{"topics":[{"question":"Почему C?"}]}`),
	}}
	if err := New(repo, nil, nil, nil, voices, nil).PrepareVoices(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(voices.texts) != 3 || voices.texts[0] != "Почему A?" || voices.texts[2] != "Почему C?" {
		t.Fatalf("queued %q", voices.texts)
	}
	// Without a queue there is nothing to prepare.
	if err := New(repo, nil, nil, nil, nil, nil).PrepareVoices(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// publishingRepo records the graph a build publishes.
type publishingRepo struct {
	requestRepo
	finished []json.RawMessage
}

func (r *publishingRepo) Finish(_ context.Context, _ coursegraph.Job, graph json.RawMessage, _ time.Time) (bool, error) {
	r.finished = append(r.finished, graph)
	return true, nil
}

type stubQuestions struct {
	err  error
	seen json.RawMessage
}

func (q *stubQuestions) Prepare(_ context.Context, graph json.RawMessage) (json.RawMessage, error) {
	q.seen = graph
	if q.err != nil {
		return nil, q.err
	}
	return json.RawMessage(`{"version":"course-graph-v1","topics":[{"question":"Почему?","rephrase":"Отчего?"}],"prerequisites":[],"quality":{"acyclic":true}}`), nil
}

func TestBuildPublishesOnlyAGraphWithPreparedQuestions(t *testing.T) {
	items := requestMaterials{{ID: "m", S3ParsedKey: "parsed", Cleaning: &material.Cleaning{Status: "ready", S3ReportKey: "report"}}}
	for name, prepareErr := range map[string]error{"prepared": nil, "not prepared": errors.New("gateway down")} {
		t.Run(name, func(t *testing.T) {
			repo := &publishingRepo{}
			voices := &queuedVoices{}
			questions := &stubQuestions{err: prepareErr}
			w := New(repo, items, progressStorage{}, &capturingBuilder{}, voices, nil)
			w.SetQuestions(questions)
			err := w.process(context.Background(), coursegraph.Job{SubjectID: "subject"})
			if questions.seen == nil {
				t.Fatal("the build was not prepared")
			}
			if prepareErr != nil {
				// The previous graph stays published.
				if err == nil || len(repo.finished) != 0 || len(voices.texts) != 0 {
					t.Fatalf("err=%v published=%d voiced=%q", err, len(repo.finished), voices.texts)
				}
				return
			}
			if err != nil || len(repo.finished) != 1 || !strings.Contains(string(repo.finished[0]), `"rephrase":"Отчего?"`) {
				t.Fatalf("err=%v published=%s", err, repo.finished)
			}
			if len(voices.texts) != 2 || voices.texts[0] != "Почему?" || voices.texts[1] != "Отчего?" {
				t.Fatalf("voiced %q", voices.texts)
			}
		})
	}
}

// failingRepo hands out one job and records why it failed.
type failingRepo struct {
	publishingRepo
	claimed bool
	failed  []string
}

func (r *failingRepo) Claim(context.Context, string, time.Time) (coursegraph.Job, error) {
	if r.claimed {
		return coursegraph.Job{}, nil
	}
	r.claimed = true
	return coursegraph.Job{SubjectID: "subject", Token: "lease"}, nil
}

func (r *failingRepo) Fail(_ context.Context, _ coursegraph.Job, message string, _ time.Time) error {
	r.failed = append(r.failed, message)
	return nil
}

func TestAFailureToPrepareQuestionsIsReportedAsSuch(t *testing.T) {
	items := requestMaterials{{ID: "m", S3ParsedKey: "parsed", Cleaning: &material.Cleaning{Status: "ready", S3ReportKey: "report"}}}
	repo := &failingRepo{}
	w := New(repo, items, progressStorage{}, &capturingBuilder{}, nil, nil)
	w.SetQuestions(&stubQuestions{err: errors.New("gateway down")})
	if worked, err := w.ProcessOne(context.Background()); !worked || err == nil {
		t.Fatalf("worked=%v err=%v", worked, err)
	}
	if len(repo.failed) != 1 || !strings.Contains(repo.failed[0], "вопросы к темам не подготовлены") || len(repo.finished) != 0 {
		t.Fatalf("failed=%q published=%d", repo.failed, len(repo.finished))
	}
}
