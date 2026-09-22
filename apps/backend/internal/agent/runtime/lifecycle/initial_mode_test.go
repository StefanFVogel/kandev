package lifecycle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
)

func claudeACPAgent(t *testing.T) agents.Agent {
	t.Helper()
	return agents.NewClaudeACP()
}

func initialModeManager(t *testing.T) *Manager {
	t.Helper()
	return &Manager{dataDir: t.TempDir(), logger: newTestLogger()}
}

func deliveredSettingsMode(t *testing.T, configDir string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(configDir, "settings.json"))
	if err != nil {
		t.Fatalf("read delivered settings: %v", err)
	}
	decoded := map[string]any{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode delivered settings: %v", err)
	}
	permissions, ok := decoded["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("delivered settings carry no permissions object: %+v", decoded)
	}
	mode, _ := permissions["defaultMode"].(string)
	return mode
}

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.7, .10
func TestApplyInitialModeConfiguresLaunchedProcess(t *testing.T) {
	m := initialModeManager(t)
	env := map[string]string{}

	outcome := m.applyInitialMode(env, "exec-1", claudeACPAgent(t), "bypassPermissions", "worktree")

	if !outcome.Delivered {
		t.Fatalf("outcome = %+v, want delivered", outcome)
	}
	configDir := env["CLAUDE_CONFIG_DIR"]
	if configDir == "" {
		t.Fatal("CLAUDE_CONFIG_DIR was not exported to the launch environment")
	}
	if got := deliveredSettingsMode(t, configDir); got != "bypassPermissions" {
		t.Fatalf("delivered mode = %q, want bypassPermissions", got)
	}
}

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.8
// The per-session directory lives under the Kandev root, never in the user's
// shared configuration directory.
func TestApplyInitialModeWritesOnlyUnderKandevRoot(t *testing.T) {
	m := initialModeManager(t)
	env := map[string]string{}

	m.applyInitialMode(env, "exec-1", claudeACPAgent(t), "acceptEdits", "worktree")

	configDir := env["CLAUDE_CONFIG_DIR"]
	rel, err := filepath.Rel(m.dataDir, configDir)
	if err != nil || rel == "" || rel[0] == '.' {
		t.Fatalf("config dir %q is not inside the kandev root %q", configDir, m.dataDir)
	}
}

// A profile that requests no mode must leave the launch environment untouched,
// which bounds this feature to sessions that actually ask for a mode.
func TestApplyInitialModeIsInertWithoutRequestedMode(t *testing.T) {
	m := initialModeManager(t)
	env := map[string]string{"EXISTING": "value"}

	outcome := m.applyInitialMode(env, "exec-1", claudeACPAgent(t), "", "worktree")

	if outcome.Mode != "" || outcome.Delivered {
		t.Fatalf("outcome = %+v, want an inert result", outcome)
	}
	if len(env) != 1 || env["EXISTING"] != "value" {
		t.Fatalf("env = %+v, want it unchanged", env)
	}
}

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.9, .11
// A container runs the agent as root, where the bundled bridge disables the
// permissive mode unless the sandbox is declared.
func TestApplyInitialModeDeclaresSandboxForContainerExecutors(t *testing.T) {
	for executorType, wantSandbox := range map[string]bool{
		"local_docker": true,
		"k8s":          true,
		"worktree":     false,
		"local_pc":     false,
	} {
		t.Run(executorType, func(t *testing.T) {
			m := initialModeManager(t)
			env := map[string]string{}

			m.applyInitialMode(env, "exec-1", claudeACPAgent(t), "bypassPermissions", executorType)

			_, present := env["IS_SANDBOX"]
			if present != wantSandbox {
				t.Fatalf("IS_SANDBOX present = %v, want %v for executor %q", present, wantSandbox, executorType)
			}
		})
	}
}

// An agent without a declared channel keeps the post-creation switch and says
// so, rather than reporting a delivery that did not happen.
func TestApplyInitialModeReportsMissingChannel(t *testing.T) {
	m := initialModeManager(t)
	env := map[string]string{}

	outcome := m.applyInitialMode(env, "exec-1", &agentWithoutInitialMode{}, "bypassPermissions", "worktree")

	if outcome.Delivered {
		t.Fatalf("outcome = %+v, want a non-delivered result", outcome)
	}
	if outcome.Reason == "" {
		t.Fatal("a non-delivered mode must carry a reason")
	}
	if len(env) != 0 {
		t.Fatalf("env = %+v, want it untouched for an agent without a channel", env)
	}
}

// A mode the agent's channel cannot express is not delivered silently.
func TestApplyInitialModeRejectsUnknownMode(t *testing.T) {
	m := initialModeManager(t)
	env := map[string]string{}

	outcome := m.applyInitialMode(env, "exec-1", claudeACPAgent(t), "totally-unknown", "worktree")

	if outcome.Delivered {
		t.Fatalf("outcome = %+v, want a non-delivered result", outcome)
	}
	if _, present := env["CLAUDE_CONFIG_DIR"]; present {
		t.Fatal("an undeliverable mode must not redirect the configuration directory")
	}
}

type agentWithoutInitialMode struct {
	agents.Agent
}

func (a *agentWithoutInitialMode) Runtime() *agents.RuntimeConfig {
	return &agents.RuntimeConfig{}
}
