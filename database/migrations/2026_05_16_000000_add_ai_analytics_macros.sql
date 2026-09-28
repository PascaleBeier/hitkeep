-- AI assistant detection macros for browser hits and referrals.
CREATE OR REPLACE MACRO hk_ai_bot(ua) AS
    CASE
        WHEN ua IS NULL OR TRIM(ua) = '' THEN NULL
        WHEN ua ILIKE '%ChatGPT-User%' THEN 'ChatGPT-User'
        WHEN ua ILIKE '%GPTBot%' THEN 'GPTBot'
        WHEN ua ILIKE '%ClaudeBot%' THEN 'ClaudeBot'
        WHEN ua ILIKE '%Claude-Web%' THEN 'Claude-Web'
        WHEN ua ILIKE '%PerplexityBot%' THEN 'PerplexityBot'
        WHEN ua ILIKE '%Google-Extended%' THEN 'Google-Extended'
        WHEN ua ILIKE '%GoogleOther%' THEN 'GoogleOther'
        WHEN ua ILIKE '%Google-Safety%' THEN 'Google-Safety'
        WHEN ua ILIKE '%Applebot-Extended%' THEN 'Applebot-Extended'
        WHEN ua ILIKE '%Bytespider%' THEN 'Bytespider'
        WHEN ua ILIKE '%CCBot%' THEN 'CCBot'
        WHEN ua ILIKE '%Meta-ExternalAgent%' THEN 'Meta-ExternalAgent'
        WHEN ua ILIKE '%Meta-ExternalFetcher%' THEN 'Meta-ExternalFetcher'
        WHEN ua ILIKE '%Amazonbot%' THEN 'Amazonbot'
        WHEN ua ILIKE '%cohere-ai%' THEN 'Cohere'
        WHEN ua ILIKE '%YouBot%' THEN 'YouBot'
        WHEN ua ILIKE '%AI2Bot%' THEN 'AI2Bot'
        WHEN ua ILIKE '%Diffbot%' THEN 'Diffbot'
        WHEN ua ILIKE '%Timpibot%' THEN 'Timpibot'
        WHEN ua ILIKE '%ImagesiftBot%' THEN 'ImagesiftBot'
        WHEN ua ILIKE '%DeepSeekBot%' THEN 'DeepSeek'
        WHEN ua ILIKE '%PetalBot%' THEN 'PetalBot'
        ELSE NULL
    END;

CREATE OR REPLACE MACRO hk_ai_source(ref) AS
    CASE
        WHEN ref IS NULL OR TRIM(ref) = '' THEN NULL
        WHEN CASE
            WHEN lower(ref) LIKE 'http%' THEN regexp_replace(regexp_extract(lower(ref), 'https?://([^/:?#]+)', 1), '^www\\.', '')
            ELSE regexp_replace(lower(TRIM(ref)), '^www\\.', '')
        END IN ('chatgpt.com', 'www.chatgpt.com', 'chat.openai.com') THEN 'ChatGPT'
        WHEN CASE
            WHEN lower(ref) LIKE 'http%' THEN regexp_replace(regexp_extract(lower(ref), 'https?://([^/:?#]+)', 1), '^www\\.', '')
            ELSE regexp_replace(lower(TRIM(ref)), '^www\\.', '')
        END IN ('perplexity.ai', 'www.perplexity.ai') THEN 'Perplexity'
        WHEN CASE
            WHEN lower(ref) LIKE 'http%' THEN regexp_replace(regexp_extract(lower(ref), 'https?://([^/:?#]+)', 1), '^www\\.', '')
            ELSE regexp_replace(lower(TRIM(ref)), '^www\\.', '')
        END IN ('claude.ai', 'www.claude.ai') THEN 'Claude'
        WHEN CASE
            WHEN lower(ref) LIKE 'http%' THEN regexp_replace(regexp_extract(lower(ref), 'https?://([^/:?#]+)', 1), '^www\\.', '')
            ELSE regexp_replace(lower(TRIM(ref)), '^www\\.', '')
        END = 'gemini.google.com' THEN 'Gemini'
        WHEN CASE
            WHEN lower(ref) LIKE 'http%' THEN regexp_replace(regexp_extract(lower(ref), 'https?://([^/:?#]+)', 1), '^www\\.', '')
            ELSE regexp_replace(lower(TRIM(ref)), '^www\\.', '')
        END = 'copilot.microsoft.com' THEN 'Copilot'
        WHEN CASE
            WHEN lower(ref) LIKE 'http%' THEN regexp_replace(regexp_extract(lower(ref), 'https?://([^/:?#]+)', 1), '^www\\.', '')
            ELSE regexp_replace(lower(TRIM(ref)), '^www\\.', '')
        END IN ('you.com', 'www.you.com') THEN 'You.com'
        WHEN CASE
            WHEN lower(ref) LIKE 'http%' THEN regexp_replace(regexp_extract(lower(ref), 'https?://([^/:?#]+)', 1), '^www\\.', '')
            ELSE regexp_replace(lower(TRIM(ref)), '^www\\.', '')
        END IN ('phind.com', 'www.phind.com') THEN 'Phind'
        WHEN CASE
            WHEN lower(ref) LIKE 'http%' THEN regexp_replace(regexp_extract(lower(ref), 'https?://([^/:?#]+)', 1), '^www\\.', '')
            ELSE regexp_replace(lower(TRIM(ref)), '^www\\.', '')
        END IN ('kagi.com', 'www.kagi.com') THEN 'Kagi'
        WHEN CASE
            WHEN lower(ref) LIKE 'http%' THEN regexp_replace(regexp_extract(lower(ref), 'https?://([^/:?#]+)', 1), '^www\\.', '')
            ELSE regexp_replace(lower(TRIM(ref)), '^www\\.', '')
        END = 'chat.deepseek.com' THEN 'DeepSeek'
        WHEN CASE
            WHEN lower(ref) LIKE 'http%' THEN regexp_replace(regexp_extract(lower(ref), 'https?://([^/:?#]+)', 1), '^www\\.', '')
            ELSE regexp_replace(lower(TRIM(ref)), '^www\\.', '')
        END = 'chat.mistral.ai' THEN 'Mistral'
        WHEN CASE
            WHEN lower(ref) LIKE 'http%' THEN regexp_replace(regexp_extract(lower(ref), 'https?://([^/:?#]+)', 1), '^www\\.', '')
            ELSE regexp_replace(lower(TRIM(ref)), '^www\\.', '')
        END IN ('huggingface.co', 'www.huggingface.co') AND lower(ref) LIKE '%/chat%' THEN 'HuggingChat'
        WHEN CASE
            WHEN lower(ref) LIKE 'http%' THEN regexp_replace(regexp_extract(lower(ref), 'https?://([^/:?#]+)', 1), '^www\\.', '')
            ELSE regexp_replace(lower(TRIM(ref)), '^www\\.', '')
        END IN ('poe.com', 'www.poe.com') THEN 'Poe'
        WHEN CASE
            WHEN lower(ref) LIKE 'http%' THEN regexp_replace(regexp_extract(lower(ref), 'https?://([^/:?#]+)', 1), '^www\\.', '')
            ELSE regexp_replace(lower(TRIM(ref)), '^www\\.', '')
        END IN ('arc.net', 'www.arc.net') THEN 'Arc Search'
        ELSE NULL
    END;
