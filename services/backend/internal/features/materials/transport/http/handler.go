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
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

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
