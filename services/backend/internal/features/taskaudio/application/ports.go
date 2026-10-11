package application

import (
	"context"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audioasset"
)

type Claim struct {
	Asset audioasset.Asset
	Token string
}

type Queue interface {
	Prioritize(ctx context.Context, assetID string) (bool, error)
	Claim(ctx context.Context, token string, lease time.Duration) (Claim, bool, error)
	ClaimRepair(ctx context.Context, assetID, token string, lease time.Duration) (Claim, bool, error)
	IsCurrent(ctx context.Context, claim Claim) (bool, error)
	Cancel(ctx context.Context, claim Claim) (bool, error)
	Complete(ctx context.Context, claim Claim, bucket, storageURI, audioURL string) (bool, error)
	// Release returns an interrupted claim without counting it as a failed attempt.
	Release(ctx context.Context, claim Claim) (bool, error)
	Fail(ctx context.Context, claim Claim, code string, delay time.Duration) (bool, error)
	// FindReady returns another ready asset with the same instruction, if any.
	FindReady(ctx context.Context, instruction, excludeID string) (audioasset.Asset, bool, error)
}

type Prioritizer interface {
	Prioritize(context.Context, string) error
}

type AssetReader interface {
	Get(ctx context.Context, assetID string) (audioasset.Asset, error)
	GetAccessible(ctx context.Context, ownerID, assetID string) (audioasset.Asset, error)
}
