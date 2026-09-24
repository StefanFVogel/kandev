// Package initialmode materializes the per-session agent configuration
// directory used to give an agent process its permission mode before it starts.
package initialmode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Request describes one materialization.
type Request struct {
	// SourceDir is the user's existing agent configuration directory. It may
	// not exist; then only the settings file is written.
	SourceDir string
	// TargetDir is the Kandev-owned per-session directory to materialize.
	TargetDir string
	// SettingsFileName is the settings file inside both directories.
	SettingsFileName string
	// ModeKeyPath is the nested key path that carries the mode.
	ModeKeyPath []string
	// ModeValue is the agent-specific value to write at that path.
	ModeValue string
	// LinkSourceEntries mirrors SourceDir into TargetDir with symlinks. A
	// container reads TargetDir through a bind mount, where a link to a host
	// path resolves to nothing, so those launches set this false and receive
	// the settings file alone.
	LinkSourceEntries bool
}

// Materialize prepares TargetDir so an agent started against it behaves exactly
// like one started against SourceDir, except that its settings file carries the
// requested mode.
//
// Every entry of SourceDir other than the settings file is linked rather than
// copied, when the caller asks for it. The configuration directory holds the agent's credentials, and a
// session must not get a stale duplicate of them: a token the agent refreshes
// has to land in the real file, and a secret must not be multiplied across one
// directory per session. The settings file itself is the one entry Kandev owns,
// so it is written as a merge of the user's content and the mode.
//
// Returns the directory to hand the agent.
func Materialize(req Request) (string, error) {
	if req.TargetDir == "" {
		return "", fmt.Errorf("initialmode: target directory is required")
	}
	if req.SettingsFileName == "" || len(req.ModeKeyPath) == 0 {
		return "", fmt.Errorf("initialmode: settings file and mode key path are required")
	}
	if err := os.MkdirAll(req.TargetDir, 0o700); err != nil {
		return "", fmt.Errorf("initialmode: create %s: %w", req.TargetDir, err)
	}
	if req.LinkSourceEntries {
		if err := linkSourceEntries(req); err != nil {
			return "", err
		}
	}
	if err := writeSettings(req); err != nil {
		return "", err
	}
	return req.TargetDir, nil
}

// linkSourceEntries mirrors SourceDir into TargetDir with symlinks. A missing
// source directory is not an error: a first-run agent simply has no config yet.
func linkSourceEntries(req Request) error {
	entries, err := os.ReadDir(req.SourceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("initialmode: read %s: %w", req.SourceDir, err)
	}
	for _, entry := range entries {
		if entry.Name() == req.SettingsFileName {
			continue
		}
		target := filepath.Join(req.TargetDir, entry.Name())
		if _, err := os.Lstat(target); err == nil {
			continue
		}
		source := filepath.Join(req.SourceDir, entry.Name())
		if err := os.Symlink(source, target); err != nil {
			// A link that cannot be created (Windows without privileges, a
			// racing session) must not fail the launch: the agent degrades to
			// missing that entry, which is visible, rather than not starting.
			return fmt.Errorf("initialmode: link %s: %w", entry.Name(), err)
		}
	}
	return nil
}

// writeSettings writes TargetDir's settings file: the user's existing content
// with the mode set at ModeKeyPath. Unreadable or malformed source settings are
// replaced rather than propagated, because the agent would reject them anyway.
func writeSettings(req Request) error {
	settings := map[string]any{}
	if req.SourceDir != "" {
		if raw, err := os.ReadFile(filepath.Join(req.SourceDir, req.SettingsFileName)); err == nil {
			decoded := map[string]any{}
			if json.Unmarshal(raw, &decoded) == nil {
				settings = decoded
			}
		}
	}
	setNested(settings, req.ModeKeyPath, req.ModeValue)

	encoded, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("initialmode: encode settings: %w", err)
	}
	path := filepath.Join(req.TargetDir, req.SettingsFileName)
	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
		return fmt.Errorf("initialmode: write %s: %w", path, err)
	}
	return nil
}

// setNested assigns value at the nested key path, replacing any non-object it
// has to traverse.
func setNested(root map[string]any, path []string, value string) {
	current := root
	for _, key := range path[:len(path)-1] {
		next, ok := current[key].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[key] = next
		}
		current = next
	}
	current[path[len(path)-1]] = value
}
