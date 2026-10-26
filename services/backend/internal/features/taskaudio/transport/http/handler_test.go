package taskaudiohttp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskaudio/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type fileOpenerStub struct {
	body      io.ReadCloser
	info      application.ObjectInfo
	err       error
	owner, id string
}

func (s *fileOpenerStub) Open(_ context.Context, owner, id string) (io.ReadCloser, application.ObjectInfo, error) {
	s.owner, s.id = owner, id
	return s.body, s.info, s.err
}

func TestFileRequiresAuthenticationAndStreamsPrivateWAV(t *testing.T) {
	service := &fileOpenerStub{body: io.NopCloser(strings.NewReader("saved wav")), info: application.ObjectInfo{Size: 9, ContentType: "audio/wav", AssetID: "audio-1"}}
	handler := New(service)
	unauthorized := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/task-audio/audio-1/file", nil)
	request.SetPathValue("id", "audio-1")
	handler.File(unauthorized, request)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d", unauthorized.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/task-audio/audio-1/file", nil)
	request.SetPathValue("id", "audio-1")
	request = httpx.WithPrincipal(request, user.User{ID: "owner-1"})
	response := httptest.NewRecorder()
	handler.File(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "audio/wav" || response.Body.String() != "saved wav" {
		t.Fatalf("file response=%d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "private, no-store" || service.owner != "owner-1" || service.id != "audio-1" {
		t.Fatalf("cache/auth: headers=%v owner=%q id=%q", response.Header(), service.owner, service.id)
	}
}

func TestFileReturnsServiceErrorsBeforeWAVHeaders(t *testing.T) {
	service := &fileOpenerStub{err: fault.New(fault.Conflict, "AUDIO_NOT_READY", "pending")}
	handler := New(service)
	request := httptest.NewRequest(http.MethodGet, "/task-audio/audio-1/file", nil)
	request.SetPathValue("id", "audio-1")
	request = httpx.WithPrincipal(request, user.User{ID: "owner-1"})
	response := httptest.NewRecorder()
	handler.File(response, request)
	if response.Code != http.StatusConflict || response.Header().Get("Content-Type") == "audio/wav" {
		t.Fatalf("pending response=%d headers=%v", response.Code, response.Header())
	}
	service.err = errors.New("storage unavailable")
	response = httptest.NewRecorder()
	handler.File(response, request)
	if response.Code != http.StatusServiceUnavailable || response.Header().Get("Content-Type") == "audio/wav" {
		t.Fatalf("internal failure response=%d headers=%v", response.Code, response.Header())
	}
}
