package initialmode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func claudeRequest(source, target string) Request {
	return Request{
		SourceDir:        source,
		TargetDir:        target,
		SettingsFileName: "settings.json",
		ModeKeyPath:      []string{"permissions", "defaultMode"},
		ModeValue:        "bypassPermissions",
		// The host launch reads the directory directly, so it keeps the links
		// to the user's real configuration.
		LinkSourceEntries: true,
	}
}

func readSettings(t *testing.T, dir string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	decoded := map[string]any{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	return decoded
}

func modeOf(t *testing.T, settings map[string]any) string {
	t.Helper()
	permissions, ok := settings["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("settings carry no permissions object: %+v", settings)
	}
	mode, _ := permissions["defaultMode"].(string)
	return mode
}

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.7
func TestMaterializeWritesModeIntoSettings(t *testing.T) {
	source := t.TempDir()
	target := filepath.Join(t.TempDir(), "session")

	dir, err := Materialize(claudeRequest(source, target))
	if err != nil {
		t.Fatalf("Materialize returned error: %v", err)
	}
	if dir != target {
		t.Fatalf("dir = %q, want %q", dir, target)
	}
	if got := modeOf(t, readSettings(t, target)); got != "bypassPermissions" {
		t.Fatalf("defaultMode = %q, want bypassPermissions", got)
	}
}

// The user's own settings must survive; only the mode is Kandev's to set.
func TestMaterializeMergesExistingSettings(t *testing.T) {
	source := t.TempDir()
	existing := `{"model":"opus","permissions":{"allow":["Bash(git status:*)"]},"env":{"FOO":"bar"}}`
	if err := os.WriteFile(filepath.Join(source, "settings.json"), []byte(existing), 0o600); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	target := filepath.Join(t.TempDir(), "session")

	if _, err := Materialize(claudeRequest(source, target)); err != nil {
		t.Fatalf("Materialize returned error: %v", err)
	}

	settings := readSettings(t, target)
	if settings["model"] != "opus" {
		t.Fatalf("model = %v, want opus", settings["model"])
	}
	permissions := settings["permissions"].(map[string]any)
	allow, ok := permissions["allow"].([]any)
	if !ok || len(allow) != 1 || allow[0] != "Bash(git status:*)" {
		t.Fatalf("allow list not preserved: %+v", permissions["allow"])
	}
	if got := modeOf(t, settings); got != "bypassPermissions" {
		t.Fatalf("defaultMode = %q, want bypassPermissions", got)
	}
}

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.8
// The configuration directory holds the agent's credentials. A session must
// reach the real file, not a stale duplicate, or a refreshed token is lost and
// the secret is multiplied across one directory per session.
func TestMaterializeLinksCredentialsRatherThanCopying(t *testing.T) {
	source := t.TempDir()
	credentials := filepath.Join(source, ".credentials.json")
	if err := os.WriteFile(credentials, []byte(`{"token":"original"}`), 0o600); err != nil {
		t.Fatalf("seed credentials: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(source, "plugins"), 0o700); err != nil {
		t.Fatalf("seed plugins dir: %v", err)
	}
	target := filepath.Join(t.TempDir(), "session")

	if _, err := Materialize(claudeRequest(source, target)); err != nil {
		t.Fatalf("Materialize returned error: %v", err)
	}

	linked := filepath.Join(target, ".credentials.json")
	info, err := os.Lstat(linked)
	if err != nil {
		t.Fatalf("lstat linked credentials: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("credentials were copied; they must be linked so a refresh reaches the real file")
	}

	// A write through the link must land in the source.
	if err := os.WriteFile(linked, []byte(`{"token":"refreshed"}`), 0o600); err != nil {
		t.Fatalf("write through link: %v", err)
	}
	raw, err := os.ReadFile(credentials)
	if err != nil {
		t.Fatalf("read source credentials: %v", err)
	}
	if string(raw) != `{"token":"refreshed"}` {
		t.Fatalf("source credentials = %s, want the refreshed value", raw)
	}

	if _, err := os.Lstat(filepath.Join(target, "plugins")); err != nil {
		t.Fatalf("directory entry not linked: %v", err)
	}
}

// A first run has no configuration directory yet; that is not a failure.
func TestMaterializeWithoutSourceDirectory(t *testing.T) {
	target := filepath.Join(t.TempDir(), "session")

	if _, err := Materialize(claudeRequest(filepath.Join(t.TempDir(), "absent"), target)); err != nil {
		t.Fatalf("Materialize returned error: %v", err)
	}
	if got := modeOf(t, readSettings(t, target)); got != "bypassPermissions" {
		t.Fatalf("defaultMode = %q, want bypassPermissions", got)
	}
}

// Malformed user settings must not block a launch.
func TestMaterializeReplacesMalformedSettings(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "settings.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	target := filepath.Join(t.TempDir(), "session")

	if _, err := Materialize(claudeRequest(source, target)); err != nil {
		t.Fatalf("Materialize returned error: %v", err)
	}
	if got := modeOf(t, readSettings(t, target)); got != "bypassPermissions" {
		t.Fatalf("defaultMode = %q, want bypassPermissions", got)
	}
}

// Re-materializing an existing session directory must be idempotent.
func TestMaterializeIsRepeatable(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, ".credentials.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatalf("seed credentials: %v", err)
	}
	target := filepath.Join(t.TempDir(), "session")

	for attempt := range 2 {
		if _, err := Materialize(claudeRequest(source, target)); err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}
	if got := modeOf(t, readSettings(t, target)); got != "bypassPermissions" {
		t.Fatalf("defaultMode = %q, want bypassPermissions", got)
	}
}

func TestMaterializeRejectsIncompleteRequest(t *testing.T) {
	if _, err := Materialize(Request{TargetDir: t.TempDir()}); err == nil {
		t.Fatal("expected an error for a request without a settings file or mode key path")
	}
	if _, err := Materialize(Request{SettingsFileName: "settings.json", ModeKeyPath: []string{"a"}}); err == nil {
		t.Fatal("expected an error for a request without a target directory")
	}
}

// A container reaches this directory through a bind mount, where a symlink to
// a host path resolves to nothing. Linking the user's entries there produced a
// directory full of dangling links instead of a usable configuration.
func TestMaterializeSkipsSourceLinksWhenNotRequested(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, ".credentials.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	target := filepath.Join(t.TempDir(), "session")

	req := claudeRequest(source, target)
	req.LinkSourceEntries = false
	if _, err := Materialize(req); err != nil {
		t.Fatalf("materialize: %v", err)
	}

	if _, err := os.Lstat(filepath.Join(target, ".credentials.json")); !os.IsNotExist(err) {
		t.Fatalf("credentials entry = %v, want it absent from a container directory", err)
	}
	if got := readSettings(t, target); got == nil {
		t.Fatal("settings file missing; the mode must still be delivered")
	}
}
