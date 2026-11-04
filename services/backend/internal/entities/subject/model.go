package subject

type Subject struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	Ready         bool               `json:"ready"`
	LearningReady bool               `json:"learning_ready"`
	ExamReady     bool               `json:"exam_ready"`
	Preparation   *CoursePreparation `json:"preparation,omitempty" binding:"optional"`
}

type ExamPreparationState string

const (
	ExamEmpty         ExamPreparationState = "empty"
	ExamQueued        ExamPreparationState = "queued"
	ExamProcessing    ExamPreparationState = "processing"
	ExamFailed        ExamPreparationState = "failed"
	ExamNeedsMetadata ExamPreparationState = "needs_metadata"
	ExamNeedsBuild    ExamPreparationState = "needs_build"
	ExamReady         ExamPreparationState = "ready"
)

type LearningPreparationState string

const (
	LearningEmpty      LearningPreparationState = "empty"
	LearningNeedsTasks LearningPreparationState = "needs_tasks"
	LearningReady      LearningPreparationState = "ready"
)

type CoursePreparation struct {
	Exam  ExamPreparation     `json:"exam"`
	Learn LearningPreparation `json:"learn"`
}

type ExamPreparation struct {
	State  ExamPreparationState `json:"state" enums:"empty,queued,processing,failed,needs_metadata,needs_build,ready"`
	Source *string              `json:"source" enums:"materials,competency_map" extensions:"x-nullable"`
	Stale  bool                 `json:"stale"`
}

type LearningPreparation struct {
	State LearningPreparationState `json:"state"`
}

// PreparationFacts is the database snapshot used to project admin-only status.
// It intentionally contains no user-facing strings or source identifiers beyond
// the durable source values stored by course graph jobs and publications.
type PreparationFacts struct {
	HasMaterials            bool
	HasMap                  bool
	JobSource               string
	JobStatus               string
	JobFingerprintCurrent   bool
	GraphSource             string
	GraphExists             bool
	GraphFingerprintCurrent bool
	GraphStale              bool
	MaterialsProcessing     bool
	MaterialsQueued         bool
	MaterialsFailed         bool
	MaterialsNeedMetadata   bool
	LearningMapActive       bool
	LearningReady           bool
}

func ProjectPreparation(f PreparationFacts) CoursePreparation {
	source := f.JobSource
	if source == "" && f.GraphExists {
		source = normalizeSource(f.GraphSource)
	}
	if source == "" {
		switch {
		case f.HasMap:
			source = "competency_map"
		case f.HasMaterials:
			source = "materials"
		}
	}
	var sourcePtr *string
	if (source == "materials" && f.HasMaterials) || (source == "competency_map" && f.HasMap) {
		value := source
		sourcePtr = &value
	}
	state := ExamEmpty
	if sourcePtr != nil {
		switch {
		case f.JobSource == source && f.JobStatus == "processing":
			state = ExamProcessing
		case source == "materials" && f.MaterialsProcessing:
			state = ExamProcessing
		case source == "materials" && f.MaterialsFailed:
			state = ExamFailed
		case source == "materials" && f.MaterialsNeedMetadata:
			state = ExamNeedsMetadata
		case source == "materials" && f.MaterialsQueued:
			state = ExamQueued
		case f.JobSource == source && f.JobStatus == "pending":
			state = ExamQueued
		case f.JobSource == source && f.JobStatus == "failed" && f.JobFingerprintCurrent:
			state = ExamFailed
		case f.GraphExists && normalizeSource(f.GraphSource) == source && f.GraphFingerprintCurrent && !f.GraphStale:
			state = ExamReady
		default:
			state = ExamNeedsBuild
		}
	}
	learnState := LearningEmpty
	if f.LearningMapActive {
		learnState = LearningNeedsTasks
		if f.LearningReady {
			learnState = LearningReady
		}
	}
	return CoursePreparation{Exam: ExamPreparation{State: state, Source: sourcePtr, Stale: f.GraphExists && f.GraphStale}, Learn: LearningPreparation{State: learnState}}
}

func normalizeSource(source string) string {
	if source == "built" {
		return "materials"
	}
	return source
}

func EmptyPreparation() *CoursePreparation {
	value := CoursePreparation{
		Exam:  ExamPreparation{State: ExamEmpty},
		Learn: LearningPreparation{State: LearningEmpty},
	}
	return &value
}
