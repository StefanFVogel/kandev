---
status: draft
system: agents
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-001
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-002
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-003
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-004
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-005
---

# Agent Permission Control Integrity System Design

## Purpose and boundaries

This design covers the three profile permission controls (`cli_flags`, `mode`,
`auto_approve`) from the point where a profile is saved to the point where the
agent process observes the control. It owns the Kandev side of that path only.

It does not own the installed agent CLI's own permission classification. Which
commands a provider treats as requiring approval, and which requests remain
bypass-immune in a permissive mode, stay with the provider. This design makes
Kandev's own layer honest and observable so a provider decision is attributable
to the provider rather than to an unverifiable Kandev claim.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-001` | [CLI flag destination](#cli-flag-destination) |
| `REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-002` | [Mode confirmation and attribution](#mode-confirmation-and-attribution) |
| `REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-003` | [Auto-approve selection](#auto-approve-selection) |
| `REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-004` | [Configure contract cleanup](#configure-contract-cleanup) |
| `REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-005` | [End-to-end evidence](#end-to-end-evidence) |

## Confirmed current behavior

| Control | Where it is written | Where it is read |
| --- | --- | --- |
| `cli_flags` (ACP launch) | `lifecycle.CommandBuilder.BuildCommand` appends the tokens to the command `Agent.BuildCommand` returned, which for every ACP agent is the bridge argv (`npx --yes --prefer-offline <bridge package>`). | The bridge process. It forwards no unrecognized argv to the agent CLI it spawns. |
| `cli_flags` (passthrough launch) | `agents.StandardPassthrough.BuildPassthroughCommand` appends them to the agent CLI argv. | The agent CLI. Correct today. |
| `mode` | `SessionManager.applyProfileSessionLayers` calls `client.SetMode`; `acp.Adapter.SetMode` issues `session/set_mode` and then emits a `session_mode` event built from the **requested** mode plus the cached mode list. | The agent applies it. Kandev never compares the agent's reported current mode with the requested one. |
| `auto_approve` | Two carriers: `ExecutorCreateRequest.AutoApprovePermissions` / `…Override` and the `AGENTCTL_AUTO_APPROVE_PERMISSIONS` environment definition. Both converge on `config.InstanceConfig.AutoApprovePermissions`. | `process.Manager.handlePermissionRequest`. Working. |
| `approval_policy` | `resolveApprovalPolicyAndDisplayName` maps `AutoApprove` to `never`/`untrusted`; the value travels in the configure request and is stored on `config.InstanceConfig.ApprovalPolicy`. | Nothing. It is assigned and logged, never consulted. |

Three silent-denial sites exist on the permission path:

1. `process.Manager.autoApprovePermission` selects `req.Options[0]` when no
   option declares `allow_once` / `allow_always`, and answers `Cancelled` when
   the option list is empty. Both return before `sendPermissionNotification`, so
   no permission request is ever recorded.
2. `acp.Client.RequestPermission` answers `Cancelled` for an empty option list
   before the handler is consulted, and `forwardPermissionRequest` converts a
   handler error into `Cancelled`.
3. `acp.Adapter.handlePermissionRequest` selects `req.Options[0]` when no
   handler is installed.

A cancelled outcome reaches the agent as a refusal, which providers render as a
denial. From the user's side that is indistinguishable from a human denial.

The effective launch mode is resolved by `Manager.effectiveSessionMode`: a
persisted `session_mode` session-metadata value wins over the profile mode. That
value is written both by the user's session mode toggle and by the workflow
`set_session_mode` step action, whose apply failure is downgraded to Debug. None
of the three sources is recorded on the session, so a profile mode that lost is
invisible.

## CLI flag destination

Add one declaration to the agent contract rather than a per-agent forwarding
mechanism, because no supported ACP bridge offers a generic argv passthrough and
a speculative one would be untestable.

`agents.PermissionSetting` gains an explicit launch-mode scope. The existing
`PermissionApplyMethodCLIFlag` value keeps its meaning for passthrough launches;
a setting that only applies there declares `PassthroughOnly: true`. Claude's
`dangerously_skip_permissions` setting already carries that property in prose
(`claude_acp.go`); the change makes it machine-readable.

Two consumers use it:

- **Save-time validation.** `settings/controller` rejects saving a profile whose
  enabled `cli_flags` contain a token that the profile's agent declares
  passthrough-only while the profile does not use CLI passthrough. The error
  names the ACP equivalent from the same declaration (for Claude: the permission
  mode control). Validation runs on the resolved token list from
  `cliflags.Resolve`, so a flag reached through a multi-token entry is caught.
- **Command preview.** `settings/controller/agent_config.go` already builds a
  preview command. It gains a destination label for the flag segment so the
  preview states that the tokens are appended to the launched bridge process,
  not to the agent CLI it wraps.

Flags that are not declared passthrough-only keep today's behavior: appended to
the launched process argv. That remains useful (bridge-level flags exist) and is
now truthfully labelled.

`CommandBuilder.BuildCommand` is unchanged. The defect was the claim, not the
append.

## Mode confirmation and attribution

### Confirmation

`acp.Adapter.SetMode` stops synthesizing the session-mode event from the
requested value. After `conn.SetSessionMode` returns it reads the adapter's
current-mode state, which the adapter already maintains from the agent's
`current_mode_update` notifications and the `session/new` mode state
(`emitInitialModeState`). The emitted `session_mode` event carries the agent's
current mode.

Because a provider may publish `current_mode_update` asynchronously after
answering `session/set_mode`, the adapter waits for a bounded settle window for
a `current_mode_update` naming either the requested mode or a different one
before emitting. The window reuses the existing convergence pattern from
`emitSetModelEvent`; on expiry the adapter emits the last known current mode and
marks the result unconfirmed rather than assuming success.

`SetMode` returns a typed result carrying `requested`, `effective`, and
`confirmed`. `SessionManager.applyProfileSessionLayers` and
`applyRuntimeSessionLayers` log `set profile mode on ACP session` only for a
confirmed exact match. A clamp or an unconfirmed result logs at Warn and records
a session-visible warning message through the existing session message path.

### Attribution

`Manager.effectiveSessionMode` returns the winning source alongside the mode:
`agent_profile`, `session_override`, or `workflow_step`. The source travels with
the mode into the session layers, is persisted on the session's runtime state
next to `session_mode`, and appears as a structured log field. The workflow
`set_session_mode` action's apply failure moves from Debug to Warn and records
the same session-visible warning, so a step that declared a mode and failed to
apply it is not silent.

No precedence changes. `session_override` continues to win over
`agent_profile`; this design only makes the winner observable.

## Auto-approve selection

`process.Manager.autoApprovePermission` returns a tri-state rather than a
response:

- an allow option was found — answer with it;
- options exist but none declares an allow kind — fall through to the pending
  permission flow;
- no options exist — fall through to the pending permission flow.

`handlePermissionRequest` treats the two fall-through cases exactly like a
non-auto-approve request: it creates the `PendingPermission`, sends the
notification, and waits. The user sees a prompt instead of an invisible denial.

Option-kind matching is normalized (trimmed, case-insensitive) so a provider that
sends `Allow_Once` is not read as an unknown kind.

`acp.Client.RequestPermission`'s pre-handler empty-option shortcut is removed so
the empty-option case reaches the handler and therefore the same pending flow.
`acp.Adapter.handlePermissionRequest`'s no-handler `Options[0]` fallback becomes
a cancellation with an explicit Warn, because a missing handler is a Kandev
wiring failure rather than a permission decision; it is unreachable in a wired
launch and must not silently approve.

Audit: `autoApprovePermission` already logs the selected option. It additionally
records a permission transcript entry through the existing permission-message
path with the option ID, option kind, and an `auto_approve` source, so an
auto-approved call is visible next to human-approved ones. The delivery-timeout
auto-cancel in `sendPermissionNotification` records a distinct `timed_out`
result rather than reusing the cancellation shape used for user-driven
cancellation.

## Configure contract cleanup

`approval_policy` is removed from the sender
(`runtime/agentctl.Client.configureAgent`, `resolveApprovalPolicyAndDisplayName`
keeps only the display-name responsibility and is renamed accordingly) and from
`config.InstanceConfig`. The agentctl request DTO keeps the field as an accepted
but ignored value so a newer backend and an older agentctl, or the reverse, both
configure successfully. A test pins that an inbound `approval_policy` does not
fail the configure call and does not change behavior.

`auto_approve` keeps a single documented carrier. The
`AGENTCTL_AUTO_APPROVE_PERMISSIONS` environment definition and the
`AutoApprovePermissions` request field currently both exist;
`applyApprovalOverrides` already resolves them with the explicit override
winning. The design keeps the request field as the carrier and documents the
environment variable as the test and container-bootstrap override only.

## End-to-end evidence

The reported failure is a workflow outcome, so unit coverage alone does not
close it. A Playwright end-to-end check in `apps/web/e2e` runs the same task
twice against the mock agent, changing only the agent profile:

- with an unattended-permission profile: the mock agent's permission-requesting
  scenario completes with no pending permission request surfaced and no user
  interaction;
- with the default profile: the same scenario surfaces a pending permission
  request, the tool call stays pending, and it completes only after the test
  answers it.

The mock agent (`cmd/mock-agent`) gains a scenario that requests permission for
a state-changing shell command, so the check does not depend on a real provider
or on network access. Backend integration coverage asserts the same contract at
`process.Manager.handlePermissionRequest` for the option shapes listed in
`REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-003`.

## Failure modes

- A provider that never publishes `current_mode_update` produces an unconfirmed
  mode result. Kandev warns and continues with the session; it does not fail the
  launch, because the mode may still have applied.
- A provider that offers only reject options makes an `auto_approve` session
  block on a user prompt. That is the intended behavior: an unattended session
  stalls visibly rather than proceeding on a denial.
- Removing `approval_policy` from the sender while an older agentctl expects it
  is safe: the field was never read.
- Save-time rejection of a passthrough-only flag can invalidate an existing
  stored profile. Validation runs on write only; an existing profile continues to
  launch, and the profile editor shows the same message on next edit.

## Observability

- Structured log `session.mode.applied` with `requested`, `effective`,
  `confirmed`, and `source` replaces the current unconditional Info line.
- Structured log `permission.auto_approve` with `option_id`, `option_kind`, and
  an `outcome` of `selected` or `fell_back_to_prompt`.
- The existing permission transcript gains the auto-approval and timeout
  results, so a permission answered without a human is auditable.
