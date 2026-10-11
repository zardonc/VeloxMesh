"""Strict window validation and scenario contracts; historical attempts remain separate."""
from collections import defaultdict
from pathlib import Path
import argparse
import json
import random
import sys

REPO = next(parent for parent in Path(__file__).resolve().parents if (parent / "go.mod").exists())
sys.path.insert(0, str(REPO / "scripts"))
from phase29_metrics import stats, summarize_windows, comparison_gate

MODEL = "LOCAL-qwen2.5-0.5b-instruct"
NAMES = {"direct": "TestLiveDirectLoad", "off": "TestLiveCacheLoadOff",
         "on": "TestLiveCacheLoadOnWithoutMemo", "memo": "TestLiveCacheLoadOnMemo"}
BLOCK_COUNT = 6
REQUEST_COUNT = 100


def records(path):
    return [json.loads(line.split("SAMPLE ", 1)[1]) for line in path.read_text().splitlines() if "SAMPLE " in line]


def samples(path, diagnostic=False):
    name = path.stem
    raw_text = path.read_text()
    passed = f"--- PASS: {name} (" in raw_text and "--- FAIL:" not in raw_text and "--- SKIP:" not in raw_text
    if not passed and (not diagnostic or f"=== RUN   {name}" not in raw_text or "--- SKIP:" in raw_text):
        raise ValueError(f"missing successful real test: {path}")
    raw, by_id = records(path), defaultdict(dict)
    for row in raw:
        if row["type"] == "stage":
            by_id[row["request_id"]][row["name"]] = row["elapsed_ms"]
    result = []
    for row in raw:
        if row["type"] != "client":
            continue
        stages = by_id[row["request_id"]]
        sample = {**row, "stages": stages, "test_passed": passed}
        if "http_handler" in stages:
            sample["application_ms"] = stages["http_handler"] - sum(stages.get(key, 0) for key in ("provider_complete", "cache_read"))
        result.append(sample)
    return result


def normal_budget(summary):
    application = summary["application"]
    return {"proposal": True, "p95_limit_ms": 10, "p99_limit_ms": 15,
            "passed": summary["failed"] == 0 and application is not None and application["n"] == summary["n"]
            and application["p95_ms"] <= 10 and application["p99_ms"] <= 15}


def gate(on, off):
    return comparison_gate(on["latency"]["p95_ms"], off["latency"]["p95_ms"], on["failed"] + off["failed"])


def bootstrap(blocks):
    rng = random.Random(29)
    trials = defaultdict(list)
    for _ in range(2000):
        chosen = [blocks[rng.randrange(len(blocks))] for _ in blocks]
        p95 = {mode: stats([row["elapsed_ms"] for block in chosen for row in block[mode]])["p95_ms"] for mode in NAMES}
        for mode in ("on", "memo"):
            trials[f"{mode}_ratio"].append(p95[mode] / p95["off"])
            trials[f"{mode}_delta_ms"].append(p95[mode] - p95["off"])
    return {"method": "joint temporal-block bootstrap; six blocks; exploratory", "replicates": 2000, "seed": 29,
            "intervals": {key: {"low": sorted(values)[49], "high": sorted(values)[1949]} for key, values in trials.items()}}


def scenario(root, label):
    windows, blocks, matched_off = defaultdict(list), [], []
    selections = {item["original"]: item["selected"] for item in json.loads((root / "block-selection.json").read_text())["blocks"]}
    expected_hits = 4 if label == "low-hit" else 0
    for index in range(1, BLOCK_COUNT + 1):
        block = {}
        for mode, name in NAMES.items():
            selected = selections[f"{label}-{index}"]
            path = root / selected / MODEL / f"{name}.log"
            rows = samples(path, diagnostic=True)
            if len(rows) != REQUEST_COUNT:
                raise ValueError(f"incomplete window: {path}")
            if mode != "direct" and any("application_ms" not in row for row in rows):
                raise ValueError(f"incomplete request-stage correlation: {path}")
            hits = sum(row["hit"] for row in rows)
            if hits != (expected_hits if mode in ("on", "memo") else 0):
                raise ValueError(f"hit activation mismatch: {path}, hits={hits}")
            if mode == "memo":
                memo_hits = sum(row["type"] == "outcome" and row["name"] == "embedding_memo/hit" for row in records(path))
                if memo_hits != expected_hits:
                    raise ValueError(f"vector memo activation mismatch: {path}, hits={memo_hits}")
            block[mode] = rows
            windows[mode].append(rows)
        if label == "low-hit":
            positions = {row["planned_ms"] for row in block["on"] if not row["hit"]}
            if positions != {row["planned_ms"] for row in block["memo"] if not row["hit"]}:
                raise ValueError("on/memo miss arrival positions differ")
            matched_off.append([row for row in block["off"] if row["planned_ms"] in positions])
        blocks.append(block)
    pooled = {mode: summarize_windows(value) for mode, value in windows.items()}
    all_tests_passed = all(row["test_passed"] for values in windows.values() for window in values for row in window)
    comparison_off = summarize_windows(matched_off) if matched_off else pooled["off"]
    application_budgets = {mode: normal_budget(pooled[mode]) for mode in ("off", "on", "memo")}
    whole_gates = {mode: gate(pooled[mode], pooled["off"]) for mode in ("on", "memo")}
    return {"pooled": pooled, "all_tests_passed": all_tests_passed,
            "candidate_contract_passed": all_tests_passed and all(item["passed"] for item in application_budgets.values())
            and all(item["candidate_passed"] for item in whole_gates.values()),
            "evidence_status": "verified windows" if all_tests_passed else "diagnostic only; failed requests included; no latency release approval",
            "per_block": [{mode: summarize_windows([rows]) for mode, rows in block.items()} for block in blocks],
            "whole_request_gates": whole_gates,
            "matched_miss_off": comparison_off,
            "miss_gates": {mode: gate(pooled[mode]["miss_only"], comparison_off) for mode in ("on", "memo")},
            "application_budgets": application_budgets,
            "bootstrap": bootstrap(blocks)}


def other_windows(root):
    result = {}
    for concurrency in (1, 2, 4, 8):
        for mode, name in (("bypass", "TestLiveCacheBypassLoad"), ("miss", "TestLiveMissBurst")):
            result[f"{mode}_c{concurrency}"] = summarize_windows([samples(root / f"capacity-c{concurrency}" / MODEL / f"{name}.log")])
    for label, name in (("hit", "TestLiveCacheHitSettlementLoad"), ("memo_hit", "TestLiveEmbeddingMemoHitBilling"), ("hit_off", "TestLiveCacheHitBaseline")):
        result[label] = summarize_windows([samples(root / "hit-comparison" / MODEL / f"{name}.log")])
    for label in ("hit", "memo_hit"):
        value = result[label]
        value["candidate_hit_budget"] = {"proposal": True, "passed": value["failed"] == 0 and value["hits"] == value["n"]
                                          and value["latency"]["p95_ms"] <= 60 and value["latency"]["p95_ms"] <= result["hit_off"]["latency"]["p95_ms"] / 2}
    return result


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", type=Path, default=Path(__file__).parent / "warm-batch")
    root = parser.parse_args().root
    result = {"method_version": "window-aware-v3; explicit pure miss; candidate AND",
              "standards_status": "scenario candidates; original 1.05 result preserved; no production authorization",
              "block_selection": json.loads((root / "block-selection.json").read_text()),
              "availability_status": "original failures retained separately; successful rechecks do not erase them",
              "scenarios": {label: scenario(root, label) for label in ("low-hit", "pure-miss")},
              "other_windows": other_windows(root), "saturation_status": "capacity evidence only; saturation SLO not approved"}
    (root / "evaluation.json").write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps({"output": str(root / "evaluation.json"), "scenarios": {
        label: {"p95_ms": {mode: value["latency"]["p95_ms"] for mode, value in item["pooled"].items()},
                "gates": item["whole_request_gates"], "application_budgets": item["application_budgets"]}
        for label, item in result["scenarios"].items()}}, indent=2))


if __name__ == "__main__":
    main()
