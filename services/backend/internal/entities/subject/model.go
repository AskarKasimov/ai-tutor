package subject

const IntroToMLID = "subject:intro-to-ml"

type Subject struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Ready bool   `json:"ready"`
}
