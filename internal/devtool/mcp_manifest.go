package devtool

import (
	"fmt"
	"os"
	"path/filepath"
)

const DeveloperMCPManifestSchemaVersion = "hk.dev/mcp-manifest/v3"

const developerMCPServerName = "hitkeep-dev"

type MCPServerDefinition struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type MCPClientConfiguration struct {
	MCPServers map[string]MCPServerDefinition `json:"mcpServers"`
}

type DeveloperMCPManifest struct {
	SchemaVersion    string                 `json:"schema_version"`
	ServerName       string                 `json:"server_name"`
	Transport        string                 `json:"transport"`
	Scope            string                 `json:"scope"`
	WorkspaceRouting string                 `json:"workspace_routing"`
	Delegation       string                 `json:"delegation"`
	Notifications    []string               `json:"notifications"`
	Command          string                 `json:"command"`
	Args             []string               `json:"args"`
	ClientConfig     MCPClientConfiguration `json:"client_config"`
}

// MCPManifest returns a one-time central registration anchored to this
// checkout's portable launcher. The launcher builds hk locally for the host,
// while the server routes client roots to each workspace's own MCP process.
func (a *App) MCPManifest() (DeveloperMCPManifest, error) {
	launcher := filepath.Join(a.workspace.Root, "hk")
	info, err := os.Lstat(launcher)
	if err != nil {
		return DeveloperMCPManifest{}, fmt.Errorf("resolve hk launcher: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return DeveloperMCPManifest{}, fmt.Errorf("hk launcher must be a regular executable file: %s", launcher)
	}
	arguments := []string{"mcp", "serve"}
	definition := MCPServerDefinition{Command: launcher, Args: arguments}
	return DeveloperMCPManifest{
		SchemaVersion:    DeveloperMCPManifestSchemaVersion,
		ServerName:       developerMCPServerName,
		Transport:        "stdio",
		Scope:            "central",
		WorkspaceRouting: "client-roots",
		Delegation:       "workspace-mcp",
		Notifications:    []string{"progress", "logging"},
		Command:          definition.Command,
		Args:             arguments,
		ClientConfig: MCPClientConfiguration{MCPServers: map[string]MCPServerDefinition{
			developerMCPServerName: definition,
		}},
	}, nil
}
