package http

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/material"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type DeleteResponse struct {
	Status string `json:"status"`
}

type UploadFailedItem struct {
	Filename string `json:"filename"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

type UploadResult struct {
	Uploaded []material.Material `json:"uploaded"`
	Failed   []UploadFailedItem  `json:"failed"`
}

type Handler struct {
	service        *application.Service
	maxUploadBytes int64
}

func NewHandler(service *application.Service, maxUploadBytes int64) *Handler {
	return &Handler{
		service:        service,
		maxUploadBytes: maxUploadBytes,
	}
}

// UploadMaterials handles POST /admin/subjects/{subject_id}/materials.
// @Summary Загрузить материалы курса
// @Description Доступно только admin. Загружает файлы лекций/лаб, сохраняет их в S3, выполняет OCR и сохраняет извлеченный Markdown с формулами LaTeX.
// @ID uploadSubjectMaterials
// @Tags Материалы
// @Security accessCookie
// @Accept multipart/form-data
// @Produce json
// @Param subject_id path string true "ID предмета"
// @Param file formData file true "Файл лекции или лабы (PDF, Markdown, текст)"
// @Success 200 {object} UploadResult
// @Failure 400 {object} fault.Error
// @Failure 401 {object} fault.Error
// @Failure 403 {object} fault.Error
// @Failure 413 {object} fault.Error
// @Failure 500 {object} fault.Error
// @Router /admin/subjects/{subject_id}/materials [post]
func (h *Handler) UploadMaterials(w http.ResponseWriter, r *http.Request) {
	actor, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	if err := h.service.Authorize(actor); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}

	subjectID := r.PathValue("subject_id")
	if subjectID == "" {
		httpx.Error(r.Context(), w, fault.New(fault.Invalid, "BAD_REQUEST", "ID предмета обязателен."))
		return
	}

	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(15 * time.Minute))
	_ = rc.SetWriteDeadline(time.Now().Add(15 * time.Minute))

	mr, err := r.MultipartReader()
	if err != nil {
		httpx.Error(r.Context(), w, fault.New(fault.Invalid, "BAD_REQUEST", "Ожидается multipart/form-data запрос."))
		return
	}

	uploaded := make([]material.Material, 0)
	failed := make([]UploadFailedItem, 0)

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("UploadMaterials: reading next part: %v", err)
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				failed = append(failed, UploadFailedItem{
					Filename: "request",
					Code:     "PAYLOAD_TOO_LARGE",
					Message:  "Превышен общий лимит размера запроса.",
				})
			} else {
				failed = append(failed, UploadFailedItem{
					Filename: "part",
					Code:     "READ_ERROR",
					Message:  "Ошибка чтения части multipart-запроса.",
				})
			}
			break
		}
		if part.FormName() != "file" && part.FormName() != "files" {
			_ = part.Close()
			continue
		}
		filename := part.FileName()
		if filename == "" {
			_ = part.Close()
			continue
		}

		lr := io.LimitReader(part, h.maxUploadBytes+1)
		data, err := io.ReadAll(lr)
		_ = part.Close()
		if err != nil {
			log.Printf("UploadMaterials: failed to read file %q: %v", filename, err)
			failed = append(failed, UploadFailedItem{
				Filename: filename,
				Code:     "READ_ERROR",
				Message:  "Не удалось прочитать содержимое файла.",
			})
			continue
		}
		if int64(len(data)) > h.maxUploadBytes {
			failed = append(failed, UploadFailedItem{
				Filename: filename,
				Code:     "PAYLOAD_TOO_LARGE",
				Message:  fmt.Sprintf("Файл %q превышает допустимый размер.", filename),
			})
			continue
		}
		if len(data) == 0 {
			failed = append(failed, UploadFailedItem{
				Filename: filename,
				Code:     "EMPTY_FILE",
				Message:  fmt.Sprintf("Файл %q пуст.", filename),
			})
			continue
		}

		contentType := part.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		mat, err := h.service.Upload(r.Context(), actor, subjectID, application.UploadInput{
			Filename:    filename,
			ContentType: contentType,
			Data:        data,
		})
		if err != nil {
			log.Printf("UploadMaterials: failed to upload material %q: %v", filename, err)
			code := "UPLOAD_ERROR"
			msg := err.Error()
			var fe *fault.Error
			if errors.As(err, &fe) {
				code = fe.Code
				msg = fe.Message
			}
			failed = append(failed, UploadFailedItem{
				Filename: filename,
				Code:     code,
				Message:  msg,
			})
			continue
		}
		uploaded = append(uploaded, mat)
	}

	if len(uploaded) == 0 {
		if len(failed) > 0 {
			first := failed[0]
			kind := fault.Invalid
			if first.Code == "PAYLOAD_TOO_LARGE" {
				kind = fault.TooLarge
			} else if first.Code == "S3_UPLOAD_ERROR" || first.Code == "DATABASE_ERROR" {
				kind = fault.Unavailable
			}
			httpx.Error(r.Context(), w, fault.New(kind, first.Code, first.Message))
			return
		}
		httpx.Error(r.Context(), w, fault.New(fault.Invalid, "BAD_REQUEST", "Не предоставлены файлы для загрузки."))
		return
	}

	httpx.JSON(w, http.StatusOK, UploadResult{
		Uploaded: uploaded,
		Failed:   failed,
	})
}

// ListMaterials handles GET /admin/subjects/{subject_id}/materials.
// @Summary Список материалов курса
// @Description Доступно только admin. Возвращает список загруженных материалов курса с извлеченным текстом.
// @ID listSubjectMaterials
// @Tags Материалы
// @Security accessCookie
// @Produce json
// @Param subject_id path string true "ID предмета"
// @Success 200 {array} material.Material
// @Failure 400 {object} fault.Error
// @Failure 401 {object} fault.Error
// @Failure 403 {object} fault.Error
// @Failure 500 {object} fault.Error
// @Router /admin/subjects/{subject_id}/materials [get]
func (h *Handler) ListMaterials(w http.ResponseWriter, r *http.Request) {
	actor, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	if err := h.service.Authorize(actor); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}

	subjectID := r.PathValue("subject_id")
	if subjectID == "" {
		httpx.Error(r.Context(), w, fault.New(fault.Invalid, "BAD_REQUEST", "Subject ID required"))
		return
	}

	items, err := h.service.List(r.Context(), actor, subjectID)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	if items == nil {
		items = []material.Material{}
	}
	httpx.JSON(w, http.StatusOK, items)
}

// DeleteMaterial handles DELETE /admin/subjects/{subject_id}/materials/{material_id}.
// @Summary Удалить материал курса
// @Description Доступно только admin. Удаляет материал из базы и S3.
// @ID deleteSubjectMaterial
// @Tags Материалы
// @Security accessCookie
// @Produce json
// @Param subject_id path string true "ID предмета"
// @Param material_id path string true "ID материала"
// @Success 200 {object} DeleteResponse
// @Failure 400 {object} fault.Error
// @Failure 401 {object} fault.Error
// @Failure 403 {object} fault.Error
// @Failure 500 {object} fault.Error
// @Router /admin/subjects/{subject_id}/materials/{material_id} [delete]
func (h *Handler) DeleteMaterial(w http.ResponseWriter, r *http.Request) {
	actor, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	if err := h.service.Authorize(actor); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}

	subjectID := r.PathValue("subject_id")
	materialID := r.PathValue("material_id")
	if subjectID == "" || materialID == "" {
		httpx.Error(r.Context(), w, fault.New(fault.Invalid, "BAD_REQUEST", "Subject ID and Material ID required"))
		return
	}

	if err := h.service.Delete(r.Context(), actor, subjectID, materialID); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, DeleteResponse{Status: "deleted"})
}

// DownloadRaw handles GET /admin/subjects/{subject_id}/materials/{material_id}/raw.
// @Summary Скачать исходный файл материала
// @Description Доступно только admin. Скачивание исходного файла материала.
// @ID downloadSubjectMaterialRaw
// @Tags Материалы
// @Security accessCookie
// @Produce application/octet-stream
// @Param subject_id path string true "ID предмета"
// @Param material_id path string true "ID материала"
// @Success 200 {file} file "Исходный файл"
// @Failure 400 {object} fault.Error
// @Failure 401 {object} fault.Error
// @Failure 403 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 500 {object} fault.Error
// @Router /admin/subjects/{subject_id}/materials/{material_id}/raw [get]
func (h *Handler) DownloadRaw(w http.ResponseWriter, r *http.Request) {
	actor, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	if err := h.service.Authorize(actor); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}

	subjectID := r.PathValue("subject_id")
	materialID := r.PathValue("material_id")
	if subjectID == "" || materialID == "" {
		httpx.Error(r.Context(), w, fault.New(fault.Invalid, "BAD_REQUEST", "Subject ID and Material ID required"))
		return
	}

	rc, contentType, size, filename, err := h.service.OpenRaw(r.Context(), actor, subjectID, materialID)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	defer rc.Close()

	safeContentType := sanitizeContentType(contentType)
	w.Header().Set("Content-Type", safeContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if size > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`,
		sanitizeASCIIFilename(filename), url.PathEscape(filename)))
	_, _ = io.Copy(w, rc)
}

func sanitizeContentType(contentType string) string {
	media := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch media {
	case "application/pdf":
		return "application/pdf"
	case "text/markdown":
		return "text/markdown; charset=utf-8"
	case "text/plain":
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

func sanitizeASCIIFilename(filename string) string {
	clean := filepath.Base(filename)
	var sb strings.Builder
	for _, r := range clean {
		if r > 32 && r < 127 && r != '"' && r != '\\' && r != '/' && r != '\'' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	if sb.Len() == 0 {
		return "download"
	}
	return sb.String()
}
