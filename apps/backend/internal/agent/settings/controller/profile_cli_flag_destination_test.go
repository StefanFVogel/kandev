package controller

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-001.3, .5
// Over ACP the launched process is the bridge, which forwards no unrecognized
// argument to the CLI it wraps. Accepting the flag there changed nothing while
// the editor reported it as enabled.
func TestPassthroughOnlyFlagRejectedOnACPProfile(t *testing.T) {
	err := validatePassthroughOnlyCLIFlags(
		agents.NewClaudeACP(),
		[]dto.CLIFlagDTO{{Flag: "--dangerously-skip-permissions", Enabled: true}},
		false,
	)

	if !errors.Is(err, ErrPassthroughOnlyCLIFlag) {
		t.Fatalf("err = %v, want ErrPassthroughOnlyCLIFlag", err)
	}
	// A refusal without a next step is not actionable.
	if got := err.Error(); !contains(got, "permission mode") {
		t.Fatalf("error = %q, want it to name the ACP equivalent", got)
	}
}

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-001.6
func TestPassthroughOnlyFlagAllowedOnPassthroughProfile(t *testing.T) {
	if err := validatePassthroughOnlyCLIFlags(
		agents.NewClaudeACP(),
		[]dto.CLIFlagDTO{{Flag: "--dangerously-skip-permissions", Enabled: true}},
		true,
	); err != nil {
		t.Fatalf("err = %v, want the flag accepted in passthrough mode", err)
	}
}

// A disabled entry is not applied to any launch, so it is not refused.
func TestDisabledPassthroughOnlyFlagIsAccepted(t *testing.T) {
	if err := validatePassthroughOnlyCLIFlags(
		agents.NewClaudeACP(),
		[]dto.CLIFlagDTO{{Flag: "--dangerously-skip-permissions", Enabled: false}},
		false,
	); err != nil {
		t.Fatalf("err = %v, want a disabled entry accepted", err)
	}
}

// The check judges resolved tokens, so a restricted flag cannot hide behind
// its neighbours in a multi-token entry.
func TestPassthroughOnlyFlagDetectedInMultiTokenEntry(t *testing.T) {
	err := validatePassthroughOnlyCLIFlags(
		agents.NewClaudeACP(),
		[]dto.CLIFlagDTO{{Flag: "--verbose --dangerously-skip-permissions", Enabled: true}},
		false,
	)
	if !errors.Is(err, ErrPassthroughOnlyCLIFlag) {
		t.Fatalf("err = %v, want the restricted token found among its neighbours", err)
	}
}

// Unrestricted flags keep working; they legitimately configure the launched
// bridge process.
func TestUnrestrictedFlagsRemainAccepted(t *testing.T) {
	if err := validatePassthroughOnlyCLIFlags(
		agents.NewClaudeACP(),
		[]dto.CLIFlagDTO{{Flag: "--verbose", Enabled: true}},
		false,
	); err != nil {
		t.Fatalf("err = %v, want an unrestricted flag accepted", err)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
