// Package geek is the HTTP transport of the Geek oral exam. Its DTOs are the
// geek.* schemas of the OpenAPI contract.
package geek

import (
	"mime"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"

	domain "github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type StartRequest struct {
	SubjectID string `json:"subject_id" minLength:"1"`
}

// AnswerRequest carries exactly one of transcription_id, text and dont_know.
type AnswerRequest struct {
	// current_turn_id сессии
	TurnID string `json:"turn_id" minLength:"1"`
	// Расшифровка из POST /voice/transcriptions, подтверждённая студентом
	TranscriptionID string `json:"transcription_id,omitempty" minLength:"1" binding:"optional"`
	// Текстовый ответ
	Text string `json:"text,omitempty" minLength:"1" maxLength:"4000" binding:"optional"`
	// Кнопка «Не знаю» без оценки моделью: на основной вопрос или переформулировку — пробел, на дополнительный — К3 и К4 на 0
	DontKnow bool `json:"dont_know,omitempty" binding:"optional"`
}

type Turn struct {
	CreatedAt int64  `json:"created_at"`
	ID        string `json:"id"`
	// question — основной вопрос, followup — дополнительный, rephrase — переформулировка, closing — завершение, answer — ответ студента
	Kind string `json:"kind" enums:"question,followup,rephrase,closing,answer"`
	// Номер строки транскрипта
	Line int `json:"line" minimum:"1"`
	// Вершина графа, к которой относится реплика; null у closing
	NodeID *string `json:"node_id" extensions:"x-nullable"`
	// Только у реплик студента: голос после подтверждения, текст или кнопка «Не знаю»
	Source  *string `json:"source" enums:"voice,text,dont_know" extensions:"x-nullable"`
	Speaker string  `json:"speaker" enums:"geek,student"`
	Text    string  `json:"text"`
	// Начало text у реплики Geek — реакция на прошлый ответ студента; пусто, если реакции нет. Озвучивается отдельно: GET .../audio?part=reply, остальное — part=question
	Reply string `json:"reply"`
}

type Session struct {
	CompletedAt *int64 `json:"completed_at" extensions:"x-nullable"`
	// Реплика Geek, на которую ждут ответ; null, если сессия не active
	CurrentTurnID *string `json:"current_turn_id" extensions:"x-nullable"`
	ID            string  `json:"id"`
	// Доля тем верхнего уровня, по которым вывод уже сделан, вместе со спуском; не уменьшается
	ProgressPercent int   `json:"progress_percent" minimum:"0" maximum:"100"`
	StartedAt       int64 `json:"started_at"`
	// cancelled — граф курса пересобран во время диагностики
	Status    string `json:"status" enums:"active,completed,cancelled"`
	SubjectID string `json:"subject_id"`
	Turns     []Turn `json:"turns"`
}

// Gap is the deepest unmastered topic whose prerequisites are all mastered.
type Gap struct {
	NodeID            string            `json:"node_id"`
	Title             string            `json:"title"`
	Weeks             []int32           `json:"weeks"`
	Sources           []SourceCitation  `json:"sources"`
	LearningMaterials LearningMaterials `json:"learning_materials"`
}

type SourceCitation struct {
	MaterialID       string  `json:"material_id"`
	BlockID          int     `json:"block_id"`
	Start            int     `json:"start"`
	End              int     `json:"end"`
	Text             string  `json:"text"`
	Page             *int    `json:"page" extensions:"x-nullable"`
	Week             *int32  `json:"week" extensions:"x-nullable"`
	Kind             *string `json:"kind" extensions:"x-nullable"`
	Filename         *string `json:"filename" extensions:"x-nullable"`
	KindLabel        *string `json:"kind_label" extensions:"x-nullable"`
	MaterialRevision *string `json:"material_revision" extensions:"x-nullable"`
}

type MaterialReference struct {
	MaterialID       string  `json:"material_id"`
	Filename         *string `json:"filename" extensions:"x-nullable"`
	Kind             *string `json:"kind" extensions:"x-nullable"`
	KindLabel        *string `json:"kind_label" extensions:"x-nullable"`
	Week             *int32  `json:"week" extensions:"x-nullable"`
	MaterialRevision *string `json:"material_revision" extensions:"x-nullable"`
}

type LearningMaterials struct {
	Theory     []MaterialReference `json:"theory"`
	Practice   []MaterialReference `json:"practice"`
	Additional []MaterialReference `json:"additional"`
}

type NodeResult struct {
	// Оценка A–D; null — оценки нет (тема не проверялась или недостаточно данных)
	Grade  *string `json:"grade" enums:"A,B,C,D" extensions:"x-nullable"`
	NodeID string  `json:"node_id"`
	// independent — самостоятельный ответ, assisted — ответ с помощью, gap — пробел, insufficient — недостаточно данных, credited_by_parent — засчитано по вышестоящей вершине (для неё это пререквизит), unchecked — не спрашивалась
	Status string `json:"status" enums:"independent,assisted,gap,insufficient,credited_by_parent,unchecked"`
	Title  string `json:"title"`
	// Вершина верхнего уровня: не пререквизит ни одной другой; по ним считается оценка курса
	TopLevel bool `json:"top_level"`
}

// Result is the student's outcome; criteria and evidence stay in the teacher package.
type Result struct {
	Gaps []Gap `json:"gaps"`
	// Предварительная оценка курса A–D по среднему баллу вершин верхнего уровня; null — ни одной такой вершины с оценкой
	Grade *string      `json:"grade" enums:"A,B,C,D" extensions:"x-nullable"`
	Nodes []NodeResult `json:"nodes"`
	// Средний балл вершин верхнего уровня (A=4…D=1) в процентах от 4; вершины «недостаточно данных» исключены
	ScorePercent *int   `json:"score_percent" minimum:"0" maximum:"100" extensions:"x-nullable"`
	SessionID    string `json:"session_id"`
}

// HistoryItem is one of the student's diagnostics on the course.
type HistoryItem struct {
	CompletedAt *int64 `json:"completed_at" extensions:"x-nullable"`
	// Предварительная оценка курса; null — диагностика не завершена или ни одной темы верхнего уровня с оценкой
	Grade           *string `json:"grade" enums:"A,B,C,D" extensions:"x-nullable"`
	ID              string  `json:"id"`
	ProgressPercent int     `json:"progress_percent" minimum:"0" maximum:"100"`
	ScorePercent    *int    `json:"score_percent" minimum:"0" maximum:"100" extensions:"x-nullable"`
	StartedAt       int64   `json:"started_at"`
	Status          string  `json:"status" enums:"active,completed,cancelled"`
}

type History struct {
	Items []HistoryItem `json:"items"`
}

type Handler struct{ service *application.Service }

func New(service *application.Service) *Handler { return &Handler{service: service} }

func principal(w http.ResponseWriter, r *http.Request) (user.User, bool) {
	u, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
	}
	return u, ok
}

// Start handles POST /diagnostic-sessions.
// @Summary Начать или продолжить диагностику
// @Description Устный экзамен Geek по графу курса. У студента одна активная диагностика на курс: если она есть, возвращается она (200), иначе создаётся новая с первым вопросом (201). 409 GRAPH_NOT_READY — граф курса не собран.
// @ID startDiagnosticSession
// @Tags Диагностика
// @Security accessCookie
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "Ключ повтора запроса"
// @Param request body StartRequest true "Курс"
// @Success 200 {object} Session
// @Success 201 {object} Session
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /diagnostic-sessions [post]
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	u, ok := principal(w, r)
	if !ok {
		return
	}
	var req StartRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	session, created, err := h.service.Start(r.Context(), u.ID, req.SubjectID, r.Header.Get("Idempotency-Key"))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	httpx.JSON(w, status, toSession(session))
}

// History handles GET /diagnostic-sessions.
// @Summary Мои диагностики курса
// @Description Диагностики текущего студента по курсу, новые первыми (не больше 50). Оценка — только у завершённых.
// @ID listDiagnosticSessions
// @Tags Диагностика
// @Security accessCookie
// @Produce json
// @Param subject_id query string true "ID курса (предмета)" minlength(1) maxlength(128)
// @Success 200 {object} History
// @Failure 401 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Router /diagnostic-sessions [get]
func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	u, ok := principal(w, r)
	if !ok {
		return
	}
	query, err := httpx.ParseQuery(r, "subject_id")
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	sessions, err := h.service.History(r.Context(), u.ID, query.Get("subject_id"))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	out := History{Items: []HistoryItem{}}
	for _, s := range sessions {
		item := HistoryItem{CompletedAt: s.CompletedAt, ID: s.ID, ProgressPercent: s.Exam.Progress(), StartedAt: s.StartedAt, Status: string(s.Status)}
		if s.Status == domain.SessionCompleted {
			r := s.Result()
			item.Grade, item.ScorePercent = grade(r.Grade), r.ScorePercent
		}
		out.Items = append(out.Items, item)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// Read handles GET /diagnostic-sessions/{id}.
// @Summary Состояние диагностики
// @Description Реплики, прогресс и текущий вопрос. Доступ владельца.
// @ID getDiagnosticSession
// @Tags Диагностика
// @Security accessCookie
// @Produce json
// @Param id path string true "ID диагностики" minlength(1) maxlength(128)
// @Success 200 {object} Session
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Router /diagnostic-sessions/{id} [get]
func (h *Handler) Read(w http.ResponseWriter, r *http.Request) {
	u, ok := principal(w, r)
	if !ok {
		return
	}
	session, err := h.service.Get(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toSession(session))
}

// Answer handles POST /diagnostic-sessions/{id}/answers.
// @Summary Ответить на реплику Geek
// @Description Синхронно: ответ содержит реплику студента и следующую реплику Geek либо завершённую сессию. Повтор с тем же ключом возвращает тот же результат. 409 — turn_id не текущий или сессия не active.
// @ID answerDiagnosticSession
// @Tags Диагностика
// @Security accessCookie
// @Accept json
// @Produce json
// @Param id path string true "ID диагностики" minlength(1) maxlength(128)
// @Param Idempotency-Key header string true "Ключ повтора запроса"
// @Param request body AnswerRequest true "Ответ на текущую реплику"
// @Success 200 {object} Session
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /diagnostic-sessions/{id}/answers [post]
func (h *Handler) Answer(w http.ResponseWriter, r *http.Request) {
	u, ok := principal(w, r)
	if !ok {
		return
	}
	var req AnswerRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	session, err := h.service.Answer(r.Context(), u.ID, r.PathValue("id"), r.Header.Get("Idempotency-Key"), application.Answer{
		TurnID: req.TurnID, TranscriptionID: req.TranscriptionID, Text: req.Text, DontKnow: req.DontKnow,
	})
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toSession(session))
}

// Result handles GET /diagnostic-sessions/{id}/result.
// @Summary Итог диагностики
// @Description Предварительная оценка A–D, статусы тем и список пробелов. 409 — диагностика не completed.
// @ID getDiagnosticResult
// @Tags Диагностика
// @Security accessCookie
// @Produce json
// @Param id path string true "ID диагностики" minlength(1) maxlength(128)
// @Success 200 {object} Result
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Router /diagnostic-sessions/{id}/result [get]
func (h *Handler) Result(w http.ResponseWriter, r *http.Request) {
	u, ok := principal(w, r)
	if !ok {
		return
	}
	result, err := h.service.Result(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toResult(result))
}

// RawMaterial serves a source file linked by the diagnostic snapshot.
// @Summary Исходный материал пробела
// @Description Выдаёт исходный PDF только если он связан со snapshot диагностики и не заменён.
// @ID getDiagnosticMaterial
// @Tags Диагностика
// @Security accessCookie
// @Produce application/pdf
// @Param id path string true "ID диагностики"
// @Param material_id path string true "ID материала"
// @Success 200 {file} binary
// @Failure 404 {object} fault.Error
// @Failure 410 {object} fault.Error
// @Router /diagnostic-sessions/{id}/materials/{material_id}/raw [get]
func (h *Handler) RawMaterial(w http.ResponseWriter, r *http.Request) {
	u, ok := principal(w, r)
	if !ok {
		return
	}
	result, err := h.service.RawMaterial(r.Context(), u.ID, r.PathValue("id"), r.PathValue("material_id"))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": result.Filename}))
	w.Header().Set("Content-Length", strconv.Itoa(len(result.Data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(result.Data)
}

// TurnAudio handles GET /diagnostic-sessions/{id}/turns/{turn_id}/audio.
// @Summary Голос реплики Geek
// @Description Backend синтезирует реплику Geek через TTS и кэширует WAV в S3; длина реплики не ограничена лимитом /voice/syntheses. Без part — вся реплика; part=reply — только реакция на прошлый ответ (поле reply), part=question — вопрос или завершение после неё. Части кэшируются по тексту, поэтому заранее подготовленный вопрос синтезируется один раз для всех студентов. 404 — реплика студента, чужая сессия или пустая часть (TURN_PART_NOT_FOUND).
// @ID getDiagnosticTurnAudio
// @Tags Диагностика
// @Security accessCookie
// @Produce audio/wav
// @Param id path string true "ID диагностики" minlength(1) maxlength(128)
// @Param turn_id path string true "ID реплики Geek" minlength(1) maxlength(128)
// @Param part query string false "Часть реплики" Enums(reply, question)
// @Success 200 {file} file "WAV реплики"
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /diagnostic-sessions/{id}/turns/{turn_id}/audio [get]
func (h *Handler) TurnAudio(w http.ResponseWriter, r *http.Request) {
	u, ok := principal(w, r)
	if !ok {
		return
	}
	part := application.AudioPart(r.URL.Query().Get("part"))
	started := time.Now()
	data, err := h.service.TurnAudio(r.Context(), u.ID, r.PathValue("id"), r.PathValue("turn_id"), part)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.LoggerFromContext(r.Context()).Info("geek turn audio", zap.String("session_id", r.PathValue("id")), zap.String("turn_id", r.PathValue("turn_id")),
		zap.String("part", string(part)), zap.Int("bytes", len(data)), zap.Duration("duration", time.Since(started)))
	w.Header().Set("Content-Type", "audio/wav")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func toSession(s domain.Session) Session {
	out := Session{CompletedAt: s.CompletedAt, ID: s.ID, ProgressPercent: s.Exam.Progress(), StartedAt: s.StartedAt, Status: string(s.Status), SubjectID: s.SubjectID, Turns: []Turn{}}
	if current, ok := s.CurrentTurn(); ok {
		out.CurrentTurnID = &current.ID
	}
	for _, t := range s.Turns {
		var source *string
		if t.Source != nil {
			v := string(*t.Source)
			source = &v
		}
		out.Turns = append(out.Turns, Turn{CreatedAt: t.CreatedAt, ID: t.ID, Kind: string(t.Kind), Line: t.Line, NodeID: t.NodeID, Source: source, Speaker: string(t.Speaker), Text: t.Text, Reply: t.Reply})
	}
	return out
}

func grade(g *domain.Grade) *string {
	if g == nil {
		return nil
	}
	v := string(*g)
	return &v
}

func toResult(r domain.Result) Result {
	out := Result{Gaps: []Gap{}, Grade: grade(r.Grade), Nodes: []NodeResult{}, ScorePercent: r.ScorePercent, SessionID: r.SessionID}
	for _, g := range r.Gaps {
		weeks, sources := g.Weeks, g.Sources
		if weeks == nil {
			weeks = []int32{}
		}
		if sources == nil {
			sources = []domain.SourceCitation{}
		}
		learning := g.LearningMaterials
		if learning.Theory == nil {
			learning.Theory = []domain.MaterialReference{}
		}
		if learning.Practice == nil {
			learning.Practice = []domain.MaterialReference{}
		}
		if learning.Additional == nil {
			learning.Additional = []domain.MaterialReference{}
		}
		out.Gaps = append(out.Gaps, Gap{NodeID: g.NodeID, Title: g.Title, Weeks: weeks,
			Sources: sourceDTOs(sources), LearningMaterials: learningDTO(learning)})
	}
	for _, n := range r.Nodes {
		out.Nodes = append(out.Nodes, NodeResult{Grade: grade(n.Grade), NodeID: n.NodeID, Status: string(n.Status), Title: n.Title, TopLevel: n.TopLevel})
	}
	return out
}

func sourceDTOs(sources []domain.SourceCitation) []SourceCitation {
	out := make([]SourceCitation, 0, len(sources))
	for _, source := range sources {
		out = append(out, SourceCitation{MaterialID: source.MaterialID, BlockID: source.BlockID,
			Start: source.Start, End: source.End, Text: source.Text, Page: source.Page, Week: source.Week,
			Kind: source.Kind, Filename: source.Filename, KindLabel: source.KindLabel, MaterialRevision: source.MaterialRevision})
	}
	return out
}

func learningDTO(value domain.LearningMaterials) LearningMaterials {
	convert := func(items []domain.MaterialReference) []MaterialReference {
		out := make([]MaterialReference, 0, len(items))
		for _, item := range items {
			out = append(out, MaterialReference{MaterialID: item.MaterialID, Filename: item.Filename,
				Kind: item.Kind, KindLabel: item.KindLabel, Week: item.Week, MaterialRevision: item.Revision})
		}
		return out
	}
	return LearningMaterials{Theory: convert(value.Theory), Practice: convert(value.Practice), Additional: convert(value.Additional)}
}
