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

func isOpenAICompatibleProvider(provider string) bool {
	switch normalizeProvider(provider) {
	case "openai-compatible", "compat", "gateway", "bifrost", "litellm":
		return true
	default:
		return false
	}
}
