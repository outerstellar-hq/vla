#!/usr/bin/env bash
# Converts .ansi files to optimized .png using aha + wkhtmltoimage + optipng.
# Usage: ansi-to-png.sh file.ansi [file2.ansi ...]
# Output: file.png (same name, .png extension)
set -euo pipefail

for ansi_file in "$@"; do
    if [[ ! -f "$ansi_file" ]]; then
        echo "skip: $ansi_file not found" >&2
        continue
    fi

    png_file="${ansi_file%.ansi}.png"
    html_file="${ansi_file}.html"

    # ANSI → HTML with dark background.
    aha --black --no-header < "$ansi_file" > "$html_file"

    # Wrap in a minimal HTML page with monospace font + dark bg.
    # Compact sizing: 14px font, minimal padding, tight line-height.
    cat > "$html_file.tmp" << HTMLWRAP
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
  pre {
    margin: 0;
    white-space: pre-wrap;
    word-wrap: break-word;
  }
</style>
</head>
<body>
<pre>$(cat "$html_file")</pre>
</body>
</html>
HTMLWRAP
    mv "$html_file.tmp" "$html_file"

    # HTML → PNG. Use --quality for JPEG compression within PNG, and
    # let wkhtmltoimage auto-crop height (--height 0 = no fixed height).
    wkhtmltoimage --quiet \
        --quality 60 \
        --width 820 \
        --height 0 \
        --enable-local-file-access \
        "$html_file" "$png_file"

    # Lossless optimization (strips metadata, recompresses).
    if command -v optipng &>/dev/null; then
        optipng -quiet -strip all "$png_file"
    fi

    rm -f "$html_file"

    # Report size.
    size=$(stat --format=%s "$png_file" 2>/dev/null || stat -f%z "$png_file" 2>/dev/null || echo "?")
    echo "generated $png_file (${size} bytes)"
done
