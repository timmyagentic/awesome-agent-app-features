# Coding-agent integration guide

这份文档是给接入方 coding agent 的施工入口。它不是 Skill：Agent 先读取契约，再根据宿主项目实际结构修改代码。

## Operating model

One feature installation is a code change in the target repository, not a
blind package install. The agent must map this kit's policy/core boundary onto
the host's existing architecture.

1. Read the selected `features/<id>/feature.json` and its README completely.
2. Inventory the host before editing.
3. Write down the host-specific mapping for every integration step and
   invariant.
4. Add the reusable core or dependency.
5. Build thin host adapters for UI, authorization, lifecycle, and configuration.
6. Add focused failure tests before declaring the feature installed.
7. Run both the manifest verification commands and the host repository's normal
   validation.

## Host inventory: Feedback

Find and record:

- Where errors and unsupported capabilities become user-visible.
- Which surface can show the complete final redacted report.
- Which explicit user action represents approval.
- How a per-install random ID can be persisted, or whether it should be omitted.
- Which product/version/OS/arch/agent fields are already trustworthy.
- Where the relay endpoint is configured and how users can disable feedback.
- The public fallback issue URL for relay failures.

Do not submit in the background. A proactive assistant may summarize a problem
and offer a button, but only the user's explicit action may call `Draft.Approve`.
Do not capture arbitrary environment variables, conversation transcripts,
reasoning, tool payloads, filesystem paths, user/chat IDs, or credentials.

## Host inventory: Updater

Find and record:

- The single authoritative current-version value.
- The exact `--version` contract of a released binary.
- Existing GitHub Release archive/checksum names for every supported platform.
- The executable path and whether it resolves through a package-manager symlink.
- Every update entry point (chat, CLI, UI, background discovery).
- Administrator/privileged gates.
- Shutdown, restart, and post-restart acknowledgement behavior.
- Existing beta/nightly channels. Keep them separate from the stable updater.

All entry points must call one `updater.Updater`; they may render different
progress copy, but they must not reimplement release selection or file
replacement. Do not shell from chat into a text-oriented CLI adapter.

## Required host-level tests

### Feedback

- A report cannot be sent without explicit approval.
- The preview contains every outbound field.
- Product-specific secret/identifier/path shapes are redacted.
- A stale error is not attached to an unrelated report.
- Relay failure produces a safe fallback and no credential leaks.

### Updater

- A prerelease returned by the source is rejected.
- Missing/duplicate checksum or archive assets are rejected before mutation.
- Checksum mismatch and staged-version mismatch leave the old executable intact.
- Installed-version mismatch restores the old executable.
- Concurrent entry points receive `ErrUpdateInProgress`.
- Successful host restart and acknowledgement are tested separately from the
  updater core.

## Adaptation boundaries

Safe adaptations include asset naming, version-probe output, UI, permissions,
configuration, translated copy, and restart behavior. Changing any manifest
invariant is a security-policy change and must be reviewed as such.

For package-manager installs, write a separate installer adapter with the same
stable and verification contract. The MVP does not claim npm rollback parity.
