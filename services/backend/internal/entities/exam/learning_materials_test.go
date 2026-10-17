package exam

import "testing"

func ptr[T any](v T) *T { return &v }

func TestSelectLearningMaterialsUsesOnlyCitedSourcesAndStableRoles(t *testing.T) {
	sources := []SourceCitation{
		{MaterialID: "later", Filename: ptr("b.pdf"), Kind: ptr("lab"), Week: ptr(int32(2))},
		{MaterialID: "theory", Filename: ptr("lecture.pdf"), Kind: ptr("lecture"), Week: ptr(int32(1))},
		{MaterialID: "custom", Filename: ptr("custom.pdf"), Kind: ptr("seminar"), Week: ptr(int32(1))},
		{MaterialID: "later", Filename: ptr("b.pdf"), Kind: ptr("lab"), Week: ptr(int32(2))},
		{MaterialID: "unknown", Filename: ptr("unknown.pdf"), Kind: ptr("tutorial"), Week: nil},
	}
	got := SelectLearningMaterials(sources)
	if len(got.Theory) != 1 || got.Theory[0].MaterialID != "theory" {
		t.Fatalf("theory = %#v", got.Theory)
	}
	if len(got.Practice) != 2 || got.Practice[0].MaterialID != "later" || got.Practice[1].MaterialID != "unknown" {
		t.Fatalf("practice = %#v", got.Practice)
	}
	if len(got.Additional) != 1 || got.Additional[0].MaterialID != "custom" {
		t.Fatalf("additional = %#v", got.Additional)
	}
}

func TestNewGapKeepsUniqueKnownWeeksSorted(t *testing.T) {
	got := NewGap("n", "Тема", []SourceCitation{{Week: ptr(int32(3))}, {Week: ptr(int32(1))}, {Week: ptr(int32(3))}, {Week: nil}})
	if len(got.Weeks) != 2 || got.Weeks[0] != 1 || got.Weeks[1] != 3 {
		t.Fatalf("weeks = %#v", got.Weeks)
	}
}
