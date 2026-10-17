package material

import (
	"errors"
)

var ErrNotFound = errors.New("material not found")

type Kind string

type KindOption struct {
	Code  Kind   `json:"code"`
	Label string `json:"label"`
}

type Status string

const (
	StatusPending Status = "pending"
	StatusParsed  Status = "parsed"
	StatusFailed  Status = "failed"
)

type Material struct {
	Week         *int32         `json:"week" extensions:"x-nullable"`
	Kind         *Kind          `json:"kind" extensions:"x-nullable"`
	Cleaning     *Cleaning      `json:"cleaning,omitempty" binding:"optional"`
	Processing   *Processing    `json:"processing,omitempty" binding:"optional"`
	ID           string         `json:"id"`
	SubjectID    string         `json:"subject_id"`
	Filename     string         `json:"filename"`
	ContentType  string         `json:"content_type"`
	FileSize     int64          `json:"file_size"`
	S3RawKey     string         `json:"s3_raw_key"`
	S3ParsedKey  string         `json:"s3_parsed_key"`
	S3OCRKey     string         `json:"-"`
	OCRReport    map[string]any `json:"ocr_report" extensions:"x-nullable"`
	ParsedText   string         `json:"parsed_text"`
	CharCount    int            `json:"char_count"`
	Status       Status         `json:"status"`
	ErrorMessage *string        `json:"error_message,omitempty" binding:"optional"`
	CreatedAt    int64          `json:"created_at"`
	UpdatedAt    int64          `json:"updated_at"`
}

type Processing struct {
	Stage     string `json:"stage" enums:"queued,waiting_models,ocr,translation,cleaning,graph,ready,failed"`
	Status    string `json:"status" enums:"pending,processing,ready,failed"`
	UpdatedAt int64  `json:"updated_at"`
	Attempts  int    `json:"attempts"`
}
