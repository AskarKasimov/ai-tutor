package audioasset

import (
	"strings"
	"testing"
)

func TestTaskAudioIdentity(t *testing.T) {
	id := IDForTask("task_1")
	if id != "taskaudio_task_1" {
		t.Fatalf("asset ID: %s", id)
	}
	if got := ObjectKey(id); got != "task-audio/v1/taskaudio_task_1.wav" {
		t.Fatalf("object key: %s", got)
	}
}

func TestInstructionEligibility(t *testing.T) {
	if !CanSynthesize(strings.Repeat("я", 500)) {
		t.Fatal("500 runes rejected")
	}
	for _, value := range []string{"", " \n ", strings.Repeat("я", 501), "a\x00b", string([]byte{0xff})} {
		if CanSynthesize(value) {
			t.Fatalf("invalid instruction accepted: %q", value)
		}
	}
}
