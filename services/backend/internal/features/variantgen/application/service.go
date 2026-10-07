package application

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/variant"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

const AlgorithmVersion = "competency-map-v2"

var bloomRanks = map[string]int{"knowledge": 1, "understanding": 2, "application": 3, "analysis": 4}

type Service struct {
	repo    Repository
	chooser TaskChooser
	ids     IDGenerator
	now     func() time.Time
}

type taskPool struct {
	outcome    variant.OutcomeProfile
	tasks      []variant.TaskProfile
	order      int64
	rank       int
	importance int
}

func New(repo Repository, chooser TaskChooser, ids IDGenerator, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, chooser: chooser, ids: ids, now: now}
}

func (s *Service) Create(ctx context.Context, ownerID, key string) (variant.Variant, error) {
	if strings.TrimSpace(key) == "" || len(key) > 128 || strings.TrimSpace(key) != key || strings.IndexFunc(key, unicode.IsControl) >= 0 {
		return variant.Variant{}, fault.Validation("Idempotency-Key", "Укажите ключ длиной от 1 до 128 символов без пробелов по краям и управляющих символов.")
	}
	return s.repo.Create(ctx, ownerID, key, func(revision int64, candidates []variant.CandidateOutcome) (variant.Variant, error) {
		return s.build(ownerID, revision, candidates)
	})
}

func (s *Service) build(ownerID string, revision int64, candidates []variant.CandidateOutcome) (variant.Variant, error) {
	if len(candidates) == 0 {
		return variant.Variant{}, noEligible([]variant.SkippedCompetency{{Code: "EMPTY_MAP"}})
	}
	type compPool struct {
		profile  variant.CompetencyProfile
		order    int64
		outcomes map[string]*taskPool
	}
	competencies := map[string]*compPool{}
	for _, candidate := range candidates {
		p := candidate.Profile
		if p.Competency.ID == "" {
			continue
		}
		comp := competencies[p.Competency.ID]
		if comp == nil {
			comp = &compPool{profile: p.Competency, order: candidate.CompetencySourceOrder, outcomes: map[string]*taskPool{}}
			competencies[p.Competency.ID] = comp
		} else if candidate.CompetencySourceOrder < comp.order {
			comp.order = candidate.CompetencySourceOrder
		}
		if !candidate.HasTask || p.Outcome.ID == "" || p.Outcome.IncludeInTest == nil || !*p.Outcome.IncludeInTest || p.Outcome.TaxonomyCode == nil || p.Outcome.Importance == nil || *p.Outcome.Importance < 1 || *p.Outcome.Importance > 5 {
			continue
		}
		rank, ok := bloomRanks[*p.Outcome.TaxonomyCode]
		if !ok || !ready(p) {
			continue
		}
		outcome := comp.outcomes[p.Outcome.ID]
		if outcome == nil {
			outcome = &taskPool{outcome: p.Outcome, order: candidate.SourceOrder, rank: rank, importance: int(*p.Outcome.Importance), tasks: []variant.TaskProfile{}}
			comp.outcomes[p.Outcome.ID] = outcome
		} else if candidate.SourceOrder < outcome.order {
			outcome.order = candidate.SourceOrder
		}
		outcome.tasks = append(outcome.tasks, p)
	}
	ordered := make([]*compPool, 0, len(competencies))
	for _, comp := range competencies {
		ordered = append(ordered, comp)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].order != ordered[j].order {
			return ordered[i].order < ordered[j].order
		}
		if ordered[i].profile.Name != ordered[j].profile.Name {
			return ordered[i].profile.Name < ordered[j].profile.Name
		}
		return ordered[i].profile.ID < ordered[j].profile.ID
	})
	result := variant.Variant{OwnerID: ownerID, MapRevision: revision, AlgorithmVersion: AlgorithmVersion,
		SkippedCompetencies: []variant.SkippedCompetency{}, Competencies: []variant.CompetencySelection{}, CreatedAt: s.now().Unix()}
	for _, comp := range ordered {
		outcomes := make([]*taskPool, 0, len(comp.outcomes))
		for _, outcome := range comp.outcomes {
			outcomes = append(outcomes, outcome)
		}
		sort.Slice(outcomes, func(i, j int) bool {
			if outcomes[i].order != outcomes[j].order {
				return outcomes[i].order < outcomes[j].order
			}
			if outcomes[i].outcome.Name != outcomes[j].outcome.Name {
				return outcomes[i].outcome.Name < outcomes[j].outcome.Name
			}
			return outcomes[i].outcome.ID < outcomes[j].outcome.ID
		})
		if len(outcomes) == 0 {
			result.SkippedCompetencies = append(result.SkippedCompetencies, variant.SkippedCompetency{CompetencyID: comp.profile.ID, CompetencyName: comp.profile.Name, Code: "NO_READY_OUTCOMES", EligibleOutcomes: len(outcomes)})
			continue
		}
		main := outcomes[0]
		for _, outcome := range outcomes[1:] {
			if harder(outcome, main) {
				main = outcome
			}
		}
		basics := make([]*taskPool, 0, len(outcomes)-1)
		for _, outcome := range outcomes {
			if outcome.outcome.ID != main.outcome.ID && outcome.rank < main.rank {
				basics = append(basics, outcome)
			}
		}
		sort.Slice(basics, func(i, j int) bool {
			if basics[i].rank != basics[j].rank {
				return basics[i].rank < basics[j].rank
			}
			if basics[i].importance != basics[j].importance {
				return basics[i].importance > basics[j].importance
			}
			if basics[i].order != basics[j].order {
				return basics[i].order < basics[j].order
			}
			if basics[i].outcome.Name != basics[j].outcome.Name {
				return basics[i].outcome.Name < basics[j].outcome.Name
			}
			return basics[i].outcome.ID < basics[j].outcome.ID
		})
		if len(basics) > 2 {
			basics = basics[:2]
		}
		picks := append([]*taskPool{main}, basics...)
		selection := variant.CompetencySelection{Position: len(result.Competencies) + 1, Competency: comp.profile, Tasks: make([]variant.VariantTask, 0, 3)}
		for i, pick := range picks {
			idx := 0
			if len(pick.tasks) > 1 {
				var err error
				idx, err = s.chooser.Choose(len(pick.tasks))
				if err != nil {
					return variant.Variant{}, err
				}
				if idx < 0 || idx >= len(pick.tasks) {
					return variant.Variant{}, fmt.Errorf("task chooser returned invalid index %d", idx)
				}
			}
			taskID, err := s.ids.New("variant-task")
			if err != nil {
				return variant.Variant{}, err
			}
			role := "basic"
			if i == 0 {
				role = "main"
			}
			selection.Tasks = append(selection.Tasks, variant.VariantTask{ID: taskID, Role: role, Task: pick.tasks[idx]})
		}
		result.Competencies = append(result.Competencies, selection)
		result.TaskCount += len(selection.Tasks)
	}
	result.IncludedCompetencyCount = len(result.Competencies)
	if result.IncludedCompetencyCount == 0 {
		return variant.Variant{}, noEligible(result.SkippedCompetencies)
	}
	variantID, err := s.ids.New("variant")
	if err != nil {
		return variant.Variant{}, err
	}
	result.ID = variantID
	return result, nil
}

func ready(p variant.TaskProfile) bool {
	if strings.TrimSpace(p.Question) == "" || p.VoiceInstruction == nil || strings.TrimSpace(*p.VoiceInstruction) == "" || p.ReferenceAnswer == nil || strings.TrimSpace(*p.ReferenceAnswer) == "" {
		return false
	}
	return true
}

func noEligible(skipped []variant.SkippedCompetency) *fault.Error {
	e := fault.New(fault.Conflict, "NO_ELIGIBLE_COMPETENCIES", "Не удалось собрать вариант: нет компетенций, подходящих по карте.")
	for _, item := range skipped {
		if item.Code == "EMPTY_MAP" {
			e.Details = append(e.Details, fault.Detail{Code: "EMPTY_MAP", Message: "Активная карта компетенций пуста."})
			continue
		}
		message := fmt.Sprintf("Компетенция «%s»: подходящих ОР — %d", item.CompetencyName, item.EligibleOutcomes)
		if item.Code == "INSUFFICIENT_LOWER_BLOOM_OUTCOMES" {
			message += fmt.Sprintf("; ОР с Блумом ниже основного — %d (ранг основного %d)", item.LowerBloomOutcomes, item.MainBloomRank)
		}
		e.Details = append(e.Details, fault.Detail{Path: "competencies/" + item.CompetencyID, Code: item.Code, Message: message})
	}
	return e
}

func EncodeCursor(cursor Cursor) string {
	raw := strconv.FormatInt(cursor.CreatedAt, 10) + ":" + cursor.ID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}
func DecodeCursor(raw string) (*Cursor, error) {
	if raw == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, fault.Validation("cursor", "Некорректный курсор.")
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 || parts[1] == "" {
		return nil, fault.Validation("cursor", "Некорректный курсор.")
	}
	createdAt, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || createdAt < 0 {
		return nil, fault.Validation("cursor", "Некорректный курсор.")
	}
	return &Cursor{CreatedAt: createdAt, ID: parts[1]}, nil
}

func (s *Service) Get(ctx context.Context, ownerID, id string) (variant.Variant, error) {
	return s.repo.Get(ctx, ownerID, id)
}
func (s *Service) Task(ctx context.Context, ownerID, variantID, taskID string) (variant.VariantTask, error) {
	return s.repo.Task(ctx, ownerID, variantID, taskID)
}
func (s *Service) List(ctx context.Context, ownerID string, limit int, cursor string) ([]variant.Variant, string, error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		return nil, "", fault.Validation("limit", "Лимит должен быть от 1 до 100.")
	}
	decoded, err := DecodeCursor(cursor)
	if err != nil {
		return nil, "", err
	}
	items, next, err := s.repo.List(ctx, ownerID, limit, decoded)
	if err != nil {
		return nil, "", err
	}
	if next == nil {
		return items, "", nil
	}
	return items, EncodeCursor(*next), nil
}

func harder(a, b *taskPool) bool {
	pa, pb := a.rank+a.importance, b.rank+b.importance
	if pa != pb {
		return pa > pb
	}
	if a.importance != b.importance {
		return a.importance > b.importance
	}
	if a.order != b.order {
		return a.order < b.order
	}
	if a.outcome.Name != b.outcome.Name {
		return a.outcome.Name < b.outcome.Name
	}
	return a.outcome.ID < b.outcome.ID
}
