// Package assessment contains the structured result returned by the grading API.
package assessment

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

type CriterionResult struct {
	Key         string `json:"key"`
	Satisfied   bool   `json:"satisfied"`
	Explanation string `json:"explanation"`
}

type Evaluation struct {
	Score            int               `json:"score"`
	MaxScore         int               `json:"max_score"`
	Verdict          string            `json:"verdict"`
	CriterionResults []CriterionResult `json:"criterion_results"`
	Feedback         []string          `json:"feedback"`
}

// UnmarshalJSON keeps omitted or null contract fields distinct from valid zero values.
func (e *Evaluation) UnmarshalJSON(data []byte) error {
	var wire struct {
		Score            *int               `json:"score"`
		MaxScore         *int               `json:"max_score"`
		Verdict          *string            `json:"verdict"`
		CriterionResults *[]CriterionResult `json:"criterion_results"`
		Feedback         *[]string          `json:"feedback"`
	}
	if err := decodeStrict(data, &wire); err != nil {
		return err
	}
	if wire.Score == nil || wire.MaxScore == nil || wire.Verdict == nil || wire.CriterionResults == nil || wire.Feedback == nil {
		return errors.New("assessment response is missing required fields")
	}
	*e = Evaluation{
		Score: *wire.Score, MaxScore: *wire.MaxScore, Verdict: *wire.Verdict,
		CriterionResults: *wire.CriterionResults, Feedback: *wire.Feedback,
	}
	return nil
}

// UnmarshalJSON rejects missing/null satisfied while preserving a valid false value.
func (c *CriterionResult) UnmarshalJSON(data []byte) error {
	var wire struct {
		Key         *string `json:"key"`
		Satisfied   *bool   `json:"satisfied"`
		Explanation *string `json:"explanation"`
	}
	if err := decodeStrict(data, &wire); err != nil {
		return err
	}
	if wire.Key == nil || wire.Satisfied == nil || wire.Explanation == nil {
		return errors.New("criterion result is missing required fields")
	}
	*c = CriterionResult{Key: *wire.Key, Satisfied: *wire.Satisfied, Explanation: *wire.Explanation}
	return nil
}

func decodeStrict(data []byte, dst any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("unexpected trailing JSON")
	}
	return nil
}
