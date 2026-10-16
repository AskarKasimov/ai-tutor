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

// TopicGrade applies the README table. An assisted answer (after a rephrase)
// can only be B or C, or D when it still falls short.
func TopicGrade(c Criteria, assisted bool) Grade {
	v := c.values()
	all := func(min int) bool {
		for _, x := range v {
			if x < min {
				return false
			}
		}
		return true
	}
	anyTwo := v[0] == 2 || v[1] == 2 || v[2] == 2 || v[3] == 2
	grade := GradeD
	switch {
	case !assisted && all(2):
		grade = GradeA
	case c.K1 == 2 && c.K2 == 2 && c.K3 >= 1 && c.K4 >= 1:
		grade = GradeB
	case assisted && all(1):
		grade = GradeB
	case c.K1 >= 1 && anyTwo:
		grade = GradeC
	}
	return grade
}

// CourseGrade averages the graded top-level topics (A = 4 … D = 1) as a share
// of the maximum 4: at least 90% is A, 75% B, 60% C, otherwise D. Topics
// without a grade ("insufficient data" or unchecked) are excluded; with none
// left both results are nil.
func CourseGrade(grades []Grade) (*Grade, *int) {
	if len(grades) == 0 {
		return nil, nil
	}
	sum := 0
	for _, g := range grades {
		sum += g.points()
	}
	share := float64(sum) / float64(4*len(grades)) * 100
	percent := int(math.Floor(share + 1e-9))
	grade := GradeD
	switch {
	case share >= 90:
		grade = GradeA
	case share >= 75:
		grade = GradeB
	case share >= 60:
		grade = GradeC
	}
	return &grade, &percent
}
