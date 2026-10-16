package material

import "errors"

var ErrNotFound = errors.New("material not found")

type Status string

const (
	StatusPending Status = "pending"
	StatusParsed  Status = "parsed"
	StatusFailed  Status = "failed"
)

type Material struct {
	Cleaning     *Cleaning `json:"cleaning,omitempty" binding:"optional"`
	ID           string    `json:"id"`
	SubjectID    string    `json:"subject_id"`
	Filename     string    `json:"filename"`
	ContentType  string    `json:"content_type"`
	FileSize     int64     `json:"file_size"`
	S3RawKey     string    `json:"s3_raw_key"`
	S3ParsedKey  string    `json:"s3_parsed_key"`
	ParsedText   string    `json:"parsed_text"`
	CharCount    int       `json:"char_count"`
	Status       Status    `json:"status"`
	ErrorMessage *string   `json:"error_message,omitempty" binding:"optional"`
	CreatedAt    int64     `json:"created_at"`
	UpdatedAt    int64     `json:"updated_at"`
}
