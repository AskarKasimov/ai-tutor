package gateway

// OpenAPIRequest is the source model for the HTTP request schema.
type OpenAPIRequest struct {
	Messages            []Message      `json:"messages" binding:"required"`
	Complexity          string         `json:"complexity" binding:"required" enums:"low,medium,high"`
	Urgent              *bool          `json:"urgent,omitempty"`
	Temperature         *float64       `json:"temperature,omitempty"`
	TopP                *float64       `json:"top_p,omitempty"`
	Stop                any            `json:"stop,omitempty"`
	PresencePenalty     *float64       `json:"presence_penalty,omitempty"`
	FrequencyPenalty    *float64       `json:"frequency_penalty,omitempty"`
	Seed                *int64         `json:"seed,omitempty"`
	MaxTokens           *int           `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int           `json:"max_completion_tokens,omitempty"`
	ResponseFormat      map[string]any `json:"response_format,omitempty"`
	ReasoningEffort     string         `json:"reasoning_effort,omitempty" enums:"low,medium,high"`
	N                   *int           `json:"n,omitempty"`
	Stream              *bool          `json:"stream,omitempty"`
}

type OpenAPIErrorBody struct {
	Message string `json:"message" binding:"required"`
	Type    string `json:"type" binding:"required"`
	Param   any    `json:"param" binding:"required"`
	Code    string `json:"code" binding:"required"`
}

type OpenAPIError struct {
	Error OpenAPIErrorBody `json:"error" binding:"required"`
}

type OpenAPICompletion struct {
	ID      string           `json:"id" binding:"required"`
	Object  string           `json:"object" binding:"required" enums:"chat.completion"`
	Created int64            `json:"created" binding:"required"`
	Model   string           `json:"model" binding:"required"`
	Choices []map[string]any `json:"choices" binding:"required"`
	Usage   map[string]any   `json:"usage,omitempty"`
}

// openAPICompletion documents POST /v1/chat/completions.
// @Summary Queue and execute a chat completion
// @Accept json
// @Produce json
// @Param Idempotency-Key header string false "Durable replay key" minlength(1) maxlength(255)
// @Param request body OpenAPIRequest true "Completion request"
// @Success 200 {object} OpenAPICompletion
// @Failure 400 {object} OpenAPIError
// @Failure 409 {object} OpenAPIError
// @Failure 413 {object} OpenAPIError
// @Failure 429 {object} OpenAPIError
// @Failure 502 {object} OpenAPIError
// @Failure 503 {object} OpenAPIError
// @Failure 504 {object} OpenAPIError
// @Router /v1/chat/completions [post]
func openAPICompletion() {}

// openAPILiveness documents GET /health/live.
// @Success 200
// @Router /health/live [get]
func openAPILiveness() {}

// openAPIReadiness documents GET /health/ready.
// @Success 200
// @Failure 503
// @Router /health/ready [get]
func openAPIReadiness() {}

// openAPIMetrics documents GET /metrics.
// @Produce plain
// @Success 200 {string} string
// @Router /metrics [get]
func openAPIMetrics() {}
