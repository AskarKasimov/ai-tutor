package application

import (
	"context"
	"errors"
	"io"
	"mime"
	"strings"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audioasset"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

const maxStoredAudioBytes = 64 << 20

type Service struct {
	reader AssetReader
	store  Storage
}

func NewService(reader AssetReader, store Storage) *Service {
	return &Service{reader: reader, store: store}
}

func (s *Service) Metadata(ctx context.Context, assetID string) (audioasset.Asset, error) {
	asset, err := s.reader.Get(ctx, assetID)
	if err != nil {
		return audioasset.Asset{}, err
	}
	return asset, nil
}

func (s *Service) Open(ctx context.Context, ownerID, assetID string) (io.ReadCloser, ObjectInfo, error) {
	asset, err := s.reader.GetAccessible(ctx, ownerID, assetID)
	if err != nil {
		if errors.Is(err, audioasset.ErrNotFound) {
			return nil, ObjectInfo{}, fault.New(fault.NotFound, "TASK_AUDIO_NOT_FOUND", "Аудиозапись задания не найдена.")
		}
		return nil, ObjectInfo{}, err
	}
	switch asset.Status {
	case audioasset.Pending, audioasset.Processing:
		return nil, ObjectInfo{}, fault.New(fault.Conflict, "AUDIO_NOT_READY", "Аудиозапись ещё готовится.")
	case audioasset.Failed:
		return nil, ObjectInfo{}, fault.New(fault.Unavailable, "AUDIO_GENERATION_FAILED", "Не удалось подготовить аудиозапись.")
	case audioasset.Ready:
	default:
		return nil, ObjectInfo{}, fault.New(fault.Unavailable, "AUDIO_STORAGE_UNAVAILABLE", "Аудиозапись временно недоступна.")
	}
	if asset.Bucket == nil || strings.TrimSpace(*asset.Bucket) == "" || asset.ObjectKey == "" {
		return nil, ObjectInfo{}, fault.New(fault.Unavailable, "AUDIO_STORAGE_UNAVAILABLE", "Аудиозапись временно недоступна.")
	}
	body, info, err := s.store.Open(ctx, *asset.Bucket, asset.ObjectKey)
	if err != nil {
		return nil, ObjectInfo{}, fault.New(fault.Unavailable, "AUDIO_STORAGE_UNAVAILABLE", "Аудиозапись временно недоступна.")
	}
	mediaType, _, parseErr := mime.ParseMediaType(info.ContentType)
	if body == nil || info.AssetID != asset.ID || info.Size < 1 || info.Size > maxStoredAudioBytes || parseErr != nil || mediaType != "audio/wav" {
		if body != nil {
			_ = body.Close()
		}
		return nil, ObjectInfo{}, fault.New(fault.Unavailable, "AUDIO_STORAGE_UNAVAILABLE", "Аудиозапись временно недоступна.")
	}
	return body, info, nil
}

var _ audioasset.MetadataReader = (*Service)(nil)
