"""Recompute scenario diagnostics from correlated real request samples."""
from collections import Counter, defaultdict
from pathlib import Path
import json
import math
import random

ROOT = Path(__file__).resolve().parent
BOOTSTRAPS = 2000
SEED = 29


def quantile(values, fraction):
    return sorted(values)[math.ceil(len(values) * fraction) - 1]


def stats(values):
    if not values:
        return None
    return {"n": len(values), "p50_ms": quantile(values, .5), "p95_ms": quantile(values, .95),
            "p99_ms": quantile(values, .99), "max_ms": max(values)}


def records(path):
    return [json.loads(line.split("SAMPLE ", 1)[1]) for line in path.read_text().splitlines() if "SAMPLE " in line]


def requests(path):
    raw = records(path)
    by_id = defaultdict(dict)
    for sample in raw:
        if sample["type"] == "stage":
            by_id[sample["request_id"]][sample["name"]] = sample
    result = []
    for sample in raw:
        if sample["type"] != "client":
            continue
        local = by_id[sample["request_id"]]
        entry = {**sample, "stages": {name: value["elapsed_ms"] for name, value in local.items()}}
        if "http_handler" in local:
            entry["application_ms"] = local["http_handler"]["elapsed_ms"] - sum(
                local.get(name, {}).get("elapsed_ms", 0) for name in ("provider_complete", "cache_read"))
        if "response_rules" in local and "provider_complete" in local:
            provider = local["provider_complete"]
            entry["post_provider_ms"] = local["response_rules"]["start_ms"] - provider["start_ms"] - provider["elapsed_ms"]
        result.append(entry)
    return result


def window(samples):
    return {"n": len(samples), "failures": sum(not s["ok"] for s in samples), "hits": sum(s["hit"] for s in samples),
            "max_observed_concurrency": max(s["concurrent"] for s in samples),
            **{key: stats([s[key] for s in samples if key in s]) for key in ("elapsed_ms", "application_ms", "post_provider_ms")},
            "stages": {name: stats([s["stages"][name] for s in samples if name in s["stages"]])
                       for name in sorted(set(name for s in samples for name in s["stages"]))}}


def stable_blocks():
    blocks = []
    for block in range(1, 7):
        directory = ROOT / f"final-stable-{block}" / "LOCAL-qwen2.5-0.5b-instruct"
        samples = {mode: requests(directory / f"{name}.log") for mode, name in
                   (("direct", "TestLiveDirectLoad"), ("off", "TestLiveCacheLoadOff"), ("on", "TestLiveCacheLoadOn"))}
        blocks.append(samples)
    return blocks


def paired_block_bootstrap(blocks):
    # Resample matched time blocks jointly. Rounds were sequential, not simultaneous;
    # six independent blocks give limited uncertainty resolution. This is exploratory.
    rng = random.Random(SEED)
    ratios = []
    for _ in range(BOOTSTRAPS):
        chosen = [rng.choice(blocks) for _ in blocks]
        off = [s["elapsed_ms"] for block in chosen for s in block["off"]]
        on = [s["elapsed_ms"] for block in chosen for s in block["on"]]
        ratios.append(quantile(on, .95) / quantile(off, .95))
    return {"replicates": BOOTSTRAPS, "seed": SEED, "method": "joint temporal-block bootstrap; exploratory with six blocks",
            "ci95_low": quantile(ratios, .025), "ci95_high": quantile(ratios, .975)}


def other_scenarios():
    scenarios = {}
    for folder, name in (("final-hit", "TestLiveCacheHitSettlementLoad"), ("final-hit", "TestLiveCacheHitBaseline"),
                         ("final-scenarios", "TestLiveCacheBypassLoad"), ("final-scenarios", "TestLiveMissBurst")):
        scenarios[name] = window(requests(ROOT / folder / "LOCAL-qwen2.5-0.5b-instruct" / f"{name}.log"))
    for folder, label in (("pg-and-bypass-final", "bypass_c4"), ("supplemental-profile", "bypass_c8_profiled")):
        scenarios[label] = window(requests(ROOT / folder / "LOCAL-qwen2.5-0.5b-instruct" / "TestLiveCacheBypassLoad.log"))
    embedding_log = ROOT / "final-embedding" / "LOCAL-qwen2.5-0.5b-instruct" / "TestLiveEmbeddingStress.log"
    embedding = [r for r in records(embedding_log) if r["type"] == "embedding_client"]
    scenarios["embedding"] = {"latency": stats([s["elapsed_ms"] for s in embedding]),
                              "failed": sum(not s["ok"] for s in embedding), "dimensions": dict(Counter(s.get("dimension") for s in embedding))}
    return scenarios


def input_comparison():
    path = ROOT / "supplemental-profile" / "LOCAL-qwen2.5-0.5b-instruct" / "TestLiveNomicPrefixComparison.log"
    samples = [r for r in records(path) if r["type"] == "input_score"]
    result = []
    for prefix in sorted(set(r["prefix"] for r in samples)):
        positive = [s["cosine"] for s in samples if s["prefix"] == prefix and s["positive"]]
        negative = [s["cosine"] for s in samples if s["prefix"] == prefix and not s["positive"]]
        result.append({"prefix": prefix, "positive_count": len(positive), "negative_count": len(negative),
                       "max_negative": max(negative), "min_positive": min(positive),
                       "thresholds": [{"threshold": threshold, "positive_hits": sum(v >= threshold for v in positive),
                                       "false_hits": sum(v >= threshold for v in negative)} for threshold in (.92, .86, .84)],
                       "safe_recall_on_seen_data": sum(v > max(negative) for v in positive)})
    return result


blocks = stable_blocks()
pooled = {mode: [s for block in blocks for s in block[mode]] for mode in ("direct", "off", "on")}
evaluation = {"input_comparison": input_comparison(), "stable": {mode: window(samples) for mode, samples in pooled.items()},
              "miss_only": window([s for s in pooled["on"] if not s["hit"]]),
              "per_block": [{"block": index + 1, **{mode: window(samples) for mode, samples in block.items()}}
                            for index, block in enumerate(blocks)], "scenarios": other_scenarios()}
evaluation["original_gate"] = {"limit": 1.05, "ratio": evaluation["stable"]["on"]["elapsed_ms"]["p95_ms"] /
                                evaluation["stable"]["off"]["elapsed_ms"]["p95_ms"], **paired_block_bootstrap(blocks)}
evaluation["original_gate"]["passed"] = evaluation["original_gate"]["ratio"] <= 1.05
evaluation["pooled_requests"] = pooled
(ROOT / "evaluation.json").write_text(json.dumps(evaluation, indent=2) + "\n")
print(json.dumps({key: value for key, value in evaluation.items() if key not in ("pooled_requests", "per_block")}, indent=2))
