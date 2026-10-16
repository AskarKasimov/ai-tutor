package coursegraph

import "encoding/json"

type Job struct {
	SubjectID, Fingerprint, Token string
	Attempts                      int
}

type Material struct {
	ID       string          `json:"id"`
	Markdown string          `json:"markdown"`
	Topics   json.RawMessage `json:"topics,omitempty"`
}

type Snapshot struct {
	Status       string          `json:"status"`
	Attempts     int32           `json:"attempts"`
	ErrorMessage string          `json:"error_message"`
	Graph        json.RawMessage `json:"graph" swaggertype:"object" extensions:"x-nullable"`
	UpdatedAt    int64           `json:"updated_at"`
	Stale        bool            `json:"stale"`
}
