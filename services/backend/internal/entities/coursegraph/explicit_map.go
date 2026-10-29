package coursegraph

import (
	"encoding/json"
	"fmt"
)

// ExplicitMapVersion preserves the author's topic IDs and prerequisites.
const ExplicitMapVersion = "competency-map-explicit-v1"

// explicitMapGraph retains every included outcome and the author's dependency
// graph. It is enabled only by both explicit graph headers in the workbook.
func explicitMapGraph(outcomes []mapOutcome) (json.RawMessage, int, error) {
	byID := make(map[string]mapOutcome, len(outcomes))
	invalid := func(row int, column, message string) error {
		return &MapError{Code: "COMPETENCY_MAP_INVALID", Message: "Ошибка явного графа карты компетенций.", Issues: []MapIssue{{Row: row, Column: columnTitle(column), Message: message}}}
	}
	for _, o := range outcomes {
		if _, exists := byID[o.id]; exists {
			return nil, 0, invalid(o.row, "topic_id", "Повтор ID темы: "+o.id)
		}
		byID[o.id] = o
	}
	required := map[string]bool{}
	for _, o := range outcomes {
		for _, id := range o.requires {
			if _, exists := byID[id]; !exists {
				return nil, 0, invalid(o.row, "prerequisites", "Пререквизит отсутствует среди включённых тем: "+id)
			}
			required[id] = true
		}
	}
	state := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return invalid(byID[id].row, "prerequisites", "Цикл пререквизитов у темы "+id)
		}
		if state[id] == 2 {
			return nil
		}
		state[id] = 1
		for _, base := range byID[id].requires {
			if err := visit(base); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for _, o := range outcomes {
		if err := visit(o.id); err != nil {
			return nil, 0, err
		}
	}
	topics := make([]mapTopic, 0, len(outcomes))
	prerequisites, hierarchy := []mapEdge{}, []mapEdge{}
	for _, o := range outcomes {
		role := "main"
		if required[o.id] {
			role = "basic"
		}
		topics = append(topics, newMapTopic(o, role, o.requires))
		for _, base := range o.requires {
			prerequisites = append(prerequisites, mapEdge{Source: base, Target: o.id, Reason: fmt.Sprintf("Явный пререквизит из карты, строка %d.", o.row)})
			hierarchy = append(hierarchy, mapEdge{Source: o.id, Target: base})
		}
	}
	data, err := json.Marshal(map[string]any{
		"version": ExplicitMapVersion, "source": SourceMap, "topics": topics,
		"prerequisites": prerequisites, "hierarchy": hierarchy, "skipped": []SkippedCompetency{},
		"quality": map[string]any{"acyclic": true, "topics": len(topics)},
	})
	return data, len(topics), err
}
