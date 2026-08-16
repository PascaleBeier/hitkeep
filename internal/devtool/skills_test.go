package devtool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillMetadataRejectsAdditionalFrontmatter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SKILL.md")
	body := "---\nname: example\ndescription: 'Example skill.'\nlicense: MIT\n---\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readSkillMetadata(path); err == nil {
		t.Fatal("unsupported skill metadata was accepted")
	}
}

func TestSkillMetadataRejectsUnclosedFrontmatter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SKILL.md")
	body := "---\nname: example\ndescription: 'Example skill.'\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readSkillMetadata(path); err == nil {
		t.Fatal("unclosed skill frontmatter was accepted")
	}
}

func TestRepositoryContributorSkillContracts(t *testing.T) {
	root := repositoryRoot(t)
	required := map[string][]string{
		"hitkeep-development": {"AGENTS.md", "callable `hk_*` tools", "explicit user approval", "registration, startup, workspace routing, or task reload", "./hk mcp manifest --output json", "configured fallback or explicit workspace selector", "hk_doctor", "hk_setup_start", "hk_dev_start", "hk_screenshot", "hk_build_start", "hk_smoke_start", "--detach --output json", "$hitkeep-workspace", "$hitkeep-qa"},
		"hitkeep-workspace":   {"AGENTS.md", "whenever they are callable", "explicit user approval", "hk_workspace_status", "hk_workspace_handoff", "hk_run_list", "hk_run_status", "hk_logs_tail", "next_cursor", "hk_dev_start", "hk_dev_stop", "hk_screenshot", "hk_run_cancel", "--output json"},
		"hitkeep-qa":          {"AGENTS.md", "whenever it is callable", "explicit user approval", "hk_qa_plan", "hk_qa_start", "hk_run_status", "hk_logs_tail", "hk_run_cancel", "--detach --output json"},
		"hitkeep-i18n":        {"AGENTS.md", "$hitkeep-qa", "frontend/dashboard/public/i18n", "every currently supported locale"},
	}
	triggerTerms := map[string][]string{
		"hitkeep-development": {"code", "UI", "database", "documentation", "setup", "local development", "builds"},
		"hitkeep-workspace":   {"worktrees", "ports", "concurrent agents", "handoff"},
		"hitkeep-qa":          {"quality gates", "complete", "CI parity", "failed"},
		"hitkeep-i18n":        {"user-visible", "Transloco", "language switching", "localized layout"},
	}
	for _, name := range contributorSkills {
		skillPath := filepath.Join(root, ".agents", "skills", name, "SKILL.md")
		assertSkillContract(t, skillPath, name, required[name], triggerTerms[name])
	}
}

func TestRepositoryProductAnalyticsSkillContracts(t *testing.T) {
	root := repositoryRoot(t)
	for _, name := range productAnalyticsSkills {
		skillPath := filepath.Join(root, "skills", name, "SKILL.md")
		assertSkillContract(t, skillPath, name, []string{"references/procedure.md", "production HitKeep MCP"}, nil)
		procedure, err := os.ReadFile(filepath.Join(root, "skills", name, "references", "procedure.md"))
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"MCP", "API client token", "./hk", "hk_doctor", "TranslocoPipe"} {
			if strings.Contains(string(procedure), forbidden) {
				t.Errorf("%s transport-neutral procedure contains %q", name, forbidden)
			}
		}
	}
}

func TestRepositorySkillLayout(t *testing.T) {
	root := repositoryRoot(t)
	if err := ValidateSkillLayout(root); err != nil {
		t.Fatal(err)
	}
	for path, expected := range map[string]int{
		filepath.Join(root, "skills"):            len(productAnalyticsSkills),
		filepath.Join(root, ".agents", "skills"): len(contributorSkills),
	} {
		entries, err := os.ReadDir(path)
		if err != nil {
			t.Fatal(err)
		}
		directories := 0
		for _, entry := range entries {
			if entry.IsDir() {
				directories++
			}
		}
		if directories != expected {
			t.Errorf("skill directory count under %s = %d, want %d", path, directories, expected)
		}
	}
}

func assertSkillContract(t *testing.T, skillPath, name string, required, triggerTerms []string) {
	t.Helper()
	raw, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	metadata, err := readSkillMetadata(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Name != name {
		t.Errorf("%s metadata name = %q", name, metadata.Name)
	}
	for _, value := range required {
		if !strings.Contains(body, value) {
			t.Errorf("%s is missing contract %q", name, value)
		}
	}
	for _, value := range triggerTerms {
		if !strings.Contains(metadata.Description, value) {
			t.Errorf("%s description is missing trigger %q", name, value)
		}
	}
	for _, forbidden := range []string{"hashicorpmetrics", "GOFLAGS=", "make dev-seed", "demo@example.com", "demo1234"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("%s contains mutable or sensitive fact %q", name, forbidden)
		}
	}

	metadataPath := filepath.Join(filepath.Dir(skillPath), "agents", "openai.yaml")
	openAI, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	short := yamlQuotedValue(string(openAI), "short_description")
	if len(short) < 25 || len(short) > 64 {
		t.Errorf("%s short_description length = %d", name, len(short))
	}
	defaultPrompt := yamlQuotedValue(string(openAI), "default_prompt")
	if !strings.Contains(defaultPrompt, "$"+name) {
		t.Errorf("%s default_prompt does not invoke the skill", name)
	}
	if name == "hitkeep-development" && !strings.Contains(defaultPrompt, "MCP") {
		t.Errorf("%s default_prompt does not reinforce MCP-first execution", name)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve test working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			break
		}
		root = parent
	}
	t.Fatal("resolve repository root")
	return ""
}

func yamlQuotedValue(raw, key string) string {
	prefix := "  " + key + ": \""
	for line := range strings.Lines(raw) {
		if after, ok := strings.CutPrefix(line, prefix); ok {
			return strings.TrimSuffix(after, "\"\n")
		}
	}
	return ""
}
