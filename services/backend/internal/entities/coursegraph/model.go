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

// Snapshot is the published graph of a course and the state of its build.
// Status is that of the last requested build; InputsReady means every
// material of the course is processed, so a build can start.
type Snapshot struct {
	Status       string          `json:"status" enums:"not_requested,pending,processing,ready,failed"`
	Attempts     int32           `json:"attempts"`
	ErrorMessage string          `json:"error_message"`
	Graph        json.RawMessage `json:"graph" swaggertype:"object" extensions:"x-nullable"`
	UpdatedAt    int64           `json:"updated_at"`
	Stale        bool            `json:"stale"`
	InputsReady  bool            `json:"inputs_ready"`
}
