package audiopreparation

import "time"

type State string

const (
	NoGraph    State = "no_graph"
	Checking   State = "checking"
	Queued     State = "queued"
	Processing State = "processing"
	RetryWait  State = "retry_wait"
	Partial    State = "partial"
	Ready      State = "ready"
	Empty      State = "empty"
)

type Status string

const (
	StatusUnchecked   Status = "unchecked"
	StatusQueued      Status = "queued"
	StatusProcessing  Status = "processing"
	StatusRetryWait   Status = "retry_wait"
	StatusReady       Status = "ready"
	StatusFailed      Status = "failed"
	StatusUnavailable Status = "unavailable"
)

type Counts struct {
	Unchecked   int `json:"unchecked"`
	Queued      int `json:"queued"`
	Processing  int `json:"processing"`
	RetryWait   int `json:"retry_wait"`
	Ready       int `json:"ready"`
	Failed      int `json:"failed"`
	Unavailable int `json:"unavailable"`
}

func (c Counts) Sum() int {
	return c.Unchecked + c.Queued + c.Processing + c.RetryWait + c.Ready + c.Failed + c.Unavailable
}

func (c Counts) State(published bool, total int) State {
	if !published {
		return NoGraph
	}
	if total == 0 {
		return Empty
	}
	if c.Unchecked > 0 {
		return Checking
	}
	if c.Processing > 0 {
		return Processing
	}
	if c.Queued > 0 {
		return Queued
	}
	if c.RetryWait > 0 {
		return RetryWait
	}
	if c.Failed+c.Unavailable > 0 {
		return Partial
	}
	if c.Ready == total {
		return Ready
	}
	return Partial
}

type Direction struct {
	Version string `json:"version"`
	State   State  `json:"state"`
	Total   int    `json:"total"`
	Counts
	ObservedAt time.Time `json:"observed_at"`
}

type Exam struct {
	Direction
	Published bool `json:"published"`
}

type ExamSnapshot struct {
	Version string
	Total   int
	Counts
	ObservedAt time.Time
}

type Learning struct {
	Direction
	Revision int64 `json:"revision"`
}

type Response struct {
	SubjectID string   `json:"subject_id"`
	Exam      Exam     `json:"exam"`
	Learn     Learning `json:"learn"`
}

type LearningAsset struct {
	TaskID        string
	Instruction   string
	AssetID       *string
	Status        *string
	Attempts      int
	NextAttemptAt *time.Time
	LeaseUntil    *time.Time
}
