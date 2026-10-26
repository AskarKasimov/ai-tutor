package application

import (
	"context"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audioasset"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
)

type audioMetadataRepo struct{ session training.Session }

func (r *audioMetadataRepo) Get(context.Context, string, string) (training.Session, error) {
	return r.session, nil
}
func (*audioMetadataRepo) ByDiagnostic(context.Context, string, string) (training.Session, error) {
	panic("unused")
}
func (*audioMetadataRepo) ByStartKey(context.Context, string, string, string) (training.Session, bool, error) {
	panic("unused")
}
func (*audioMetadataRepo) Practice(context.Context, string) (int64, []training.Task, error) {
	panic("unused")
}
func (*audioMetadataRepo) Create(context.Context, training.Session, string, *int64) (training.Session, bool, error) {
	panic("unused")
}
func (*audioMetadataRepo) Reserve(context.Context, string, string, string, string, string, string) (*training.Progress, *training.Reservation, training.Session, error) {
	panic("unused")
}
func (*audioMetadataRepo) Accept(context.Context, string, training.Reservation, training.Session, training.Attempt) (training.Progress, error) {
	panic("unused")
}
func (*audioMetadataRepo) Fail(context.Context, string, string, training.Reservation) error {
	panic("unused")
}
func (*audioMetadataRepo) ResetReservation(context.Context, string, string) error { panic("unused") }
func (*audioMetadataRepo) History(context.Context, string, string, int64, int) ([]training.Attempt, error) {
	panic("unused")
}

type audioMetadataReader struct{ asset audioasset.Asset }

func (r audioMetadataReader) Metadata(context.Context, string) (audioasset.Asset, error) {
	return r.asset, nil
}

type audioMetadataRegenerator struct{ asset audioasset.Asset }

func (r audioMetadataRegenerator) Regenerate(context.Context, string) (audioasset.Asset, error) {
	return r.asset, nil
}

func TestTrainingAudioMetadataCanonicalizesLegacyURLAndRegeneration(t *testing.T) {
	assetID := "asset-1"
	legacy := "/task-audio/asset-1/file"
	asset := audioasset.Asset{ID: assetID, Status: audioasset.Ready, AudioURL: &legacy}
	repo := &audioMetadataRepo{session: training.Session{ID: "s", OwnerID: "u", Current: training.Exercise{ID: "e", Task: training.Task{AudioAssetID: &assetID}}}}
	service := New(repo, nil, nil, nil, nil, nil, nil).WithAudio(audioMetadataReader{asset})
	got, err := service.CurrentAudio(context.Background(), "u", "s", "e")
	if err != nil || got.AudioURL == nil || *got.AudioURL != audioasset.FileURL(assetID) {
		t.Fatalf("current metadata=%#v err=%v", got, err)
	}

	service = New(repo, nil, nil, nil, nil, nil, nil).WithAudioRegenerator(audioMetadataRegenerator{asset})
	got, err = service.RegenerateCurrentAudio(context.Background(), "u", "s", "e")
	if err != nil || got.AudioURL == nil || *got.AudioURL != audioasset.FileURL(assetID) {
		t.Fatalf("regenerated metadata=%#v err=%v", got, err)
	}

	foreign := "/task-audio/other-asset/file"
	service = New(repo, nil, nil, nil, nil, nil, nil).WithAudio(audioMetadataReader{audioasset.Asset{ID: assetID, Status: audioasset.Ready, AudioURL: &foreign}})
	if _, err = service.CurrentAudio(context.Background(), "u", "s", "e"); err == nil {
		t.Fatal("accepted URL for a different asset")
	}

	wrongIDURL := audioasset.FileURL("other-asset")
	service = New(repo, nil, nil, nil, nil, nil, nil).WithAudioRegenerator(audioMetadataRegenerator{audioasset.Asset{
		ID: "other-asset", Status: audioasset.Ready, AudioURL: &wrongIDURL,
	}})
	if _, err = service.RegenerateCurrentAudio(context.Background(), "u", "s", "e"); err == nil {
		t.Fatal("accepted regenerated metadata for a different asset")
	}
}
