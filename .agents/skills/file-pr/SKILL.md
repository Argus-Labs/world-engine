---
name: file-pr
description: Draft, file, or update a concise pull request using this repository's template, reviewed demo evidence, and AI attribution. Interviews the PR's creator on its premise first: the problem, its source, alternatives, and product sign-off.
---

# File PR

<!-- Adapted from t3dotgg/tstack skills/file-pr/SKILL.md at 6efae364201abc9d9f07443e9caa461f42e51090 (MIT; see LICENSE).
Show-me guidance inlined and adapted from Scott's personal/scott-engineering 0.3.2 skills/show-me. -->

## Establish the Change

Use the contributor's existing version-control and GitHub tooling.
Identify this task's branch and its intended base, then inspect their diff and
commits. Check whether a PR already exists for that branch before creating one.
Account for the complete intended change, including required stacked dependencies;
preserve unrelated work. Follow the repository's review and verification requirements.

Use the original request, the actual diff, and checks performed as evidence.
Tradeoff consideration belongs before or during implementation, before the PR is
filed. Record the options, tradeoffs, choice, rationale, and decision-maker in the
task conversation as the decision is made. Writing the PR description reports
that record; it is not the time to invent a comparison that never happened.
Read the linked task when available. Use its real ID in `Closes #<task tracker id>`;
for an external tracker, use its supported issue reference or URL. If there is no
linked task, omit the closing line rather than inventing one.

## Interrogate the Premise

Before drafting, make the PR's creator show the thinking behind the change. Their
answers fill the template's `Premise` section and the `AI Usage` journal.

- **If the run is autonomous, skip the interview.** A run is autonomous when no human
  is in the conversation to answer: scheduled automations, bug hunts, threads another
  agent dispatched, or a request that says to proceed without questions. Fill `Premise`
  from the task record and write `**Interview:** Skipped — autonomous run.`
- **Otherwise, ask before drafting,** in one round, using the harness's question or form
  tool when it has one:
  1. **Problem:** What specifically is this PR trying to address: the problem developers
     hit, not the feature they asked for?
  2. **Source:** Where did it come from: developer or game-team feedback (link or quote it),
     a bug report, telemetry, a teammate's request, tech debt, or the creator's own idea?
  3. **Alternatives:** What other approaches did they consider, and why this one?
  4. **Product sign-off,** only for a significant product decision (defined below): was
     a maintainer who owns product decisions (Scott, for Argus Labs) consulted, and where?
     Skip it when that maintainer is the creator.
- **Judge the answers against the diff.** Read the linked feedback, report, or task.
  Decide whether the change fixes that problem at its root, or instead masks a symptom
  (a clamp, null guard, retry, delay, or one-caller special case), solves a different
  problem, or rebuilds something an existing engine feature, system, or plugin provides.
- **Separate the ask from the need.** Feedback often arrives as a solution ("add a config
  flag"); a literal build of it gives developers faster horses. Restate the need behind
  it: what the developer was trying to do and what got in the way. Judge the PR against that
  need, and propose a different change when it serves the need better.
- **Push back once, with evidence.** If the problem or source is vague ("users
  complained"), ask for the specific feedback. If the change misses the stated problem,
  or no alternatives were considered, name the gap with `path:line` or the feedback text
  and propose one or two credible alternatives.
- **Then let the creator decide.** Record their final answer, and any concern you still
  hold, in `Premise`; the reviewer weighs it from there.
- **Significant product decision:** a change that adds a new service, vendor, or
  infrastructure, or a feature that adds significant server cost or operational
  complexity. Voice chat is the example: a media relay, its bandwidth, and its moderation
  load. Bug fixes, tuning, UI changes, and features that fit the existing services at
  similar cost are not.
- **If no maintainer was consulted on one:** recommend consulting one before filing. If
  the creator files anyway, open the PR as a draft and write
  `**Product sign-off:** Needed — no maintainer consulted yet.`

Done when every `Premise` field holds the creator's answer, or the task record for an
autonomous run, and your assessment of the fix is recorded.

## Gather Screenshots and Recordings

Capture a screenshot or recording when it provides useful review evidence and is
feasible. Read the vendored
[demo skill](../demo/SKILL.md) for capture, review, and delivery. Show completed
behavior from the current change, with before/after screenshots when they explain
the difference. Use a recording for interaction, animation, or a multistep flow;
use a screenshot for a static state. CLI changes can use a terminal recording.

Verify the demonstrated result and inspect the media before attaching it. Retake
affected media after any later change that makes it stale. Include only media
accessible to the intended reviewers; a local path or BB-only embed is not a GitHub
attachment. Use the available authorized upload path and verify its returned URL.

Include `Screenshot / Recordings` only when it contains useful, reviewed media
accessible to reviewers. Otherwise omit the heading and its contents; do not keep
an empty section or fill it with an explanation of why media is absent. Put relevant
capture or runtime-verification limits in `Verification` or `Caveats` instead.
Documentation-only changes may use a rendered preview when useful. An explanatory
diagram can clarify a change, but is not runtime proof.

## Write the Title and Body

Read [the PR template](../../../.github/pull_request_template.md) and fill its
applicable sections in order, omitting `Screenshot / Recordings` when there is no
useful media to attach. Remove its instructional comments from the submitted body.

Keep `Human Notes` at the very top as optional space for the human to edit. Leave
it blank when unused; do not generate notes or rewrite existing human-authored text.

PR titles often become commit messages. Follow the repository's title conventions
and inspect recent titles when needed. Prefer a concise outcome over a mechanism:

- Mechanism: `perf(server): negotiate permessage-deflate on the websocket`
- Outcome: `perf(server): reduce websocket traffic with compression`

In `What Changed`, open with the problem from the original request and who benefits,
then briefly explain the resulting behavior. Avoid opening with a file inventory.
For example: "Starting a thread ignored the new-worktree preference. New threads
now use the configured default."

### Communication and Formatting

Write for a GitHub reviewer reading the PR without the conversation.
Keep the required template headings in order;
chat status labels and a `Your turn` block belong in the agent's reply, not the PR.

- **Put impact before mechanism.** Within each flow or section, state who
  benefits and what changes for them before implementation details. Explain what the next maintainer inherits when relevant.
- **Keep it scannable.** Default to bullets for groups of two or more items, with
  one or two sentences per bullet. Use title-case section headings, bold key facts and
  bullet labels, and blank lines or horizontal rules to separate groups.
  Scale detail to the change.
- **Make review self-contained.** Link selectively to relevant issues, code, and
  demos using links accessible to GitHub reviewers. Follow `Verification` below
  for verification evidence. Keep screenshots and recordings in `Screenshot / Recordings`; place explanatory
  visuals beside the text they support.
- **Explain consequential decisions.** Include the reason and tradeoff when they
  affect review. Describe the final behavior and relevant limitations; omit routine
  command sequences, unrelated Git state, and test or lint repair narration.
- **Guide review first.** After `Human Notes`, start the generated content with `Premise`, then `Review Guide`, grouping related changes and their
  design decisions as described below.

### Verification and Caveats

- **Verification:** Keep routine results to short bullets such as `**Tests:** PASS`
  and `**CI:** PASS`. Include meaningful failures, skips, and verification limits.
  Use only checks actually run; distinguish prior implementation results from checks
  run for the current update when needed. Link evidence only when it explains a
  failure, unusual result, or material limit. Passing checks need no routine links,
  command narration, or review history. Put detailed limitations in `Caveats`.
  Use `**CI:** Pending` until CI finishes; report FAIL when it fails.
- **Caveats:** Make each bold label the impact punchline, such as `**Board CLI loses
  its RPC backend**`, rather than a category such as `Installation` or `Runtime`.
  Follow with an em dash and only the cause or required action needed to understand it.
  Keep one impact per bullet; use sub-bullets for essential details. Cut process
  history, repeated verification, and commentary. Use `None known.` when none were found.

### Blast Radius

Place `Blast Radius` immediately before `Caveats`. Trace affected callers, APIs,
shared dependencies, configuration, storage, installation, and deployment boundaries.
For Argus Labs work, check monorepo, world-engine, rampage-client, and rampage-backend
for relevant consumers when you have access; search alternate contract/tool names
before claiming absence.
Use current-main source and inspect related open/merged PRs and their actual changes.

Summarize by repository or affected area with impact and coverage: included here,
already merged, open, or missing. Link the consumer and the PR/commit that handles it.
Verify that the follow-up actually covers the changed contract; a matching title or
passing CI is insufficient. Distinguish an inherited gap from a new regression.
State the search scope and runtime limits, including when no direct consumer is found.
Keep unresolved downstream migration gaps visible. Do not invent follow-up work or
change another repository merely to make the description look complete.

### AI Usage

Write this section as bullets, with AI attribution followed by a journal of human
contributions drawn from the task's prompt inputs and review feedback.
Track substantive inputs as the task progresses. Use the full task conversation,
including follow-ups, when drafting; keep the journal current on PR updates.

- **AI:** Write `Created with <model> in <harness>.` using the public model name
  reported by the harness. Include every subagent model and harness that wrote part
  of the change. If identity is unavailable, name the harness without guessing a
  model. Use `No AI used.` when applicable.
- For human inputs, summarize each substantive input in order as a clean, short
  bullet. Capture the judgment supplied: the problem and desired outcome; scope,
  constraints, and acceptance criteria; architecture or design choices; options
  selected or rejected and stated tradeoffs; corrections, review findings, and
  verification the human performed. Record human-authored code when provided.
- Use specific labels such as `**Human — Request:**`, `**Human — Scope:**`, or
  `**Human — Review:**`. Identify contributors other than the requester by name or
  handle, for example `**Human (@handle) — Review:**`. State what the person decided
  or contributed and how it shaped the change.
  Paraphrase prompts rather than quoting transcripts; combine repeated inputs
  and omit acknowledgments. Preserve meaningful changes of direction and the
  final decision; mark superseded direction explicitly. Include reasons only when
  the human supplied them.
- Attribute only observed contributions. An agent's proposal is not human judgment
  unless the human selected or endorsed it; say `Approved the agent-proposed…` or
  `Chose option B over A` to distinguish approval from originating a design.
  If the human only supplied the initial request, record it and add
  `**Implementation:** Fully autonomous after
  the initial request.` Additional human direction or review rules out that label.
  Use the autonomous label only with complete task history. If history is partial,
  add `**Human — Journal scope:**` naming the available sources and missing history
  instead of claiming full autonomy.
- Keep the journal relevant to the change and exclude secrets and private details.

### Review Guide

Place `Review Guide` after `Premise`. Put any closing reference at the start of
`Review Guide`, before its numbered blocks. Number its sections with collapsed
`<details>` blocks and numbered summaries, for example
`<summary><strong>1. Plugin Ownership</strong></summary>`. Open each block's content
with the effect on users, then choose the matching show-me view: a shallow file
or responsibility tree for a refactor/review order, a component tree for UI, or
an indented call tree for runtime control flow. Use nested bullets directly;
omit wrapper bullets such as `Review path:` and `Program path:`.

In a call tree, put one defined routine per line. Children are calls made by the
parent; siblings follow execution order. Mark boundaries with trailing `— db`,
`— network`, or `— disk` annotations. Use uppercase flow markers on their own lines
when they change which calls run. Component trees include relevant hooks and state;
file trees stay shallow with one responsibility per entry. Do not present a
suggested file-reading order as a runtime call relationship. Keep only the path
needed for review. Use `<pre>` and ASCII connectors if clearer, retaining clickable links.

Each linked entry uses `[filename.ts — Human-readable title](diff-url)`. Always
include the filename first, then an em dash and the readable title, inside the link.
Use an em dash for annotations after the link too; never use `//` or `:` there.
Omit LOC from visible text; retain exact file/hunk/line anchors in URLs. Expand the filename
to the shortest path needed to disambiguate duplicate filenames. Keep the routine or
component name in the linked title when tracing calls. Link the relevant hunk
rather than the whole file.

Use pinned comparison URLs:
`https://github.com/<owner>/<repo>/compare/<base-sha>...<head-sha>#diff-<sha256(path)>R<line>`
(use `L` for base-side lines). Verify the file and anchored line exist in that diff.
The comparison covers the whole PR, including earlier commits.

Nest every consequential design choice in its own collapsed `<details>` block
with `<summary>❓ Q1: <design question></summary>`. Leave blank lines around Markdown
inside `<details>` so bullets and tables render. After the options table, state
`**Implemented:** A (Human)` or `A (AI)`, followed by a short `**Why:**` answer
explaining the selection. Put attribution in parentheses; do not add a separate
`Decision by` label. Use `(AI-proposed, human-approved)` or `(Unknown)` when needed,
using the conversation and review record for attribution.
Human-attributed choices must agree with the `AI Usage` journal.

Compare actual alternatives in a table with columns `⭐ [A] <recommended option>`,
`[B] <option>`, and optionally `[C] <option>`. Use rows **What**, **Effort**, **Wins**,
and **Costs**, at most eight words per fact, `·` between facts, and `—` for empty
cells. Add a row only when it changes the decision. Identify the implemented option
separately even if it differs from the recommendation. Keep tradeoffs in the table;
put `Implemented` and `Why` below it, in that order. For unresolved choices, label
the decision `Open` and state the current behavior without claiming an option is chosen.

Never fabricate tradeoff deliberation after the fact or infer historical reasoning
from the finished code. Include only alternatives and reasons supported by the
contemporaneous record; use `Not recorded` when absent. Do not populate a comparison
table with hypothetical pros and cons and present it as a decision that occurred.
Keep later review analysis explicitly separate from the original decision record.
Update the implemented answer and attribution when a reviewer decides. Ask
for a reply only for an actual open choice. For a small change, one numbered section with a short tree
is enough; omit design Q&A when there is no meaningful choice. Follow the template's
example rather than flattening the path or writing options as prose bullets.

## Show Me: Explain the Change Visually

This section incorporates the show-me workflow; no separate show-me installation
is required. Pick the smallest view that answers the reviewer's question, and put
it beside the short text it supports, usually in `What Changed`. One view usually
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

### Call and Component Trees

For call trees, put one defined routine per line. Indentation shows calls made by
the parent; top to bottom shows order. Use trailing comments for boundaries such
as `— db`, `— network`, `— disk`, or `— clock`. Mark concurrency with `spawn`
and note blocking waits. Put uppercase flow markers above affected children when
they change which routines run. Split deep subtrees; use a sequence diagram when
process boundaries are the point. Component trees include only relevant hooks,
state, and modules, with the owning file or package where useful.

### Sequence Diagrams

Use GitHub-supported `mermaid` fences. Each participant is a boundary or a routine
that owns state; helpers remain arrows. Label boundaries, such as `Store (db)`.
Use solid arrows for calls and dashed arrows for returns that affect the next call.
Use `loop` for iteration, `alt`/`else` for decisions, and `par` for concurrency.
For data flow rather than interaction, prefer a Mermaid flowchart.

### Trees, Diffs, Code, and Custom Visuals

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

## Publish and Verify

Drafting a description does not authorize publishing it. When the user has asked
to open or update a PR, use that authorization without asking again. Otherwise,
deliver the completed draft and media for review.
For an unpublished draft, mark code permalinks as pending until its head is pushed;
fill or refresh them when publication is authorized.

Before submission, check the title, body, quotes, and attachments for secrets and
private details. Confirm required sections are filled and `Screenshot / Recordings`
is omitted unless it contains useful media that works for reviewers. Check that code
permalinks resolve at the pushed head SHA,
attribution is accurate, and the diff matches the request.

Create a ready-for-review PR unless the user requested a draft. Supply the title
and filled template body in the format the chosen tool expects. For file inputs,
check whether the tool expects only the body or a title followed by the body.
Keep `Closes #<task tracker id>` in the description, not the title. Preserve the
intended head, base, and any branch dependencies when publishing.
If publication fails, inspect remote state before retrying
to avoid duplicate PRs and preserve the prepared body and evidence.

For an existing PR, preserve unrelated text and media and use the available supported
update operation. Read back the final title, body (including the closing reference
when applicable), base, head, media links, and Review Guide diff links. Refresh code
permalinks when the head changes, then return the PR URL.
Do not post follow-up comments unless they serve the request.
If the user also requested monitoring, use `babysit-pr` when installed; otherwise
monitor checks and review feedback with the available GitHub tools.
