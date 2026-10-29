package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
)

func TestConversationDoesNotSpendAttemptOrHintOrChangeScores(t *testing.T) {
	for _, initial := range []string{"", "частично", "слабо"} {
		f := newFixture(t)
		s, _, err := f.service.Start(context.Background(), "student", "subject", "start")
		if err != nil {
			t.Fatal(err)
		}
		if initial != "" {
			s = f.answer(t, s, Answer{Text: initial})
		}
		nodeID := s.Exam.Current.NodeID
		before, _ := json.Marshal(s.Exam)
		feedbacks, questions := len(f.examiner.feedbacks), len(f.examiner.questions)
		for _, text := range []string{"поправка обращения: женский род", "поясни формулировку", "пауза"} {
			previous := s.Turns[len(s.Turns)-1].ID
			s = f.answer(t, s, Answer{Text: text})
			after, _ := json.Marshal(s.Exam)
			if string(after) != string(before) {
				t.Fatalf("conversation changed exam: %s", text)
			}
			last := s.Turns[len(s.Turns)-1]
			if !last.Meta || !s.Turns[len(s.Turns)-2].Meta || last.ID == previous {
				t.Fatalf("invalid conversational transcript: %+v", last)
			}
			if s.AddressPreference != exam.AddressFeminine {
				t.Fatal("address preference lost on persistence")
			}
		}
		if len(f.examiner.feedbacks) != feedbacks || len(f.examiner.questions) != questions {
			t.Fatal("conversation made extra model calls")
		}
		s = f.answer(t, s, Answer{Text: "сильно"})
		want := exam.GradeA
		if initial == "слабо" {
			want = exam.GradeB
		}
		if state := s.Exam.Nodes[nodeID]; state.Grade == nil || *state.Grade != want {
			t.Fatalf("answer after conversation: %+v", state)
		}
		if f.examiner.feedbacks[len(f.examiner.feedbacks)-1].AddressPreference != exam.AddressFeminine {
			t.Fatal("feedback did not receive address preference")
		}
	}
}

func TestMixedAnswerAndAddressCorrectionStillGradesTheAnswer(t *testing.T) {
	f := newFixture(t)
	s, _, _ := f.service.Start(context.Background(), "student", "subject", "start")
	id := s.Exam.Current.NodeID
	s = f.answer(t, s, Answer{Text: "сильно; женский род"})
	if s.AddressPreference != exam.AddressFeminine || *s.Exam.Nodes[id].Grade != exam.GradeA {
		t.Fatal("mixed answer lost either knowledge or preference")
	}
}

func TestReplySpeechPartsPreserveTextAndBoundLongClauses(t *testing.T) {
	for _, text := range []string{"", "Одна мысль.", "Первая мысль. Вторая мысль! Третья?", "Вероятность 0.5, т. е. половина. Дальше — вывод.", "Он сказал «да». Затем объяснил почему.", strings.Repeat("Здесь длинная мысль, ", 20) + "и вывод."} {
		parts := ReplySpeechParts(text)
		if strings.Join(strings.Fields(strings.Join(parts, " ")), " ") != strings.Join(strings.Fields(text), " ") {
			t.Fatalf("text changed: %q -> %q", text, parts)
		}
		for _, p := range parts {
			if len([]rune(p)) > 180 || strings.TrimSpace(p) == "" {
				t.Fatalf("invalid part %q", p)
			}
		}
	}
	if got := ReplySpeechParts("Вероятность 0.5, т. е. половина. Дальше — вывод."); len(got) != 2 {
		t.Fatalf("abbreviation/decimal split: %q", got)
	}
}

func TestChunkAudioAndIntroUseTextCacheAndValidateBounds(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	s, _, _ := f.service.Start(ctx, "student", "subject", "start")
	s = f.answer(t, s, Answer{Text: "сильно"})
	last := &s.Turns[len(s.Turns)-1]
	last.Reply = "Первое объяснение. Второе объяснение."
	last.Text = last.Reply + "\n\nСледующий вопрос?"
	last.AudioIntro = feedbackLeadIn
	data, _ := json.Marshal(s)
	f.store.sessions[s.ID] = data
	for _, part := range []struct {
		kind  AudioPart
		chunk []int
		want  string
	}{{AudioIntro, nil, feedbackLeadIn}, {AudioReply, []int{0}, "Первое объяснение."}, {AudioReply, []int{1}, "Второе объяснение."}} {
		if _, err := f.service.TurnAudio(ctx, "student", s.ID, last.ID, part.kind, part.chunk...); err != nil {
			t.Fatal(err)
		}
		if f.voice.calls[len(f.voice.calls)-1] != part.want {
			t.Fatalf("wrong speech: %q", f.voice.calls)
		}
	}
	calls := len(f.voice.calls)
	f.service.TurnAudio(ctx, "student", s.ID, last.ID, AudioReply, 0)
	if len(f.voice.calls) != calls {
		t.Fatal("chunk cache missed")
	}
	for _, c := range []int{-1, 2} {
		if _, err := f.service.TurnAudio(ctx, "student", s.ID, last.ID, AudioReply, c); err == nil {
			t.Fatal("bad chunk accepted")
		}
	}
	if _, err := f.service.TurnAudio(ctx, "student", s.ID, last.ID, AudioQuestion, 0); err == nil {
		t.Fatal("chunk accepted for question")
	}
}
