"""Inventory all evidence, retaining red tests and rejected configurations."""
from pathlib import Path
import hashlib
import json
import subprocess

ROOT = Path(__file__).resolve().parent
REPO = ROOT.parents[4]


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def case(path):
    folder = path.parents[1].name
    excluded = "before-fix regression" if folder == "red-memo" else ""
    if folder.startswith("stable-"):
        excluded = "memo not activated; invalid optimization comparison"
    if path.stem == "TestLiveComponentCapacity":
        excluded = "nonexistent selected test; runner verdict subsequently fixed"
    if folder == "missing-case-rejected":
        excluded = "expected post-fix rejection of nonexistent selected test"
    return {"log": path.relative_to(ROOT).as_posix(), "passed": f"--- PASS: {path.stem} (" in path.read_text(),
            "excluded_from_current_verdict": excluded, "name": path.stem, "profile": folder}


def inventory():
    logs = [path for path in ROOT.rglob("Test*.log") if "-vm-" not in path.stem]
    latest = {}
    all_cases = []
    for path in sorted(logs, key=lambda p: p.stat().st_mtime):
        item = case(path)
        all_cases.append(item)
        if item["excluded_from_current_verdict"]:
            continue
        condition = item["profile"] if item["name"] in ("TestLiveSemanticIndependentHoldout", "TestLiveCacheBypassLoad") else ""
        latest[(path.parent.name, path.stem, condition)] = item
    source = list((REPO/"internal").rglob("*.go"))+list((REPO/"cmd").rglob("*.go"))+[REPO/"go.mod", REPO/"go.sum"]
    snapshot = json.loads((ROOT/"verified-build-source.json").read_text())
    tracked = {r["path"] for r in snapshot["source"]}
    production_matches = all(sha(REPO/r["path"]) == r["sha256"] for r in snapshot["source"] if not r["path"].endswith("_test.go"))
    return {"head": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=REPO, text=True).strip(),
            "baseline_head": "f19f316", "merged_main": False, "production_cache_enabled": False,
            "report_sha256": sha(ROOT.parents[1]/"29-FOLLOWUP-RESULTS-20261003.md"),
            "all_cases": all_cases, "latest_cases": list(latest.values()),
            "original_release_gate_passed": False, "semantic_policy_approved": False,
            "skipped_providers": {"OR": "known HTTP429; quota type not preserved; authorized resource exclusion", "GEM": "resource unavailable", "SANS": "resource unavailable"},
            "verified_production_sources_still_match": production_matches,
            "additional_source_after_verified_build": [p.relative_to(REPO).as_posix() for p in source if p.relative_to(REPO).as_posix() not in tracked],
            "source": [{"path": p.relative_to(REPO).as_posix(), "sha256": sha(p)} for p in sorted(source)],
            "binaries": [{"path": p.relative_to(REPO).as_posix(), "sha256": sha(p)} for p in sorted((REPO/".tmp"/"phase29-followup-20261003").glob("*")) if p.is_file()],
            "artifacts": [{"path": p.relative_to(ROOT).as_posix(), "sha256": sha(p), "bytes": p.stat().st_size}
                          for p in sorted(ROOT.rglob("*")) if p.is_file() and p.name != "manifest.json"]}


result = inventory()
(ROOT/"manifest.json").write_text(json.dumps(result, indent=2)+"\n")
print(json.dumps({"latest_cases": len(result["latest_cases"]), "unpassed": [c for c in result["latest_cases"] if not c["passed"]],
                  "production_matches_verified": result["verified_production_sources_still_match"], "artifacts": len(result["artifacts"])}))
