---
name: file-pr
description: Draft, file, or update a concise pull request using this repository's template, reviewed demo evidence, and AI attribution.
---

# File PR

<!-- Adapted from t3dotgg/tstack skills/file-pr/SKILL.md at 6efae364201abc9d9f07443e9caa461f42e51090 (MIT; see LICENSE).
Show-me guidance inlined and adapted from Scott's personal/scott-engineering 0.3.2 skills/show-me. -->

## Establish the change

Use the contributor's existing version-control and GitHub tooling.
Identify this task's branch and its intended base, then inspect their diff and
commits. Check whether a PR already exists for that branch before creating one.
Account for the complete intended change, including required stacked dependencies;
preserve unrelated work. Follow the repository's review and verification requirements.

Use the original request, the actual diff, and checks performed as evidence.
Read the linked task when available. Use its real ID in `Closes #<task tracker id>`;
for an external tracker, use its supported issue reference or URL. If there is no
linked task, omit the closing line rather than inventing one.

## Gather screenshots and recordings

Every PR needs a screenshot or recording when feasible. Read the vendored
[demo skill](../demo/SKILL.md) for capture, review, and delivery. Show completed
behavior from the current change, with before/after screenshots when they explain
the difference. Use a recording for interaction, animation, or a multistep flow;
use a screenshot for a static state. CLI changes can use a terminal recording.

Verify the demonstrated result and inspect the media before attaching it. Retake
affected media after any later change that makes it stale. Include only media
accessible to the intended reviewers; a local path or BB-only embed is not a GitHub
attachment. Use the available authorized upload path and verify its returned URL.

If capture or delivery is unavailable, or the change has no meaningful visual
behavior, state the specific reason in `Screenshot / Recordings` and point to
the actual verification. Documentation-only changes may use a rendered preview
when useful. An explanatory diagram can clarify a change, but is not runtime proof.

## Write the title and body

Read [the PR template](../../../.github/pull_request_template.md) and fill its
sections in order. Remove its instructional comments from the submitted body.

PR titles often become commit messages. Follow the repository's title conventions
and inspect recent titles when needed. Prefer a concise outcome over a mechanism:

- Mechanism: `perf(server): negotiate permessage-deflate on the websocket`
- Outcome: `perf(server): reduce websocket traffic with compression`

In `What changed`, open with the problem from the original request and who benefits,
then briefly explain the resulting behavior. Avoid opening with a file inventory.
For example: "Starting a thread ignored the new-worktree preference. New threads
now use the configured default."

- **How did you verify?** Report checks actually run, their results, and relevant
  environment or evidence. Distinguish runtime results from static checks. Name
  failed, skipped, and unavailable checks; do not imply CI passed before it ran.
- **Caveats:** State known regressions, tradeoffs, downsides, and remaining
  limitations. Use `None known.` when no caveats were found.
- **AI Usage:** Write `Created with <model> in <harness>.` using the public model
  name reported by the harness. Include every subagent model and harness that
  wrote part of the change. If identity is unavailable, name the harness without
  guessing a model. Use `No AI used.` when applicable. Describe the human's actual
  contribution: feature request or bug report, architecture or plan, implementation,
  review, and issues they caught. When the human only supplied a feature request
  or bug report, write `Fully autonomous`. Record any additional direction or review
  instead of claiming full autonomy.

## Show me: explain the change visually

This section incorporates the show-me workflow; no separate show-me installation
is required. Pick the smallest view that answers the reviewer's question, and put
it beside the short text it supports, usually in `What changed`. One view usually
suffices; add another only when it answers a different question. Keep only the
calls, files, props, states, and boundaries relevant to the change. Label observed
behavior and proposed designs explicitly.

| Explain | Use |
| --- | --- |
| Logic or an algorithm | Structured pseudocode |
| Runtime control flow | Indented call tree |
| Cross-process interaction or data flow | Mermaid sequence diagram or flowchart |
| UI structure | Component tree with relevant hooks and state |
| File responsibilities or broad refactor | Shallow file tree with responsibility comments |
| Changes within an existing structure | Small `diff` code fence |
| New code whose ownership or order matters, or a copyable target | Whole code block |
| Layout, visual states, or a custom concept | Screenshot or rendered custom visual |

### Pseudocode

Use structured English with Go-style declarations and explicit types, parameters,
and returns. Use `type Name struct`, `type Name interface`, and `func Name(...)`;
write fields as `name: Type`. Use `[]T`, `map[K]V`, `T?` for optional values, and
`Result<T>` for fallible results. Bodies use lowercase domain actions and uppercase
control flow with braces: `IF`, `ELSE`, `WHILE`, `REPEAT ... UNTIL`, `FOR EACH`,
`SWITCH`, `CASE`, `RETURN`, `CONTINUE`, and `BREAK`. Use domain bounds, not index
arithmetic. Propagate errors with `or RETURN err`; show wrapping or fallback when
it matters. Put guards before the happy path. Omit imports and language plumbing.
Show locks, cleanup, ownership, and cancellation when correctness depends on their
order; a concurrency-safety claim does not replace the mechanism. Use an untagged
code fence.

### Call and component trees

For call trees, put one defined routine per line. Indentation shows calls made by
the parent; top to bottom shows order. Use trailing comments for boundaries such
as `// db`, `// network`, `// disk`, or `// clock`. Mark concurrency with `spawn`
and note blocking waits. Put uppercase flow markers above affected children when
they change which routines run. Split deep subtrees; use a sequence diagram when
process boundaries are the point. Component trees include only relevant hooks,
state, and modules, with the owning file or package where useful.

### Sequence diagrams

Use GitHub-supported `mermaid` fences. Each participant is a boundary or a routine
that owns state; helpers remain arrows. Label boundaries, such as `Store (db)`.
Use solid arrows for calls and dashed arrows for returns that affect the next call.
Use `loop` for iteration, `alt`/`else` for decisions, and `par` for concurrency.
For data flow rather than interaction, prefer a Mermaid flowchart.

### Trees, diffs, code, and custom visuals

Keep file trees shallow, with one responsibility comment per entry. In GitHub PRs,
use `text` fences for trees and `diff` fences for additions/removals. Show a whole
code block when most of it is new, when context establishes ownership or order, or
when reviewers need a copyable target. Keep code samples tied to the actual change.

For custom visuals, match the product's colors, typography, spacing, and components.
Use real labels and data when available; label illustrative values or proposed
states. Fit the review surface. Use available canvas or HTML tools when they help,
then attach an accessible image or artifact for GitHub. Keep temporary render files
in ignored scratch space or a system temporary directory. These explanatory visuals
supplement the screenshots and recordings of actual behavior.

## Publish and verify

Drafting a description does not authorize publishing it. When the user has asked
to open or update a PR, use that authorization without asking again. Otherwise,
deliver the completed draft and media for review.

Before submission, check the title, body, quotes, and attachments for secrets and
private details. Confirm every template section is filled, media works for reviewers
or has a concrete exception, attribution is accurate, and the diff matches the request.

Create a ready-for-review PR unless the user requested a draft. Supply the title
and filled template body in the format the chosen tool expects. For file inputs,
check whether the tool expects only the body or a title followed by the body.
Keep `Closes #<task tracker id>` in the description, not the title. Preserve the
intended head, base, and any branch dependencies when publishing.
If publication fails, inspect remote state before retrying
to avoid duplicate PRs and preserve the prepared body and evidence.

For an existing PR, preserve unrelated text and media and use the available supported
update operation. Read back the final title, body (including the closing reference
when applicable), base, head, and media links, then return the PR URL.
Do not post follow-up comments unless they serve the request.
If the user also requested monitoring, use `babysit-pr` when installed; otherwise
monitor checks and review feedback with the available GitHub tools.
