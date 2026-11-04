"""Compares what tools/parity/scenarios.json leaves in Typora and in Aurink.

Usage: python tools/parity/compare.py typora.json aurink.json

Line endings are made LF and trailing blank lines dropped before comparing;
everything else must match byte for byte.
"""
import json
import sys


def norm(s: str) -> str:
    return s.replace("\r\n", "\n").rstrip("\n")


def main() -> int:
    typora = json.load(open(sys.argv[1], encoding="utf-8-sig"))
    aurink = json.load(open(sys.argv[2], encoding="utf-8-sig"))
    order = [s["name"] for s in json.load(open(__file__.replace("compare.py", "scenarios.json"), encoding="utf-8"))]
    same, differ, missing = [], [], []
    for name in order:
        if name not in typora or name not in aurink:
            missing.append(name)
            continue
        t, a = norm(typora[name]), norm(aurink[name])
        (same if t == a else differ).append((name, t, a))
    for name, t, a in differ:
        print(f"DIFF {name}\n  typora: {t!r}\n  aurink: {a!r}")
    print(f"\n{len(same)} same, {len(differ)} differ, {len(missing)} missing: {missing}")
    return 1 if differ else 0


if __name__ == "__main__":
    sys.exit(main())
