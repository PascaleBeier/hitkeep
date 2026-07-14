package devtool

import (
	"fmt"
	"os"
	"path/filepath"
)

const DeveloperMCPManifestSchemaVersion = "hk.dev/mcp-manifest/v1"

const developerMCPServerName = "hitkeep-dev"

type MCPServerDefinition struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type MCPClientConfiguration struct {
	MCPServers map[string]MCPServerDefinition `json:"mcpServers"`
}

type CodexProjectMCP struct {
	Automatic              bool   `json:"automatic"`
	ConfigPath             string `json:"config_path"`
	ServerName             string `json:"server_name"`
	RequiresTrustedProject bool   `json:"requires_trusted_project"`
}

// MCPProjectIntegration describes a checked-in, worktree-relative registration
// consumed by an agent host. The MCP server itself is model-agnostic; these
// files only adapt each host's project discovery convention.
type MCPProjectIntegration struct {
	ClientID               string `json:"client_id"`
	ClientName             string `json:"client_name"`
	Automatic              bool   `json:"automatic"`
	ConfigPath             string `json:"config_path"`
	ServerName             string `json:"server_name"`
	RequiresTrustedProject bool   `json:"requires_trusted_project"`
}

type mcpProjectClientSpec struct {
	ID                     string
	Name                   string
	ConfigPath             string
	ServerName             string
	RequiresTrustedProject bool
	RequiredFragments      []string
}

var mcpProjectClientSpecs = []mcpProjectClientSpec{
	{
		ID:                     "claude-code",
		Name:                   "Claude Code",
		ConfigPath:             ".mcp.json",
		ServerName:             developerMCPServerName,
		RequiresTrustedProject: true,
		RequiredFragments: []string{
			`"mcpServers"`,
			`"hitkeep-dev"`,
			`"command": "${CLAUDE_PROJECT_DIR:-.}/hk"`,
			`"args": ["--workspace", "${CLAUDE_PROJECT_DIR:-.}", "mcp", "serve"]`,
		},
	},
	{
		ID:                     "codex",
		Name:                   "Codex",
		ConfigPath:             ".codex/config.toml",
		ServerName:             developerMCPServerName,
		RequiresTrustedProject: true,
		RequiredFragments: []string{
			"[mcp_servers.hitkeep-dev]",
			`command = "../hk"`,
			`args = ["--workspace", "..", "mcp", "serve"]`,
			`cwd = "."`,
		},
	},
	{
		ID:                     "cursor",
		Name:                   "Cursor",
		ConfigPath:             ".cursor/mcp.json",
		ServerName:             developerMCPServerName,
		RequiresTrustedProject: true,
		RequiredFragments: []string{
			`"mcpServers"`,
			`"hitkeep-dev"`,
			`"command": "./hk"`,
			`"args": ["--workspace", ".", "mcp", "serve"]`,
		},
	},
	{
		ID:                     "gemini-cli",
		Name:                   "Gemini CLI",
		ConfigPath:             ".gemini/settings.json",
		ServerName:             developerMCPServerName,
		RequiresTrustedProject: true,
		RequiredFragments: []string{
			`"mcpServers"`,
			`"hitkeep-dev"`,
			`"command": "./hk"`,
			`"args": ["--workspace", ".", "mcp", "serve"]`,
			`"cwd": "."`,
			`"trust": false`,
		},
	},
	{
		ID:                     "vscode",
		Name:                   "VS Code",
		ConfigPath:             ".vscode/mcp.json",
		ServerName:             "hitkeepDev",
		RequiresTrustedProject: true,
		RequiredFragments: []string{
			`"servers"`,
			`"hitkeepDev"`,
			`"type": "stdio"`,
			`"command": "${workspaceFolder}/hk"`,
			`"args": ["--workspace", "${workspaceFolder}", "mcp", "serve"]`,
			`"cwd": "${workspaceFolder}"`,
		},
	},
}

type DeveloperMCPManifest struct {
	SchemaVersion  string                  `json:"schema_version"`
	ServerName     string                  `json:"server_name"`
	Transport      string                  `json:"transport"`
	WorkspaceID    string                  `json:"workspace_id"`
	WorkspaceRoot  string                  `json:"workspace_root"`
	Command        string                  `json:"command"`
	Args           []string                `json:"args"`
	ClientConfig   MCPClientConfiguration  `json:"client_config"`
	Codex          CodexProjectMCP         `json:"codex"`
	ProjectClients []MCPProjectIntegration `json:"project_clients"`
}

// MCPManifest returns a worktree-specific stdio registration without editing
// any client-owned configuration. The repository launcher is stable across hk
// rebuilds, unlike the content-addressed bootstrap binary behind it.
func (a *App) MCPManifest() (DeveloperMCPManifest, error) {
	launcher := filepath.Join(a.workspace.Root, "hk")
	info, err := os.Lstat(launcher)
	if err != nil {
		return DeveloperMCPManifest{}, fmt.Errorf("resolve hk launcher: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return DeveloperMCPManifest{}, fmt.Errorf("hk launcher must be a regular executable file: %s", launcher)
	}
	serverName := developerMCPServerName + "-" + a.workspace.ID[:8]
	arguments := []string{"--workspace", a.workspace.Root, "mcp", "serve"}
	definition := MCPServerDefinition{Command: launcher, Args: arguments}
	projectClients := make([]MCPProjectIntegration, 0, len(mcpProjectClientSpecs))
	for _, spec := range mcpProjectClientSpecs {
		projectClients = append(projectClients, MCPProjectIntegration{
			ClientID:               spec.ID,
			ClientName:             spec.Name,
			Automatic:              true,
			ConfigPath:             filepath.Join(a.workspace.Root, filepath.FromSlash(spec.ConfigPath)),
			ServerName:             spec.ServerName,
			RequiresTrustedProject: spec.RequiresTrustedProject,
		})
	}
	return DeveloperMCPManifest{
		SchemaVersion: DeveloperMCPManifestSchemaVersion,
		ServerName:    serverName,
		Transport:     "stdio",
		WorkspaceID:   a.workspace.ID,
		WorkspaceRoot: a.workspace.Root,
		Command:       launcher,
		Args:          arguments,
		Codex: CodexProjectMCP{
			Automatic:              true,
			ConfigPath:             filepath.Join(a.workspace.Root, ".codex", "config.toml"),
			ServerName:             "hitkeep-dev",
			RequiresTrustedProject: true,
		},
		ProjectClients: projectClients,
		ClientConfig: MCPClientConfiguration{MCPServers: map[string]MCPServerDefinition{
			serverName: definition,
		}},
	}, nil
}
