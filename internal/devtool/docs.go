package devtool

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func ValidateDevelopmentDocs(root string) error {
	files := map[string][]string{
		"README.md":       {"./hk setup", "./hk dev --seed", "./hk qa pr", "CONTRIBUTING.md"},
		"CONTRIBUTING.md": {"./hk help", "./hk catalog --output json", "model-agnostic", "project-scoped MCP", "approve or trust the repository", "./hk mcp manifest", "./hk mcp serve", "./hk skills check", "./hk fmt", "./hk fix check", "./hk cache status", "./hk run list", "next_cursor", "AGENTS.md", "source repository is private"},
		"AGENTS.md":       {"Use `./hk` as the workflow source of truth", "$hitkeep-development", "$hitkeep-workspace", "$hitkeep-qa", "private `PascaleBeier/hitkeep-docs`"},
		".agents/skills/hitkeep-development/references/delivery.md": {
			"private source for the public documentation website",
			"separate from HitKeep's public MIT-licensed source",
		},
		"skills/README.md": {"Official HitKeep Analytics Skills", "transport-neutral procedure", "hitkeep-traffic-diagnosis", "Ask AI"},
	}
	for name, required := range files {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return err
		}
		for _, value := range required {
			if !bytes.Contains(raw, []byte(value)) {
				return fmt.Errorf("%s is missing canonical development reference %q", name, value)
			}
		}
	}
	if err := validateAgentInstructionDrift(root); err != nil {
		return err
	}
	if err := validateProjectMCPConfigurations(root); err != nil {
		return err
	}
	if err := validateCIWorkflowContract(root); err != nil {
		return err
	}
	makefile, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		return err
	}
	if lines := strings.Count(string(makefile), "\n"); lines > 100 {
		return fmt.Errorf("make compatibility adapter grew to %d lines", lines)
	}
	return ValidateSkillLayout(root)
}

func validateCIWorkflowContract(root string) error {
	groups := map[string][]string{}
	for _, gate := range gates {
		if !slices.Contains(gate.Profiles, "pr") {
			continue
		}
		if gate.CIGroup == "" {
			return fmt.Errorf("PR QA gate %s has no canonical CI group", gate.ID)
		}
		groups[gate.CIGroup] = append(groups[gate.CIGroup], gate.ID)
	}
	workflowEntries, err := os.ReadDir(filepath.Join(root, ".github", "workflows"))
	if err != nil {
		return err
	}
	var workflows []byte
	for _, entry := range workflowEntries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".yml") && !strings.HasSuffix(entry.Name(), ".yaml")) {
			continue
		}
		raw, readErr := os.ReadFile(filepath.Join(root, ".github", "workflows", entry.Name()))
		if readErr != nil {
			return readErr
		}
		workflows = append(workflows, raw...)
		workflows = append(workflows, '\n')
	}
	for group, gateIDs := range groups {
		if !bytes.Contains(workflows, []byte("--group "+group)) {
			return fmt.Errorf("canonical CI group %s for gates %s is not referenced by a workflow", group, strings.Join(gateIDs, ", "))
		}
	}
	return nil
}

func validateProjectMCPConfigurations(root string) error {
	for _, spec := range mcpProjectClientSpecs {
		path := filepath.Join(root, filepath.FromSlash(spec.ConfigPath))
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, required := range spec.RequiredFragments {
			if !bytes.Contains(raw, []byte(required)) {
				return fmt.Errorf("%s is missing worktree-confined MCP setting %q", spec.ConfigPath, required)
			}
		}
		for _, forbidden := range []string{"/Users/", "/home/", "bearer", "token", "secret"} {
			if bytes.Contains(bytes.ToLower(raw), []byte(strings.ToLower(forbidden))) {
				return fmt.Errorf("%s contains non-portable or secret-bearing value %q", spec.ConfigPath, forbidden)
			}
		}
	}
	return nil
}

func validateAgentInstructionDrift(root string) error {
	paths := make([]string, 0, 1+len(contributorSkills)+4)
	paths = append(paths, filepath.Join(root, "AGENTS.md"))
	for _, skillName := range contributorSkills {
		paths = append(paths, filepath.Join(root, ".agents", "skills", skillName, "SKILL.md"))
	}
	for _, reference := range []string{"backend.md", "frontend.md", "product.md", "delivery.md"} {
		paths = append(paths, filepath.Join(root, ".agents", "skills", "hitkeep-development", "references", reference))
	}
	staleFragments := []string{
		"GOFLAGS=",
		"scripts/go-build-tags.sh",
		"make dev-seed",
		"make dev-cloud",
		"make build-docker",
		"go test ./",
		"go test -race",
		"golangci-lint run",
		"cd frontend/dashboard && npm run",
		"npm run fmt:check",
		"npm run test:ci",
		"npm run e2e",
		"gofmt -",
		"go fix -",
	}
	for _, path := range paths {
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, stale := range staleFragments {
			if bytes.Contains(raw, []byte(stale)) {
				relative, _ := filepath.Rel(root, path)
				return fmt.Errorf("%s duplicates mutable hk workflow fact %q", relative, stale)
			}
		}
	}
	return nil
}
