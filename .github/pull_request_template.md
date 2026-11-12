## Human Notes

<!-- Optional space for the human to add context, concerns, or reviewer requests.
Leave blank if unused. Agents must not fill this section or rewrite existing human notes. -->

## Premise

<!-- Fill from the PR creator's interview answers (see the file-pr skill); paraphrase them and never invent one.
Autonomous runs fill this from the task record and mark the interview skipped. -->

- **Problem:** <the need developers hit, not the feature they asked for>
- **Ask:** <the request as it arrived, when it named a solution; omit otherwise>
- **Source:** <developer or game-team feedback · bug report · telemetry · teammate request · tech debt · own idea · autonomous run — link or quote it>
- **Why this fix:** <how the change addresses that problem at its root>
- **Alternatives considered:** <options weighed and why this one won, or "None">
- **Product sign-off:** <Maintainer consulted (link) · Not a product decision · Needed — no maintainer consulted yet>
- **Interview:** <Done · Skipped — autonomous run>

## Review Guide

Closes #<task tracker id>
<!-- Replace with the real task reference. Omit this line if no task is linked; never invent an ID. -->

<!-- Make each numbered review section a collapsed <details> block with a numbered
<summary>, such as "1. Session Expiry". Nest each design decision in its own
collapsed <details> block. Leave blank lines around Markdown inside the blocks.
Group changes by behavior. Use the show-me view that fits: call tree for runtime,
component tree for UI, or shallow file tree for responsibilities/review order.
Use nested bullets directly, without a "Review path:" or "Program path:" wrapper.
For calls: one defined routine per line, children are calls made by the parent,
and siblings follow execution order. Put boundaries in trailing — network, — db,
or — disk annotations; use uppercase IF/ELSE markers when they change the calls.
For a review/file tree, show one responsibility per entry without implying calls.
Use <pre> and ASCII connectors only when clearer; keep links clickable.

Link format: [session.ts — Session validation](<pinned diff link>). Always include
the filename followed by an em dash and the readable title inside the link. Omit LOC from link
text; expand the filename to a minimal path only to disambiguate duplicate filenames.
Use an em dash for any annotation after the link too, never // or a colon.
Keep exact hunk/line anchors in URLs.
Pin links to specific hunks in /compare/<base-sha>...<head-sha>#diff-<sha256(path)>R<line>
(use L for base-side lines). Verify file and anchored lines exist.

Use numbered questions for consequential decisions. Write **Implemented:** A (Human)
or A (AI); use (AI-proposed, human-approved) or (Unknown) when appropriate.
Put Implemented and Why below the options table, in that order. Recommend A, keep facts to eight words, use ·
between facts and — for empty cells. Keep What/Effort/Wins/Costs rows; add a row
only when it changes the decision. Mark the implemented option even if it differs
from the recommendation. Attribute choices and reasons from the actual record,
matching the AI Usage journal. Tradeoffs must have been considered before or during
implementation, before filing the PR. Report that contemporaneous record; do not
invent alternatives, tradeoffs, or historical reasons from the finished code.
Write "Not recorded" when the original deliberation is unavailable.

Example (replace illustrative files, LOC, routines, and decisions with actual evidence):

<details>
<summary><strong>1. Session Expiry</strong></summary>


Expired sessions return users to sign-in before account data is loaded.

- [handler.ts — HandleRequest() — request entry](<pinned diff link>)
  - [session.ts — ValidateSession() — expiry check](<pinned diff link>)
  - [account.ts — LoadAccount() — account lookup](<pinned diff link>) — db
- [session.test.ts — Expiry and refresh tests](<pinned diff link>)

<details>
<summary>❓ Q1: Where should expiry validation live?</summary>


|            | ⭐ [A] Shared handler | [B] Each caller |
| :--------- | :------------------- | :-------------- |
| **What**   | Validate at entry | Validate in each caller |
| **Effort** | One validation path | Update every caller |
| **Wins**   | Consistent policy · centralized tests | Caller-specific exceptions |
| **Costs**  | Exceptions need shared policy changes | Duplicated checks · policy drift |

**Implemented:** A (AI-proposed, human-approved).
**Why:** The human approved one policy shared by every caller.

</details>

</details>

For an open question, state the current implementation and mark **Decision:** Open.
Ask for a reply only when a decision is needed. For a small change, one numbered
review section and a short tree are enough. -->

## Screenshot / Recordings

<!-- Optional: keep this section only when it contains useful, reviewed media accessible to reviewers.
Otherwise delete the heading and this section entirely; do not replace media with an absence explanation.
Embed a screenshot or recording of the completed change.
Use before/after images when helpful, and a recording for interactions or animation.
Put relevant capture or runtime-verification limits in Verification or Caveats.
Use links accessible to reviewers, not local paths. -->

## What Changed

<!-- Start with the problem and who benefits, then explain the resulting behavior.
Keep it concise. Include a small diagram, diff, or example if it makes the change clearer. -->

## Verification

<!-- Keep this terse: name checks and outcomes, with meaningful failures/skips.
Routine passing results do not need links or narration. Add evidence only when it
helps explain a failure, unusual result, or verification limit. Put detailed caveats below.
Write CI: Pending until CI finishes, and FAIL when it fails.

Example (replace with checks actually run):

- **Tests:** PASS — 186 passed; 3 platform-specific skips.
- **Typecheck / build:** PASS.
- **CI:** PASS. -->

## Blast Radius

<!-- Trace changed contracts, callers, dependencies, configuration, storage, and deployment
boundaries before filing. Check affected repositories, not just files in this diff.
For Argus Labs changes, check monorepo, world-engine, rampage-client, and rampage-backend
for relevant consumers when you have access. Inspect current-main source plus related open and merged PRs.

Group by affected area/repository. State the impact and whether handling is included here,
already merged, still open, or missing. Link relevant source and follow-up PRs/commits;
a related title alone does not prove compatibility. Distinguish pre-existing gaps from
regressions this change introduces. For no-impact claims, state the search scope and limits.
Merge status proves landing, not runtime deployment or integration-test success.

| Area | Impact | Coverage / Follow-up |
| :--- | :----- | :------------------- |
| API consumers | Updated request field required | Covered by linked merged PR |
| Background worker | Still uses previous request shape | Missing migration; link caller |

Replace illustrative rows with findings. Do not invent a follow-up PR or claim that an
unmerged change is handled. Keep this section concise. -->

## Caveats

<!-- Use short bullets whose bold labels ARE the punchline: "Board CLI loses its RPC backend",
not vague categories like "Installation", "Runtime", or "Open findings". Lead with impact;
include only the cause or required action needed to understand it. Keep one impact
per bullet, and use sub-bullets for essential supporting details. Cut process history,
repeated verification, and commentary. Use "None known." when there are no caveats.

Example:

- **BB requires a prepared checkout** — Managed Git installation cannot resolve workspace dependencies.
- **Clean installs can fail** — Native-addon compilation requires a working Python toolchain.
- **Ubuntu cutover is unverified** — No live source-switch validation was run. -->

## AI Usage

<!-- Use bullets: AI attribution first, then a journal of substantive human inputs in order.
Name the reported public model and harness, including subagents that wrote changes.
If the model is unavailable, name the harness without guessing. Use "No AI used." if applicable.

Summarize and clean the human's prompts and feedback to extract their judgment:

- Problem, desired outcome, scope, constraints, and acceptance criteria.
- Architecture/design choices, selected or rejected options, and stated tradeoffs.
- Corrections, review findings, human-authored code, and verification they performed.

Use the full task conversation, including follow-ups, and keep the journal current on PR updates.
Use one short bullet per substantive input; combine repeats and omit acknowledgments.
State the decision or contribution and its effect, including meaningful changes of direction
and the final choice. Attribute only observed input and stated reasons, not agent proposals
the human never endorsed. Paraphrase rather than quote prompts; exclude private details.
Mark superseded direction explicitly. When the human selects an agent proposal, say
"Approved the agent-proposed…" or "Chose option B over A" rather than imply authorship.
Identify contributors other than the requester by name or handle, such as
**Human (@handle) — Review:**.

Example (replace with actual contributions):

- **AI:** Created with <model> in <harness>.
- **Human — Request:** Asked for session expiry to return users to sign-in.
- **Human — Scope:** Required existing sessions to remain valid during rollout.
- **Human — Design (superseded):** Initially requested validation in each caller.
- **Human — Direction change:** Approved the agent-proposed shared handler after the first
  draft so callers share one policy; replaced the per-caller design.
- **Human — Design:** Chose one retry over three to prioritize faster feedback.
- **Human — Review:** Flagged the missing refresh scenario; added it to acceptance criteria.

Only with complete task history, if the human supplied just the initial request, record it and add:

- **Implementation:** Fully autonomous after the initial request.

Additional human direction or review rules out that label. If history is partial, use
**Human — Journal scope:** to name available sources and missing history instead of
claiming full autonomy. -->
