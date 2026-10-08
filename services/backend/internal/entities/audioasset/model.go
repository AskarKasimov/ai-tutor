// Package audioasset contains metadata shared by the task and audio features.
package audioasset

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

var ErrNotFound = errors.New("audio asset not found")
var ErrNotRepairable = errors.New("audio asset cannot be repaired in its current state")
var ErrStorageUnavailable = errors.New("audio storage unavailable")
var ErrGenerationFailed = errors.New("audio regeneration failed")

type Regenerator interface {
	Regenerate(context.Context, string) (Asset, error)
}

type MetadataReader interface {
	Metadata(context.Context, string) (Asset, error)
}

type Status string

const (
	Pending    Status = "pending"
	Processing Status = "processing"
	Ready      Status = "ready"
	Failed     Status = "failed"
	Cancelled  Status = "cancelled"
	Missing    Status = "missing"
)

type Asset struct {
	ID          string
	Instruction string
	ObjectKey   string
	Bucket      *string
	StorageURI  *string
	AudioURL    *string
	Status      Status
	Attempts    int
}

type Metadata struct {
	VariantTaskID string
	Status        Status
	AudioURL      *string
}

func IDForTask(taskID string) string { return "taskaudio_" + taskID }

func ObjectKey(assetID string) string { return "task-audio/v1/" + assetID + ".wav" }

// CanSynthesize validates instruction text without changing the text sent to TTS.
func CanSynthesize(instruction string) bool {
	if !utf8.ValidString(instruction) || strings.ContainsRune(instruction, '\x00') {
		return false
	}
	length := utf8.RuneCountInString(instruction)
	return length >= 1 && length <= 500 && strings.TrimSpace(instruction) != ""
}
