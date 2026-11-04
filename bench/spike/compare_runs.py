"""Quantify run-to-run noise: compare two runs of the same plan set.

usage: compare_runs.py <dir_run_a> <dir_run_b> [markdown_out]
Reports per query: shape equal?, full tree (node types, relations, indexes, every rows/cost) identical?, relative top-cost change.
"""
import json, os, sys
from compare_plans import compare

if __name__ == "__main__":
    a_dir, b_dir = sys.argv[1:3]
    md_out = sys.argv[3] if len(sys.argv) > 3 else None
    results = list(compare(a_dir, b_dir))
    shape_same = sum(1 for _, s, _, _, _ in results if s)
    identical = sum(1 for _, _, t, _, _ in results if t)
    worst, worst_q = 0.0, ""
    rows = []
    for n, s, t, a, b in results:
        rel = abs(a["Total Cost"] - b["Total Cost"]) / max(abs(a["Total Cost"]), 1e-9)
        if rel > worst:
            worst, worst_q = rel, n
        rows.append(f"| {n} | {'same' if s else 'DIFF'} | {'identical' if t else 'differs'} | {a['Total Cost']:,.2f} | {b['Total Cost']:,.2f} | {rel*100:.4f}% |")
    summary = (f"shape equal {shape_same}/{len(results)}; full tree identical {identical}/{len(results)}; "
               f"max relative top-cost change {worst*100:.4f}% ({worst_q})")
    print(summary)
    if md_out:
        open(md_out, "w").write("| Query | Shape | Full tree | Run A top cost | Run B top cost | Rel. change |\n|---|---|---|---:|---:|---:|\n" + "\n".join(rows) + f"\n\n{summary}\n")
