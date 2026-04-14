#!/usr/bin/env python3
"""Combine per-platform benchmark JSON files into a README markdown section.

Usage:
    python update-readme.py linux-cgo.json linux-nocgo.json darwin-cgo.json ...

Reads each JSON file (as produced by TestCIBenchmarks) and generates a single
unified HTML table. Each platform+cgo combination gets its own column group,
e.g. "macOS (arm64)" and "macOS (arm64) no cgo" side by side.
"""

import json
import sys
import os
from datetime import datetime, timezone

MARKER_START = "<!-- BENCHMARK_RESULTS_START -->"
MARKER_END = "<!-- BENCHMARK_RESULTS_END -->"

PLATFORM_LABELS = {
    "linux": "Linux",
    "darwin": "macOS",
    "windows": "Windows",
}

PLATFORM_ORDER = ["linux", "darwin", "windows"]


def load_reports(paths):
    reports = []
    for path in paths:
        if not os.path.exists(path):
            continue
        with open(path) as f:
            reports.append(json.load(f))
    return reports


def build_columns(reports):
    """Return ordered list of (key, heading, report) tuples.

    Ordered by platform, then cgo before nocgo within each platform.
    key is a unique string like 'darwin-cgo' used for cell lookups.
    """
    # Group: {platform: {True: report, False: report}}
    grouped = {}
    for r in reports:
        p = r["platform"]
        cgo = r.get("cgo", False)
        grouped.setdefault(p, {})[cgo] = r

    columns = []
    for p in PLATFORM_ORDER:
        if p not in grouped:
            continue
        variants = grouped[p]
        label = PLATFORM_LABELS.get(p, p)
        has_both = True in variants and False in variants

        for cgo in [True, False]:
            if cgo not in variants:
                continue
            r = variants[cgo]
            arch = r.get("arch", "")
            heading = f"{label} ({arch})" if arch else label
            if has_both and not cgo:
                heading += " no cgo"
            key = f"{p}-{'cgo' if cgo else 'nocgo'}"
            columns.append((key, heading, r))

    return columns


# ---------- row builders ----------

def sleep_rows(columns):
    ref = columns[0][2]  # first report as reference for durations
    rows = []
    for i, e in enumerate(ref["sleep"]):
        cells = {}
        for key, _, r in columns:
            s = r["sleep"][i]
            cells[key] = [
                f'<code>{s["hpt_mean"]}</code>',
                f'<code>{s["stdlib_mean"]}</code>',
                f'<b>{s["mean_improvement"]}</b>',
            ]
        rows.append((e["duration"], cells))
    return rows


def ticker_rows(columns):
    entries = [
        ("Median jitter", "hpt_median_jitter", "stdlib_median_jitter", None),
        ("Mean jitter",   "hpt_mean_jitter",   "stdlib_mean_jitter",   "mean_improvement"),
        ("p95 jitter",    "hpt_p95_jitter",    "stdlib_p95_jitter",    None),
        ("p99 jitter",    "hpt_p99_jitter",    "stdlib_p99_jitter",    "p99_improvement"),
        ("Max jitter",    "hpt_max_jitter",    "stdlib_max_jitter",    None),
        ("Total drift",   "hpt_total_drift",   "stdlib_total_drift",   None),
    ]
    rows = []
    for label, hpt_key, std_key, impr_key in entries:
        cells = {}
        for key, _, r in columns:
            tk = r["ticker"]
            hpt_val = tk.get(hpt_key, "—")
            std_val = tk.get(std_key, "—")
            impr = f'<b>{tk[impr_key]}</b>' if impr_key and impr_key in tk else "—"
            cells[key] = [
                f'<code>{hpt_val}</code>',
                f'<code>{std_val}</code>',
                impr,
            ]
        rows.append((label, cells))
    return rows


def timer_rows(columns):
    ref = columns[0][2]
    rows = []
    for i, e in enumerate(ref["timer"]):
        cells = {}
        for key, _, r in columns:
            t = r["timer"][i]
            cells[key] = [
                f'<code>{t["hpt_mean"]}</code>',
                f'<code>{t["stdlib_mean"]}</code>',
                f'<b>{t["mean_improvement"]}</b>',
            ]
        rows.append((e["duration"], cells))
    return rows


# ---------- table generator ----------

def generate_markdown(reports):
    if not reports:
        return "_No benchmark data available._\n"

    columns = build_columns(reports)
    if not columns:
        return "_No benchmark data available._\n"

    col_keys = [c[0] for c in columns]

    sections = [
        ("Sleep",  sleep_rows(columns)),
        ("Ticker", ticker_rows(columns)),
        ("Timer",  timer_rows(columns)),
    ]

    n_sub = 3  # hpt, time, impr
    lines = []

    now = datetime.now(timezone.utc).strftime("%Y-%m-%d")
    lines.append(f"> Auto-generated on {now} by CI &mdash; "
                 f"[view workflow](../../actions/workflows/benchmarks.yml)")
    lines.append("")
    lines.append("Lower is better for all metrics. "
                 "Impr. = how many times more precise `hpt` is vs `time`. "
                 "Columns without \"no cgo\" use the default cgo build "
                 "(pthread ticker, GC-immune).")
    lines.append("")

    lines.append("<table>")

    # Header row 1: platform spans
    lines.append("<tr>")
    lines.append('  <th colspan="2" rowspan="2"></th>')
    for _, heading, _ in columns:
        lines.append(f'  <th colspan="{n_sub}" align="center">{heading}</th>')
    lines.append("</tr>")

    # Header row 2: sub-columns
    lines.append("<tr>")
    for _ in columns:
        lines.append("  <th><code>hpt</code></th>")
        lines.append("  <th><code>time</code></th>")
        lines.append("  <th>Impr.</th>")
    lines.append("</tr>")

    # Data rows
    for section_name, rows in sections:
        n_rows = len(rows)
        for idx, (label, cells_by_key) in enumerate(rows):
            lines.append("<tr>")
            if idx == 0:
                lines.append(
                    f'  <th rowspan="{n_rows}" align="left">{section_name}</th>'
                )
            lines.append(f"  <td><b>{label}</b></td>")
            for key in col_keys:
                for cell in cells_by_key[key]:
                    lines.append(f"  <td>{cell}</td>")
            lines.append("</tr>")

    lines.append("</table>")
    lines.append("")

    return "\n".join(lines)


def update_readme(markdown):
    readme_path = os.path.join(os.path.dirname(__file__), "..", "..", "README.md")
    readme_path = os.path.normpath(readme_path)

    with open(readme_path) as f:
        content = f.read()

    start = content.find(MARKER_START)
    end = content.find(MARKER_END)
    if start == -1 or end == -1:
        print("ERROR: benchmark markers not found in README.md", file=sys.stderr)
        sys.exit(1)

    new_content = (
        content[:start + len(MARKER_START)]
        + "\n\n"
        + markdown
        + "\n"
        + content[end:]
    )

    with open(readme_path, "w") as f:
        f.write(new_content)

    print(f"Updated {readme_path}")


def main():
    if len(sys.argv) < 2:
        print(f"Usage: {sys.argv[0]} <results1.json> [results2.json] ...",
              file=sys.stderr)
        sys.exit(1)

    reports = load_reports(sys.argv[1:])
    markdown = generate_markdown(reports)
    update_readme(markdown)


if __name__ == "__main__":
    main()
