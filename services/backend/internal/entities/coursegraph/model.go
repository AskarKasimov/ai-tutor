package coursegraph

import "encoding/json"

type Job struct {
	SubjectID, Fingerprint, Token string
	Attempts                      int
}

type Material struct {
	ID               string          `json:"id"`
	Markdown         string          `json:"markdown"`
	Week             *int32          `json:"week"`
	Kind             *string         `json:"kind"`
	Filename         string          `json:"filename"`
	KindLabel        string          `json:"kind_label"`
	MaterialRevision string          `json:"material_revision"`
	Topics           json.RawMessage `json:"topics,omitempty"`
}

// Where the graph of a snapshot comes from: a build from the course
// materials, the fixture Geek examines by until the first build, or nothing.
const (
	SourceBuilt   = "built"
	SourceFixture = "fixture"
	SourceNone    = "none"
)

// Snapshot is the published graph of a course and the state of its build.
// Status is that of the last requested build; InputsReady means every
// material of the course is processed, so a build can start.
type Snapshot struct {
	Status       string          `json:"status" enums:"not_requested,pending,processing,ready,failed"`
	Attempts     int32           `json:"attempts"`
	ErrorMessage string          `json:"error_message"`
	Graph        json.RawMessage `json:"graph" swaggertype:"object" extensions:"x-nullable"`
	GraphSource  string          `json:"graph_source" enums:"built,fixture,none"`
	UpdatedAt    int64           `json:"updated_at"`
	Stale        bool            `json:"stale"`
	InputsReady  bool            `json:"inputs_ready"`
	// Progress is where a running build is; only while Status is processing.
	// It lives in API memory, so after a restart it returns with the next poll.
	Progress *BuildProgress `json:"progress" extensions:"x-nullable"`
}

// BuildProgress is stage StageIndex of StageCount of a running build and the
// model batches of that stage finished so far; a stage without work has
// Total 0.
type BuildProgress struct {
	Stage      string `json:"stage" enums:"topics,synonyms,prerequisites,attach,questions"`
	StageIndex int    `json:"stage_index"`
	StageCount int    `json:"stage_count"`
	Done       int    `json:"done"`
	Total      int    `json:"total"`
}
