"""Compare two recorded plan sets (populated vs empty clone) query by query.

usage: compare_plans.py <populated_dir> <clone_dir> [markdown_out]
Shape = depth-first list of (node type, relation, index, join type). Estimates are NOT part of shape.
Full tree = shape plus every node's Plan Rows and Total Cost (rounded to 2 dp).
"""
import json, os, sys

def shape(node, acc):
    acc.append((node["Node Type"], node.get("Relation Name"), node.get("Index Name"), node.get("Join Type")))
    for child in node.get("Plans", []):
        shape(child, acc)
    return acc

def flatten(node, acc):
    acc.append((node["Node Type"], node.get("Relation Name"), node.get("Index Name"), node["Plan Rows"], round(node["Total Cost"], 2)))
    for c in node.get("Plans", []):
        flatten(c, acc)
    return acc

def names(d):
    return sorted(f[:-5] for f in os.listdir(d) if f.endswith(".json"))

def compare(a_dir, b_dir):
    """Yield (name, shape_equal, tree_equal, plan_a, plan_b) for every query in both directories."""
    for n in names(a_dir):
        if not os.path.exists(os.path.join(b_dir, n + ".json")):
            continue
        a = json.load(open(os.path.join(a_dir, n + ".json")))
        b = json.load(open(os.path.join(b_dir, n + ".json")))
        yield n, shape(a, []) == shape(b, []), flatten(a, []) == flatten(b, []), a, b

if __name__ == "__main__":
    a_dir, b_dir = sys.argv[1:3]
    md_out = sys.argv[3] if len(sys.argv) > 3 else None
    results = list(compare(a_dir, b_dir))
    same = sum(1 for _, s, _, _, _ in results if s)
    md = ["| Query | Shape | Populated top node | rows | cost | Clone top node | rows | cost |", "|---|---|---|---:|---:|---|---:|---:|"]
    print(f"{'query':8} {'shape':6} {'populated (top, rows)':44} {'clone (top, rows)':44}")
    for n, eq, _, a, b in results:
        print(f"{n:8} {'same' if eq else 'DIFF':6} {a['Node Type'] + ' rows=' + str(a['Plan Rows']):44} {b['Node Type'] + ' rows=' + str(b['Plan Rows']):44}")
        md.append(f"| {n} | {'same' if eq else 'DIFF'} | {a['Node Type']} | {a['Plan Rows']} | {a['Total Cost']:,.2f} | {b['Node Type']} | {b['Plan Rows']} | {b['Total Cost']:.2f} |")
    print(f"\nshape agreement: {same}/{len(results)}")
    if md_out:
        md.append(f"\nShape agreement: {same}/{len(results)}")
        open(md_out, "w").write("\n".join(md) + "\n")
