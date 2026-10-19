package exam

import "math"

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

func (g Grade) points() int {
	switch g {
	case GradeA:
		return 4
	case GradeB:
		return 3
	case GradeC:
		return 2
	}
	return 1
}

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

// CourseGrade averages the graded top-level topics (A = 4 … D = 1) and rounds
// the mean to the nearest grade: at least 3.5 is A, 2.5 B, 1.5 C, otherwise D,
// so topics all on one grade give that grade (decision of 20.10.2026). The
// percent is the mean as a share of the maximum 4. Topics without a grade
// ("insufficient data" or unchecked) are excluded; with none left both
// results are nil.
func CourseGrade(grades []Grade) (*Grade, *int) {
	if len(grades) == 0 {
		return nil, nil
	}
	sum := 0
	for _, g := range grades {
		sum += g.points()
	}
	// Twice the mean compared with whole thresholds keeps the halves exact.
	twice := 2 * sum
	n := len(grades)
	percent := int(math.Floor(float64(sum)/float64(4*n)*100 + 1e-9))
	grade := GradeD
	switch {
	case twice >= 7*n:
		grade = GradeA
	case twice >= 5*n:
		grade = GradeB
	case twice >= 3*n:
		grade = GradeC
	}
	return &grade, &percent
}
