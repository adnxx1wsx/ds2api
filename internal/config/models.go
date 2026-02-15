package config

type ModelInfo struct {
	ID         string `json:"id"`
	Object     string `json:"object"`
	Created    int64  `json:"created"`
	OwnedBy    string `json:"owned_by"`
	Permission []any  `json:"permission,omitempty"`
}

var DeepSeekModels = []ModelInfo{
	{ID: "deepseek-chat", Object: "model", Created: 1677610602, OwnedBy: "deepseek", Permission: []any{}},
	{ID: "deepseek-reasoner", Object: "model", Created: 1677610602, OwnedBy: "deepseek", Permission: []any{}},
	{ID: "deepseek-chat-search", Object: "model", Created: 1677610602, OwnedBy: "deepseek", Permission: []any{}},
	{ID: "deepseek-reasoner-search", Object: "model", Created: 1677610602, OwnedBy: "deepseek", Permission: []any{}},
}

var ClaudeModels = []ModelInfo{
	{ID: "claude-sonnet-4-20250514", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},
	{ID: "claude-sonnet-4-20250514-fast", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},
	{ID: "claude-sonnet-4-20250514-slow", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},
}

func GetModelConfig(model string) (thinking bool, search bool, ok bool) {
	switch lower(model) {
	case "deepseek-chat":
		return false, false, true
	case "deepseek-reasoner":
		return true, false, true
	case "deepseek-chat-search":
		return false, true, true
	case "deepseek-reasoner-search":
		return true, true, true
	default:
		return false, false, false
	}
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func OpenAIModelsResponse() map[string]any {
	return map[string]any{"object": "list", "data": DeepSeekModels}
}

func ClaudeModelsResponse() map[string]any {
	return map[string]any{"object": "list", "data": ClaudeModels}
}
