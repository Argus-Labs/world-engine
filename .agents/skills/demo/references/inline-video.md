# Show a video inline in BB

Use this for a reviewed recording or rendered demo. Show screenshots with the client's
inline image support.

1. Save the HTML wrapper and final MP4 together in a dedicated directory under the
   system temporary directory or BB thread storage. For example, create a directory
   with `mktemp -d "${TMPDIR:-/tmp}/pr-demo.XXXXXX"`. Keep it readable on the thread's
   host and outside tracked source; this repository does not ignore `.scratch/`.
2. Use a static relative media URL such as `src="./clip.mp4"`. BB fetches the file through
   its authenticated host API and transfers it into the sandboxed iframe. Keep the video
   separate rather than converting it to base64.
3. The 5 MiB limit applies to the HTML wrapper. On BB 0.43.3, a separate video may be up
   to 25 MiB. Preserve the original export; make a delivery copy only if needed.
4. Emit `inline-vis` with the wrapper's absolute filesystem path, on its own line outside
   a code fence. There is no `source` attribute or relative-path fallback. Expand
   `$BB_THREAD_STORAGE` to its actual value if used. Leave both files in place.

Example wrapper saved beside `clip.mp4`:

```html
<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <style>
      html,
      body {
        margin: 0;
        height: 100%;
        background: #0b0b0d;
        overflow: hidden;
      }
      body {
        display: flex;
        align-items: center;
        justify-content: center;
      }
      video {
        display: block;
        width: 100%;
        height: auto;
        aspect-ratio: 16/9;
        object-fit: contain;
        border: 1px solid #3a3a40;
        border-radius: 8px;
        box-sizing: border-box;
        background: #000;
      }
    </style>
  </head>
  <body>
    <video controls autoplay muted loop playsinline src="./clip.mp4"></video>
  </body>
</html>
```

Emit `::inline-vis{file="/tmp/pr-demo-example/player.html" height="400"}` for a typical landscape clip,
replacing the example path with the actual absolute path to the saved wrapper.
The height is fixed. Use the clip's actual aspect ratio in the CSS and a suitable iframe
height for portrait or other formats. Keep `object-fit: contain` to avoid cropping.

Verify that the HTML wrapper and video both exist, load in BB, and fit their respective size limits.
Remote users need this inline delivery or a reachable shared URL. A localhost or local
filesystem link does not make a recording accessible from another machine.
