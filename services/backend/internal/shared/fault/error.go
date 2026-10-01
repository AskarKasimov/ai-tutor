// Package fault describes application failures without an HTTP dependency.
package fault

type Kind uint8

const (
	Invalid Kind = iota
	Unauthorized
	Forbidden
	Conflict
	TooLarge
	Unsupported
	Unavailable
	Upstream
	Timeout
	RateLimited
	NotFound
)

type Detail struct {
	Path    string `json:"path,omitempty" binding:"optional"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Error struct {
	Kind       Kind     `json:"-"`
	Code       string   `json:"code"`
	Message    string   `json:"message"`
	Details    []Detail `json:"details,omitempty" binding:"optional"`
	RetryAfter int64    `json:"-"`
}

func (e *Error) Error() string { return e.Code }
func New(kind Kind, code, message string) *Error {
	return &Error{Kind: kind, Code: code, Message: message}
}
func Validation(path, message string) *Error {
	e := New(Invalid, "VALIDATION_ERROR", "Некорректные поля запроса.")
	e.Details = []Detail{{Path: path, Code: "INVALID_FIELD", Message: message}}
	return e
}
