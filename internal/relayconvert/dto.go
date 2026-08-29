// Package relayconvert ports new-api's relaykit protocol conversions
// (OpenAI Chat Completions ⇄ Anthropic Messages) so downstream formats can be
// bridged. Conversions are pure (no IO) and unit-tested.
package relayconvert

// OpenAI message/request/response types (subset of the OpenAI Chat Completions
// contract, matching new-api's GeneralOpenAIRequest / Message fields).
type OpenAIMessage struct {
	Role       string           `json:"role"`
	Content    any              `json:"content"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	Name       string           `json:"name,omitempty"`
}

type OpenAIToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function OpenAIFunction `json:"function"`
}

type OpenAIFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type OpenAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	Stream      bool            `json:"stream,omitempty"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	TopP        *float64        `json:"top_p,omitempty"`
	Stop        any             `json:"stop,omitempty"`
	Tools       []OpenAITool    `json:"tools,omitempty"`
	ToolChoice  any             `json:"tool_choice,omitempty"`
}

type OpenAITool struct {
	Type     string       `json:"type"`
	Function OpenAIToolFn `json:"function"`
}

type OpenAIToolFn struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

// Claude (Anthropic Messages API) types.
type ClaudeContentBlock struct {
	Type    string             `json:"type"`
	Text    string             `json:"text,omitempty"`
	Content any                `json:"content,omitempty"`
	ID      string             `json:"id,omitempty"`
	Name    string             `json:"name,omitempty"`
	Input   any                `json:"input,omitempty"`
	Source  *ClaudeImageSource `json:"source,omitempty"`
}

type ClaudeImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type ClaudeMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type ClaudeRequest struct {
	Model         string          `json:"model"`
	MaxTokens     int             `json:"max_tokens"`
	Messages      []ClaudeMessage `json:"messages"`
	System        string          `json:"system,omitempty"`
	Stream        bool            `json:"stream,omitempty"`
	Temperature   *float64        `json:"temperature,omitempty"`
	TopP          *float64        `json:"top_p,omitempty"`
	StopSequences []string        `json:"stop_sequences,omitempty"`
	Tools         []ClaudeTool    `json:"tools,omitempty"`
}

type ClaudeTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema,omitempty"`
}

type ClaudeResponse struct {
	ID         string               `json:"id"`
	Type       string               `json:"type"`
	Role       string               `json:"role"`
	Model      string               `json:"model"`
	Content    []ClaudeContentBlock `json:"content"`
	StopReason string               `json:"stop_reason"`
	Usage      ClaudeUsage          `json:"usage"`
}

type ClaudeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// OpenAIChatResponse is the normalized OpenAI chat completion response.
type OpenAIChatResponse struct {
	ID      string      `json:"id"`
	Object  string      `json:"object"`
	Created int64       `json:"created"`
	Model   string      `json:"model"`
	Choices []OAIChoice `json:"choices"`
	Usage   OAIUsage    `json:"usage"`
}

type OAIChoice struct {
	Index        int                `json:"index"`
	Message      OAIResponseMessage `json:"message"`
	FinishReason string             `json:"finish_reason"`
}

type OAIResponseMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []OpenAIToolCall `json:"tool_calls,omitempty"`
}

type OAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
