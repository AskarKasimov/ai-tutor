package traininghttp

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/training/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type ChatHandler struct {
	service  *application.ChatService
	maxBytes int64
}

func NewChatHandler(service *application.ChatService, maxBytes int64) *ChatHandler {
	return &ChatHandler{service: service, maxBytes: maxBytes}
}

type ChatTextRequest struct {
	Revision int64  `json:"revision"`
	Kind     string `json:"kind"`
	Text     string `json:"text"`
}

type EmptyChatRequest struct{}

func parseChatKind(value string) (training.ChatKind, error) {
	kind := training.ChatKind(value)
	if kind != training.ChatKindMessage && kind != training.ChatKindCheck {
		return "", fault.Validation("kind", "Укажите тип реплики message или check.")
	}
	return kind, nil
}

func parseChatInput(w http.ResponseWriter, r *http.Request, maxBytes int64, key, exerciseID string) (training.TurnInput, error) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return training.TurnInput{}, fault.Validation("Content-Type", "Укажите JSON или multipart/form-data.")
	}
	switch media {
	case "application/json":
		r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
		var request ChatTextRequest
		if err = httpx.DecodeJSON(r, &request); err != nil {
			return training.TurnInput{}, err
		}
		kind, err := parseChatKind(request.Kind)
		if err != nil {
			return training.TurnInput{}, err
		}
		return training.TurnInput{ExerciseID: exerciseID, Key: key, Revision: request.Revision, Kind: kind, Source: training.AnswerSourceText, Text: request.Text}, nil
	case "multipart/form-data":
		return parseChatAudioInput(w, r, maxBytes, key, exerciseID)
	default:
		return training.TurnInput{}, fault.New(fault.Unsupported, "UNSUPPORTED_MEDIA_TYPE", "Ожидается JSON или multipart/form-data.")
	}
}

func parseChatAudioInput(w http.ResponseWriter, r *http.Request, maxBytes int64, key, exerciseID string) (training.TurnInput, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+32*1024)
	reader, err := r.MultipartReader()
	if err != nil {
		return training.TurnInput{}, fault.Validation("body", "Некорректный multipart-запрос.")
	}
	fields := map[string]string{}
	var audioData []byte
	var audioMedia string
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return training.TurnInput{}, httpx.BodyError(nextErr)
		}
		name := part.FormName()
		if name == "audio" {
			if part.FileName() == "" || audioData != nil {
				part.Close()
				return training.TurnInput{}, fault.Validation("audio", "Передайте один файл audio.")
			}
			parsedMedia, params, mediaErr := mime.ParseMediaType(part.Header.Get("Content-Type"))
			if mediaErr != nil {
				parsedMedia = ""
			}
			audioData, err = io.ReadAll(io.LimitReader(part, maxBytes+1))
			part.Close()
			if err != nil {
				return training.TurnInput{}, httpx.BodyError(err)
			}
			if int64(len(audioData)) > maxBytes {
				return training.TurnInput{}, fault.New(fault.TooLarge, "AUDIO_TOO_LARGE", "Голосовая реплика превышает допустимый размер.")
			}
			audioMedia = mime.FormatMediaType(strings.ToLower(parsedMedia), params)
			continue
		}
		if name != "revision" && name != "kind" || part.FileName() != "" {
			part.Close()
			return training.TurnInput{}, fault.Validation(name, "Неизвестное или файловое поле запрещено.")
		}
		if _, exists := fields[name]; exists {
			part.Close()
			return training.TurnInput{}, fault.Validation(name, "Поле нельзя передавать повторно.")
		}
		value, readErr := io.ReadAll(io.LimitReader(part, 33*1024))
		part.Close()
		if readErr != nil {
			return training.TurnInput{}, httpx.BodyError(readErr)
		}
		if len(value) > 32*1024 {
			return training.TurnInput{}, fault.New(fault.TooLarge, "UPLOAD_TOO_LARGE", "Поля запроса слишком велики.")
		}
		fields[name] = string(value)
	}
	if audioData == nil {
		return training.TurnInput{}, fault.Validation("audio", "Файл audio обязателен.")
	}
	revisionText, ok := fields["revision"]
	if !ok {
		return training.TurnInput{}, fault.Validation("revision", "Укажите ревизию диалога.")
	}
	revision, err := strconv.ParseInt(revisionText, 10, 64)
	if err != nil || revision < 0 {
		return training.TurnInput{}, fault.Validation("revision", "Ревизия должна быть неотрицательным целым числом.")
	}
	kind, err := parseChatKind(fields["kind"])
	if err != nil {
		return training.TurnInput{}, err
	}
	return training.TurnInput{ExerciseID: exerciseID, Key: key, Revision: revision, Kind: kind, Source: training.AnswerSourceVoice, Audio: audioData, MediaType: audioMedia}, nil
}

// Read loads a page of the persistent tutor conversation.
// @Summary Прочитать диалог репетитора
// @Tags Тренировка
// @Security accessCookie
// @Produce json
// @Param id path string true "ID тренировки"
// @Param exercise_id path string true "ID упражнения"
// @Param cursor query string false "Sequence старого сообщения"
// @Param limit query int false "Размер страницы, 1–100"
// @Success 200 {object} training.ChatPage
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Router /learn/training-sessions/{id}/exercises/{exercise_id}/chat [get]
func (h *ChatHandler) Read(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := owner(w, r)
	if !ok {
		return
	}
	query, err := httpx.ParseQuery(r, "limit", "cursor")
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	limit := 40
	if raw := query.Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			httpx.Error(r.Context(), w, fault.Validation("limit", "Размер страницы должен быть от 1 до 100."))
			return
		}
	}
	var before int64
	if raw := query.Get("cursor"); raw != "" {
		before, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || before < 1 {
			httpx.Error(r.Context(), w, fault.Validation("cursor", "Курсор должен быть положительным sequence."))
			return
		}
	}
	page, err := h.service.Read(r.Context(), ownerID, r.PathValue("id"), r.PathValue("exercise_id"), before, limit)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}

// Send creates an ordinary tutor turn or an explicit final-answer check.
// @Summary Отправить реплику репетитору
// @Tags Тренировка
// @Security accessCookie
// @Accept json
// @Accept mpfd
// @Produce json
// @Param id path string true "ID тренировки"
// @Param exercise_id path string true "ID упражнения"
// @Param Idempotency-Key header string true "Ключ операции"
// @Param body body ChatTextRequest true "Текстовая реплика; для аудио используйте поля multipart"
// @Param revision formData int true "Ревизия диалога"
// @Param kind formData string true "message или check"
// @Param audio formData file true "WAV, Ogg/Opus или WebM/Opus"
// @Success 200 {object} training.ChatTurnResult
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Failure 413 {object} fault.Error
// @Failure 415 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 502 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Failure 504 {object} fault.Error
// @Router /learn/training-sessions/{id}/exercises/{exercise_id}/chat/turns [post]
func (h *ChatHandler) Send(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := owner(w, r)
	if !ok {
		return
	}
	input, err := parseChatInput(w, r, h.maxBytes, r.Header.Get("Idempotency-Key"), r.PathValue("exercise_id"))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	result, err := h.service.Send(r.Context(), ownerID, r.PathValue("id"), input)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, result.Response)
}

// Retry resumes a persisted failed or expired turn using its stored input.
// @Summary Повторить реплику репетитору
// @Tags Тренировка
// @Security accessCookie
// @Accept json
// @Produce json
// @Param id path string true "ID тренировки"
// @Param exercise_id path string true "ID упражнения"
// @Param turn_id path string true "ID реплики"
// @Param body body EmptyChatRequest true "Пустой объект"
// @Success 200 {object} training.ChatTurnResult
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /learn/training-sessions/{id}/exercises/{exercise_id}/chat/turns/{turn_id}/retry [post]
func (h *ChatHandler) Retry(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := owner(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	var empty struct{}
	if err := httpx.DecodeJSON(r, &empty); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	result, err := h.service.Retry(r.Context(), ownerID, r.PathValue("id"), r.PathValue("exercise_id"), r.PathValue("turn_id"))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, result.Response)
}

// Cancel removes an unresolved pending turn and fences any late provider result.
// @Summary Отменить ожидающую реплику
// @Tags Тренировка
// @Security accessCookie
// @Accept json
// @Param id path string true "ID тренировки"
// @Param exercise_id path string true "ID упражнения"
// @Param turn_id path string true "ID реплики"
// @Param body body EmptyChatRequest true "Пустой объект"
// @Success 204
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Router /learn/training-sessions/{id}/exercises/{exercise_id}/chat/turns/{turn_id}/cancel [post]
func (h *ChatHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := owner(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	var empty struct{}
	if err := httpx.DecodeJSON(r, &empty); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	if err := h.service.Cancel(r.Context(), ownerID, r.PathValue("id"), r.PathValue("exercise_id"), r.PathValue("turn_id")); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
