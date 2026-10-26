package subject

type Subject struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Ready         bool   `json:"ready"`
	LearningReady bool   `json:"learning_ready"`
	ExamReady     bool   `json:"exam_ready"`
}
