// geek-eval runs the real Geek prompts and grading on a saved graph without
// Docker, PostgreSQL, STT/TTS or the course-graph service.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/application"
	geekllm "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/infrastructure/llm"
)

type testCase struct {
	ID       string   `json:"id"`
	NodeID   string   `json:"node_id"`
	Question string   `json:"question"`
	Answers  []string `json:"answers"`
	Expected string   `json:"expected"`
}

type result struct {
	ID          string                   `json:"id"`
	NodeID      string                   `json:"node_id"`
	Question    string                   `json:"question,omitempty"`
	Expected    string                   `json:"expected,omitempty"`
	Assessments []application.Assessment `json:"assessments,omitempty"`
	Dialogue    []exam.Turn              `json:"dialogue,omitempty"`
	State       *exam.NodeState          `json:"state,omitempty"`
	Next        *exam.Step               `json:"next,omitempty"`
	Error       string                   `json:"error,omitempty"`
}

type report struct {
	CreatedAt   time.Time `json:"created_at"`
	Graph       string    `json:"graph"`
	GraphSHA256 string    `json:"graph_sha256"`
	Preview     bool      `json:"preview"`
	Model       string    `json:"model,omitempty"`
	Reasoning   string    `json:"reasoning"`
	Results     []result  `json:"results"`
	Calls       []call    `json:"calls"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	graphPath := flag.String("graph", "../../api/fixtures/intro-to-ai.weeks-1-2.ru.graph.json", "saved graph; never modified")
	casesPath := flag.String("cases", "testdata/geek-eval/cases.json", "scripted answers to fixed questions")
	only := flag.String("only", "", "evaluate only this case id")
	nodes := flag.String("nodes", "AI-01-13,AI-01-14", "nodes for fresh question generation")
	task := flag.String("task", "all", "questions, evaluate, or all")
	endpoint := flag.String("endpoint", "", "explicit evaluation-only OpenAI-compatible /v1 endpoint")
	model := flag.String("model", "gpt-oss-120b", "evaluation model")
	reasoning := flag.String("reasoning", "production", "production defaults, or override all calls: low, medium, high")
	preview := flag.Bool("preview", false, "export real requests without contacting a model")
	timeout := flag.Duration("timeout", 90*time.Second, "timeout per call")
	out := flag.String("out", "bin/geek-eval.json", "JSON report; Markdown is saved alongside it")
	flag.Parse()
	if *task != "all" && *task != "questions" && *task != "evaluate" {
		return errors.New("task must be questions, evaluate, or all")
	}
	if *reasoning != "production" && *reasoning != "low" && *reasoning != "medium" && *reasoning != "high" {
		return errors.New("reasoning must be production, low, medium, or high")
	}
	if !*preview {
		u, err := url.Parse(*endpoint)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
			return errors.New("provide -endpoint http://host:port/v1 or use -preview")
		}
	}
	data, err := os.ReadFile(*graphPath)
	if err != nil {
		return err
	}
	var graph exam.Graph
	if err := json.Unmarshal(data, &graph); err != nil {
		return err
	}
	if err := graph.Validate(); err != nil {
		return err
	}
	graphHash := fmt.Sprintf("%x", sha256.Sum256(data))
	var cases []testCase
	if *task != "questions" {
		data, err := os.ReadFile(*casesPath)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &cases); err != nil {
			return err
		}
		for _, c := range cases {
			if _, ok := graph.Node(c.NodeID); !ok || c.ID == "" || c.Question == "" || len(c.Answers) == 0 {
				return fmt.Errorf("invalid case %q", c.ID)
			}
		}
		if *only != "" {
			selected := []testCase{}
			for _, c := range cases {
				if c.ID == *only {
					selected = append(selected, c)
				}
			}
			if len(selected) == 0 {
				return fmt.Errorf("unknown case %q", *only)
			}
			cases = selected
		}
	}
	t := &transport{endpoint: *endpoint, model: *model, reasoning: *reasoning, preview: *preview, http: &http.Client{Timeout: *timeout}}
	g := geekllm.NewGateway(t, *timeout)
	r := report{CreatedAt: time.Now().UTC(), Graph: *graphPath, GraphSHA256: graphHash, Preview: *preview, Model: *model, Reasoning: *reasoning}
	ctx := context.Background()
	failed := false
	if *task != "evaluate" {
		for _, id := range strings.Split(*nodes, ",") {
			n, ok := graph.Node(strings.TrimSpace(id))
			if !ok {
				return fmt.Errorf("unknown node %q", id)
			}
			t.label = "question:" + n.ID
			fmt.Fprintln(os.Stderr, t.label)
			q, err := g.Question(ctx, application.QuestionRequest{Kind: exam.KindQuestion, Node: n})
			entry := result{ID: t.label, NodeID: n.ID, Question: q}
			if err != nil && !errors.Is(err, errPreview) {
				entry.Error = err.Error()
				failed = true
			}
			r.Results = append(r.Results, entry)
			r.Calls = t.calls
			if err := saveReport(*out, r); err != nil {
				return err
			}
		}
	}
	for _, c := range cases {
		fmt.Fprintln(os.Stderr, "evaluate:"+c.ID)
		entry := evaluateCase(ctx, g, t, graph, c)
		failed = failed || entry.Error != ""
		r.Results = append(r.Results, entry)
		r.Calls = t.calls
		if err := saveReport(*out, r); err != nil {
			return err
		}
	}
	r.Calls = t.calls
	if err := saveReport(*out, r); err != nil {
		return err
	}
	fmt.Printf("Saved %s and %s.md (%d calls, preview=%v)\n", *out, strings.TrimSuffix(*out, filepath.Ext(*out)), len(r.Calls), r.Preview)
	if failed {
		return errors.New("some calls failed; see report")
	}
	return nil
}

func evaluateCase(ctx context.Context, g *geekllm.Gateway, t *transport, graph exam.Graph, c testCase) result {
	n, _ := graph.Node(c.NodeID)
	r := result{ID: c.ID, NodeID: n.ID, Question: c.Question, Expected: c.Expected}
	prereqs := []string{}
	for _, id := range graph.Closure(n.ID) {
		p, _ := graph.Node(id)
		prereqs = append(prereqs, p.Title)
	}
	// Isolate one topic's state transitions while retaining its real prompt
	// content and prerequisite disclosure restrictions from the saved graph.
	isolated := n
	isolated.Prerequisites = nil
	e, step := exam.NewExam(exam.Graph{Nodes: []exam.Node{isolated}}, 3, nil)
	appendTurn := func(speaker exam.Speaker, kind exam.TurnKind, text string) {
		id := n.ID
		r.Dialogue = append(r.Dialogue, exam.Turn{Line: len(r.Dialogue) + 1, Speaker: speaker, Kind: kind, NodeID: &id, Text: text})
	}
	appendTurn(exam.SpeakerGeek, step.Kind, c.Question)
	for i, answer := range c.Answers {
		appendTurn(exam.SpeakerStudent, exam.KindAnswer, answer)
		t.label = fmt.Sprintf("evaluate:%s:%d", c.ID, i+1)
		a, err := g.Evaluate(ctx, application.EvaluationRequest{Kind: step.Kind, Node: n, Prerequisites: prereqs, Dialogue: r.Dialogue})
		if errors.Is(err, errPreview) {
			break
		}
		if err != nil {
			r.Error = err.Error()
			break
		}
		r.Assessments = append(r.Assessments, a)
		step = e.Apply(a.Evaluation)
		r.Next = &step
		if step.Kind == exam.KindClosing {
			appendTurn(exam.SpeakerGeek, step.Kind, strings.TrimSpace(a.Reaction+" "+a.Explanation))
			break
		}
		t.label = fmt.Sprintf("%s:%s:%d", step.Kind, c.ID, i+1)
		q, err := g.Question(ctx, application.QuestionRequest{Kind: step.Kind, Node: n, Dialogue: r.Dialogue})
		if err != nil {
			r.Error = err.Error()
			break
		}
		appendTurn(exam.SpeakerGeek, step.Kind, strings.TrimSpace(a.Reaction+"\n\n"+q))
	}
	if !t.preview {
		r.State = e.Nodes[n.ID]
	}
	return r
}

func saveReport(path string, r report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	var md strings.Builder
	fmt.Fprintf(&md, "# Geek: isolated prompt evaluation\n\nModel: %s; reasoning: %s; preview: %v.\n\nGraph: `%s`.\n\n", r.Model, r.Reasoning, r.Preview, r.Graph)
	md.WriteString("Fixed questions test grading independently of fresh generated questions. Expected outcomes are human-written review notes, not an automated quality score.\n\n")
	for _, entry := range r.Results {
		fmt.Fprintf(&md, "## %s (%s)\n\n%s\n\n", entry.ID, entry.NodeID, entry.Question)
		if entry.Expected != "" {
			fmt.Fprintf(&md, "Expected: %s\n\n", entry.Expected)
		}
		for _, turn := range entry.Dialogue {
			fmt.Fprintf(&md, "**%s:** %s\n\n", turn.Speaker, turn.Text)
		}
		for i, a := range entry.Assessments {
			fmt.Fprintf(&md, "Assessment %d: %s; K1=%d, K2=%d, K3=%s, K4=%s.\n\n", i+1, a.Verdict, a.Criteria.K1, a.Criteria.K2, criterion(a.Criteria.K3, a.Unassessed.K3), criterion(a.Criteria.K4, a.Unassessed.K4))
		}
		if entry.State != nil {
			state, _ := json.Marshal(entry.State)
			fmt.Fprintf(&md, "State: `%s`\n\n", state)
		}
		if entry.Error != "" {
			fmt.Fprintf(&md, "ERROR: %s\n\n", entry.Error)
		}
	}
	md.WriteString("## Calls\n\n| Operation | Seconds | Error |\n| --- | ---: | --- |\n")
	for _, c := range r.Calls {
		fmt.Fprintf(&md, "| %s | %.2f | %s |\n", c.Label, c.Seconds, strings.ReplaceAll(c.Error, "|", "/"))
	}
	return os.WriteFile(strings.TrimSuffix(path, filepath.Ext(path))+".md", []byte(md.String()), 0o644)
}

func criterion(value int, unassessed bool) string {
	if unassessed {
		return "null"
	}
	return fmt.Sprint(value)
}
