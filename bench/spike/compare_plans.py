"""Compare two recorded plan sets (populated vs empty clone).

usage: compare_plans.py <populated_dir> <clone_dir> [markdown_out]
Shape = depth-first list of (node type, relation, index, join type). Estimates are NOT part of shape.
Prints a comparison; if markdown_out is given, also writes a Markdown table of top nodes and costs.
"""
import json, os, sys

def shape(node, acc):
    acc.append((node["Node Type"], node.get("Relation Name"), node.get("Index Name"), node.get("Join Type")))
    for child in node.get("Plans", []):
        shape(child, acc)
    return acc

a_dir, b_dir = sys.argv[1:3]
md_out = sys.argv[3] if len(sys.argv) > 3 else None
same, md = 0, ["| Query | Shape | Populated top node | rows | cost | Empty clone top node | rows | cost |",
               "|---|---|---|---:|---:|---|---:|---:|"]
print(f"{'query':6} {'shape':6} {'populated (top, rows)':44} {'clone (top, rows)':44}")
for i in range(1, 23):
    name = f"q{i:02d}.json"
    a = json.load(open(os.path.join(a_dir, name)))
    b = json.load(open(os.path.join(b_dir, name)))
    eq = shape(a, []) == shape(b, [])
    same += eq
    print(f"q{i:02d}   {'same' if eq else 'DIFF':6} {a['Node Type'] + ' rows=' + str(a['Plan Rows']):44} {b['Node Type'] + ' rows=' + str(b['Plan Rows']):44}")
    md.append(f"| q{i:02d} | {'same' if eq else 'DIFF'} | {a['Node Type']} | {a['Plan Rows']} | {a['Total Cost']:,.2f} | {b['Node Type']} | {b['Plan Rows']} | {b['Total Cost']:.2f} |")
print(f"\nshape agreement: {same}/22")
if md_out:
    md.append(f"\nShape agreement: {same}/22")
    open(md_out, "w").write("\n".join(md) + "\n")
