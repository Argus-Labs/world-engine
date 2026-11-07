# Edit recorded demos with Remotion

Use this after capturing and checking a real workflow, or when the user supplies footage.
A clean single take can be delivered directly. Use Remotion when the result needs edits,
captions, callouts, zooms, transitions, or a combination of takes.

## Prepare the footage

1. Keep the original recording and the assertions or transcript that establish its result.
2. Inspect the clip's duration, dimensions, frame rate, audio, and the moments to retain.
   When installed, use `media:remotion-multimedia` for metadata or media processing;
   otherwise use available media tools such as `ffprobe`.
3. Record a short edit list with source timestamps, intended cuts, captions, and emphasis.
   Keep event order and timing intact when the demo is evidence of latency or animation.
   Label speed changes when used in an instructional walkthrough.

## Build the edit

Reuse the target project's Remotion composition when it has one. Otherwise, create an
editable composition in an ignored scratch directory or system temporary directory, keeping
original footage alongside it. When the optional media plugin is installed, load
`media:remotion-create` for its current setup instructions. Otherwise use an available
Remotion installation and its documentation. Keep generated files out of tracked source.

When installed, load the relevant optional media skills; otherwise use the installed
Remotion documentation for the same task:

- `media:remotion-markup` for embedding recorded video, trimming, sequencing, labels, and motion.
  Consult that skill's video editing guide for Studio-editable clips.
- `media:remotion-captions` for transcribed speech or supplied subtitles. Brief visual callouts
  can be authored directly in the composition.
- `media:remotion-interactivity` when the user should adjust the edit in Studio.

Use the source clip's dimensions and frame rate as the starting point. Convert timestamps
according to the installed API's units. Check current documentation with
the installed `media:remotion-docs` skill or current Remotion documentation when a
prop or version is uncertain.

Add presentation overlays in the composition so wording and timing can change without
another application capture. Do not add a second cursor or duplicate captions if the take
already includes them. Position callouts against the recorded viewport, accounting for
cropping, scaling, and any movement of the target. Avoid covering the state being proved.

## Preview, export, and review

1. Preview the edit with available Remotion tooling, using `media:remotion-studio`
   when installed. Check cuts, overlay timing, readability,
   audio, and the final state against the original take.
2. When the requested deliverable is a demo video, export it with available Remotion
   tooling, using `media:remotion-render` when installed.
   That request includes the export. A preview-only request ends at the preview.
3. Inspect the final video and a matching thumbnail. Check its actual duration, dimensions,
   and playback rate. A 60 fps export does not turn a lower-rate source into 60 fps capture.
4. Keep the editable composition, source footage, and export together. For BB delivery,
   follow [inline video](inline-video.md). For authorized external delivery, follow
   [publishing](publishing.md).
