---
spec: docs/specs/agents/requirements/agent-permission-control-integrity.md
created: 2026-09-22
status: pending
---

# Implementation Plan: Agent Permission Control Integrity

## Overview

A field report on v0.94.0 describes three independent defects in agent-session
permission control. All three share one shape: Kandev reports a control as
applied after writing it somewhere, not after the component that enforces it has
observed it.

This package makes each control either effective or honestly refused, and proves
the result in both directions: an unattended-permission profile must run a
state-changing Git command without a human, and the default profile must still
not.

## Specifications

| Document | Role |
| --- | --- |
| [`agent-permission-control-integrity.md`](../../specs/agents/requirements/agent-permission-control-integrity.md) | Requirements for the three profile permission controls |
| [`agent-permission-control-integrity.md`](../../specs/agents/system-design/agent-permission-control-integrity.md) | Design for flag destination, mode confirmation, auto-approve selection, contract cleanup, evidence |
| [`mcp-create-task-agent-profile-validation.md`](../../specs/tasks/requirements/mcp-create-task-agent-profile-validation.md) | Requirements for `create_task_kandev` profile validation |
| [`mcp-create-task-agent-profile-validation.md`](../../specs/tasks/system-design/mcp-create-task-agent-profile-validation.md) | Design for synchronous validation and async launch-failure visibility |

## Confirmed root causes

**Report defect 1 — CLI flags never reach the agent.**
`lifecycle.CommandBuilder.BuildCommand` appends the profile's enabled
`cli_flags` to whatever `Agent.BuildCommand` returned. For every ACP agent that
is the bridge argv (`npx --yes --prefer-offline <bridge package>`), and no
supported bridge forwards unrecognized argv to the agent CLI it wraps. Only CLI
passthrough puts the flags on the real agent binary. Claude's
`dangerously_skip_permissions` permission setting already documents itself as
passthrough-only in prose, but nothing enforces that, so the flag can be saved
on an ACP profile and silently lands on the bridge.

**Report defect 2 — permission controls do not change enforcement.** Four
separate Kandev-side faults:

- **Kandev never configures the agent process with a permission mode.**
  `acp.Adapter.NewSession` sends only `Cwd` and `McpServers`; no `_meta`, no
  agent settings. The process starts in the runtime's default mode and Kandev
  issues `session/set_mode` afterwards. For the bundled Claude bridge that
  switch is a mid-session control request, which is exactly the shape the report
  measured: the mode reaches the agent's instruction layer while the process
  keeps enforcing what it started with. This is the fault that matches the
  reported symptom; work order 07 addresses it.
- `acp.Adapter.SetMode` emits its session-mode event from the **requested** mode
  and the cached mode list, never from the agent's reported current mode. A
  clamped or ineffective mode is logged identically to an applied one.
- The effective launch mode comes from `Manager.effectiveSessionMode`, where a
  persisted `session_mode` session-metadata value (written by the user toggle
  and by the workflow `set_session_mode` action, whose apply failure is
  downgraded to Debug) wins over the profile mode. Nothing records which source
  won.
- `process.Manager.autoApprovePermission` selects `req.Options[0]` when no
  option declares an allow kind, and answers `Cancelled` when the list is empty.
  Both return before `sendPermissionNotification`, so the agent sees a refusal,
  the user sees no prompt, and no permission request exists to inspect.
  `acp.Client.RequestPermission` has the same empty-option shortcut before the
  handler, and `acp.Adapter.handlePermissionRequest` has the same `Options[0]`
  fallback when no handler is installed.

Also confirmed: `approval_policy`, derived from the profile's `auto_approve` in
`resolveApprovalPolicyAndDisplayName`, is transmitted, stored on
`config.InstanceConfig.ApprovalPolicy`, logged, and read by nothing.
`auto_approve` itself reaches agentctl correctly through
`ExecutorCreateRequest.AutoApprovePermissions`.

**Report defect 3 — `agent_profile_id: "current_task"` creates a task with no
session.** `current_task` is a value of the per-user setting
`mcp_task_agent_profile_default`, not an argument value, but the tool
description names it inside the `agent_profile_id` property description. The
string is passed through unvalidated, counts as an explicit profile (defeating
every inheritance and workspace-default fallback), and fails in
`runtime.ValidateProfile` inside `launchAutoStartTask`'s fire-and-forget
goroutine, which logs the error and returns. The tool already reported success.

### Where the denial comes from

The refusals are raised by the running agent itself, not by a Bash-tool
permission request that Kandev answers. That is consistent with the report's
measurement of zero permission requests for those sessions, and it means the
auto-approve faults above are a second, independent defect rather than the cause
of the reported symptom.

The agent denies internally because it is still running under the permission
mode it was launched with. Kandev supplies no mode at session creation, so the
process starts in the runtime default; the later `session/set_mode` changes what
the agent is told, and the report measured that it does not change what the
process enforces. Work order 07 closes that gap.

What this package does not claim: that the runtime's mid-session mode switch is
itself broken. That is the report's measurement, taken as evidence, and it
belongs to the provider. The Kandev-side gap — no initial mode at all — is
provable from the launch path and is the right fix either way. Work order 06 is
the check that decides whether anything survives it.

## Work orders

| Order | Title | Wave | Depends on |
| --- | --- | --- | --- |
| [`task-07-initial-session-mode.md`](task-07-initial-session-mode.md) | Deliver the permission mode at session start | 1 | — |
| [`task-01-auto-approve-never-denies.md`](task-01-auto-approve-never-denies.md) | Auto-approve approves or prompts | 1 | — |
| [`task-02-confirm-session-mode.md`](task-02-confirm-session-mode.md) | Confirm and attribute the applied session mode | 1 | — |
| [`task-03-cli-flag-destination.md`](task-03-cli-flag-destination.md) | Declare and enforce the CLI flag destination | 1 | — |
| [`task-04-remove-dead-approval-policy.md`](task-04-remove-dead-approval-policy.md) | Remove the unread `approval_policy` field | 1 | — |
| [`task-05-mcp-create-task-profile-validation.md`](task-05-mcp-create-task-profile-validation.md) | Validate `create_task_kandev` agent profile | 1 | — |
| [`task-06-unattended-permission-evidence.md`](task-06-unattended-permission-evidence.md) | Prove unattended and attended behavior end to end | 2 | 07, 01, 02 |

Work order 07 is the one that addresses the reported symptom. Start there.

Wave 1 work orders touch disjoint files and have no shared schema, generated
contract, or package config. That is planning information only; execute
sequentially unless the user explicitly authorizes subagents.

## ASCII UI previews

Two work orders change rendered UI. Spacing here is illustrative; the structural
requirements are the control order, the destination label, and the warning
placement. Copy is localized (`agents`, `settings`, `task` namespaces); the
English strings below are placeholders for the catalog keys the work orders add.

### UI-01: Profile editor, CLI flags section (work order 03)

Entry point: Settings → Agents → *agent* → Profiles → *profile*. State: a
passthrough-only flag enabled on a profile that launches over ACP.

Current behavior (save succeeds, flag silently lands on the bridge):

```text
+-- Agent CLI flags -------------------------------- 1 of 3 enabled --+
| [x] --dangerously-skip-permissions                            [Del] |
|     Pass --dangerously-skip-permissions so Claude Code does not     |
|     prompt for tool approvals.                                      |
| [ ] --verbose                                                 [Del] |
|                                            [ + Add flag ]           |
+---------------------------------------------------------------------+
                                                       [ Save profile ]
```

Proposed:

```text
+-- Agent CLI flags -------------------------------- 1 of 3 enabled --+
| Flags are passed to the launched ACP bridge process, not to the     |
| agent CLI it wraps.                                                 |
|                                                                     |
| [x] --dangerously-skip-permissions                            [Del] |
|     (!) Not available over ACP. Use Start mode > Bypass             |
|         permissions instead.                                        |
| [ ] --verbose                                                 [Del] |
|                                            [ + Add flag ]           |
+---------------------------------------------------------------------+
  (!) Remove or disable --dangerously-skip-permissions to save.
                                                       [ Save profile ]
                                                        ^ disabled
```

Command preview card, same page, gains a destination line:

```text
+-- Command preview --------------------------------------------[Copy]+
| Launched process (ACP bridge):                                      |
|   npx --yes --prefer-offline @agentclientprotocol/claude-agent-acp  |
|   --verbose                                                         |
+---------------------------------------------------------------------+
```

Phone composition is the same single-column stack; the inline flag warning wraps
under its row and the save-blocking message stays directly above the sticky save
action. No hover-only information, no horizontal scrolling.

### UI-02: Session mode warning (work order 02)

Entry point: task chat. State: the profile requested `bypassPermissions` and the
agent clamped it.

```text
  +-------------------------------------------------------------+
  | (!) Requested mode "Bypass permissions" is unavailable in    |
  |     this session. The session is running in "Manual".        |
  +-------------------------------------------------------------+

  [ ... conversation ... ]

  +-- chat input ----------------------------------------------+
  | > _                                                         |
  | [Model v]  [Mode: Manual v]                        [ Send ] |
  +-------------------------------------------------------------+
```

The mode selector shows the agent's reported mode, not the requested one. On
phone the same warning renders full width above the transcript and the selector
stays in the mobile toolbar row.

## Risks

- **Auto-approve fall-through changes unattended behavior.** A session that
  previously proceeded on an arbitrary option now blocks on a prompt. That is
  the intended correction, but it can surface as a task that stalls where it
  used to (wrongly) continue. Work order 01 covers it with explicit transcript
  entries so the stall is visible rather than mysterious.
- **Mode confirmation depends on a provider notification.** A provider that
  never publishes `current_mode_update` yields an unconfirmed result. The design
  warns and continues rather than failing the launch; the settle window must not
  add perceptible launch latency.
- **Save-time flag rejection can invalidate stored profiles.** Validation is
  write-only, so existing profiles keep launching; the message appears on next
  edit.
- **`approval_policy` removal crosses the backend/agentctl binary boundary.**
  The field is unread, and the receiving DTO keeps accepting it, so version skew
  in either direction is safe. The work order pins that with a test.

## Verification strategy

Each work order owns its targeted commands. The package is complete when work
order 06's two directions both pass; unit coverage alone does not close the
reported workflow failure.

## Verification Results

Pending implementation.
