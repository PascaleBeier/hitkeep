package devtool

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

var productAnalyticsSkills = []string{
	"hitkeep-ai-visibility-analyst",
	"hitkeep-analytics",
	"hitkeep-ecommerce-analyst",
	"hitkeep-tracking-verifier",
	"hitkeep-traffic-diagnosis",
}

var contributorSkills = []string{"hitkeep-development", "hitkeep-i18n", "hitkeep-qa", "hitkeep-workspace"}

// ValidateSkillLayout enforces two canonical, non-overlapping skill packs:
// product analytics under skills and contributor workflows under .agents/skills.
func ValidateSkillLayout(root string) error {
	for _, name := range productAnalyticsSkills {
		if containsSkill(contributorSkills, name) {
			return fmt.Errorf("skill %q appears in both product and contributor catalogs", name)
		}
	}
	if err := validateSkillRoot(filepath.Join(root, "skills"), productAnalyticsSkills, true, false); err != nil {
		return fmt.Errorf("product analytics skill pack: %w", err)
	}
	if err := validateSkillRoot(filepath.Join(root, ".agents", "skills"), contributorSkills, false, true); err != nil {
		return fmt.Errorf("contributor skill pack: %w", err)
	}
	ignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		return err
	}
	for line := range strings.Lines(string(ignore)) {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ".agents") && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "!") {
			return fmt.Errorf(".gitignore must not hide canonical contributor skills under .agents")
		}
	}
	return nil
}

func validateSkillRoot(root string, expected []string, requireProcedure, allowAdditional bool) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if !allowAdditional && !containsSkill(expected, entry.Name()) {
			return fmt.Errorf("unexpected skill entry %q", entry.Name())
		}
	}
	for _, name := range expected {
		skillPath := filepath.Join(root, name, "SKILL.md")
		metadata, readErr := readSkillMetadata(skillPath)
		if readErr != nil {
			return readErr
		}
		if metadata.Name != name {
			return fmt.Errorf("skill directory %q declares name %q", name, metadata.Name)
		}
		if _, statErr := os.Stat(filepath.Join(root, name, "agents", "openai.yaml")); statErr != nil {
			return fmt.Errorf("skill %q is missing agents/openai.yaml: %w", name, statErr)
		}
		if requireProcedure {
			procedurePath := filepath.Join(root, name, "references", "procedure.md")
			procedure, readProcedureErr := os.ReadFile(procedurePath)
			if readProcedureErr != nil {
				return fmt.Errorf("skill %q is missing references/procedure.md: %w", name, readProcedureErr)
			}
			if !strings.Contains(string(procedure), "procedure") {
				return fmt.Errorf("skill %q has an invalid transport-neutral procedure", name)
			}
			skillBody, readSkillErr := os.ReadFile(skillPath)
			if readSkillErr != nil {
				return readSkillErr
			}
			if !strings.Contains(string(skillBody), "references/procedure.md") {
				return fmt.Errorf("skill %q does not route to references/procedure.md", name)
			}
		}
	}
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Name() != "SKILL.md" {
			return nil
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) != 2 || parts[1] != "SKILL.md" || (!allowAdditional && !containsSkill(expected, parts[0])) {
			return fmt.Errorf("nested or unexpected skill body: %s", relative)
		}
		return nil
	})
}

func containsSkill(skills []string, target string) bool {
	return slices.Contains(skills, target)
}

type skillMetadata struct {
	Name        string
	Description string
}

func readSkillMetadata(path string) (skillMetadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return skillMetadata{}, err
	}
	defer file.Close()
	metadata := skillMetadata{}
	frontmatterClosed := false
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() || scanner.Text() != "---" {
		return metadata, fmt.Errorf("skill is missing frontmatter: %s", path)
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			frontmatterClosed = true
			break
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), "'\"")
		switch key {
		case "name":
			metadata.Name = value
		case "description":
			metadata.Description = value
		default:
			return metadata, fmt.Errorf("skill frontmatter contains unsupported field %q: %s", key, path)
		}
	}
	if err := scanner.Err(); err != nil {
		return metadata, err
	}
	if !frontmatterClosed {
		return metadata, fmt.Errorf("skill frontmatter is not closed: %s", path)
	}
	if metadata.Name == "" || metadata.Description == "" {
		return metadata, fmt.Errorf("skill metadata is incomplete: %s", path)
	}
	return metadata, nil
}
