#!/usr/bin/env bash
# Converts .ansi files to .png using aha (ANSI HTML Adapter) + wkhtmltoimage.
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
    cat > "$html_file.tmp" << HTMLWRAP
<!DOCTYPE html>
<html>
<head>
<style>
  body {
    margin: 0;
    padding: 16px;
    background: #0d1117;
    font-family: 'Cascadia Code', 'Fira Code', 'JetBrains Mono', 'Consolas', monospace;
    font-size: 14px;
    line-height: 1.4;
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

    # HTML → PNG.
    wkhtmltoimage --quiet \
        --width 1200 \
        --height 800 \
        --quality 90 \
        --enable-local-file-access \
        "$html_file" "$png_file"

    rm -f "$html_file"
    echo "generated $png_file"
done
