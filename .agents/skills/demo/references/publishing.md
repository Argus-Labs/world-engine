# Share a demo

Use [inline video](inline-video.md) for delivery in BB chat. Read this reference when the
requested delivery also includes an external upload or a change to an existing PR or issue.
Use authorization already given for that action. Recording alone does not authorize either.

## Upload

Use the session's available upload capability or an authenticated CLI for the intended
destination. Check its current inputs and supported formats. Send the reviewed video and
matching thumbnail, then use the returned URLs. Do not assume a named upload tool exists
or invent a destination, asset URL, or pending-upload file to delete.

Verify that the intended audience can reach the returned media. If an upload succeeds but
editing the PR or issue is outside scope, return the media URL. If external delivery is
unavailable, use supported inline delivery and report the unavailable destination.
Inspect remote state before retrying a partially completed upload or edit.

## Attach to an existing PR or issue

1. Read the current body. Preserve text and media unrelated to this demo.
2. Reuse the existing demo section or managed media region, including its existing markers.
   If none exists, add the repository's `## Screenshot / Recordings` section. Do not create a PR or issue merely to
   attach a recording.
3. Replace stale versions of this demo and its thumbnail instead of accumulating duplicate
   takes. Keep only the assets needed to show the relevant result.
4. Use the upload tool's returned embed, or Markdown built from its actual URLs. Put the
   video beside the short claim it supports and add supporting screenshots when useful.
5. Apply the authorized edit with the session's GitHub tooling. Read the body back and
   verify the links, relevant media, and preservation of unrelated content.
