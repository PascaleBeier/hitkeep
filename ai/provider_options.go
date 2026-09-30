package ai

import goaisdk "github.com/zendev-sh/goai"

func mantleStructuredOutputOptions(conf Config) []goaisdk.Option {
	if !isOpenAICompatibleProvider(conf.Provider) || !isBedrockMantleBaseURL(conf.BaseURL) {
		return nil
	}
	return []goaisdk.Option{goaisdk.WithProviderOptions(map[string]any{
		"strictJsonSchema": true,
	})}
}

func mantleAskAIToolOptions(conf Config, tools []goaisdk.Tool) []goaisdk.Option {
	if len(tools) == 0 || !isOpenAICompatibleProvider(conf.Provider) || !isBedrockMantleBaseURL(conf.BaseURL) {
		return nil
	}
	return []goaisdk.Option{goaisdk.WithToolChoice(goaisdk.ToolChoiceRequired)}
}

// promptCachingOptions caches the system prompt and tool definitions across
// tool-loop steps. Only providers with explicit cache markers get the option;
// OpenAI-style providers cache stable prefixes on their own, and others warn.
func promptCachingOptions(conf Config) []goaisdk.Option {
	switch normalizeProvider(conf.Provider) {
	case "anthropic", "bedrock":
		return []goaisdk.Option{goaisdk.WithPromptCaching(true)}
	default:
		return nil
	}
}

func isOpenAICompatibleProvider(provider string) bool {
	switch normalizeProvider(provider) {
	case "openai-compatible", "compat", "gateway", "bifrost", "litellm":
		return true
	default:
		return false
	}
}
