# Pyroscope v2 architecture animations

Source for the animated diagrams used in the repository `README.md`
("How Does Pyroscope Work?" section). There are three independent scenes:

| Scene             | GIF                                   | Shows                                              |
| ----------------- | ------------------------------------- | -------------------------------------------------- |
| `write-path`      | `images/pyroscope-v2-write-path.gif`  | clients → distributor → segment-writer → object storage (+ metastore) |
| `compaction`      | `images/pyroscope-v2-compaction.gif`  | segments merged into larger blocks by compaction-workers |
| `read-path`       | `images/pyroscope-v2-read-path.gif`   | query-frontend / query-backend reading object storage into a flame graph |

Each scene is a standalone HTML file (`<scene>.html`) that draws an SVG and
exposes a deterministic `seek(t)` function (`t` in `[0, 1]`). Passing
`?t=<value>` renders a single static frame, which is how frames are captured.
Shared drawing primitives live in `lib.js`; edit a scene without touching the
others.

## Regenerate the GIFs

Requires a headless Chrome/Chromium binary and `ffmpeg`. `chrome-headless-shell`
is expected on `PATH`; override with the `CHROME` env var.

```sh
# Build all three GIFs into ../../images/
CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" ./build.sh

# Or one scene at a time:
./render.sh write-path            # frames → frames/write-path/
./assemble.sh write-path          # frames → ../../images/pyroscope-v2-write-path.gif
# assemble.sh <scene> [width] [fps] [colors] [bayer_scale]
```

## Preview locally

Open any `<scene>.html` with no query string to watch it loop; add `?t=0.6` to
inspect a specific frame.
