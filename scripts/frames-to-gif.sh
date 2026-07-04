#!/usr/bin/env bash
# Converts a directory of .ansi frames into an animated GIF.
#
# Pipeline: .ansi → aha → .html → wkhtmltoimage → .png → ImageMagick → .gif
#
# Usage: frames-to-gif.sh <frames_dir> <output.gif> [delay_ms]
#   frames_dir: directory containing frame-NNN.ansi files (ordered)
#   output.gif: path for the output animated GIF
#   delay_ms:   delay between frames in milliseconds (default: 1200 = 1.2s)
set -euo pipefail

FRAMES_DIR="${1:?usage: frames-to-gif.sh <frames_dir> <output.gif> [delay_ms]}"
OUTPUT_GIF="${2:?usage: frames-to-gif.sh <frames_dir> <output.gif> [delay_ms]}"
DELAY_MS="${3:-1200}"

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

echo "converting ANSI frames to PNG..."

# Convert each .ansi file to .png.
count=0
for ansi_file in "$FRAMES_DIR"/frame-*.ansi; do
    [[ -f "$ansi_file" ]] || continue
    base=$(basename "$ansi_file" .ansi)
    html_file="$TMP_DIR/${base}.html"
    png_file="$TMP_DIR/${base}.png"

    # ANSI → HTML.
    aha --black --no-header < "$ansi_file" > "$html_file"

    # Wrap in styled HTML (same compact theme as ansi-to-png.sh).
    cat > "${html_file}.tmp" << HTMLWRAP
<!DOCTYPE html>
<html>
<head>
<style>
  body {
    margin: 0;
    padding: 12px;
    background: #0d1117;
    font-family: 'Cascadia Code', 'Fira Code', 'JetBrains Mono', 'Consolas', monospace;
    font-size: 13px;
    line-height: 1.35;
  }
  pre { margin: 0; white-space: pre-wrap; word-wrap: break-word; }
</style>
</head>
<body>
<pre>$(cat "$html_file")</pre>
</body>
</html>
HTMLWRAP
    mv "${html_file}.tmp" "$html_file"

    # HTML → PNG.
    wkhtmltoimage --quiet \
        --quality 60 \
        --width 820 \
        --height 0 \
        --enable-local-file-access \
        "$html_file" "$png_file"

    rm -f "$html_file"
    count=$((count + 1))
done

echo "assembling $count PNGs into animated GIF..."

# Convert delay to centiseconds for ImageMagick (-delay uses 1/100 sec).
DELAY_CS=$((DELAY_MS / 10))

# Assemble PNGs into an animated GIF.
# -delay sets frame duration, -loop 0 = infinite loop, -layers optimize
# reduces file size by keeping only changed pixels between frames.
convert "$TMP_DIR"/frame-*.png \
    -delay "$DELAY_CS" \
    -loop 0 \
    -layers Optimize \
    -colors 128 \
    "$OUTPUT_GIF"

# Optimize further with gifsicle if available.
if command -v gifsicle &>/dev/null; then
    gifsicle --batch --optimize=3 "$OUTPUT_GIF"
fi

size=$(stat --format=%s "$OUTPUT_GIF" 2>/dev/null || stat -f%z "$OUTPUT_GIF" 2>/dev/null || echo "?")
echo "generated $OUTPUT_GIF ($count frames, ${size} bytes)"
