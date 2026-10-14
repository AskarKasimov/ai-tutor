package transcription

import "context"

// Reader returns only a transcription owned by the authenticated student.
type Reader interface {
	TextByOwner(context.Context, string, string) (string, error)
}

type Transcription struct {
	ID        string
	OwnerID   string
	Text      string
	CreatedAt int64
}
