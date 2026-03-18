package llm

// Role defines the AI provider's assigned role in the system.
type Role string

const (
	RoleLogAnalyzer Role = "log_analyzer" // OpenAI GPT
	RolePlanner     Role = "planner"      // Anthropic Claude
	RoleSummarizer  Role = "summarizer"   // Google Gemini
)

// CompletionRequest is the input to any AI provider.
type CompletionRequest struct {
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
	Temperature  float64
}

// CompletionResponse is the output from any AI provider.
type CompletionResponse struct {
	Content      string
	FinishReason string
	TokensUsed   int
	Model        string
}

// Note: Provider interfaces (AIProvider, LogAnalyzer, Planner, Summarizer)
// are owned by the app layer (see internal/app/agent/ports.go).
// This package only defines value objects.
