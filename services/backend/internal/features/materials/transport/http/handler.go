package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
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

type UploadItemResult struct {
	ClientID string             `json:"client_id"`
	Material *material.Material `json:"material,omitempty" binding:"optional"`
	Error    *UploadFailedItem  `json:"error,omitempty" binding:"optional"`
}
type UploadResult struct {
	Uploaded []material.Material `json:"uploaded"`
	Failed   []UploadFailedItem  `json:"failed"`
	Results  []UploadItemResult  `json:"results"`
}
type uploadManifestItem struct {
	ClientID string        `json:"client_id"`
	Week     int32         `json:"week"`
	Kind     material.Kind `json:"kind"`
}

type UpdateMetadataRequest struct {
	Week *int32         `json:"week" binding:"optional"`
	Kind *material.Kind `json:"kind" binding:"optional"`
}

const (
	MaxUploadFileBytes = 50 * 1024 * 1024
	MaxFilesPerRequest = 48
)

type Handler struct {
	service        *application.Service
	maxUploadBytes int64
}

// ListKinds returns the configured material kinds for the admin editor.
// @Summary Получить виды материалов
// @ID listMaterialKinds
// @Tags Материалы
// @Security accessCookie
// @Produce json
// @Success 200 {array} material.KindOption
// @Failure 401 {object} fault.Error
// @Failure 403 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /admin/material-kinds [get]
func (h *Handler) ListKinds(w http.ResponseWriter, r *http.Request) {
	actor, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	kinds, err := h.service.MaterialKinds(r.Context(), actor)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, kinds)
}

func NewHandler(service *application.Service, maxUploadBytes int64) *Handler {
	return &Handler{
		service:        service,
		maxUploadBytes: maxUploadBytes,
	}
}

// UploadMaterials handles POST /admin/subjects/{subject_id}/materials.
// @Summary Загрузить материалы курса
// @Description Доступно только admin. Для новых клиентов передайте items (JSON-манифест с client_id, week и kind для каждого файла), затем повторяющиеся file в том же порядке. Legacy week/kind остаются совместимы.
// @ID uploadSubjectMaterials
// @Tags Материалы
// @Security accessCookie
// @Accept multipart/form-data
// @Produce json
// @Param subject_id path string true "ID предмета"
// @Param items formData string false "JSON-манифест файлов"
// @Param week formData int false "Неделя для legacy-клиентов"
// @Param kind formData string false "Тип для legacy-клиентов"
// @Param file formData []file true "PDF-файлы в порядке манифеста"
// @Success 200 {object} UploadResult
// @Failure 400 {object} fault.Error
// @Failure 401 {object} fault.Error
// @Failure 403 {object} fault.Error
// @Failure 413 {object} fault.Error
// @Failure 422 {object} fault.Error
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
	_ = rc.SetWriteDeadline(time.Time{})
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.Error(r.Context(), w, fault.New(fault.TooLarge, "PAYLOAD_TOO_LARGE", "Превышен общий лимит размера запроса."))
		} else {
			httpx.Error(r.Context(), w, fault.New(fault.Invalid, "BAD_REQUEST", "Ожидается корректный multipart/form-data запрос."))
		}
		return
	}
	if r.MultipartForm != nil {
		defer func() { _ = r.MultipartForm.RemoveAll() }()
	}
	form := r.MultipartForm
	if form == nil {
		httpx.Error(r.Context(), w, fault.New(fault.Invalid, "BAD_REQUEST", "Не предоставлены файлы для загрузки."))
		return
	}
	manifestValues := form.Value["items"]
	hasManifest := len(manifestValues) > 0
	if len(form.File) > 1 || (len(form.File) == 1 && len(form.File["file"]) == 0 && len(form.File["files"]) == 0) {
		httpx.Error(r.Context(), w, fault.Validation("file", "Используйте только поле file."))
		return
	}
	files := form.File["file"]

	if len(form.File["files"]) > 0 {
		if len(files) > 0 {
			httpx.Error(r.Context(), w, fault.Validation("file", "Используйте одно имя поля file."))
			return
		}
		files = form.File["files"]

	}
	var manifest []uploadManifestItem
	var week int32
	var kind material.Kind
	if hasManifest {
		if len(manifestValues) != 1 || len(form.Value["week"]) > 0 || len(form.Value["kind"]) > 0 {
			httpx.Error(r.Context(), w, fault.Validation("items", "Манифест должен быть единственным полем метаданных."))
			return
		}
		if err := json.Unmarshal([]byte(manifestValues[0]), &manifest); err != nil || len(manifest) == 0 {
			httpx.Error(r.Context(), w, fault.Validation("items", "Некорректный манифест файлов."))
			return
		}
		ids := map[string]bool{}
		for _, item := range manifest {
			if strings.TrimSpace(item.ClientID) == "" || ids[item.ClientID] {
				httpx.Error(r.Context(), w, fault.Validation("items", "client_id должен быть уникальным и непустым."))
				return
			}
			ids[item.ClientID] = true
			if err := h.service.ValidateMetadata(r.Context(), item.Week, item.Kind); err != nil {
				httpx.Error(r.Context(), w, err)
				return
			}
		}
		if len(files) != len(manifest) {
			httpx.Error(r.Context(), w, fault.Validation("file", "Число файлов не совпадает с манифестом."))
			return
		}
	} else if len(form.Value["week"]) > 0 || len(form.Value["kind"]) > 0 {
		if len(form.Value["week"]) != 1 || len(form.Value["kind"]) != 1 {
			httpx.Error(r.Context(), w, fault.Validation("week", "Укажите неделю и тип перед файлами."))
			return
		}
		parsed, err := strconv.ParseInt(strings.TrimSpace(form.Value["week"][0]), 10, 32)
		if err != nil {
			httpx.Error(r.Context(), w, fault.Validation("week", "Укажите положительный номер недели."))
			return
		}
		week = int32(parsed)
		kind = material.Kind(strings.TrimSpace(form.Value["kind"][0]))
		if err := h.service.ValidateMetadata(r.Context(), week, kind); err != nil {
			httpx.Error(r.Context(), w, err)
			return
		}
	}
	if len(files) == 0 {
		httpx.Error(r.Context(), w, fault.New(fault.Invalid, "BAD_REQUEST", "Не предоставлены файлы для загрузки."))
		return
	}
	uploaded := make([]material.Material, 0, len(files))
	failed := make([]UploadFailedItem, 0)
	results := make([]UploadItemResult, 0, len(files))
	_ = rc.SetReadDeadline(time.Now().Add(15 * time.Minute))
	for i, header := range files {
		_ = rc.SetReadDeadline(time.Now().Add(15 * time.Minute))
		clientID := fmt.Sprintf("legacy-%d", i)
		fileWeek, fileKind := week, kind
		if hasManifest {
			clientID = manifest[i].ClientID
			fileWeek = manifest[i].Week
			fileKind = manifest[i].Kind
		}
		filename := filepath.Base(header.Filename)
		if i >= MaxFilesPerRequest {
			failure := UploadFailedItem{Filename: filename, Code: "TOO_MANY_FILES", Message: fmt.Sprintf("За один запрос можно загрузить не больше %d файлов.", MaxFilesPerRequest)}
			failed = append(failed, failure)
			results = append(results, UploadItemResult{ClientID: clientID, Error: &failure})
			continue
		}
		file, err := header.Open()
		var data []byte
		if err == nil {
			data, err = io.ReadAll(io.LimitReader(file, h.maxUploadBytes+1))
			_ = file.Close()
		}
		var mat material.Material
		if err == nil && int64(len(data)) <= h.maxUploadBytes && len(data) > 0 {
			mat, err = h.service.Upload(r.Context(), actor, subjectID, application.UploadInput{Filename: filename, ContentType: header.Header.Get("Content-Type"), Data: data, Week: fileWeek, Kind: fileKind})
		}
		if err != nil || len(data) == 0 || int64(len(data)) > h.maxUploadBytes {
			itemErr := UploadFailedItem{Filename: filename, Code: "UPLOAD_ERROR", Message: "Не удалось обработать файл."}
			switch {
			case len(data) == 0 && err == nil:
				itemErr.Code = "EMPTY_FILE"
				itemErr.Message = "Файл пуст."
			case int64(len(data)) > h.maxUploadBytes:
				itemErr.Code = "PAYLOAD_TOO_LARGE"
				itemErr.Message = fmt.Sprintf("Файл %q превышает допустимый размер.", filename)
			case err != nil:
				itemErr.Message = err.Error()
				var fe *fault.Error
				if errors.As(err, &fe) {
					itemErr.Code = fe.Code
					itemErr.Message = fe.Message
				}
			}
			failed = append(failed, itemErr)
			if hasManifest {
				results = append(results, UploadItemResult{ClientID: clientID, Error: &itemErr})
			}
			continue
		}
		uploaded = append(uploaded, mat)
		results = append(results, UploadItemResult{ClientID: clientID, Material: &mat})
	}
	if len(uploaded) == 0 && !hasManifest {
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
	_ = rc.SetWriteDeadline(time.Now().Add(30 * time.Second))
	httpx.JSON(w, http.StatusOK, UploadResult{Uploaded: uploaded, Failed: failed, Results: results})
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

// UpdateMaterialMetadata updates course placement without reprocessing the source file.
// @Summary Исправить метаданные материала
// @ID updateSubjectMaterialMetadata
// @Tags Материалы
// @Security accessCookie
// @Accept json
// @Produce json
// @Param subject_id path string true "ID предмета"
// @Param material_id path string true "ID материала"
// @Param request body UpdateMetadataRequest true "Метаданные материала"
// @Success 200 {object} material.Material
// @Failure 400 {object} fault.Error
// @Failure 401 {object} fault.Error
// @Failure 403 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /admin/subjects/{subject_id}/materials/{material_id} [patch]
func (h *Handler) UpdateMaterialMetadata(w http.ResponseWriter, r *http.Request) {
	actor, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	var request UpdateMetadataRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	updated, err := h.service.UpdateMetadata(r.Context(), actor, r.PathValue("subject_id"), r.PathValue("material_id"), request.Week, request.Kind)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, updated)
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

// ReplaceMaterial handles PUT /admin/subjects/{subject_id}/materials/{material_id}/raw.
// @Summary Заменить исходный PDF материала
// @Description Сохраняет PDF и ставит новое durable OCR-задание; затем выполняются перевод, очистка и темы.
// @ID replaceSubjectMaterialRaw
// @Tags Материалы
// @Security accessCookie
// @Accept multipart/form-data
// @Produce json
// @Param subject_id path string true "ID предмета"
// @Param material_id path string true "ID материала"
// @Param file formData file true "Новый PDF"
// @Success 200 {object} material.Material
// @Failure 400 {object} fault.Error
// @Failure 401 {object} fault.Error
// @Failure 403 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 413 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /admin/subjects/{subject_id}/materials/{material_id}/raw [put]
func (h *Handler) ReplaceMaterial(w http.ResponseWriter, r *http.Request) {
	actor, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	if err := h.service.Authorize(actor); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	// ReadTimeout and WriteTimeout also cover multipart transfer time. Give a
	// replacement PDF the same upload window as a new material, then bound
	// writing the small response after the durable job has been enqueued.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(15 * time.Minute))
	_ = rc.SetWriteDeadline(time.Time{})
	if err := r.ParseMultipartForm(h.maxUploadBytes); err != nil {
		httpx.Error(r.Context(), w, fault.New(fault.Invalid, "BAD_REQUEST", "Ожидается один PDF-файл."))
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	if len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["file"]) != 1 {
		httpx.Error(r.Context(), w, fault.Validation("file", "Нужно выбрать ровно один PDF-файл."))
		return
	}
	files := r.MultipartForm.File["file"]
	if len(files) != 1 {
		httpx.Error(r.Context(), w, fault.Validation("file", "Нужно выбрать ровно один PDF-файл."))
		return
	}
	if files[0].Size > h.maxUploadBytes {
		httpx.Error(r.Context(), w, fault.New(fault.TooLarge, "PAYLOAD_TOO_LARGE", "PDF превышает допустимый размер."))
		return
	}
	f, err := files[0].Open()
	if err != nil {
		httpx.Error(r.Context(), w, fault.New(fault.Invalid, "BAD_REQUEST", "Не удалось прочитать PDF."))
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, h.maxUploadBytes+1))
	if err != nil || int64(len(data)) > h.maxUploadBytes {
		httpx.Error(r.Context(), w, fault.New(fault.TooLarge, "PAYLOAD_TOO_LARGE", "PDF превышает допустимый размер."))
		return
	}
	updated, err := h.service.Replace(r.Context(), actor, r.PathValue("subject_id"), r.PathValue("material_id"), application.ReplaceInput{Filename: files[0].Filename, Data: data})
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	_ = rc.SetWriteDeadline(time.Now().Add(30 * time.Second))
	httpx.JSON(w, http.StatusOK, updated)
}
