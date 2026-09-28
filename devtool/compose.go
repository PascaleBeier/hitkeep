package devtool

const developmentComposeFile = "compose.dev.yaml"

// composeArgs keeps the workspace-scoped Compose process boundary in one place.
// Compose remains responsible for provider compatibility and project resource
// ownership; hk owns the development-session lifecycle layered on top.
func (a *App) composeArgs(action ...string) []string {
	args := make([]string, 0, 7+len(action))
	args = append(args, "docker", "compose", "-p", a.workspace.ComposeProject, "-f", developmentComposeFile)
	return append(args, action...)
}
