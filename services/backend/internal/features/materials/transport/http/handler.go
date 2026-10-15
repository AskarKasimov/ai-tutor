package http

import (
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/material"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type DeleteResponse struct {
	Status string `json:"status"`
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
// @Success 200 {array} material.Material
// @Failure 400 {object} fault.Error
// @Failure 401 {object} fault.Error
// @Failure 403 {object} fault.Error
// @Failure 500 {object} fault.Error
// @Router /admin/subjects/{subject_id}/materials [post]
func (h *Handler) UploadMaterials(w http.ResponseWriter, r *http.Request) {
	subjectID := r.PathValue("subject_id")
	if subjectID == "" {
		httpx.Error(r.Context(), w, fault.New(fault.Invalid, "BAD_REQUEST", "Subject ID required"))
		return
	}

	if err := r.ParseMultipartForm(h.maxUploadBytes); err != nil {
		log.Printf("UploadMaterials: ParseMultipartForm error: %v", err)
		httpx.Error(r.Context(), w, fault.New(fault.Invalid, "BAD_REQUEST", "Failed to parse multipart form"))
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	var allFiles []*multipart.FileHeader
	if r.MultipartForm != nil {
		for _, fhs := range r.MultipartForm.File {
			allFiles = append(allFiles, fhs...)
		}
	}

	if len(allFiles) == 0 {
		httpx.Error(r.Context(), w, fault.New(fault.Invalid, "BAD_REQUEST", "No files provided"))
		return
	}

	var results []material.Material
	for _, fh := range allFiles {
		file, err := fh.Open()
		if err != nil {
			log.Printf("UploadMaterials: failed to open file %q: %v", fh.Filename, err)
			continue
		}

		data, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			log.Printf("UploadMaterials: failed to read file %q: %v", fh.Filename, err)
			continue
		}

		contentType := fh.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		mat, err := h.service.Upload(r.Context(), subjectID, application.UploadInput{
			Filename:    fh.Filename,
			ContentType: contentType,
			Data:        data,
		})
		if err != nil {
			log.Printf("UploadMaterials: failed to upload material %q: %v", fh.Filename, err)
			continue
		}
		results = append(results, mat)
	}

	httpx.JSON(w, http.StatusOK, results)
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
	subjectID := r.PathValue("subject_id")
	if subjectID == "" {
		httpx.Error(r.Context(), w, fault.New(fault.Invalid, "BAD_REQUEST", "Subject ID required"))
		return
	}

	items, err := h.service.List(r.Context(), subjectID)
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
	subjectID := r.PathValue("subject_id")
	materialID := r.PathValue("material_id")
	if subjectID == "" || materialID == "" {
		httpx.Error(r.Context(), w, fault.New(fault.Invalid, "BAD_REQUEST", "Subject ID and Material ID required"))
		return
	}

	if err := h.service.Delete(r.Context(), subjectID, materialID); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, DeleteResponse{Status: "deleted"})
}

// DownloadRaw handles GET /admin/subjects/{subject_id}/materials/{material_id}/raw.
// @Summary Скачать исходный файл материала
// @Description Скачивание или просмотр исходного файла материала.
// @ID downloadSubjectMaterialRaw
// @Tags Материалы
// @Security accessCookie
// @Produce application/octet-stream
// @Param subject_id path string true "ID предмета"
// @Param material_id path string true "ID материала"
// @Success 200 {file} file "Исходный файл"
// @Failure 400 {object} fault.Error
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 500 {object} fault.Error
// @Router /admin/subjects/{subject_id}/materials/{material_id}/raw [get]
func (h *Handler) DownloadRaw(w http.ResponseWriter, r *http.Request) {
	subjectID := r.PathValue("subject_id")
	materialID := r.PathValue("material_id")
	if subjectID == "" || materialID == "" {
		httpx.Error(r.Context(), w, fault.New(fault.Invalid, "BAD_REQUEST", "Subject ID and Material ID required"))
		return
	}

	rc, contentType, size, filename, err := h.service.OpenRaw(r.Context(), subjectID, materialID)
	if err != nil {
		httpx.Error(r.Context(), w, fault.New(fault.NotFound, "NOT_FOUND", "Material not found"))
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", contentType)
	if size > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filename))
	_, _ = io.Copy(w, rc)
}
