---
name: demo
description: Capture, edit, and review demo videos of real browser, desktop, or terminal workflows. Use for screen recordings, feature walkthroughs, supporting screenshots, and delivery of existing demo clips.
---

# Demo

<!-- Vendored from Scott's personal/media 0.1.0 skills/demo; adapted for repository-local paths, PR sections, and toolkit layout/target correctness. -->

Own the demo from captured behavior through the reviewed deliverable. Show the smallest
flow that demonstrates the completed behavior. Reuse existing footage when the request
only needs editing or delivery.

External skills and BB delivery are optional integrations. Use the client's available
capture and publishing tools when those integrations are absent.

## Choose the capture and delivery path

| Task                                                       | Use                                                                                                                                                                                |
| ---------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Browser walkthrough                                        | Honor the user's browser choice and session routing. Read that browser's current recording instructions. For agent-browser, use [demo-recording.md](references/demo-recording.md). |
| Desktop workflow                                           | Use the available desktop capture tool and its current instructions. Include native windows when their behavior is the subject.                                                    |
| Terminal workflow                                          | Use `control-cli` when installed, or the available shell tools, to drive a reproducible flow through a terminal recorder. Keep the transcript with the recording.                  |
| Edited video, captions, callouts, zooms, or multiple takes | Use [Remotion editing](references/remotion-editing.md) to compose the captured footage.                                                                                            |
| Video delivery in BB chat                                  | Use [inline video](references/inline-video.md).                                                                                                                                    |
| Authorized upload or PR embed                              | Use [publishing](references/publishing.md).                                                                                                                                        |

Browser and terminal tools capture the application. Remotion edits and renders that footage.
Keep the original take and its assertions. A recreated UI animation does not prove the
application behaved that way.

## Capture and review

- Finish implementation before capturing its demo. A request to record an existing flow
  does not itself require changing the application.
- Prefer 60 fps capture unless the user requests another rate. Use supported recorder
  options and preserve the rate through export. Report capture limitations. Duplicating
  frames does not meet the capture target.
- For desktop browser demos, default to 1920x1080 capture. Set the viewport and verify
  the saved video dimensions using [the recording preflight](references/demo-recording.md#resolution-preflight).
  Honor explicit mobile, responsive, or other requested dimensions.
- Start from a reproducible initial state. Verify the demonstrated outcome before adding
  success captions. Stop the recorder even when the walkthrough fails.
- Review the finished clip's first frame, interactions, and final state. Capture and inspect
  a matching thumbnail after recording. Review the rendered export again if it was edited.
- Retake affected media if later code, copy, style, data, or state changes make it stale.

## Share when requested

Recording, uploading, and changing a PR are separate actions. Use existing authorization
for the requested delivery. A PR's existence alone does not authorize an edit.

In BB, deliver the reviewed clip inline using the reference above. For other clients, use
their supported media presentation. If remote delivery is unavailable, identify the
artifact as local to the host. Do not create a PR merely to attach a demo.

Report the artifact, what it demonstrates, and any failed assertions or recording limitations.
