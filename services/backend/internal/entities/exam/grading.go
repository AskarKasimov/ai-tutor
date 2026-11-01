package exam

// Criteria are the four exam criteria, each 0 (not shown), 1 (partly) or 2 (fully):
// K1 correctness and terminology, K2 logic and justification, K3 objections and
// counterexamples, K4 changed conditions and limits of the answer.
type Criteria struct {
	K1 int `json:"k1"`
	K2 int `json:"k2"`
	K3 int `json:"k3"`
	K4 int `json:"k4"`
}

// UnassessedCriteria distinguishes an untested skill from a failed test (0).
// K1 and K2 are always assessed; K3 and K4 may be outside the question's scope.
type UnassessedCriteria struct {
	K3 bool `json:"k3,omitempty"`
	K4 bool `json:"k4,omitempty"`
}

func (u UnassessedCriteria) values(c Criteria) []int {
	v := []int{c.K1, c.K2}
	if !u.K3 {
		v = append(v, c.K3)
	}
	if !u.K4 {
		v = append(v, c.K4)
	}
	return v
}

// Complete reports whether every assessed criterion is fully demonstrated.
func (u UnassessedCriteria) Complete(c Criteria) bool {
	for _, v := range u.values(c) {
		if v != 2 {
			return false
		}
	}
	return true
}

// Valid reports whether every criterion is 0, 1 or 2.
func (c Criteria) Valid() bool {
	for _, v := range c.values() {
		if v < 0 || v > 2 {
			return false
		}
	}
	return true
}

func (c Criteria) values() [4]int { return [4]int{c.K1, c.K2, c.K3, c.K4} }

type Grade string

const (
	GradeA Grade = "A"
	GradeB Grade = "B"
	GradeC Grade = "C"
	GradeD Grade = "D"
)

// TopicGrade applies the README table to the assessed criteria. An assisted
// answer is capped at B; help never raises identical observed scores.
func TopicGrade(c Criteria, assisted bool, unassessed UnassessedCriteria) Grade {
	v := unassessed.values(c)
	all := func(min int) bool {
		for _, x := range v {
			if x < min {
				return false
			}
		}
		return true
	}
	anyTwo := false
	for _, x := range v {
		anyTwo = anyTwo || x == 2
	}
	grade := GradeD
	switch {
	case !assisted && all(2):
		grade = GradeA
	case c.K1 == 2 && c.K2 == 2 && all(1):
		grade = GradeB
	case c.K1 >= 1 && (c.K2 >= 1 || anyTwo):
		grade = GradeC
	}
	return grade
}

// CourseGrade translates earned points into the overall grade, the same way in
// the exam and the learning diagnostic: at least 90% is A, 70% B, 60% C,
// otherwise D (decision of 01.11.2026). The percent is rounded down, so 89.9%
// stays B. With nothing to grade both results are nil.
func CourseGrade(score, maximum int) (*Grade, *int) {
	if maximum <= 0 {
		return nil, nil
	}
	percent := score * 100 / maximum
	grade := GradeD
	switch {
	case percent >= 90:
		grade = GradeA
	case percent >= 70:
		grade = GradeB
	case percent >= 60:
		grade = GradeC
	}
	return &grade, &percent
}
