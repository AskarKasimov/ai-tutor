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
	Claim(ctx context.Context, token string, lease time.Duration) (Claim, bool, error)
	ClaimRepair(ctx context.Context, assetID, token string, lease time.Duration) (Claim, bool, error)
	IsCurrent(ctx context.Context, claim Claim) (bool, error)
	Cancel(ctx context.Context, claim Claim) (bool, error)
	Complete(ctx context.Context, claim Claim, bucket, storageURI, audioURL string) (bool, error)
	Fail(ctx context.Context, claim Claim, code string, delay time.Duration) (bool, error)
}

type AssetReader interface {
	Get(ctx context.Context, assetID string) (audioasset.Asset, error)
	GetAccessible(ctx context.Context, ownerID, assetID string) (audioasset.Asset, error)
}
