"""Produce an evidence inventory; retain expected failures and historical failures."""
from collections import defaultdict
from pathlib import Path
import hashlib
import json
import subprocess

ROOT = Path(__file__).resolve().parent
REPO = ROOT.parents[4]


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def samples(path):
    return [json.loads(line.split("SAMPLE ", 1)[1]) for line in path.read_text().splitlines() if "SAMPLE " in line]


def inventory():
    latest = {}
    for path in sorted(ROOT.rglob("Test*.log"), key=lambda item: item.stat().st_mtime):
        if "-vm-" in path.stem:
            continue
        latest[(path.parent.name, path.stem)] = {"log": path.relative_to(ROOT).as_posix(),
                                              "passed": f"--- PASS: {path.stem} (" in path.read_text()}
    files = [path for path in ROOT.rglob("*") if path.is_file() and path.name != "manifest.json"]
    binaries = list((REPO / ".tmp" / "phase29-remediation-20261003").glob("*.test"))
    binaries += list((REPO / ".tmp" / "phase29-remediation-20261003").glob("runner*.exe"))
    source = list((REPO / "internal").rglob("*.go")) + list((REPO / "cmd").rglob("*.go"))
    source += [REPO / "go.mod", REPO / "go.sum"]
    report = ROOT.parents[1] / "29-REMEDIATION-RESULTS-20261003.md"
    return {"report_sha256": sha(report), "baseline_head": "b2eb95ff", "head": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=REPO, text=True).strip(),
            "latest_cases": list(latest.values()),
            "artifacts": [{"path": path.relative_to(ROOT).as_posix(), "sha256": sha(path), "bytes": path.stat().st_size}
                          for path in sorted(files)],
            "binaries": [{"path": path.relative_to(REPO).as_posix(), "sha256": sha(path)} for path in sorted(binaries)],
            "source": [{"path": path.relative_to(REPO).as_posix(), "sha256": sha(path)} for path in sorted(source)]}


result = inventory()
(ROOT / "manifest.json").write_text(json.dumps(result, indent=2) + "\n")
print(json.dumps({"latest_cases": len(result["latest_cases"]), "unpassed": [item for item in result["latest_cases"] if not item["passed"]],
                  "artifacts": len(result["artifacts"]), "binaries": len(result["binaries"])}))
