package application

import (
	"encoding/json"
	"testing"
)

func TestTopicQuestionsAreThePreparedQuestionsOfTheGraph(t *testing.T) {
	graph := json.RawMessage(`{"topics":[{"question":" Почему ACO находит короткий путь? "},{"question":""},{"title":"без вопроса"}]}`)
	if got := TopicQuestions(graph); len(got) != 1 || got[0] != "Почему ACO находит короткий путь?" {
		t.Fatalf("questions = %q", got)
	}
	if got := TopicQuestions(json.RawMessage(`not json`)); len(got) != 0 {
		t.Fatalf("questions of an invalid graph = %q", got)
	}
}
