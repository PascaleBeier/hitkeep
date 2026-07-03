package skills

import (
	"embed"
	"io/fs"
	"sort"
	"strings"
)

//go:embed */SKILL.md
var skillFS embed.FS

// PublicAnalyticsSkillPack returns the public HitKeep analytics skill text used
// to ground the built-in Ask AI assistant.
func PublicAnalyticsSkillPack() string {
	names, err := fs.Glob(skillFS, "*/SKILL.md")
	if err != nil {
		return ""
	}
	sort.Strings(names)

	var builder strings.Builder
	for _, name := range names {
		if strings.Contains(name, "hitkeep-i18n/") {
			continue
		}
		data, err := skillFS.ReadFile(name)
		if err != nil {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n---\n\n")
		}
		builder.WriteString("# ")
		builder.WriteString(strings.TrimSuffix(name, "/SKILL.md"))
		builder.WriteString("\n\n")
		builder.Write(data)
	}
	return builder.String()
}
