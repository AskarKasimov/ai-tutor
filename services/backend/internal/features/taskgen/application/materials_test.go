package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type materialRepository struct {
	subjectID, name  string
	outcomes, chunks []string
	createdAt        int64
}

func (r *materialRepository) ImportMaterial(_ context.Context, subjectID, name string, outcomes, chunks []string, createdAt int64) error {
	r.subjectID, r.name, r.outcomes, r.chunks, r.createdAt = subjectID, name, outcomes, chunks, createdAt
	return nil
}

func TestMaterialImportRequiresAdminBeforeValidationAndPersistence(t *testing.T) {
	for _, role := range []user.Role{user.Student, ""} {
		repository := &materialRepository{}
		service := NewMaterialService(repository, func() int64 { return 42 })
		err := service.Import(context.Background(), user.User{Role: role}, MaterialInput{})
		var failure *fault.Error
		if !errors.As(err, &failure) || failure.Kind != fault.Forbidden || repository.name != "" {
			t.Fatalf("unauthorized import: error=%v repository=%#v", err, repository)
		}
	}
}

func TestMaterialImportPersistsValidAdminInput(t *testing.T) {
	repository := &materialRepository{}
	service := NewMaterialService(repository, func() int64 { return 42 })
	err := service.Import(context.Background(), user.User{Role: user.Admin}, MaterialInput{
		SubjectID: "subject:test", Name: " Notes ", Content: "if x:\n    print(x)\n", OutcomeIDs: []string{" o1 ", "o1"},
	})
	if err != nil || repository.name != "Notes" || len(repository.outcomes) != 1 || repository.outcomes[0] != "o1" || strings.Join(repository.chunks, "") != "if x:\n    print(x)\n" || repository.createdAt != 42 {
		t.Fatalf("admin import: error=%v repository=%#v", err, repository)
	}
}

func TestSplitChunksPreservesMaterialAcrossBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, text string
	}{
		{"code", strings.Repeat("if x > 0:\r\n    print('значение 😀')\n\n", 100)},
		{"long token", strings.Repeat("я😀", 2000)},
		{"leading whitespace", strings.Repeat(" ", 2500) + "Вопрос"},
		{"internal whitespace", "Первая строка" + strings.Repeat(" ", 2500) + "Вторая строка"},
		{"trailing whitespace", "Ответ" + strings.Repeat(" ", 2500)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chunks := splitChunks(tc.text, 1200)
			if got := strings.Join(chunks, ""); got != tc.text {
				t.Fatalf("chunking changed material: got %d bytes, want %d", len(got), len(tc.text))
			}
			for i, chunk := range chunks {
				if !utf8.ValidString(chunk) || strings.TrimSpace(chunk) == "" {
					t.Fatalf("chunk %d is invalid or has no content", i)
				}
				if (tc.name == "code" || tc.name == "long token") && utf8.RuneCountInString(chunk) > 1200 {
					t.Fatalf("chunk %d exceeds the target size", i)
				}
			}
		})
	}
}
