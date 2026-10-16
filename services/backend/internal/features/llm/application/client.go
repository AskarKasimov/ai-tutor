package application

import "context"

// Completer hides endpoint selection and queueing from all consumers.
type Completer interface {
	Complete(context.Context, string, string) (string, error)
}
