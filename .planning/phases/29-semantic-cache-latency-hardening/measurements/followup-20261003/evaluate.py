"""Scenario budgets are diagnostic proposals, never replacement release gates."""
from collections import defaultdict
from pathlib import Path
import json
import math
import random
import sys

ROOT = Path(__file__).resolve().parent
MODEL = "LOCAL-qwen2.5-0.5b-instruct"
REPO_ROOT = next(parent for parent in ROOT.parents if (parent/"go.mod").exists())
sys.path.insert(0, str(REPO_ROOT/"scripts"))
from phase29_metrics import summarize_windows, comparison_gate


def records(path):
    return [json.loads(line.split("SAMPLE ", 1)[1]) for line in path.read_text().splitlines() if "SAMPLE " in line]


def stats(values):
    if not values:
        return None
    ordered = sorted(values)
    return {"n": len(values), **{f"p{q}_ms": ordered[math.ceil(len(values)*q/100)-1] for q in (50, 95, 99)}, "max_ms": max(values)}


def requests(path):
    raw, by_id = records(path), defaultdict(dict)
    for sample in raw:
        if sample["type"] == "stage":
            by_id[sample["request_id"]][sample["name"]] = sample["elapsed_ms"]
    result = []
    for sample in raw:
        if sample["type"] != "client":
            continue
        stages = by_id[sample["request_id"]]
        request = {**sample, "stages": stages}
        if "http_handler" in stages:
            request["application_ms"] = stages["http_handler"] - sum(stages.get(name, 0) for name in ("provider_complete", "cache_read"))
        result.append(request)
    return result


def window(samples):
    return summarize_windows([samples])


def request_window(folder, name):
    return window(requests(ROOT/folder/MODEL/f"{name}.log"))


def stable():
    names = {"direct": "TestLiveDirectLoad", "off": "TestLiveCacheLoadOff", "on": "TestLiveCacheLoadOnWithoutMemo", "memo": "TestLiveCacheLoadOnMemo"}
    pooled, blocks, condition_windows = defaultdict(list), [], defaultdict(list)
    for block in range(1, 7):
        windows = {}
        for mode, name in names.items():
            path = ROOT/f"verified-stable-{block}"/MODEL/f"{name}.log"
            samples = requests(path)
            if len(samples) != 100:
                raise ValueError(f"incomplete scheduled window: {path}, {len(samples)} requests")
            if mode != "direct" and any("application_ms" not in s for s in samples):
                raise ValueError(f"incomplete request/stage correlation: {path}")
            if mode == "memo":
                outcomes = [r for r in records(path) if r["type"] == "outcome" and r["name"] == "embedding_memo/hit"]
                if len(outcomes) != 4:
                    raise ValueError(f"memo activation/reuse mismatch: {path}, {len(outcomes)} hits")
            windows[mode] = window(samples)
            pooled[mode].extend(samples)
            condition_windows[mode].append(samples)
        blocks.append(windows)
    return {mode: summarize_windows(windows) for mode, windows in condition_windows.items()}, blocks, pooled


def diagnostic_gate(on, off):
    return comparison_gate(on["latency"]["p95_ms"], off["latency"]["p95_ms"], on["failed"]+off["failed"])


def normal_budget(sample):
    application = sample["application"]
    return {"proposal": True, "p95_limit_ms": 10, "p99_limit_ms": 15,
            "passed": sample["failed"] == 0 and application is not None and application["n"] == sample["n"]
            and application["p95_ms"] <= 10 and application["p99_ms"] <= 15}


def holdouts():
    result = []
    for folder in ("holdout-collection-raw", "holdout-nomic-raw", "holdout-nomic-prefix"):
        items = [r for r in records(ROOT/folder/MODEL/"TestLiveSemanticIndependentHoldout.log") if r["type"] == "independent_holdout"]
        if len(items) != 1:
            raise ValueError(f"Missing independent holdout summary: {folder}")
        item = items[0]
        result.append({**item, "label": folder, "recall": item["positive_hits"]/item["positive_count"],
                       "observed_false_hit_rate": item["false_hits"]/item["negative_count"],
                       "zero_unsafe_hits": item["false_hits"] == 0, "recall_70_percent_example_passed": item["positive_hits"]/item["positive_count"] >= .7,
                       "production_approved": False, "limits": "12 positive/20 negative independent questions; not a 1% population error guarantee"})
    return result


def bootstrap():
    rng = random.Random(29)
    trials = {"on_ratio": [], "memo_ratio": [], "memo_minus_on_ms": []}
    for _ in range(2000):
        chosen = [rng.randint(1, 6) for _ in range(6)]
        values = {}
        for mode, name in (("off", "TestLiveCacheLoadOff"), ("on", "TestLiveCacheLoadOnWithoutMemo"), ("memo", "TestLiveCacheLoadOnMemo")):
            values[mode] = stats([s["elapsed_ms"] for block in chosen for s in block_samples[block][mode]])["p95_ms"]
        trials["on_ratio"].append(values["on"]/values["off"])
        trials["memo_ratio"].append(values["memo"]/values["off"])
        trials["memo_minus_on_ms"].append(values["memo"]-values["on"])
    return {"method": "joint temporal-block bootstrap, six blocks, exploratory only", "seed": 29, "replicates": 2000,
            "intervals": {name: {"low": sorted(v)[49], "high": sorted(v)[1949]} for name, v in trials.items()}}


pooled, blocks, samples = stable()
block_samples = {block: {mode: requests(ROOT/f"verified-stable-{block}"/MODEL/f"{name}.log")
                        for mode, name in (("off", "TestLiveCacheLoadOff"), ("on", "TestLiveCacheLoadOnWithoutMemo"), ("memo", "TestLiveCacheLoadOnMemo"))}
                 for block in range(1, 7)}
scenarios = {name: request_window(folder, test) for name, folder, test in (
    ("hit", "hit-comparison", "TestLiveCacheHitSettlementLoad"),
    ("memo_hit", "hit-comparison", "TestLiveEmbeddingMemoHitBilling"),
    ("hit_baseline", "hit-comparison", "TestLiveCacheHitBaseline"),
    ("bypass_c4", "c4-bypass", "TestLiveCacheBypassLoad"),
    ("bypass_c8", "c8-saturation", "TestLiveCacheBypassLoad"),
    ("miss_c8", "c8-saturation", "TestLiveMissBurst"))}
for name in ("hit", "memo_hit"):
    scenario = scenarios[name]
    scenario["candidate_hit_budget"] = {"proposal": True, "passed": scenario["failed"] == 0 and scenario["hits"] == scenario["n"]
                                      and scenario["latency"]["p95_ms"] <= 60 and scenario["latency"]["p95_ms"] <= scenarios["hit_baseline"]["latency"]["p95_ms"]*.5}
result = {"standards_status": "diagnostic proposals; original release gate preserved", "stable": pooled,
          "per_block": blocks, "normal_budgets": {mode: normal_budget(window) for mode, window in pooled.items() if mode != "direct"},
          "gates": {mode: diagnostic_gate(pooled[mode], pooled["off"]) for mode in ("on", "memo")},
          "miss_only": {mode: pooled[mode]["miss_only"] for mode in ("on", "memo")},
          "scenarios": scenarios, "holdouts": holdouts(), "requests": samples,
          "c8_standards": "capacity boundary report only; no approved saturation SLO"}
result["bootstrap"] = bootstrap()
result["method_version"] = "window-aware-v2; candidate AND; legacy evaluation.json preserved"
(ROOT/"evaluation-v2.json").write_text(json.dumps(result, indent=2)+"\n")
print(json.dumps({"output": "evaluation-v2.json", "method_version": result["method_version"],
                  "stable": {mode: {"n": value["n"], "actual_rps": value["actual_rps"], "p95_ms": value["latency"]["p95_ms"]}
                             for mode, value in pooled.items()}, "gates": result["gates"]}, indent=2))
