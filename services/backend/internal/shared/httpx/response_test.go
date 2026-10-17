package httpx

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

func TestGoneFaultUsesHTTP410(t *testing.T) {
	w := httptest.NewRecorder()
	Error(context.Background(), w, fault.New(fault.Gone, "SOURCE_UNAVAILABLE", "Исходник недоступен."))
	if w.Code != 410 {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}
