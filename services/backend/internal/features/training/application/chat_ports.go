package application

import (
	"context"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/assessment"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
)

type Tutor interface {
	Reply(context.Context, training.TutorContext) (training.TutorReply, error)
}

type FinalAnswerGrader interface {
	GradeFinal(context.Context, training.Exercise, string) (assessment.Evaluation, error)
}

type ChatRepository interface {
	Read(context.Context, string, string, string, int64, int) (training.ChatPage, error)
	ReserveTurn(context.Context, string, string, training.TurnInput) (training.TurnReservation, *training.TurnResult, error)
	ResumeTurn(context.Context, string, string, string, string) (training.TurnReservation, *training.TurnResult, error)
	SaveTranscript(context.Context, training.TurnReservation, string, string) error
	CommitTurn(context.Context, training.TurnReservation, training.TurnCommit) (training.TurnResult, error)
	FailTurn(context.Context, training.TurnReservation, string) error
	CancelTurn(context.Context, string, string, string, string) error
}

type CheckedNextRepository interface {
	ReserveNext(context.Context, string, string, string, string, int64, string) (*training.Progress, *training.NextReservation, error)
	AcceptNext(context.Context, training.NextReservation, training.Session, training.Attempt) (training.Progress, error)
	FailNext(context.Context, training.NextReservation) error
}
