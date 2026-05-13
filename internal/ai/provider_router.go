package ai

import (
	"strings"

	"github.com/zendev-sh/goai/provider"
	"github.com/zendev-sh/goai/provider/anthropic"
	"github.com/zendev-sh/goai/provider/bedrock"
	"github.com/zendev-sh/goai/provider/cerebras"
	"github.com/zendev-sh/goai/provider/cohere"
	"github.com/zendev-sh/goai/provider/compat"
	"github.com/zendev-sh/goai/provider/deepseek"
	"github.com/zendev-sh/goai/provider/fireworks"
	"github.com/zendev-sh/goai/provider/google"
	"github.com/zendev-sh/goai/provider/groq"
	"github.com/zendev-sh/goai/provider/mistral"
	"github.com/zendev-sh/goai/provider/ollama"
	"github.com/zendev-sh/goai/provider/openai"
	"github.com/zendev-sh/goai/provider/openrouter"
	"github.com/zendev-sh/goai/provider/perplexity"
	"github.com/zendev-sh/goai/provider/together"
	"github.com/zendev-sh/goai/provider/xai"
)

type modelBuilder func(Config) provider.LanguageModel

var modelBuilders = map[string]modelBuilder{
	"openai":            openAIModel,
	"openai-compatible": compatModel,
	"compat":            compatModel,
	"gateway":           compatModel,
	"bifrost":           compatModel,
	"litellm":           compatModel,
	"bedrock":           bedrockModel,
	"anthropic":         anthropicModel,
	"google":            googleModel,
	"gemini":            googleModel,
	"mistral":           mistralModel,
	"ollama":            ollamaModel,
	"openrouter":        openRouterModel,
	"deepseek":          deepSeekModel,
	"groq":              groqModel,
	"xai":               xaiModel,
	"cerebras":          cerebrasModel,
	"cohere":            cohereModel,
	"perplexity":        perplexityModel,
	"together":          togetherModel,
	"fireworks":         fireworksModel,
}

var providerRequiresAPIKey = map[string]bool{
	"openai":            true,
	"openai-compatible": true,
	"compat":            true,
	"gateway":           true,
	"bifrost":           true,
	"litellm":           true,
	"anthropic":         true,
	"google":            true,
	"gemini":            true,
	"mistral":           true,
	"openrouter":        true,
	"deepseek":          true,
	"groq":              true,
	"xai":               true,
	"cerebras":          true,
	"cohere":            true,
	"perplexity":        true,
	"together":          true,
	"fireworks":         true,
}

var providerRequiresBaseURL = map[string]bool{
	"openai-compatible": true,
	"compat":            true,
	"gateway":           true,
	"bifrost":           true,
	"litellm":           true,
}

func buildModel(conf Config) (provider.LanguageModel, error) {
	if err := ValidateConfig(conf); err != nil {
		return nil, err
	}
	builder := modelBuilders[normalizeProvider(conf.Provider)]
	return builder(conf), nil
}

func openAIModel(conf Config) provider.LanguageModel {
	opts := []openai.Option{}
	if conf.APIKey != "" {
		opts = append(opts, openai.WithAPIKey(conf.APIKey))
	}
	if conf.BaseURL != "" {
		opts = append(opts, openai.WithBaseURL(conf.BaseURL))
	}
	return openai.Chat(conf.Model, opts...)
}

func compatModel(conf Config) provider.LanguageModel {
	opts := []compat.Option{compat.WithProviderID(normalizeProvider(conf.Provider))}
	if conf.APIKey != "" {
		opts = append(opts, compat.WithAPIKey(conf.APIKey))
	}
	if conf.BaseURL != "" {
		opts = append(opts, compat.WithBaseURL(conf.BaseURL))
	}
	return compat.Chat(conf.Model, opts...)
}

func bedrockModel(conf Config) provider.LanguageModel {
	opts := []bedrock.Option{}
	if conf.Region != "" {
		opts = append(opts, bedrock.WithRegion(conf.Region))
	}
	if conf.APIKey != "" {
		opts = append(opts, bedrock.WithBearerToken(conf.APIKey))
	}
	if conf.BaseURL != "" {
		opts = append(opts, bedrock.WithBaseURL(conf.BaseURL))
	}
	return bedrock.Chat(conf.Model, opts...)
}

func anthropicModel(conf Config) provider.LanguageModel {
	opts := []anthropic.Option{}
	if conf.APIKey != "" {
		opts = append(opts, anthropic.WithAPIKey(conf.APIKey))
	}
	if conf.BaseURL != "" {
		opts = append(opts, anthropic.WithBaseURL(conf.BaseURL))
	}
	return anthropic.Chat(conf.Model, opts...)
}

func googleModel(conf Config) provider.LanguageModel {
	opts := []google.Option{}
	if conf.APIKey != "" {
		opts = append(opts, google.WithAPIKey(conf.APIKey))
	}
	if conf.BaseURL != "" {
		opts = append(opts, google.WithBaseURL(conf.BaseURL))
	}
	return google.Chat(conf.Model, opts...)
}

func mistralModel(conf Config) provider.LanguageModel {
	opts := []mistral.Option{}
	if conf.APIKey != "" {
		opts = append(opts, mistral.WithAPIKey(conf.APIKey))
	}
	if conf.BaseURL != "" {
		opts = append(opts, mistral.WithBaseURL(conf.BaseURL))
	}
	return mistral.Chat(conf.Model, opts...)
}

func ollamaModel(conf Config) provider.LanguageModel {
	opts := []ollama.Option{}
	if conf.BaseURL != "" {
		opts = append(opts, ollama.WithBaseURL(conf.BaseURL))
	}
	return ollama.Chat(conf.Model, opts...)
}

func openRouterModel(conf Config) provider.LanguageModel {
	opts := []openrouter.Option{}
	if conf.APIKey != "" {
		opts = append(opts, openrouter.WithAPIKey(conf.APIKey))
	}
	if conf.BaseURL != "" {
		opts = append(opts, openrouter.WithBaseURL(conf.BaseURL))
	}
	return openrouter.Chat(conf.Model, opts...)
}

func deepSeekModel(conf Config) provider.LanguageModel {
	opts := []deepseek.Option{}
	if conf.APIKey != "" {
		opts = append(opts, deepseek.WithAPIKey(conf.APIKey))
	}
	if conf.BaseURL != "" {
		opts = append(opts, deepseek.WithBaseURL(conf.BaseURL))
	}
	return deepseek.Chat(conf.Model, opts...)
}

func groqModel(conf Config) provider.LanguageModel {
	opts := []groq.Option{}
	if conf.APIKey != "" {
		opts = append(opts, groq.WithAPIKey(conf.APIKey))
	}
	if conf.BaseURL != "" {
		opts = append(opts, groq.WithBaseURL(conf.BaseURL))
	}
	return groq.Chat(conf.Model, opts...)
}

func xaiModel(conf Config) provider.LanguageModel {
	opts := []xai.Option{}
	if conf.APIKey != "" {
		opts = append(opts, xai.WithAPIKey(conf.APIKey))
	}
	if conf.BaseURL != "" {
		opts = append(opts, xai.WithBaseURL(conf.BaseURL))
	}
	return xai.Chat(conf.Model, opts...)
}

func cerebrasModel(conf Config) provider.LanguageModel {
	opts := []cerebras.Option{}
	if conf.APIKey != "" {
		opts = append(opts, cerebras.WithAPIKey(conf.APIKey))
	}
	if conf.BaseURL != "" {
		opts = append(opts, cerebras.WithBaseURL(conf.BaseURL))
	}
	return cerebras.Chat(conf.Model, opts...)
}

func cohereModel(conf Config) provider.LanguageModel {
	opts := []cohere.Option{}
	if conf.APIKey != "" {
		opts = append(opts, cohere.WithAPIKey(conf.APIKey))
	}
	if conf.BaseURL != "" {
		opts = append(opts, cohere.WithBaseURL(conf.BaseURL))
	}
	return cohere.Chat(conf.Model, opts...)
}

func perplexityModel(conf Config) provider.LanguageModel {
	opts := []perplexity.Option{}
	if conf.APIKey != "" {
		opts = append(opts, perplexity.WithAPIKey(conf.APIKey))
	}
	if conf.BaseURL != "" {
		opts = append(opts, perplexity.WithBaseURL(conf.BaseURL))
	}
	return perplexity.Chat(conf.Model, opts...)
}

func togetherModel(conf Config) provider.LanguageModel {
	opts := []together.Option{}
	if conf.APIKey != "" {
		opts = append(opts, together.WithAPIKey(conf.APIKey))
	}
	if conf.BaseURL != "" {
		opts = append(opts, together.WithBaseURL(conf.BaseURL))
	}
	return together.Chat(conf.Model, opts...)
}

func fireworksModel(conf Config) provider.LanguageModel {
	opts := []fireworks.Option{}
	if conf.APIKey != "" {
		opts = append(opts, fireworks.WithAPIKey(conf.APIKey))
	}
	if conf.BaseURL != "" {
		opts = append(opts, fireworks.WithBaseURL(conf.BaseURL))
	}
	return fireworks.Chat(conf.Model, opts...)
}

func normalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}
