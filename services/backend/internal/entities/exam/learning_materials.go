package exam

import "sort"

// SelectLearningMaterials selects only documents cited by the topic and keeps
// their order stable for both completed diagnostics and future study flows.
func SelectLearningMaterials(sources []SourceCitation) LearningMaterials {
	byID := map[string]MaterialReference{}
	for _, source := range sources {
		if source.MaterialID == "" {
			continue
		}
		ref := MaterialReference{MaterialID: source.MaterialID, Filename: source.Filename,
			Kind: source.Kind, KindLabel: source.KindLabel, Week: source.Week, Revision: source.MaterialRevision}
		if previous, ok := byID[source.MaterialID]; !ok || referenceLess(ref, previous) {
			byID[source.MaterialID] = ref
		}
	}
	all := make([]MaterialReference, 0, len(byID))
	for _, ref := range byID {
		all = append(all, ref)
	}
	sort.Slice(all, func(i, j int) bool { return referenceLess(all[i], all[j]) })
	out := LearningMaterials{Theory: []MaterialReference{}, Practice: []MaterialReference{}, Additional: []MaterialReference{}}
	for _, ref := range all {
		if ref.Kind != nil && *ref.Kind == "lecture" {
			out.Theory = append(out.Theory, ref)
		} else if ref.Kind != nil && (*ref.Kind == "lab" || *ref.Kind == "tutorial") {
			out.Practice = append(out.Practice, ref)
		} else {
			out.Additional = append(out.Additional, ref)
		}
	}
	return out
}

func referenceLess(a, b MaterialReference) bool {
	if (a.Week == nil) != (b.Week == nil) {
		return a.Week != nil
	}
	if a.Week != nil && b.Week != nil && *a.Week != *b.Week {
		return *a.Week < *b.Week
	}
	filename := func(v *string) string {
		if v == nil {
			return ""
		}
		return *v
	}
	if filename(a.Filename) != filename(b.Filename) {
		return filename(a.Filename) < filename(b.Filename)
	}
	return a.MaterialID < b.MaterialID
}
