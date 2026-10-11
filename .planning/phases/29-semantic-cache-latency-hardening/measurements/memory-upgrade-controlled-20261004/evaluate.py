"""Retain failed windows; bracket drift and startup are reported separately."""
from collections import Counter, defaultdict
from datetime import datetime
from pathlib import Path
import importlib.util
import json
import os
import random
import sys

SCRIPT = Path(__file__).resolve().parent
ROOT = Path(os.environ.get("SHIP_RESULTS_ROOT", str(SCRIPT)))
REPO = next(p for p in SCRIPT.parents if (p / "go.mod").exists())
sys.path.insert(0, str(REPO / "scripts"))
from phase29_metrics import comparison_gate, stats, summarize_windows

SPEC = importlib.util.spec_from_file_location("old_evaluator", SCRIPT.parent / "policy-execution-20261003/evaluate.py")
OLD = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(OLD)
MODEL = "LOCAL-qwen2.5-0.5b-instruct"
NAMES = {"off_before": "TestLiveControlledOff", "on": "TestLiveControlledOn",
         "memo": "TestLiveControlledMemo", "off_after": "TestLiveControlledOffAfter"}


def percentile(values, q):
    values = sorted(values)
    return values[min(len(values)-1, int(len(values)*q))] if values else None


def bootstrap(blocks):
    rng, trials = random.Random(29), defaultdict(list)
    for _ in range(2000):
        selected = [blocks[rng.randrange(len(blocks))] for _ in blocks]
        p95 = {mode: stats([r["elapsed_ms"] for b in selected for r in b[mode]])["p95_ms"] for mode in NAMES}
        off = stats([r["elapsed_ms"] for b in selected for mode in ("off_before", "off_after") for r in b[mode]])["p95_ms"]
        for mode in ("on", "memo"):
            trials[f"{mode}_ratio"].append(p95[mode]/off)
            trials[f"{mode}_delta_ms"].append(p95[mode]-off)
    return {"method": "joint six-block bootstrap; exploratory 95 percent interval", "seed": 29,
            "replicates": 2000, "intervals": {k: [percentile(v, .025), percentile(v, .975)] for k, v in trials.items()}}


def load_block(label, index):
    block, detail = {}, {}
    for mode, name in NAMES.items():
        path = ROOT / f"{label}-{index}" / MODEL / f"{name}.log"
        rows = OLD.samples(path, diagnostic=True)
        raw = OLD.records(path)
        if len(rows) != 100 or any("application_ms" not in r for r in rows):
            raise ValueError(f"incomplete 100-request/stage evidence: {path}")
        expected = 4 if label == "low-hit" and mode in ("on", "memo") else 0
        hits = sum(r["hit"] for r in rows)
        ids = {r["request_id"] for r in rows}
        formal_start = min(r["start_ms"] for r in raw if r["type"] == "stage"
                           and r["name"] == "http_handler" and r["request_id"] in ids)
        outcomes = Counter(r["name"] for r in raw if r["type"] == "outcome" and r["start_ms"] >= formal_start)
        memo_hits = outcomes["embedding_memo/hit"]
        valid = hits == expected and memo_hits == (expected if mode == "memo" else 0)
        start = next(r["utc"] for r in raw if r["type"] == "measurement_start")
        end = next(r["utc"] for r in raw if r["type"] == "measurement_end")
        block[mode] = rows
        detail[mode] = {"summary": summarize_windows([rows]), "expected_hits": expected,
                        "activation_valid": valid, "formal_outcomes": dict(outcomes), "start": start, "end": end,
                        "outcome_scope": "aggregate after first formal handler through background drain; no request IDs",
                        "all_outcomes": dict(Counter(r["name"] for r in raw if r["type"] == "outcome"))}
    before, after = (detail[m]["summary"]["latency"]["p95_ms"] for m in ("off_before", "off_after"))
    return block, {"block": index, "windows": detail, "off_anchor_drift_pct": 100*(after/before-1),
                   "drift_alert": abs(after/before-1) > .10,
                   "drift_policy": "diagnostic alert above 10 percent; retain every block"}


def scenario(label):
    blocks, details, windows = [], [], defaultdict(list)
    for i in range(1, 7):
        block, detail = load_block(label, i)
        blocks.append(block)
        details.append(detail)
        for mode in NAMES:
            windows[mode].append(block[mode])
    windows["off"] = [w for mode in ("off_before", "off_after") for w in windows[mode]]
    pooled = {mode: summarize_windows(value) for mode, value in windows.items()}
    failures = sum(pooled[m]["failed"] for m in NAMES)
    valid = all(v["activation_valid"] for d in details for v in d["windows"].values())
    passed = all(r["test_passed"] for b in blocks for rows in b.values() for r in rows)
    gates = {mode: comparison_gate(pooled[mode]["latency"]["p95_ms"], pooled["off"]["latency"]["p95_ms"], failures)
             for mode in ("on", "memo")}
    matched = [r for b in blocks for mode in ("off_before", "off_after") for r in b[mode]
               if r["planned_ms"] in {x["planned_ms"] for x in b["on"] if not x["hit"]}]
    matched_p95 = stats([r["elapsed_ms"] for r in matched])["p95_ms"]
    budgets = {m: OLD.normal_budget(pooled[m]) for m in ("off", "on", "memo")}
    return {"pooled": pooled, "blocks": details, "all_named_tests_passed": passed,
            "activation_valid": valid, "failed": failures, "whole_request_gates": gates,
            "candidate_contract_point_passed": passed and valid and failures == 0
                and all(g["candidate_passed"] for g in gates.values()) and all(b["passed"] for b in budgets.values()),
            "point_pass_is_release_approval": False,
            "matched_off_miss": stats([r["elapsed_ms"] for r in matched]),
            "miss_gates": {m: comparison_gate(pooled[m]["miss_only"]["latency"]["p95_ms"], matched_p95, failures)
                           for m in ("on", "memo")},
            "application_budgets": budgets,
            "bracket_drift_alert_blocks": [d["block"] for d in details if d["drift_alert"]],
            "bootstrap": bootstrap(blocks)}


def utc(value):
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def vm_rows():
    result, columns, boot = [], [], True
    for line in (ROOT / "vmstat.jsonl").read_text().splitlines():
        row = json.loads(line)
        fields = row["line"].split()
        if fields and fields[0] == "r":
            columns = fields[:fields.index("UTC")]
        elif fields and fields[0].isdigit() and columns:
            if boot:
                boot = False
                continue
            result.append({"utc": row["utc"], **dict(zip(columns, map(int, fields[:len(columns)])))})
    return result


def resources(rows, host):
    return {"samples": len(rows), "seconds_with_swap": sum(r["si"] > 0 or r["so"] > 0 for r in rows),
            "max_swap_in_kib_s": max((r["si"] for r in rows), default=0),
            "max_swap_out_kib_s": max((r["so"] for r in rows), default=0),
            "max_system_cpu_pct": max((r["sy"] for r in rows), default=0),
            "max_iowait_pct": max((r["wa"] for r in rows), default=0),
            "max_block_read_kib_s": max((r["bi"] for r in rows), default=0),
            "host_samples": len(host), "host_cpu_p95_pct": percentile([r["cpu_busy_pct"] for r in host], .95),
            "host_cpu_max_pct": max((r["cpu_busy_pct"] for r in host), default=0),
            "host_available_min_bytes": min((r["memory_available_bytes"] for r in host), default=0)}


def resource_readout(scenarios):
    vm = vm_rows()
    host = [json.loads(line) for line in (ROOT / "host-resources.jsonl").read_text().splitlines()]
    periods = [v for s in scenarios.values() for d in s["blocks"] for v in d["windows"].values()]
    ranges = [(utc(p["start"]), utc(p["end"])) for p in periods]
    during = lambda r: any(start <= utc(r["utc"]) <= end for start, end in ranges)
    first = min(start for start, _ in ranges)
    return {"formal": resources([r for r in vm if during(r)], [r for r in host if during(r)]),
            "startup_and_preflight": resources([r for r in vm if utc(r["utc"]) < first],
                                                [r for r in host if utc(r["utc"]) < first]),
            "all_observation": resources(vm, host),
            "method": "one-second passive vmstat and native host APIs; first vmstat boot average excluded"}


def main():
    scenarios = {label: scenario(label) for label in ("low-hit", "pure-miss")}
    progress = [json.loads(line) for line in (ROOT / "progress.jsonl").read_text().splitlines()]
    result = {"method": "six balanced bracket blocks; all attempts retained; no replacement",
              "production_approval": False, "scenarios": scenarios, "resources": resource_readout(scenarios),
              "stage_status": {r["label"]: r["status"] for r in progress}}
    (ROOT / "evaluation.json").write_text(json.dumps(result, indent=2)+"\n")
    print(json.dumps({"resources": result["resources"], "stage_status": result["stage_status"],
                      "scenarios": {k: {"gates": v["whole_request_gates"], "miss_gates": v["miss_gates"],
                                          "application": v["application_budgets"],
                                          "drift_alerts": v["bracket_drift_alert_blocks"]} for k, v in scenarios.items()}}, indent=2))


if __name__ == "__main__":
    main()
