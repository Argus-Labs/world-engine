---
name: go-cli
description: Design, edit, or review Go command-line tools and terminal interfaces, including Kong commands and Bubble Tea behavior.
---

# Go CLI

Use [Go development](../go-development/SKILL.md) for shared Go conventions when they are
not already loaded. Apply this skill's terminal guidance only to command or TUI work.

## Command boundaries

- Prefer Kong for new CLIs unless the repository uses another parser. Keep command structs
  declarative, use grouped structs and tags, and put orchestration in `Run`.
- Normalize and validate input at entry. Capture command telemetry there, where flags and
  inputs are available, instead of spreading it through lower-level helpers.
- Commands depend inward on clients, models, stores, services, and UI components. Lower
  layers return plain values and domain errors and do not import command packages.
- Keep formatting, prompts, and terminal cleanup at the command or terminal UI boundary.

## Interactive behavior

For Bubble Tea, expose a `Run(ctx, ...)` boundary that owns the program, handles aborts,
and returns plain values. Keep model logic independent of process-global terminal state.
Use the repository's terminal cleanup helpers and silent-error convention for expected
exits or already-rendered validation failures.

Verify parsing and orchestration at command boundaries. Send messages directly to Bubble
Tea models for deterministic interaction tests. Use a real terminal check only when the
behavior depends on terminal integration.
