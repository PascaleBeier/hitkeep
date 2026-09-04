package devtool

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	runtimeconfig "hitkeep/config"
	json "hitkeep/jsonapi"
)

var releaseVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
var workflowSHA256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var configurationEnvironmentPattern = regexp.MustCompile(`\bHITKEEP_[A-Z0-9_]+\b`)
var composeConfigurationDefaultPattern = regexp.MustCompile(`\$\{(HITKEEP_[A-Z0-9_]+):-([^}]*)\}`)
var dockerfileDataPathPattern = regexp.MustCompile(`(?m)^\s*ENV\s+HITKEEP_DATA_PATH(?:=|\s+)(?:"([^"]+)"|'([^']+)'|([^\s#]+))`)
var dockerfileVolumePattern = regexp.MustCompile(`(?m)^\s*VOLUME\s+(.+)$`)
var dockerfileQuotedPathPattern = regexp.MustCompile(`"([^"]+)"`)

type releasePleaseConfig struct {
	Draft            bool `json:"draft"`
	Prerelease       bool `json:"prerelease"`
	ForceTagCreation bool `json:"force-tag-creation"`
	Packages         map[string]struct {
		Draft      *bool                    `json:"draft"`
		Prerelease *bool                    `json:"prerelease"`
		ExtraFiles []releasePleaseExtraFile `json:"extra-files"`
	} `json:"packages"`
}

type releasePleaseExtraFile struct {
	Type     string `json:"type"`
	Path     string `json:"path"`
	JSONPath string `json:"jsonpath"`
}

type workflowNeeds []string

func (needs *workflowNeeds) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		*needs = []string{value.Value}
		return nil
	case yaml.SequenceNode:
		result := make([]string, 0, len(value.Content))
		for _, child := range value.Content {
			if child.Kind != yaml.ScalarNode {
				return fmt.Errorf("workflow needs entry must be a string")
			}
			result = append(result, child.Value)
		}
		*needs = result
		return nil
	case yaml.DocumentNode, yaml.MappingNode, yaml.AliasNode:
		return fmt.Errorf("workflow needs must be a string or list")
	default:
		return fmt.Errorf("workflow needs must be a string or list")
	}
}

type workflowStep struct {
	Name string            `yaml:"name"`
	Run  string            `yaml:"run"`
	Env  map[string]string `yaml:"env"`
	With map[string]string `yaml:"with"`
}

type workflowJob struct {
	Needs workflowNeeds  `yaml:"needs"`
	Uses  string         `yaml:"uses"`
	If    string         `yaml:"if"`
	Steps []workflowStep `yaml:"steps"`
}

type releaseWorkflow struct {
	Jobs map[string]workflowJob `yaml:"jobs"`
}

func validateDefaultTenantMigrationAcceptanceWorkflow(raw []byte) error {
	var workflow struct {
		On yaml.Node `yaml:"on"`
	}
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		return fmt.Errorf("decode default-tenant migration acceptance workflow: %w", err)
	}
	manual := false
	for index := 0; index+1 < len(workflow.On.Content); index += 2 {
		switch event := workflow.On.Content[index].Value; event {
		case "workflow_dispatch":
			manual = true
		case "workflow_call":
			return fmt.Errorf("default-tenant migration acceptance workflow must not support workflow_call")
		default:
			return fmt.Errorf("default-tenant migration acceptance workflow must not support %s", event)
		}
	}
	if !manual {
		return fmt.Errorf("default-tenant migration acceptance workflow must support workflow_dispatch")
	}
	return nil
}

func validateReleaseWorkflowGraph(raw []byte) error {
	var workflow releaseWorkflow
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		return fmt.Errorf("decode release workflow: %w", err)
	}
	for job, required := range map[string][]string{
		"build-release":                {"release-please"},
		"upgrade-from-supported-floor": {"build-release"},
		"publish-helm":                 {"build-release"},
		"verify-tracker-package":       {"build-release"},
		"docs-attestation":             {"release-please", "build-release", "upgrade-from-supported-floor", "publish-helm", "verify-tracker-package"},
		"finalize-release":             {"release-please", "build-release", "upgrade-from-supported-floor", "publish-helm", "verify-tracker-package", "docs-attestation"},
		"sync-docs-release":            {"finalize-release"},
		"deploy-cloud":                 {"finalize-release"},
	} {
		definition, ok := workflow.Jobs[job]
		if !ok {
			return fmt.Errorf("release workflow is missing %s", job)
		}
		for _, dependency := range required {
			if !slices.Contains(definition.Needs, dependency) {
				return fmt.Errorf("release workflow job %s must need %s", job, dependency)
			}
		}
	}
	for job, definition := range workflow.Jobs {
		if definition.Uses == "./.github/workflows/default-tenant-migration-acceptance.yml" {
			return fmt.Errorf("release workflow job %s must not call the manual default-tenant migration acceptance workflow", job)
		}
	}
	fixtureResolved := false
	upgradeSmokes := map[string]bool{}
	smokeScripts := map[string]string{
		"docker":  "./scripts/docker-smoke.sh",
		"compose": "./scripts/compose-smoke.sh",
		"helm":    "./scripts/helm-smoke.sh",
	}
	for _, step := range workflow.Jobs["upgrade-from-supported-floor"].Steps {
		if strings.Contains(step.Run, "tests/fixtures/release-fixtures.json") &&
			strings.Contains(step.Run, `repository="${GITHUB_REPOSITORY,,}"`) &&
			strings.Contains(step.Run, `candidate="ghcr.io/${repository}@${CANDIDATE_DIGEST}"`) &&
			strings.Contains(step.Run, "previous_version") {
			fixtureResolved = true
		}
		for surface, script := range smokeScripts {
			if strings.Contains(step.Run, script) &&
				strings.Contains(step.Env["CANDIDATE_IMAGE"], "steps.fixture.outputs.candidate") &&
				strings.Contains(step.Env["HITKEEP_PREVIOUS_IMAGE"], "steps.fixture.outputs.previous") {
				upgradeSmokes[surface] = true
			}
		}
	}
	if !fixtureResolved {
		return fmt.Errorf("release workflow upgrade-from-supported-floor must resolve the supported upgrade-floor fixture against the candidate digest")
	}
	workflowText := string(raw)
	for _, surface := range []string{"docker", "compose", "helm"} {
		if !strings.Contains(workflowText, "- surface: "+surface) || !upgradeSmokes[surface] {
			return fmt.Errorf("release workflow upgrade-from-supported-floor must smoke the %s matrix surface", surface)
		}
	}

	trackerArtifact := map[string]bool{}
	trackerPlanBound := false
	for _, step := range workflow.Jobs["verify-tracker-package"].Steps {
		if step.Name == "Pack verified tracker artifact" &&
			strings.Contains(step.Run, "npm pack --json") &&
			strings.Contains(step.Run, "to_entries | first | .value.filename") {
			trackerArtifact[step.Name] = true
		}
		if step.Name == "Upload verified tracker artifact" {
			trackerArtifact[step.Name] = true
		}
		if strings.Contains(step.Run, `plan_id="$(./hk qa plan pr --output json | jq -r '.data.plan_id')"`) &&
			strings.Contains(step.Run, `./hk qa pr --plan-id "$plan_id" --gate tracker-package`) {
			trackerPlanBound = true
		}
	}
	if !trackerPlanBound {
		return fmt.Errorf("release workflow verify-tracker-package must run tracker QA with its persisted plan ID")
	}
	if !trackerArtifact["Pack verified tracker artifact"] || !trackerArtifact["Upload verified tracker artifact"] {
		return fmt.Errorf("release workflow verify-tracker-package must pack and upload the verified tracker artifact")
	}

	finalizer := workflow.Jobs["finalize-release"]
	trackerDownload := false
	trackerPublish := false
	for _, step := range finalizer.Steps {
		if step.Name == "Download verified tracker artifact" {
			trackerDownload = true
		}
		if step.Name != "Publish verified tracker" {
			continue
		}
		for _, fragment := range []string{
			`npm publish "$tarball" --tag latest || true`,
			"dist.integrity",
			"dist-tags.latest",
			"for attempt in {1..6}",
			`[[ "$existing" == "$integrity" && "$latest" == "$VERSION" ]]`,
			`[[ "$existing" != "$integrity" ]]`,
			`[[ "$latest" != "$VERSION" ]]`,
			"openssl dgst -sha512",
		} {
			if !strings.Contains(step.Run, fragment) {
				return fmt.Errorf("release workflow tracker publication is missing %q", fragment)
			}
		}
		trackerPublish = true
	}
	if !trackerDownload {
		return fmt.Errorf("release workflow finalizer must download the verified tracker artifact")
	}
	if !trackerPublish {
		return fmt.Errorf("release workflow finalizer must publish the verified tracker artifact as latest with npm integrity verification")
	}
	draftPublication := false
	for _, step := range finalizer.Steps {
		if step.Name == "Promote GitHub release" && strings.Contains(step.Run, `gh release edit "$TAG" --draft=false --latest`) {
			draftPublication = true
		}
	}
	if !draftPublication {
		return fmt.Errorf("release workflow finalizer must publish the draft as latest")
	}
	for _, step := range finalizer.Steps {
		for _, values := range []map[string]string{step.Env, step.With} {
			for _, value := range values {
				if strings.Contains(value, "secrets.GHT") {
					return fmt.Errorf("release workflow finalizer must not use secrets.GHT")
				}
			}
		}
	}
	positions := make(map[string]int, len(finalizer.Steps))
	for index, step := range finalizer.Steps {
		positions[step.Name] = index
	}
	orderedSteps := []string{
		"Publish verified tracker",
		"Promote immutable image to mutable tags",
		"Promote GitHub release",
	}
	previous := -1
	previousName := ""
	for _, name := range orderedSteps {
		position, ok := positions[name]
		if !ok {
			return fmt.Errorf("release workflow finalizer is missing step %q", name)
		}
		if position <= previous {
			return fmt.Errorf("release workflow finalizer must run %q before %q", previousName, name)
		}
		previous = position
		previousName = name
	}
	docsAttestation := workflow.Jobs["docs-attestation"]
	attestedRelease := false
	for _, step := range docsAttestation.Steps {
		required := []string{
			"gh workflow run sync-hitkeep-release.yml",
			"--ref main",
			"--event workflow_dispatch",
			"source_run_id=\"$source_run_id\"",
			"source_head_sha=\"$GITHUB_SHA\"",
			"source_workflow_sha256=\"$source_workflow_sha256\"",
			"source_catalog_sha256=\"$catalog_sha256\"",
			"source_example_sha256=\"$example_sha256\"",
			"source_manifest_sha256=\"$manifest_sha256\"",
			"gh run watch \"$docs_run_id\"",
			"actions/runs/$docs_run_id",
			".id == $run_id",
			".head_sha == $docs_head_sha",
			"check-suites/$check_suite_id",
			".app.id == 15368",
			"hitkeep-docs-release-attestation",
			".conclusion == \"success\"",
			".source.tag == $tag",
			".source.run_id == $source_run_id",
		}
		if slices.ContainsFunc(required, func(fragment string) bool { return !strings.Contains(step.Run, fragment) }) {
			continue
		}
		if step.Env["DOCS_REPOSITORY"] == "PascaleBeier/hitkeep-docs" && workflowSHA256Pattern.MatchString(step.Env["DOCS_WORKFLOW_SHA256"]) {
			attestedRelease = true
			break
		}
	}
	if !attestedRelease {
		return fmt.Errorf("release workflow must block publication on an immutable documentation attestation")
	}
	if !strings.Contains(workflowText, "-f prepublication=true") {
		return fmt.Errorf("release workflow documentation attestation must be prepublication-only")
	}
	postPublicationSync := false
	for _, step := range workflow.Jobs["sync-docs-release"].Steps {
		required := []string{
			"-f prepublication=false",
			"source_workflow_sha256=\"${SOURCE_WORKFLOW_SHA256}\"",
			"source_catalog_sha256=\"${SOURCE_CATALOG_SHA256}\"",
			"source_example_sha256=\"${SOURCE_EXAMPLE_SHA256}\"",
			"source_manifest_sha256=\"${SOURCE_MANIFEST_SHA256}\"",
		}
		if slices.ContainsFunc(required, func(fragment string) bool { return !strings.Contains(step.Run, fragment) }) {
			continue
		}
		if step.Env["SOURCE_WORKFLOW_SHA256"] == "${{ needs.docs-attestation.outputs.source_workflow_sha256 }}" &&
			step.Env["SOURCE_CATALOG_SHA256"] == "${{ needs.docs-attestation.outputs.source_catalog_sha256 }}" &&
			step.Env["SOURCE_EXAMPLE_SHA256"] == "${{ needs.docs-attestation.outputs.source_example_sha256 }}" &&
			step.Env["SOURCE_MANIFEST_SHA256"] == "${{ needs.docs-attestation.outputs.source_manifest_sha256 }}" {
			postPublicationSync = true
			break
		}
	}
	if !postPublicationSync {
		return fmt.Errorf("release workflow post-publication documentation synchronization must use the attested immutable inputs")
	}
	return nil
}

func ValidateDevelopmentDocs(root string) error {
	files := map[string][]string{
		"README.md":                    {"./hk setup", "./hk dev --seed", "./hk dev --detach", "./hk screenshot", "./hk catalog commands --output json", "./hk catalog configuration --output json", "Compose stack", "./hk qa pr", "CONTRIBUTING.md"},
		"CONTRIBUTING.md":              {"./hk help", "./hk catalog --output json", "./hk catalog commands --output json", "agent_command", "model-agnostic", "central MCP", "configured or enabled", "explicit user approval", "server-catalog routing", "stateless protocol mode", "--fallback-workspace", "macOS or Linux", "AMD64 or ARM64", "executable POSIX launcher", "./hk mcp manifest", "./hk mcp serve", "./hk skills check", "./hk screenshot", "hk_screenshot", "./hk fmt", "./hk fmt --scope frontend", "./hk fix check", "./hk cache status", "./hk run list", "hk.dev/v3", "hk_dev_status", "next_cursor", "AGENTS.md", "source repository is private"},
		"AGENTS.md":                    {"Use `./hk` as the workflow source of truth", "callable central developer MCP", "configured or enabled", "explicit user approval", "./hk catalog configuration --output json", "container-only session", "$hitkeep-development", "$hitkeep-workspace", "$hitkeep-qa", "private `PascaleBeier/hitkeep-docs`"},
		"frontend/dashboard/README.md": {"./hk dev --seed", "./hk screenshot", "./hk fmt --scope frontend", "./hk qa changed --gate frontend-unit", "./hk qa changed --gate frontend-e2e"},
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
	if err := validateCIWorkflowContract(root); err != nil {
		return err
	}
	if err := validateReleaseMetadata(root); err != nil {
		return err
	}
	if err := validateConfigurationDocumentation(root); err != nil {
		return err
	}
	return ValidateSkillLayout(root)
}

func validateConfigurationDocumentation(root string) error {
	catalog := runtimeconfig.Catalog()
	known := make(map[string]runtimeconfig.ConfigurationSetting, len(catalog.Settings))
	for _, setting := range catalog.Settings {
		if setting.Environment != "" {
			known[setting.Environment] = setting
		}
	}
	nonRuntime := map[string]bool{
		"HITKEEP_GO_BUILD_TAGS": true,
		"HITKEEP_HOSTNAME":      true,
		"HITKEEP_SEED_DAYS":     true,
		"HITKEEP_SEED_DOMAIN":   true,
		"HITKEEP_SEED_EMAIL":    true,
		"HITKEEP_SEED_PASSWORD": true,
		"HITKEEP_VARIANT":       true,
		"HITKEEP_VERSION":       true,
	}

	requirements := runtimeconfig.PublicationRequirements()
	if err := validateRequiredConfigurationPublications(requirements, func(path string) string {
		if path == "config.example.yaml" {
			return string(runtimeconfig.RenderExampleYAML())
		}
		raw, readErr := os.ReadFile(filepath.Join(root, path))
		if readErr != nil {
			return ""
		}
		contents := string(raw)
		if path == "charts/hitkeep/templates/statefulset.yaml" {
			values, valuesErr := os.ReadFile(filepath.Join(root, "charts/hitkeep/values.yaml"))
			if valuesErr != nil {
				return ""
			}
			contents += helmPublicationValuesSeparator + string(values)
		}
		return contents
	}); err != nil {
		return err
	}

	paths := []string{
		filepath.Join(root, "README.md"),
		filepath.Join(root, "Dockerfile"),
	}
	composePaths, err := filepath.Glob(filepath.Join(root, "compose*.yaml"))
	if err != nil {
		return err
	}
	paths = append(paths, composePaths...)
	for _, directory := range []string{"charts", "examples"} {
		err = filepath.WalkDir(filepath.Join(root, directory), func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			extension := strings.ToLower(filepath.Ext(path))
			if slices.Contains([]string{".md", ".yaml", ".yml"}, extension) || entry.Name() == "Dockerfile" || entry.Name() == "Caddyfile.custom-tracking" {
				paths = append(paths, path)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	slices.Sort(paths)
	paths = slices.Compact(paths)
	for _, path := range paths {
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		relative, _ := filepath.Rel(root, path)
		if filepath.ToSlash(relative) == "Dockerfile" {
			if _, ok := known["HITKEEP_DATA_PATH"]; !ok {
				return fmt.Errorf("configuration catalog is missing HITKEEP_DATA_PATH")
			}
			if err := validateContainerDataPath(filepath.ToSlash(relative), string(raw)); err != nil {
				return err
			}
		}
		checkDefaults := filepath.Base(path) != "compose.dev.yaml"
		relativePath := filepath.ToSlash(relative)
		if err := validateConfigurationDocument(relativePath, string(raw), known, nonRuntime, checkDefaults); err != nil {
			return err
		}
	}
	return nil
}

var helmDataPathTemplatePattern = regexp.MustCompile(`(?ms)- name:\s*HITKEEP_DATA_PATH\s*\n\s*value:\s*\{\{\s*\.Values\.persistence\.mountPath\s*\|\s*quote\s*\}\}`)

const helmPublicationValuesSeparator = "\n--- hitkeep-publication-values ---\n"

func validateRequiredConfigurationPublications(requirements []runtimeconfig.ConfigurationPublication, contents func(string) string) error {
	for _, requirement := range requirements {
		for _, surface := range requirement.Surfaces {
			for _, path := range requirement.Paths[surface] {
				if actual := configurationPublicationSurface(path); actual != surface {
					return fmt.Errorf("%s maps to publication surface %q, want %q", path, actual, surface)
				}
				if err := validateConfigurationPublication(path, contents(path), []runtimeconfig.ConfigurationPublication{requirement}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateConfigurationPublication(path, contents string, requirements []runtimeconfig.ConfigurationPublication) error {
	surface := configurationPublicationSurface(path)
	if surface == "" {
		return nil
	}
	for _, requirement := range requirements {
		if !slices.Contains(requirement.Surfaces, surface) {
			continue
		}
		actual, found := configurationPublicationDefault(contents, requirement, surface)
		if !found {
			return fmt.Errorf("%s omits required published configuration setting %s", path, requirement.Environment)
		}
		if expected := requirement.Defaults[surface]; actual != expected {
			return fmt.Errorf("%s gives %s published default %q; catalog default is %q", path, requirement.Environment, actual, expected)
		}
	}
	return nil
}

func configurationPublicationSurface(path string) runtimeconfig.ConfigurationPublicationSurface {
	base := filepath.Base(path)
	switch {
	case path == "Dockerfile":
		return runtimeconfig.ConfigurationPublicationDocker
	case strings.HasPrefix(path, "examples/") && strings.HasPrefix(base, "compose") && (strings.HasSuffix(base, ".yaml") || strings.HasSuffix(base, ".yml")):
		return runtimeconfig.ConfigurationPublicationExample
	case strings.HasPrefix(base, "compose") && strings.HasSuffix(base, ".yaml"):
		return runtimeconfig.ConfigurationPublicationCompose
	case path == "charts/hitkeep/templates/statefulset.yaml":
		return runtimeconfig.ConfigurationPublicationHelm
	case base == "config.example.yaml":
		return runtimeconfig.ConfigurationPublicationCanonicalExample
	default:
		return ""
	}
}

func configurationPublicationDefault(contents string, requirement runtimeconfig.ConfigurationPublication, surface runtimeconfig.ConfigurationPublicationSurface) (string, bool) {
	for line := range strings.SplitSeq(contents, "\n") {
		line = strings.TrimSpace(line)
		switch surface {
		case runtimeconfig.ConfigurationPublicationDocker:
			value, found := strings.CutPrefix(line, "ENV "+requirement.Environment+"=")
			if found {
				return strings.Trim(value, "\\\"'"), true
			}
		case runtimeconfig.ConfigurationPublicationCompose, runtimeconfig.ConfigurationPublicationExample:
			value, found := strings.CutPrefix(line, requirement.Environment+":")
			if found {
				return strings.Trim(strings.TrimSpace(value), "\\\"'"), true
			}
		case runtimeconfig.ConfigurationPublicationCanonicalExample:
			value, found := strings.CutPrefix(line, requirement.ConfigFileKey+":")
			if found {
				return strings.Trim(strings.TrimSpace(value), "\\\"'"), true
			}
		case runtimeconfig.ConfigurationPublicationHelm:
			continue
		}
	}
	if surface != runtimeconfig.ConfigurationPublicationHelm {
		return "", false
	}
	template, values, found := strings.Cut(contents, helmPublicationValuesSeparator)
	if !found || !helmDataPathTemplatePattern.MatchString(template) {
		return "", false
	}
	var chartValues struct {
		Persistence struct {
			MountPath string `yaml:"mountPath"`
		} `yaml:"persistence"`
	}
	if err := yaml.Unmarshal([]byte(values), &chartValues); err != nil {
		return "", false
	}
	mountPath := strings.TrimSpace(chartValues.Persistence.MountPath)
	return mountPath, mountPath != ""
}

func validateContainerDataPath(path, contents string) error {
	matches := dockerfileDataPathPattern.FindAllStringSubmatch(contents, -1)
	if len(matches) != 1 {
		return fmt.Errorf("%s must declare exactly one HITKEEP_DATA_PATH image default", path)
	}
	dataPath := firstNonEmpty(matches[0][1:])
	for _, match := range dockerfileVolumePattern.FindAllStringSubmatch(contents, -1) {
		for _, volume := range dockerfileVolumePaths(match[1]) {
			if dataPath == volume || strings.HasPrefix(dataPath, strings.TrimRight(volume, "/")+"/") {
				return nil
			}
		}
	}
	return fmt.Errorf("%s HITKEEP_DATA_PATH image default %q is not beneath a declared persistent VOLUME", path, dataPath)
}

func dockerfileVolumePaths(instruction string) []string {
	quoted := dockerfileQuotedPathPattern.FindAllStringSubmatch(instruction, -1)
	if len(quoted) > 0 {
		paths := make([]string, 0, len(quoted))
		for _, match := range quoted {
			paths = append(paths, match[1])
		}
		return paths
	}
	return strings.Fields(strings.TrimSpace(instruction))
}

func firstNonEmpty(values []string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func validateConfigurationDocument(path, contents string, known map[string]runtimeconfig.ConfigurationSetting, nonRuntime map[string]bool, checkDefaults bool) error {
	for lineNumber, line := range strings.Split(contents, "\n") {
		for _, environment := range configurationEnvironmentPattern.FindAllString(line, -1) {
			if _, ok := known[environment]; !ok && !nonRuntime[environment] {
				return fmt.Errorf("%s:%d documents unknown HitKeep configuration variable %s", path, lineNumber+1, environment)
			}
		}
		if !checkDefaults {
			continue
		}
		for _, match := range composeConfigurationDefaultPattern.FindAllStringSubmatch(line, -1) {
			setting, ok := known[match[1]]
			if !ok || match[2] == setting.Default {
				continue
			}
			if !strings.Contains(line, "config-default-override:") {
				return fmt.Errorf("%s:%d gives %s a Compose default %q; runtime default is %q (add config-default-override: with a reason when intentional)", path, lineNumber+1, match[1], match[2], setting.Default)
			}
		}
	}
	return nil
}

func validateReleaseMetadata(root string) error {
	manifest := map[string]string{}
	if err := readJSONFile(filepath.Join(root, ".release-please-manifest.json"), &manifest); err != nil {
		return err
	}
	version := strings.TrimSpace(manifest["."])
	if !releaseVersionPattern.MatchString(version) {
		return fmt.Errorf(".release-please-manifest.json has invalid root version %q", version)
	}

	var registry struct {
		Version string `json:"version"`
	}
	if err := readJSONFile(filepath.Join(root, "server.json"), &registry); err != nil {
		return err
	}
	if err := requireReleaseVersion("server.json $.version", registry.Version, version); err != nil {
		return err
	}

	var dashboardPackage struct {
		Version string `json:"version"`
	}
	if err := readJSONFile(filepath.Join(root, "frontend", "dashboard", "package.json"), &dashboardPackage); err != nil {
		return err
	}
	if err := requireReleaseVersion("frontend/dashboard/package.json $.version", dashboardPackage.Version, version); err != nil {
		return err
	}

	var trackerPackage struct {
		Version string `json:"version"`
	}
	if err := readJSONFile(filepath.Join(root, "frontend", "tracker", "package.json"), &trackerPackage); err != nil {
		return err
	}
	if err := requireReleaseVersion("frontend/tracker/package.json $.version", trackerPackage.Version, version); err != nil {
		return err
	}

	trackerVersionPath := filepath.Join(root, "frontend", "dashboard", "src", "tracker", "version.ts")
	trackerVersionRaw, err := os.ReadFile(trackerVersionPath)
	if err != nil {
		return err
	}
	trackerAnnotatedLines := 0
	for lineNumber, line := range strings.Split(string(trackerVersionRaw), "\n") {
		if !strings.Contains(line, "x-release-please-version") {
			continue
		}
		trackerAnnotatedLines++
		if !strings.Contains(line, version) {
			return fmt.Errorf("frontend/dashboard/src/tracker/version.ts:%d release annotation does not contain %s", lineNumber+1, version)
		}
	}
	if trackerAnnotatedLines == 0 {
		return fmt.Errorf("frontend/dashboard/src/tracker/version.ts has no x-release-please-version annotations")
	}

	var dashboardLock struct {
		Packages map[string]struct {
			Version string `json:"version"`
		} `json:"packages"`
	}
	if err := readJSONFile(filepath.Join(root, "frontend", "dashboard", "package-lock.json"), &dashboardLock); err != nil {
		return err
	}
	rootPackage, ok := dashboardLock.Packages[""]
	if !ok {
		return fmt.Errorf("frontend/dashboard/package-lock.json is missing packages['']")
	}
	if err := requireReleaseVersion("frontend/dashboard/package-lock.json $.packages[''].version", rootPackage.Version, version); err != nil {
		return err
	}

	chartPath := filepath.Join(root, "charts", "hitkeep", "Chart.yaml")
	chartRaw, err := os.ReadFile(chartPath)
	if err != nil {
		return err
	}
	chartVersions := map[string]string{}
	for line := range strings.SplitSeq(string(chartRaw), "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "version" || key == "appVersion" {
			chartVersions[key] = strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	for _, key := range []string{"version", "appVersion"} {
		if err := requireReleaseVersion("charts/hitkeep/Chart.yaml $."+key, chartVersions[key], version); err != nil {
			return err
		}
	}

	chartREADMEPath := filepath.Join(root, "charts", "hitkeep", "README.md")
	chartREADME, err := os.ReadFile(chartREADMEPath)
	if err != nil {
		return err
	}
	annotatedLines := 0
	for lineNumber, line := range strings.Split(string(chartREADME), "\n") {
		if !strings.Contains(line, "x-release-please-version") {
			continue
		}
		annotatedLines++
		if !strings.Contains(line, version) {
			return fmt.Errorf("charts/hitkeep/README.md:%d release annotation does not contain %s", lineNumber+1, version)
		}
	}
	if annotatedLines == 0 {
		return fmt.Errorf("charts/hitkeep/README.md has no x-release-please-version annotations")
	}

	var config releasePleaseConfig
	if err := readJSONFile(filepath.Join(root, "release-please-config.json"), &config); err != nil {
		return err
	}
	rootPackageConfig, ok := config.Packages["."]
	if !ok {
		return fmt.Errorf("release-please-config.json is missing packages['.']")
	}
	effectiveDraft := config.Draft
	if rootPackageConfig.Draft != nil {
		effectiveDraft = *rootPackageConfig.Draft
	}
	if !effectiveDraft {
		return fmt.Errorf("release-please-config.json effective packages['.'].draft must be true")
	}
	effectivePrerelease := config.Prerelease
	if rootPackageConfig.Prerelease != nil {
		effectivePrerelease = *rootPackageConfig.Prerelease
	}
	if effectivePrerelease {
		return fmt.Errorf("release-please-config.json effective packages['.'].prerelease must be false")
	}
	if !config.ForceTagCreation {
		return fmt.Errorf("release-please-config.json force-tag-creation must be true")
	}
	expectedFiles := []releasePleaseExtraFile{
		{Type: "json", Path: "server.json", JSONPath: "$.version"},
		{Type: "json", Path: "frontend/dashboard/package.json", JSONPath: "$.version"},
		{Type: "json", Path: "frontend/dashboard/package-lock.json", JSONPath: "$['packages']['']['version']"},
		{Type: "json", Path: "frontend/tracker/package.json", JSONPath: "$.version"},
		{Type: "generic", Path: "frontend/dashboard/src/tracker/version.ts"},
		{Type: "yaml", Path: "charts/hitkeep/Chart.yaml", JSONPath: "$.version"},
		{Type: "yaml", Path: "charts/hitkeep/Chart.yaml", JSONPath: "$.appVersion"},
		{Type: "generic", Path: "charts/hitkeep/README.md"},
	}
	for _, expected := range expectedFiles {
		if !slices.Contains(rootPackageConfig.ExtraFiles, expected) {
			return fmt.Errorf("release-please-config.json does not manage %s %s", expected.Path, expected.JSONPath)
		}
	}
	workflowContracts := map[string][]string{
		".github/workflows/pipeline.yml": {
			"github.com/goreleaser/goreleaser/v2@v2.18.0",
			"--snapshot",
			"--clean",
			"--single-target",
			"--id self-hosted",
			"--id cloud",
			"./hk catalog configuration --output json",
			"./hk catalog configuration-manifest",
			"hitkeep-configuration.json",
			"hitkeep.example.yaml",
			"hitkeep-configuration-manifest.json",
			"release_tag: $tag",
			"release_version: $version",
			"printf '%s\\n' hitkeep-configuration.json hitkeep-configuration-manifest.json >> .git/info/exclude",
			"github.com/goreleaser/goreleaser/v2@v2.18.0 release --clean --skip=publish",
		},
		".github/workflows/release.yml": {
			"finalize-release:",
			"sync-hitkeep-release.yml",
		},
	}
	for name, fragments := range workflowContracts {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return err
		}
		for _, fragment := range fragments {
			if !bytes.Contains(raw, []byte(fragment)) {
				return fmt.Errorf("%s is missing release metadata contract %q", name, fragment)
			}
		}
		if name == ".github/workflows/pipeline.yml" && bytes.Contains(raw, []byte("./hk ci build-binaries")) {
			return fmt.Errorf(".github/workflows/pipeline.yml must not run ./hk ci build-binaries")
		}
	}
	pipelineRaw, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "pipeline.yml"))
	if err != nil {
		return err
	}
	pipeline := string(pipelineRaw)
	const runnerLocalExclude = "printf '%s\\n' hitkeep-configuration.json hitkeep-configuration-manifest.json >> .git/info/exclude"
	if strings.Count(pipeline, ".git/info/exclude") != 1 || !strings.Contains(pipeline, runnerLocalExclude) {
		return fmt.Errorf(".github/workflows/pipeline.yml must write exactly the generated configuration JSON inputs to .git/info/exclude")
	}
	if strings.Index(pipeline, runnerLocalExclude) >= strings.Index(pipeline, "github.com/goreleaser/goreleaser/v2@v2.18.0 release --clean --skip=publish") {
		return fmt.Errorf(".github/workflows/pipeline.yml must write generated configuration JSON inputs to .git/info/exclude before tagged GoReleaser")
	}
	goreleaserManifest, err := os.ReadFile(filepath.Join(root, ".goreleaser.yaml"))
	if err != nil {
		return err
	}
	for _, artifact := range []string{runtimeconfig.ConfigurationCatalogFilename, runtimeconfig.ConfigurationExampleFilename, runtimeconfig.ConfigurationReleaseManifestFilename} {
		if !bytes.Contains(goreleaserManifest, []byte(artifact)) {
			return fmt.Errorf(".goreleaser.yaml is missing release configuration artifact %q", artifact)
		}
	}
	releaseWorkflowRaw, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "release.yml"))
	if err != nil {
		return err
	}
	if err := validateReleaseWorkflowGraph(releaseWorkflowRaw); err != nil {
		return err
	}
	migrationWorkflow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "default-tenant-migration-acceptance.yml"))
	if err != nil {
		return err
	}
	if err := validateDefaultTenantMigrationAcceptanceWorkflow(migrationWorkflow); err != nil {
		return err
	}
	var parsedReleaseWorkflow releaseWorkflow
	if err := yaml.Unmarshal(releaseWorkflowRaw, &parsedReleaseWorkflow); err != nil {
		return fmt.Errorf("decode release workflow: %w", err)
	}
	for _, step := range parsedReleaseWorkflow.Jobs["sync-docs-release"].Steps {
		for _, fragment := range []string{"gh run watch", "--log-failed", "::error::hitkeep-docs"} {
			if strings.Contains(step.Run, fragment) {
				return fmt.Errorf("release workflow post-publication docs notification must not surface downstream failures through %q", fragment)
			}
		}
	}
	return nil
}

func readJSONFile(path string, target any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func requireReleaseVersion(source, actual, expected string) error {
	actual = strings.TrimSpace(actual)
	if actual != expected {
		return fmt.Errorf("%s has version %q; want %q", source, actual, expected)
	}
	return nil
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
		if bytes.Contains(raw, []byte("actions/setup-node@")) {
			return fmt.Errorf(".github/workflows/%s bypasses the canonical Node and npm setup action", entry.Name())
		}
		if bytes.Contains(raw, []byte("npm ")) && !bytes.Contains(raw, []byte("./.github/actions/setup-node-npm")) {
			return fmt.Errorf(".github/workflows/%s runs npm without the canonical Node and npm setup action", entry.Name())
		}
		workflows = append(workflows, raw...)
		workflows = append(workflows, '\n')
	}
	action, err := os.ReadFile(filepath.Join(root, ".github", "actions", "setup-node-npm", "action.yml"))
	if err != nil {
		return err
	}
	if bytes.Contains(action, []byte("cache: npm")) {
		return fmt.Errorf("canonical Node and npm setup action invokes npm through setup-node before installing the canonical npm version")
	}
	for _, fragment := range []string{"frontend/dashboard/.node-version", "frontend/dashboard/package-lock.json", "package-manager-cache: false", "actions/cache@", "path: ~/.npm", "./hk ci toolchain --output json", "npm install --global"} {
		if !bytes.Contains(action, []byte(fragment)) {
			return fmt.Errorf("canonical Node and npm setup action is missing %q", fragment)
		}
	}
	for group, gateIDs := range groups {
		if !bytes.Contains(workflows, []byte("--group "+group)) {
			return fmt.Errorf("canonical CI group %s for gates %s is not referenced by a workflow", group, strings.Join(gateIDs, ", "))
		}
	}
	govulncheck, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "govulncheck.yml"))
	if err != nil {
		return err
	}
	return validateGovulncheckWorkflowContract(govulncheck)
}

func validateGovulncheckWorkflowContract(workflow []byte) error {
	const plan = "./hk qa plan pr --output json"
	if !bytes.Contains(workflow, []byte(plan)) {
		return fmt.Errorf(".github/workflows/govulncheck.yml must obtain a PR QA plan ID through %q", plan)
	}
	for line := range bytes.SplitSeq(workflow, []byte{'\n'}) {
		if bytes.Contains(line, []byte("./hk qa ")) && !bytes.Contains(line, []byte("./hk qa plan ")) && !bytes.Contains(line, []byte("--plan-id")) {
			return fmt.Errorf(".github/workflows/govulncheck.yml QA invocation must pass --plan-id")
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
