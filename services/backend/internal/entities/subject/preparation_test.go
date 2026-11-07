package subject

import "testing"

func TestProjectPreparation(t *testing.T) {
	tests := []struct {
		name   string
		facts  PreparationFacts
		exam   ExamPreparationState
		source string
		learn  LearningPreparationState
	}{
		{name: "new course", exam: ExamEmpty, learn: LearningEmpty},
		{name: "materials wait for OCR", facts: PreparationFacts{HasMaterials: true, MaterialsQueued: true}, exam: ExamQueued, source: "materials", learn: LearningEmpty},
		{name: "pending build remains queued after retryable error", facts: PreparationFacts{HasMaterials: true, JobSource: "materials", JobStatus: "pending"}, exam: ExamQueued, source: "materials", learn: LearningEmpty},
		{name: "materials build processing", facts: PreparationFacts{HasMaterials: true, JobSource: "materials", JobStatus: "processing"}, exam: ExamProcessing, source: "materials", learn: LearningEmpty},
		{name: "active OCR outranks another failed file", facts: PreparationFacts{HasMaterials: true, MaterialsProcessing: true, MaterialsFailed: true}, exam: ExamProcessing, source: "materials", learn: LearningEmpty},
		{name: "material terminal failure", facts: PreparationFacts{HasMaterials: true, MaterialsFailed: true}, exam: ExamFailed, source: "materials", learn: LearningEmpty},
		{name: "metadata blocks queued graph", facts: PreparationFacts{HasMaterials: true, MaterialsNeedMetadata: true, JobSource: "materials", JobStatus: "pending"}, exam: ExamNeedsMetadata, source: "materials", learn: LearningEmpty},
		{name: "terminal current graph failure", facts: PreparationFacts{HasMaterials: true, JobSource: "materials", JobStatus: "failed", JobFingerprintCurrent: true}, exam: ExamFailed, source: "materials", learn: LearningEmpty},
		{name: "old graph failure is ignored", facts: PreparationFacts{HasMaterials: true, JobSource: "materials", JobStatus: "failed"}, exam: ExamNeedsBuild, source: "materials", learn: LearningEmpty},
		{name: "map build ignores materials", facts: PreparationFacts{HasMap: true, JobSource: "competency_map", JobStatus: "processing", MaterialsFailed: true, MaterialsProcessing: true}, exam: ExamProcessing, source: "competency_map", learn: LearningEmpty},
		{name: "published materials are ready", facts: PreparationFacts{HasMaterials: true, GraphExists: true, GraphSource: "materials", GraphFingerprintCurrent: true}, exam: ExamReady, source: "materials", learn: LearningEmpty},
		{name: "active learning map without tasks", facts: PreparationFacts{LearningMapActive: true}, exam: ExamEmpty, learn: LearningNeedsTasks},
		{name: "learning readiness", facts: PreparationFacts{LearningMapActive: true, LearningReady: true}, exam: ExamEmpty, learn: LearningReady},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProjectPreparation(tt.facts)
			if got.Exam.State != tt.exam {
				t.Fatalf("exam state = %q, want %q", got.Exam.State, tt.exam)
			}
			if got.Learn.State != tt.learn {
				t.Fatalf("learn state = %q, want %q", got.Learn.State, tt.learn)
			}
			if tt.source == "" && got.Exam.Source != nil {
				t.Fatalf("source = %q, want nil", *got.Exam.Source)
			}
			if tt.source != "" && (got.Exam.Source == nil || *got.Exam.Source != tt.source) {
				t.Fatalf("source = %v, want %q", got.Exam.Source, tt.source)
			}
		})
	}
}

func TestProjectPreparationStaleOldPublication(t *testing.T) {
	got := ProjectPreparation(PreparationFacts{HasMaterials: true, GraphExists: true, GraphSource: "materials", GraphStale: true})
	if got.Exam.State != ExamNeedsBuild || !got.Exam.Stale {
		t.Fatalf("got %+v", got.Exam)
	}
}
