---
id: "03-cli-flag-destination"
title: "Declare and enforce the CLI flag destination"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-001
acceptance_criteria:
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-001.1
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-001.2
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-001.3
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-001.4
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-001.5
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-001.6
system_design:
  - docs/specs/agents/system-design/agent-permission-control-integrity.md#cli-flag-destination
---

# Task 03: Declare and enforce the CLI flag destination

## Summary

Make a permission setting's launch-mode scope machine-readable, reject saving a
passthrough-only flag on an ACP profile with a message naming the ACP
equivalent, and state the destination process in the command preview.

## Scope

- Add a `PassthroughOnly` property (and the ACP-equivalent control reference) to
  `agents.PermissionSetting`. Set it on Claude's
  `dangerously_skip_permissions` setting, which already documents the constraint
  in prose.
- Save-time validation in `internal/agent/settings/controller`: reject a profile
  whose resolved enabled `cli_flags` tokens contain a passthrough-only flag
  while the profile does not use CLI passthrough. Validate the output of
  `cliflags.Resolve`, so a multi-token entry is covered. The error names the ACP
  equivalent.
- `internal/agent/settings/controller/agent_config.go`: the command preview
  response carries the destination of the launched process for the flag
  segment.
- Frontend: `CliFlagsField` renders the destination note and the per-row
  blocking warning; `CommandPreviewCard` renders the destination label; the save
  action is disabled while a blocking flag is enabled, in both
  `AgentProfilePage` and `CliProfileEditor`.

## Exclusions

- No change to `CommandBuilder.BuildCommand`. Non-restricted flags keep landing
  on the launched process argv; the defect was the claim, not the append.
- No new per-agent argv forwarding channel into the wrapped agent CLI. No
  supported bridge offers a generic passthrough today, and a speculative one
  would be untestable.
- No change to `command_prefix` handling.

## ASCII UI preview

See [UI-01 in the plan](plan.md#ui-01-profile-editor-cli-flags-section-work-order-03).
Structural requirements: the destination note sits above the flag list, the
blocking warning sits inline under its flag row, the save-blocking message sits
directly above the save action, and the command preview names the launched
process. Phone uses the same single-column stack with wrapped warnings.

## Acceptance

1. Saving a Claude ACP profile with `--dangerously-skip-permissions` enabled is
   rejected with a message naming the permission mode control; the stored
   profile is unchanged.
2. Saving the same flag on a Claude CLI-passthrough profile succeeds and the
   built passthrough argv contains the flag.
3. The command preview names the launched process for the flag segment, and the
   profile editor shows the destination note without hover-only text or
   horizontal scrolling at narrow widths.

## Files likely touched

- `apps/backend/internal/agent/agents/agent.go`
- `apps/backend/internal/agent/agents/claude_acp.go`
- `apps/backend/internal/agent/settings/controller/profile_crud.go`
- `apps/backend/internal/agent/settings/controller/agent_config.go`
- `apps/backend/internal/agent/settings/cliflags/tokenise.go`
- `apps/web/components/settings/cli-flags-field.tsx`
- `apps/web/app/settings/agents/[agentId]/profiles/[profileId]/command-preview-card.tsx`
- `apps/web/components/settings/agent-profile-page.tsx`
- `apps/web/components/agent/cli-profile-editor.tsx`
- `apps/web/src/locales/*/agents.json` (six locales plus `pseudo`)

## TDD sequence

1. Add failing backend tests: saving a passthrough-only flag on an ACP profile
   returns a validation error naming the equivalent control; the same save on a
   passthrough profile succeeds; the preview response carries the destination.
2. Add a failing frontend test that the flags field renders the destination note
   and blocks save for a restricted flag.
3. Run the focused commands and confirm the expected failures.
4. Implement the declaration, validation, preview field, and UI.
5. Add locale entries in all six languages plus `pseudo`; run the i18n checks.
6. Re-run the focused commands; all pass.

## Verification

```bash
cd "$(git rev-parse --show-toplevel)/apps/backend" && go test ./internal/agent/agents/... ./internal/agent/settings/... -race -count=1
cd "$(git rev-parse --show-toplevel)/apps/web" && pnpm vitest run components/settings && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet
```

## Dependencies

None.

## Risks

An existing stored profile can hold a now-rejected flag. Validation is
write-only, so launches are unaffected; the message appears on next edit. Cover
that with a test so the constraint is not accidentally applied at launch.

## Results

Pending implementation.
