#!/usr/bin/env python3
"""Makes config sets from the reference config (.pg_procrustes.yaml) for scripts/stmt_matrix.go.

The option space is read from the inline comments of the reference config ("# preserve | add | remove"),
so a new option is covered as soon as it is documented there. Two sets are written below <out>:

  ofat/  one config per option value (all others at their defaults)
  pair/  a small set of configs in which every pair of option values occurs together
         (greedy pairwise cover: ~50 configs for ~90 options)

data_types.form stays "preserve", syntax_errors "skip" (output must keep the parse tree),
schema_qualification and line_endings are left out (they change the tree / the line ends on purpose).

  go build -o /tmp/smx scripts/stmt_matrix.go
  python3 scripts/option_sweep.py .pg_procrustes.yaml /tmp/sweep 1
  /tmp/smx ~/some/sql/dir 3 $(for f in /tmp/sweep/pair/*.yaml; do echo fp:$f; done)
  python3 scripts/matrix_triage.py /tmp/smx ~/some/sql/dir /tmp/sweep/pair/p004.yaml ...   # minimal failing configs
"""
import itertools, json, os, random, re, sys


def parse(path):
    stack, opts = [], {}
    for raw in open(path):
        m = re.match(r'^(\s*)([A-Za-z_]+):\s*("[^"]*"|[^\s#]*)\s*(#\s*(.*))?$', raw.rstrip("\n"))
        if not m:
            continue
        ind, key, val, cm = len(m.group(1)), m.group(2), m.group(3).strip('"'), m.group(5) or ""
        while stack and stack[-1][0] >= ind:
            stack.pop()
        if val == "":
            stack.append((ind, key))
            continue
        mm = re.match(r'^((?:"[^"]*"|[A-Za-z0-9_:\-\.]+)(?:\s*\|\s*(?:"[^"]*"|[A-Za-z0-9_:\-\.]+))+)', cm)
        if mm:
            vals = [v.strip().strip('"') for v in mm.group(1).split("|")]
            if len(vals) >= 2:
                opts[tuple(k for _, k in stack) + (key,)] = vals
    return opts


def setp(d, path, val):
    for p in path[:-1]:
        d = d.setdefault(p, {})
    d[path[-1]] = val


def main():
    ref, out = sys.argv[1], sys.argv[2]
    seed = int(sys.argv[3]) if len(sys.argv) > 3 else 1
    space = parse(ref)
    space[("layout", "line_length")] = [40, 60, 80, 120, 200]
    space[("layout", "indent", "size")] = [2, 3, 4, 8]
    skip = {("data_types", "form"), ("syntax_errors",), ("line_endings",), ("schema_qualification",)}
    keys = [k for k in space if k not in skip]

    def mk(assign):
        c = {"data_types": {"form": "preserve"}, "syntax_errors": "skip"}
        for k, v in assign.items():
            setp(c, k, v)
        return c

    for d in ("ofat", "pair"):
        os.makedirs(f"{out}/{d}", exist_ok=True)
    n = 0
    for k in keys:
        for v in space[k]:
            json.dump(mk({k: v}), open(f"{out}/ofat/o{n:03d}.yaml", "w"))
            n += 1
    rnd = random.Random(seed)
    uncovered = {(a, va, b, vb) for a, b in itertools.combinations(range(len(keys)), 2)
                 for va in range(len(space[keys[a]])) for vb in range(len(space[keys[b]]))}
    cfgs = []
    while uncovered:
        best, bestc = None, -1
        for _ in range(40):
            pick = [rnd.randrange(len(space[k])) for k in keys]
            a, va, b, vb = rnd.choice(tuple(uncovered)) if len(uncovered) < 5000 else next(iter(uncovered))
            pick[a], pick[b] = va, vb
            for i in range(len(keys)):
                bv, bc = pick[i], -1
                for v in range(len(space[keys[i]])):
                    c = sum(1 for j in range(len(keys)) if j != i and
                            ((i, v, j, pick[j]) if i < j else (j, pick[j], i, v)) in uncovered)
                    if c > bc:
                        bc, bv = c, v
                pick[i] = bv
            cnt = sum(1 for i in range(len(keys)) for j in range(i + 1, len(keys)) if (i, pick[i], j, pick[j]) in uncovered)
            if cnt > bestc:
                bestc, best = cnt, pick
        if bestc <= 0:
            a, va, b, vb = next(iter(uncovered))
            best = [rnd.randrange(len(space[k])) for k in keys]
            best[a], best[b] = va, vb
        for i in range(len(keys)):
            for j in range(i + 1, len(keys)):
                uncovered.discard((i, best[i], j, best[j]))
        cfgs.append(best)
    for i, p in enumerate(cfgs):
        json.dump(mk({keys[j]: space[keys[j]][p[j]] for j in range(len(keys))}), open(f"{out}/pair/p{i:03d}.yaml", "w"))
    print(len(keys), "options;", n, "ofat configs;", len(cfgs), "pairwise configs")


if __name__ == "__main__":
    main()
