# Demo Recording

Capture product demos with `agent-browser record`. Load the installed browser CLI's
current recording instructions before using these examples. The toolkit is optional for
simple in-capture pointer cues. For editable captions, callouts, zooms, or multiple takes,
capture clean footage and follow [Remotion editing](remotion-editing.md).

Use this reference when the output is meant for PRs, changelogs, onboarding notes, or
customer-facing walkthroughs.

## Core Rules

Record at **60 fps by default**, unless the user requests another frame rate. Use supported
recorder options to set the capture rate and preserve it through trimming and export. Report
recorder limitations instead of treating duplicated frames as 60 fps capture.

1. Use `agent-browser` commands for behavior. Use toolkit helpers for visuals.
2. Do a no-recording discovery pass before recording.
3. Check that every selector works before `record start`. If a selector is wrong, fail before
   producing a partial video.
4. Start recording from the verified initial state. The current recorder keeps the active page
   as-is unless a URL is supplied. After any explicit navigation, wait for the intended state.
5. Add a trap so failed scripts stop the active recording.
6. Use hard assertions for form values and final UI state.
7. Do not show success captions unless the final assertion passed.
8. End on a clean final frame: move the cursor away from hover-sensitive controls, hold briefly,
   then stop recording.
9. If any code, copy, styling, seed data, navigation, or state change after recording could affect
   the walkthrough, final frame, thumbnail, or supporting screenshots, treat those assets as stale
   and retake every affected asset before upload or embedding.

## Resolution preflight

Default desktop recordings to a **1920x1080 viewport**. Honor a requested mobile device
or other dimensions. Set the viewport before discovery so selectors and coordinates
match the final capture. For an attached or reused session, inspect its dimensions first.

```bash
agent-browser set viewport 1920 1080 2
agent-browser eval 'if (innerWidth !== 1920 || innerHeight !== 1080) throw new Error("Unexpected recording viewport"); ({width: innerWidth, height: innerHeight, dpr: devicePixelRatio})'
```

Device scale factor 2 sharpens screenshots but does not guarantee double-resolution
video. On agent-browser 0.37.1, a fresh default session recorded at 1280x578, and a
1728x1117 viewport at scale 2 recorded at 1728x1118. Verify the encoded artifact:

```bash
ffprobe -v error -select_streams v:0 \
  -show_entries stream=width,height,r_frame_rate -of json "$OUTPUT"
```

Check that the saved dimensions meet the chosen capture target before delivery, and
check again after editing or conversion. Retake an undersized source at the correct
viewport. Upscaling an existing recording does not recover missing detail.

## Workflow

### Phase 1: Define The Actions

List the actions the recording should show before writing the recording script:

```text
1. Click the task title
2. Type the edited title
3. Press Enter
4. Show the saved state
```

Include the smallest set of actions that proves the change. A demo can cover multiple short flows
when they show distinct states of the same feature, such as create/edit/delete,
empty/populated/error, or before/after variants.

Use short captions to separate states or flows. Captions should label what the viewer is seeing, not
narrate every click.

Skip login, setup, and unrelated navigation unless that is the behavior being demonstrated. Prefer a
separate recording when flows are unrelated or the video would become hard to scan.

### Phase 2: Discover Without Recording

Navigate the full flow once before recording. Use this pass to choose selectors or semantic
locators, open modals/dropdowns, and confirm the app can reach the requested final state. If the
opening state matters, reload or reset the app explicitly; a warm browser session may already be
past skeleton, onboarding, or empty states. If discovery mutates state, the recording script should
start from a fresh navigation, reload, or explicit reset so the recorded flow begins from the
intended initial state.

```bash
agent-browser set viewport 1920 1080 2
agent-browser open http://localhost:3000
agent-browser wait --fn "document.readyState === 'complete'"
agent-browser snapshot -i
```

Modern React apps often render the shell before data-backed lists appear. Wait for the specific list
item, content span, or loaded-state text you need before preflight; `document.readyState` only
proves the document loaded.

Avoid `agent-browser wait --load networkidle` on apps with SSE, polling, analytics, or streaming
updates. Prefer visible text waits, element waits, `document.readyState`, or a bounded condition.
For delayed pages with known copy, `agent-browser wait --text "Ready"` is usually the cleanest
readiness check.

For loading-state demos, assert both the intermediate loading text and the final loaded state. A
useful pattern is: click, immediately assert/screenshot the loading copy, then `wait --text` for the
loaded sentinel.

Clear console/page errors before discovery, then check them after the interaction if something
appears to do nothing:

```bash
agent-browser errors --clear
# run the interaction being explored
agent-browser errors
```

Prefer real browser commands for behavior:

- `agent-browser click "#submit"`
- `agent-browser find role button click --name "Submit"`
- `agent-browser fill "#email" "user@example.com"`
- `agent-browser focus "#editor"` + `agent-browser keyboard type "Text"`

Use `agent-browser fill` for normal inputs and textareas, especially React-controlled fields.
Reserve `keyboard type` for contenteditable/rich text editors or when actual key events are the
behavior being demonstrated. For clear-search flows, `agent-browser fill "#search" ""` is preferred
over Control+A/Backspace because it reliably fires input/change handlers.

Use the toolkit only for cursor, ripple, caption, and zoom visuals. `demoClickEl(selector)` performs
a visual click plus a best-effort DOM `.click()`. Do not rely on it for React handlers, form
submits, navigation, or any state-changing action.

Do not use hard-coded `demoMoveTo(x, y)` coordinates for state-changing clicks. They drift when the
viewport, route, Storybook chrome, fonts, or data change. Use `demoMoveToEl(selector)` for CSS-
selectable controls, or `demoMoveToText(text)` when the real action uses `agent-browser find ...`.
Reserve `demoMoveTo(x, y)` for parking the cursor or intentionally pointing at non-interactive
screen regions.

Prefer stable selectors when using CSS:

- Data attributes: `[data-action="save"]`, `[data-testid="submit"]`
- ARIA attributes: `[aria-label="Close"]`, `[role="checkbox"]`
- Stable classes: `.new-todo`, `.toggle`, `.clear-completed`
- Structural selectors only when stable: `.todo-list li:first-child .toggle`

Toolkit CSS selectors are native `document.querySelector` selectors. Playwright-only selectors such
as `button:has-text("Save")` are invalid in toolkit helpers; use `demoMoveToText("Save")` for those
text-targeted actions.

Avoid href-pattern guesses unless you inspected the actual hrefs during discovery. Apps may use repo
paths, integration IDs, or nested route shapes that do not match `/items/:id` assumptions.

For elements without obvious selectors, enumerate candidates:

```bash
agent-browser eval --stdin <<'EVALEOF'
JSON.stringify(
  Array.from(document.querySelectorAll("button")).map((button, index) => ({
    index,
    text: button.textContent?.trim()?.substring(0, 30),
    className: button.className,
    ariaLabel: button.getAttribute("aria-label"),
  }))
)
EVALEOF
```

Before using positional selectors such as `:nth-child`, inspect the container `outerHTML`. Wrapper
elements often make `.items a:nth-child(2)` mean something different than intended; prefer stable
attributes such as `a[href="#/active"]` or wrapper selectors such as `li:nth-child(2) a`.

When asserting lists or filtered results, inspect the actual rendered element type and role before
writing assertions. Do not assume task rows are anchors; many apps use `button`, `div[role="link"]`,
or nested interactive wrappers. Plain visual `<div>` rows may not appear in accessibility snapshots;
switch to CSS/JS assertions when the visible list is not represented in the snapshot tree. Scope
selectors to the container being demonstrated, such as a sidebar, modal root, table, or menu. Avoid
document-wide assertions when similar labels appear in multiple page regions.

For long pages, remember `snapshot -i` is viewport-oriented. Use a one-shot DOM inventory with
`querySelectorAll` to find off-screen sections, buttons, and IDs before deciding where to scroll.

When writing or patching fixture/demo JavaScript, avoid relying on bare global variables created
from element IDs. IDs such as `name`, `status`, `location`, `history`, `top`, and `parent` collide
with browser globals. Use `document.querySelector` or `document.getElementById` instead.

For modals, dropdowns, and menus, open the state during discovery, scope selectors to the modal or
menu root, verify uniqueness inside that scope, then close/reset before recording.

### Phase 3: Preflight

Every recording script should assert that CSS selectors used by the script are unique and visible
before `record start`.

Only preflight elements that should exist in the current state. If an element is hidden until data
is loaded, a modal opens, or a list is populated, assert it after the step that makes it visible.

```bash
agent-browser eval --stdin <<'EVALEOF'
if (innerWidth !== 1920 || innerHeight !== 1080) {
  throw new Error(`Unexpected recording viewport: ${innerWidth}x${innerHeight}`);
}
const checks = {
  taskTitle: ".task-title",
  saveButton: '[data-action="save"]',
};

const failures = [];

for (const [name, selector] of Object.entries(checks)) {
  const elements = document.querySelectorAll(selector);
  if (elements.length !== 1) {
    failures.push(`${name}: ${selector} matched ${elements.length}`);
    continue;
  }

  const element = elements[0];
  const rect = element.getBoundingClientRect();
  const style = window.getComputedStyle(element);
  const visible =
    rect.width > 0 &&
    rect.height > 0 &&
    style.display !== "none" &&
    style.visibility !== "hidden";
  const disabled = "disabled" in element && element.disabled;

  if (!visible) failures.push(`${name}: ${selector} is not visible`);
  if (disabled) failures.push(`${name}: ${selector} is disabled`);
}

if (failures.length) throw new Error(failures.join("\n"));
"OK";
EVALEOF
```

If a selector only exists after opening a modal or menu, do that during discovery and preflight,
then return the app to the starting state before recording.

### Phase 4: Record One Clean Script

Write a single bash script that starts recording, performs the planned actions, asserts state,
and stops recording. Include toolkit injection and visual helper calls only for in-capture
cues. Omit those lines when recording clean footage for Remotion. Do not run individual
commands interactively during the recording.

"Single script" means the recording itself is one clean execution. Discovery, script creation,
preflight retries, and post-recording screenshots are still real turns and should be counted when
evaluating process efficiency.

For a state-changing click, pair the visual helper with a real browser click:

```bash
agent-browser eval "demoMoveToEl('#submit')"
agent-browser wait 550
agent-browser eval "demoClick()"
agent-browser click "#submit"
agent-browser wait 350
```

For form input, use a real click or focus before typing:

```bash
agent-browser eval "demoMoveToEl('#name')"
agent-browser wait 550
agent-browser eval "demoClick()"
agent-browser click "#name"
agent-browser keyboard type "Sarah Mitchell"
agent-browser wait 250
```

For standard inputs, prefer clearing/filling with `agent-browser fill` after the visual click:

```bash
agent-browser eval "demoMoveToEl('#name')"
agent-browser wait 550
agent-browser eval "demoClick()"
agent-browser click "#name"
agent-browser fill "#name" "Sarah Mitchell"
agent-browser wait 250
```

After every navigation during the recording, wait for the SPA to settle:

```bash
agent-browser click 'a[href="/contacts"]'
agent-browser wait --text "Contacts"
```

Choose a visible readiness assertion or bounded function wait for the destination.
After a full document navigation (rather than SPA routing), inject the toolkit again
before calling its helpers: `agent-browser eval --stdin < "$TOOLKIT"`.

## Recording Commands

Use a named session for the whole walkthrough. Reuse the session that owns the verified
page, or create one before opening it. Choose a task-specific `--prefix` when multiple
demos share a worktree. Resolve output paths before sending them to the
browser daemon. Relative paths can resolve from the daemon's launch directory instead
of the calling shell's current directory, including for screenshots.

```bash
AGENT_BROWSER_SESSION="${AGENT_BROWSER_SESSION:-$(agent-browser session id --scope worktree --prefix demo)}"
: "${AGENT_BROWSER_SESSION:?Failed to select a browser session}"
export AGENT_BROWSER_SESSION
RECORDINGS_DIR="$(mktemp -d "${TMPDIR:-/tmp}/pr-demo.XXXXXX")"
mkdir -p "$RECORDINGS_DIR"

# Start recording to file
agent-browser record start "$RECORDINGS_DIR/output.webm" --fps 60

# Stop current recording
agent-browser record stop

# Restart with a new file
agent-browser record restart "$RECORDINGS_DIR/take2.webm" --fps 60
```

Use descriptive filenames when the recording will be stored as an artifact:

```bash
agent-browser record start "$RECORDINGS_DIR/inline-editing.webm" --fps 60
agent-browser record start "$RECORDINGS_DIR/settings-empty-state.webm" --fps 60
```

## Toolkit

Use the toolkit only when cues should be baked into the capture. Skip it for a clean take
that will be edited in Remotion. Set `DEMO_SKILL_DIR` to this repository's `.agents/skills/demo` directory, or run the following from the repository root:

```bash
TOOLKIT="${DEMO_SKILL_DIR:-$PWD/.agents/skills/demo}/scripts/demo-toolkit.js"
[[ -f "$TOOLKIT" ]] || { echo "Missing demo toolkit" >&2; exit 1; }
agent-browser eval --stdin < "$TOOLKIT"
```

All helpers are synchronous. Do not use `await`; call the helper, then use `agent-browser wait <ms>`
for pacing.

Target helpers throw when a target is missing, hidden, or ambiguous, so a failed
eval stops the recording script. Scope repeated labels with the optional selector,
for example `demoMoveToText("Save", "#dialog button")`.

| Helper                            | Use                                                                                         |
| --------------------------------- | ------------------------------------------------------------------------------------------- |
| `demoMoveToEl(selector)`          | Move the cursor to an element without clicking.                                             |
| `demoMoveToText(text, selector?)` | Move the cursor to a visible control by text, `aria-label`, or button input value.          |
| `demoMoveTo(x, y)`                | Move the cursor to coordinates, usually for parking it. Do not use for real click targets.  |
| `demoClick()`                     | Show a click ripple at the current cursor position. Pair with a real browser action.        |
| `demoClickEl(selector)`           | Visual click plus best-effort DOM `.click()`. Do not rely on it for state-changing actions. |
| `demoKey(keyName)`                | Show a short key caption, such as Enter or Escape.                                          |
| `demoSubtitle(text, durationMs?)` | Show a short caption near the bottom of the viewport.                                       |
| `demoZoom(x, y, scale)`           | Zoom/pan the page around a viewport coordinate.                                             |
| `demoReset()`                     | Reset zoom/pan to 1x.                                                                       |

The toolkit renders cursor, click ripples, and captions in a fixed overlay on `<html>`, outside the
`body` transform used by `demoZoom`. This keeps overlays aligned during scroll and zoom.

## Script Template

This example edits an `input.task-title`. Adapt the selector and assertions to the
actual control discovered before recording.

```bash
#!/bin/bash
set -euo pipefail

AGENT_BROWSER_SESSION="${AGENT_BROWSER_SESSION:-$(agent-browser session id --scope worktree --prefix demo)}"
: "${AGENT_BROWSER_SESSION:?Failed to select a browser session}"
export AGENT_BROWSER_SESSION
OUTPUT="$(mktemp -d "${TMPDIR:-/tmp}/pr-demo.XXXXXX")/demo.webm"
TOOLKIT="${DEMO_SKILL_DIR:-$PWD/.agents/skills/demo}/scripts/demo-toolkit.js"
[[ -f "$TOOLKIT" ]] || { echo "Missing demo toolkit" >&2; exit 1; }

cleanup() {
  agent-browser record stop 2>/dev/null || true
}

agent-browser set viewport 1920 1080 2
agent-browser open http://localhost:3000
agent-browser wait --fn "document.readyState === 'complete'"

agent-browser eval --stdin <<'EVALEOF'
if (innerWidth !== 1920 || innerHeight !== 1080) {
  throw new Error(`Unexpected recording viewport: ${innerWidth}x${innerHeight}`);
}
const checks = {
  taskTitle: ".task-title",
};

const failures = [];
for (const [name, selector] of Object.entries(checks)) {
  const elements = document.querySelectorAll(selector);
  if (elements.length !== 1) {
    failures.push(`${name}: ${selector} matched ${elements.length}`);
    continue;
  }
  const element = elements[0];
  const rect = element.getBoundingClientRect();
  const style = window.getComputedStyle(element);
  if (
    rect.width <= 0 ||
    rect.height <= 0 ||
    style.display === "none" ||
    style.visibility === "hidden"
  ) {
    failures.push(`${name}: ${selector} is not visible`);
  }
}
if (failures.length) throw new Error(failures.join("\n"));
"OK";
EVALEOF

agent-browser record start "$OUTPUT" --fps 60
trap cleanup EXIT

agent-browser eval --stdin < "$TOOLKIT"
agent-browser wait 400
agent-browser wait --fn 'Boolean(document.querySelector(".task-title")?.value?.trim())'

# Optional short caption when the demo needs context.
# agent-browser eval "demoSubtitle('Inline editing', 1200)"
# agent-browser wait 1700

agent-browser eval "demoMoveToEl('.task-title')"
agent-browser wait 550
agent-browser eval "demoClick()"
agent-browser click ".task-title"
agent-browser wait 350

agent-browser fill ".task-title" "Updated task title"
agent-browser wait 250

agent-browser eval "demoKey('Enter')"
agent-browser press Enter
agent-browser wait 250

agent-browser eval --stdin <<'EVALEOF'
const value = document.querySelector(".task-title")?.value?.trim();
if (value !== "Updated task title") {
  throw new Error(`Expected updated title, got ${JSON.stringify(value)}`);
}
"OK";
EVALEOF

agent-browser eval "demoMoveTo(640, 300)"
agent-browser wait 700

agent-browser record stop
trap - EXIT
dimensions="$(ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=s=x:p=0 "$OUTPUT")"
[[ "$dimensions" == "1920x1080" ]] || { echo "Unexpected video dimensions: $dimensions" >&2; exit 1; }
echo "DONE"
```

Key patterns:

- Visual click plus real click: `demoMoveToEl(selector)` + `wait 550` + `demoClick()` +
  `agent-browser click selector` + `wait 350`
- Semantic text click: `demoMoveToText("Save")` + `wait 550` + `demoClick()` +
  `agent-browser find role button click --name "Save"` + `wait 350`
- Semantic action: `agent-browser find role button click --name "Save"` + `wait 350`
- Text input: visual click + real click/focus + `agent-browser fill selector "text"` +
  `wait 250-300`
- Special key: `demoKey(keyName)` + `agent-browser press <key>` + `wait 250`
- Caption: `demoSubtitle(text, ms)` + `wait <ms + 200>`
- Cursor parking: `demoMoveTo(x, y)` + `wait 550-700` before stopping the recording

## Assertions

Use hard assertions, not printed JSON, for required state. A failed assertion should stop the script
and trigger the recording cleanup trap.

Form value assertion:

```bash
agent-browser eval --stdin <<'EVALEOF'
const expected = {
  "#name": "Sarah Mitchell",
  "#email": "sarah@northwind.com",
  "#company": "Northwind Trading",
};

for (const [selector, value] of Object.entries(expected)) {
  const actual = document.querySelector(selector)?.value;
  if (actual !== value) {
    throw new Error(`${selector}: expected ${JSON.stringify(value)}, got ${JSON.stringify(actual)}`);
  }
}
"OK";
EVALEOF
```

Final state assertion:

```bash
agent-browser eval --stdin <<'EVALEOF'
if (!document.body.textContent?.includes("Sarah Mitchell")) {
  throw new Error("Expected Sarah Mitchell to appear after submit");
}
"OK";
EVALEOF
```

Only add success captions after the final state assertion passes.

## Modals And Menus

Modals often duplicate button text from the underlying page. Scope selectors to the modal root and
preflight inside that scope.

```bash
agent-browser eval --stdin <<'EVALEOF'
const modal = document.querySelector(".fixed.inset-0.z-50");
if (!modal) throw new Error("Modal not found");

const submitButtons = Array.from(modal.querySelectorAll("button")).filter(
  (button) => button.textContent?.trim() === "Add Contact"
);

if (submitButtons.length !== 1) {
  throw new Error(`Expected 1 modal submit button, found ${submitButtons.length}`);
}
"OK";
EVALEOF
```

Avoid broad selectors such as `button.bg-primary` after a modal opens. Prefer scoped selectors,
semantic locators, or text filters inside the modal root.

## Captions

Use captions to label a feature, state, or transition between related flows:

```bash
agent-browser eval "demoSubtitle('Empty state', 1200)"
agent-browser wait 1400
```

Avoid captions for every click. If the action is obvious from the UI, skip the caption.

## Zoom And Pan

Default to no zoom. Use zoom only when a detail is too small to read at 1x, such as code text,
compact table cells, or small inline status UI.

When zoom is needed:

- Use one focused zoom cycle when possible.
- Use `demoZoom(newX, newY, sameScale)` to pan between nearby targets while zoomed.
- Use a scale around `1.4` to `1.7`.
- Reset zoom before opening dropdowns, popovers, or menus that may render in a portal.

Example:

```bash
agent-browser eval "demoZoom(400, 260, 1.5)"
agent-browser eval "demoMoveTo(400, 260)"
agent-browser wait 600

agent-browser eval "demoMoveToEl('.small-status')"
agent-browser wait 550
agent-browser eval "demoClick()"
agent-browser click ".small-status"
agent-browser wait 350

agent-browser eval "demoReset()"
agent-browser eval "demoMoveTo(640, 300)"
agent-browser wait 600
```

## Clicking Strategies

Use real browser actions for behavior. Use toolkit helpers to make those actions visible.

| Scenario                             | Method                                                                                 |
| ------------------------------------ | -------------------------------------------------------------------------------------- |
| CSS-selectable state-changing click  | `demoMoveToEl(selector)`, `demoClick()`, then `agent-browser click selector`           |
| Semantic state-changing click        | `demoMoveToText("Save")`, `demoClick()`, then `agent-browser find ... click`           |
| Standard input or textarea           | Visual move/click, then `agent-browser fill selector "..."`                            |
| Contenteditable / ProseMirror editor | Visual move/click, then real `agent-browser click` or `agent-browser focus`, then type |
| Special key press                    | `demoKey("Enter")`, then `agent-browser press Enter` + `wait 250`                      |
| No stable selector                   | Prefer semantic `find`; use refs only if the page will not navigate or re-snapshot     |

## Scrolling

For scroll-based demos, use native smooth scroll:

```bash
agent-browser eval 'window.scrollBy({ top: 350, behavior: "smooth" })'
agent-browser wait 700
```

Do not scroll and zoom at the same time. Scroll first, let it settle, then zoom if needed. After
`scrollintoview`, wait 600-800ms before the next action so smooth scrolling is visible and the
target has settled.

## Timing Reference

| Action                                  | Wait after |
| --------------------------------------- | ---------- |
| Toolkit injection                       | 400ms      |
| `demoSubtitle(text, ms)`                | `ms + 500` |
| `demoMoveToEl(selector)`                | 550ms      |
| `demoClick()` + real browser click      | 350ms      |
| `agent-browser keyboard type "text"`    | 250-300ms  |
| `demoKey(name)` + `agent-browser press` | 250ms      |
| `demoMoveTo(x, y)`                      | 550ms      |
| `demoZoom(...)` / `demoReset()`         | 600ms      |
| Hold after an important UI change       | 250-500ms  |
| Final hold before stop                  | 500ms      |

Wait at least 550 ms after cursor movement before showing a click ripple.

Target roughly 10-25 seconds for PR demos. Longer recordings are fine when multiple related states
need to be shown, but check that each segment is distinct.

## Review the artifact

For screenshot sets that illustrate mid-flow states, it is fine to stop the recording and replay the
same states for screenshots. Keep the recording itself a single clean script; use the replay pass
for static artifacts.

Review the first frame, demonstrated flow, and final frame. Keep loading or waiting that is
part of the behavior being proved. For a cut, caption, zoom, or combined take, follow
[Remotion editing](remotion-editing.md), then review the export against the original capture.

If implementation changes after a recording or thumbnail capture, re-run the affected recording or
capture before upload. Do not embed a demo, thumbnail, or screenshot that predates relevant final
changes.

After the recording stops, reset zoom and remove the synthetic overlay before
capturing a thumbnail or supporting screenshot. Keep it beside the recording:

```bash
agent-browser eval 'demoReset(); document.getElementById("demo-overlay")?.remove(); document.getElementById("demo-toolkit-style")?.remove()'
agent-browser wait 600
agent-browser screenshot "$(dirname "$OUTPUT")/thumbnail.png"
```

Skip the cleanup eval when the take did not use the toolkit. If recording resumes
after cleanup, inject the toolkit again first.

Inspect the thumbnail source and confirm that it matches the validated final state.

Deliver the reviewed recording through [inline video](inline-video.md) in BB. For an
authorized external upload, follow [publishing](publishing.md).

## Failure Signals

If the same workaround fails three times, stop and report the failure with a concise reflection
instead of continuing to debug indefinitely. A partial recording plus a clear failure diagnosis is
more useful than a long run that times out without a summary.

1. **`demoClickEl` returns successfully but nothing changes.** It used a DOM `.click()`, not a real
   browser click. Pair visual helpers with `agent-browser click` or `agent-browser find ... click`.

2. **`agent-browser click` times out or says the target is not interactable.** The selector may
   match multiple elements, a hidden element, or an element under a modal overlay. Preflight
   uniqueness and visibility, and scope modal selectors to the modal root.

3. **A script fails and the next run says a recording is already active.** Add the cleanup trap from
   the template and run `agent-browser record stop` once to clean up the old session.

4. **Refs invalidated.** Refs persist until another snapshot or page change invalidates them. Avoid
   taking snapshots during recording.

5. **Assuming a form worked.** Verify input values before submitting, then verify the expected UI
   state after submitting. Both checks should throw on failure.

6. **Shell escaping in eval.** Use `agent-browser eval --stdin <<'HEREDOC'` for JavaScript with
   quotes, template literals, or arrow functions.

7. **Rich text editor focus.** ProseMirror, TipTap, and similar editors may need a real click/focus
   event before typing.

8. **Portal position mismatch while zoomed.** Reset zoom before opening dropdowns or popovers.

9. **Click ripple appears in the wrong place.** The visual cursor was probably moved with hard-
   coded `demoMoveTo(x, y)` coordinates while the real click used a semantic locator. Replace it
   with `demoMoveToEl(selector)` or `demoMoveToText(text)` so the ripple reads live layout from the
   target element.
