package exam

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// RubricItem is an instructor's criterion, not a universal Geek skill.
type RubricItem struct {
	ID     string `json:"id"`
	Rule   string `json:"rule"`
	Levels []int  `json:"levels"`
}

type RubricMark struct {
	RubricItem
	Score    int    `json:"score"`
	Evidence []int  `json:"evidence_lines"`
	Reason   string `json:"reason"`
	Quote    string `json:"quote,omitempty"`
}

type RubricResult struct {
	Items []RubricMark `json:"items"`
}

var rubricStart = regexp.MustCompile(`(?m)^[\t ]*(?:Пункт рубрики\s+|[КK])(\d+)\s`)
var rubricLevel = regexp.MustCompile(`(?:^|[:;\n])\s*(\d+)\s*[—–−-]`)
var rubricMaximum = regexp.MustCompile(`(?i)макс\.?(?:имум)?\s*[:=]?\s*(\d+)`)

// ParseRubric reads only explicit numeric levels; it never invents criteria or
// uses the model to determine maxima. Non-numeric authored guidance stays legacy.
func ParseRubric(text string) ([]RubricItem, error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	starts := rubricStart.FindAllStringSubmatchIndex(text, -1)
	if len(starts) == 0 && !rubricMaximum.MatchString(text) && !rubricLevel.MatchString(text) {
		return nil, nil
	}
	if len(starts) == 0 && !strings.Contains(text, "Балл") && !strings.Contains(text, "Аналитическая рубрика") {
		return nil, nil
	}
	var items []RubricItem
	add := func(id, rule string) error {
		levels := []int{}
		for _, m := range rubricLevel.FindAllStringSubmatch(rule, -1) {
			v, _ := strconv.Atoi(m[1])
			if !slices.Contains(levels, v) {
				levels = append(levels, v)
			}
		}
		slices.Sort(levels)
		if len(levels) < 2 || levels[0] != 0 || levels[len(levels)-1] <= 0 {
			return fmt.Errorf("rubric %s needs explicit zero and positive scoring levels", id)
		}
		for _, item := range items {
			if item.ID == id {
				return fmt.Errorf("duplicate rubric item %s", id)
			}
		}
		items = append(items, RubricItem{ID: id, Rule: strings.TrimSpace(rule), Levels: levels})
		return nil
	}
	if len(starts) > 0 {
		for i, m := range starts {
			end := len(text)
			if i+1 < len(starts) {
				end = starts[i+1][0]
			}
			if err := add("r"+text[m[2]:m[3]], text[m[0]:end]); err != nil {
				return nil, err
			}
		}
	} else {
		if err := add("r1", text); err != nil {
			return nil, err
		}
	}
	total := 0
	for _, item := range items {
		total += item.Levels[len(item.Levels)-1]
	}
	if m := rubricMaximum.FindStringSubmatch(text); m != nil {
		declared, _ := strconv.Atoi(m[1])
		if total != declared {
			return nil, fmt.Errorf("rubric maximum %d differs from item total %d", declared, total)
		}
	}
	return items, nil
}

func (r RubricResult) Totals() (score, maximum int) {
	for _, m := range r.Items {
		score += m.Score
		maximum += m.Levels[len(m.Levels)-1]
	}
	return
}
func (r RubricResult) Complete() bool { s, m := r.Totals(); return m > 0 && s == m }
func (r RubricResult) HasScore() bool { s, _ := r.Totals(); return s > 0 }
func (r RubricResult) Grade(assisted, complete bool) Grade {
	score, max := r.Totals()
	switch {
	case max > 0 && score == max && complete && !assisted:
		return GradeA
	case max > 0 && score*4 >= max*3:
		return GradeB
	case score > 0:
		return GradeC
	default:
		return GradeD
	}
}

// Validate ties each awarded mark to a real student answer, not the reference,
// a conversational control message or a teacher-provided solution.
func (r RubricResult) Validate(spec []RubricItem, turns []Turn) error {
	if len(r.Items) != len(spec) {
		return fmt.Errorf("rubric item count mismatch")
	}
	for i, m := range r.Items {
		want := spec[i]
		if m.ID != want.ID || !slices.Equal(m.Levels, want.Levels) || m.Rule != want.Rule || !slices.Contains(want.Levels, m.Score) {
			return fmt.Errorf("invalid rubric item %s", m.ID)
		}
		if strings.TrimSpace(m.Reason) == "" || m.Score > 0 && len(m.Evidence) == 0 {
			return fmt.Errorf("rubric %s missing reason or evidence", m.ID)
		}
		quoted := false
		actual := []string{}
		for _, line := range m.Evidence {
			found := false
			for _, t := range turns {
				if t.Line == line && t.Speaker == SpeakerStudent && !t.Meta {
					found = true
					actual = append(actual, t.Text)
					quote := strings.Join(strings.Fields(m.Quote), " ")
					if quote != "" && strings.Contains(strings.Join(strings.Fields(t.Text), " "), quote) {
						quoted = true
					}
					break
				}
			}
			if !found {
				return fmt.Errorf("rubric %s has invalid student evidence %d", m.ID, line)
			}
		}
		if m.Score > 0 && !quoted {
			return fmt.Errorf("rubric %s: quote %q is not an exact substring. Copy a SHORT unchanged fragment from these actual student texts (do not change person, verbs or punctuation): %q", m.ID, m.Quote, actual)
		}
	}
	return nil
}
