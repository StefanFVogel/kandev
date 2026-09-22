package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle/initialmode"
	agentruntime "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// initialModeOutcome records what happened when Kandev tried to give an agent
// process the mode it should start in.
type initialModeOutcome struct {
	// Mode is the effective session mode Kandev wanted to deliver.
	Mode string
	// Delivered is true when the launched process is configured with Mode from
	// its first turn, rather than only switched afterwards.
	Delivered bool
	// Reason explains a mode that could not be delivered.
	Reason string
}

// applyInitialMode configures the launch environment so the agent process
// starts in the effective session mode.
//
// Applying a mode after session/new is not equivalent: the agent is told about
// the new mode while the process keeps enforcing what it was launched with. The
// post-creation switch stays in place for agents without a declared channel and
// for later mode changes; this is what makes the mode true at the first turn.
//
// Nothing happens when no mode is requested. A profile that leaves the mode
// empty keeps today's launch environment byte for byte, which bounds this to
// the sessions that actually ask for a mode.
func (m *Manager) applyInitialMode(
	env map[string]string,
	executionID string,
	agentConfig agents.Agent,
	mode string,
	executorType string,
) initialModeOutcome {
	if mode == "" {
		return initialModeOutcome{}
	}
	outcome := initialModeOutcome{Mode: mode}
	if agentConfig == nil || env == nil {
		outcome.Reason = "no agent configuration for this launch"
		return outcome
	}
	runtimeCfg := agentConfig.Runtime()
	if runtimeCfg == nil {
		outcome.Reason = "no agent configuration for this launch"
		return outcome
	}
	delivery := runtimeCfg.InitialMode
	if !delivery.Delivers(mode) {
		outcome.Reason = "agent declares no start-mode channel for " + mode
		return outcome
	}

	settingsValue, _ := delivery.SettingsValue(mode)
	configDir, err := m.materializeInitialModeConfigDir(executionID, runtimeCfg, delivery, settingsValue)
	if err != nil {
		outcome.Reason = err.Error()
		m.logger.Warn("could not deliver the session mode at start",
			zap.String("execution_id", executionID),
			zap.String("mode", mode),
			zap.Error(err))
		return outcome
	}

	env[delivery.ConfigDirEnvVar] = configDir
	// A permissive mode can be disabled for the launched process identity. A
	// container is the isolation that escape hatch exists for, so declare it
	// there rather than leaving the mode silently downgraded.
	if containerRuntimeNeedsSandboxDeclaration(executorType) {
		for key, value := range delivery.SandboxEnv {
			if _, present := env[key]; !present {
				env[key] = value
			}
		}
	}

	outcome.Delivered = true
	m.logger.Info("delivered session mode at start",
		zap.String("execution_id", executionID),
		zap.String("mode", mode),
		zap.String("config_dir_env", delivery.ConfigDirEnvVar))
	return outcome
}

// materializeInitialModeConfigDir prepares the per-session configuration
// directory and returns the path to hand the agent.
func (m *Manager) materializeInitialModeConfigDir(
	executionID string,
	runtimeCfg *agents.RuntimeConfig,
	delivery agents.InitialModeDelivery,
	settingsValue string,
) (string, error) {
	target := SessionDirHostPath(m.dataDir, executionID, runtimeCfg.SessionConfig.SessionDirTemplate)
	if target == "" {
		target = filepath.Join(InstanceSessionRoot(m.dataDir, executionID), "agent-config")
	}
	return initialmode.Materialize(initialmode.Request{
		SourceDir:        resolveAgentConfigDir(delivery),
		TargetDir:        target,
		SettingsFileName: delivery.SettingsFileName,
		ModeKeyPath:      delivery.ModeKeyPath,
		ModeValue:        settingsValue,
	})
}

// resolveAgentConfigDir locates the user's existing agent configuration
// directory: the agent's own environment variable when the operator set one,
// otherwise the agent's documented default under the user's home.
func resolveAgentConfigDir(delivery agents.InitialModeDelivery) string {
	if delivery.ConfigDirEnvVar != "" {
		if existing := strings.TrimSpace(os.Getenv(delivery.ConfigDirEnvVar)); existing != "" {
			return existing
		}
	}
	if delivery.DefaultConfigDirTemplate == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, strings.TrimPrefix(delivery.DefaultConfigDirTemplate, "{home}/"))
}

// containerRuntimeNeedsSandboxDeclaration reports whether this executor runs
// the agent inside container isolation, where a permissive mode would otherwise
// be disabled for the container's root process identity.
func containerRuntimeNeedsSandboxDeclaration(executorType string) bool {
	switch models.ExecutorType(executorType).Runtime() {
	case agentruntime.RuntimeDocker, agentruntime.RuntimeRemoteDocker, agentruntime.RuntimeKubernetes:
		return true
	default:
		return false
	}
}

// launchSessionMode resolves the mode this launch should start in.
//
// It mirrors effectiveSessionMode's precedence: a persisted session mode — set
// by the user's toggle or a set_session_mode workflow action — wins over the
// agent profile's mode. Reading it here rather than after session/new is the
// point: the value has to be known before the process starts.
func (m *Manager) launchSessionMode(ctx context.Context, req *LaunchRequest, profileInfo *AgentProfileInfo) string {
	profileMode := ""
	if profileInfo != nil {
		profileMode = profileInfo.Mode
	}
	if req == nil || m.workspaceInfoProvider == nil || req.SessionID == "" {
		return profileMode
	}
	info, err := m.workspaceInfoProvider.GetWorkspaceInfoForSession(ctx, req.TaskID, req.SessionID)
	if err != nil || info == nil || info.SessionMode == "" {
		return profileMode
	}
	return info.SessionMode
}
