#!/usr/bin/env python3
"""Shrinks the configs that scripts/stmt_matrix.go found problems with to the options that matter.

  python3 scripts/matrix_triage.py <smx binary> <sql dir> <config.yaml>...

For every config it takes the first reports of stmt_matrix, drops one option after the other while the
statement still fails (`stmt_matrix -check`), and prints the minimal configs, grouped, each with the
statement. The samples are written to <triage dir>/cl_N.sql / cl_N.yaml (default: ./triage_out).
"""
import collections, copy, json, os, re, subprocess, sys

smx, corpus, cfgs = sys.argv[1], sys.argv[2], sys.argv[3:]
out_dir = os.environ.get("TRIAGE_OUT", "triage_out")
os.makedirs(out_dir, exist_ok=True)
kind_of = [None]


def leaves(d, p=()):
    for k, v in d.items():
        if isinstance(v, dict):
            yield from leaves(v, p + (k,))
        else:
            yield p + (k,)


def without(d, path):
    d = copy.deepcopy(d)
    x = d
    for k in path[:-1]:
        x = x[k]
    del x[path[-1]]
    return d


def check(cfg, sql):
    json.dump(cfg, open(out_dir + "/_c.yaml", "w"))
    open(out_dir + "/_in.sql", "w").write(sql)
    r = subprocess.run([smx, "-check", out_dir + "/_c.yaml", out_dir + "/_in.sql"], capture_output=True, text=True).stdout.strip()
    return r


clusters = collections.defaultdict(list)
for c in cfgs:
    res = subprocess.run([smx, corpus, "4", "fp:" + c], capture_output=True, text=True).stdout
    for m in re.finditer(r"=== (\S+) \[.*?\] wrap=(\w+)\s+(.*?)\n--- in\n(.*?)\n--- out\n", res, re.S):
        sql = m.group(4)
        cfg = json.load(open(c))
        kind = check(cfg, sql)
        if kind in ("ok", "input-error", "config-error"):
            continue
        changed = True
        while changed:
            changed = False
            for p in list(leaves(cfg)):
                if p in {("data_types", "form"), ("syntax_errors",)}:
                    continue
                c2 = without(cfg, p)
                if check(c2, sql) == kind:
                    cfg, changed = c2, True
        flat = {".".join(p): eval("cfg" + "".join(f"[{k!r}]" for k in p)) for p in leaves(cfg)}
        clusters[json.dumps(flat, sort_keys=True)].append((kind, sql, cfg))
for n, (key, lst) in enumerate(sorted(clusters.items(), key=lambda x: -len(x[1]))):
    kind, sql, cfg = lst[0]
    print("=" * 70, "\nMIN CONFIG:", key, "\n%s x%d -> %s/cl_%d.sql" % (kind, len(lst), out_dir, n))
    open("%s/cl_%d.sql" % (out_dir, n), "w").write(sql + "\n")
    json.dump(cfg, open("%s/cl_%d.yaml" % (out_dir, n), "w"))
