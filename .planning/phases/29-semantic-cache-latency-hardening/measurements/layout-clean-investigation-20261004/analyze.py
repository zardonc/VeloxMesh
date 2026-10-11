"""Evaluate all preregistered layout trials, without omitting failures."""
import json
import math
import sys
from datetime import datetime
from pathlib import Path

ROOT = Path(sys.argv[1]).resolve() if len(sys.argv) > 1 else Path(__file__).resolve().parent
MIB = 1024**2


def rows(name):
    return [json.loads(line) for line in (ROOT / name).read_text().splitlines() if line]


def utc(value):
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def percentile(values, fraction):
    return sorted(values)[max(0, math.ceil(len(values) * fraction) - 1)]


def summarize_samples(samples):
    if not samples:
        return {"samples": 0}
    valid = [s for s in samples if s["delta_valid"]]
    return {
        "samples": len(samples),
        "memory_peak_mib": max(s.get("cgroup", {}).get("memory.current", 0) for s in samples) / MIB,
        "memory_last_mib": samples[-1].get("cgroup", {}).get("memory.current", 0) / MIB,
        "file_last_mib": samples[-1].get("cgroup", {}).get("file", 0) / MIB,
        "file_mapped_last_mib": samples[-1].get("cgroup", {}).get("file_mapped", 0) / MIB,
        "anon_last_mib": samples[-1].get("cgroup", {}).get("anon", 0) / MIB,
        "cgroup_swap_peak_mib": max(s.get("cgroup", {}).get("memory.swap.current", 0) for s in samples) / MIB,
        "oom_kill_max": max(s.get("cgroup", {}).get("event_oom_kill", 0) for s in samples),
        "available_min_mib": min(s["available_bytes"] for s in samples) / MIB,
        "psi_max": max(s["psi_avg10"] for s in samples),
        "swap_in_peak_mib_sec": max((s["delta"]["swap_in_bytes_sec"] for s in valid), default=0) / MIB,
        "swap_out_peak_mib_sec": max((s["delta"]["swap_out_bytes_sec"] for s in valid), default=0) / MIB,
        "read_peak_mib_sec": max((s["delta"]["read_kib_sec"] for s in valid), default=0) / 1024,
        "system_cpu_peak": max((s["delta"]["system_pct"] for s in valid), default=0),
        "wait_cpu_peak": max((s["delta"]["wait_pct"] for s in valid), default=0),
    }


def validate_preparation():
    details = {}
    for arm, count in (("shared", 1), ("many", 323)):
        data = rows(f"prepare-{arm}.jsonl")
        validation = [r for r in data if r["type"] == "collection-validation"]
        assert len(validation) == count
        assert sum(r["detail"]["points_count"] for r in validation) == 16542
        assert all(r["detail"]["indexed_vectors_count"] == 0 for r in validation)
        config = validation[0]["detail"]["config"]
        assert all(r["detail"]["config"] == config for r in validation)
        assert all(r["detail"]["payload_schema"]["scope"]["data_type"] == "keyword" for r in validation)
        details[arm] = {"config": config, "points": 16542, "collections": count,
                        "segments": sum(r["detail"]["segments_count"] for r in validation),
                        "request_digest": data[-1]["request_digest"]}
    assert details["shared"]["config"] == details["many"]["config"]
    assert details["shared"]["request_digest"] == details["many"]["request_digest"]
    return details


def evaluate_trial(name, events):
    relevant = {e["type"]: e for e in events if e.get("detail", {}).get("trial") == name}
    required = {"trial-start", "trial-ready", "query-start", "query-end", "trial-complete"}
    assert required <= relevant.keys(), (name, "missing completion")
    gate = rows(name + "-gate.jsonl")
    assert gate[-1]["type"] == "gate-pass" and gate[-1]["streak"] == 10
    for arm in ("shared", "many"):
        eviction = rows(name + f"-evict-{arm}.jsonl")
        assert eviction[-1]["verified"] and eviction[-1]["totals"]["resident_after_bytes"] == 0
    resource_rows = rows(name + "-resources.jsonl")
    assert resource_rows[-1]["type"] == "observer-stop"
    samples = [r["sample"] for r in resource_rows if r["type"] == "resource-sample"]
    ready = utc(relevant["trial-ready"]["utc"])
    query = utc(relevant["query-start"]["utc"])
    assert (query - ready).total_seconds() >= 60
    idle = [s for s in samples if ready <= utc(s["utc"]) < query]
    assert len(idle) >= 58
    queries = rows(name + "-queries.jsonl")
    assert len(queries) == 100 and all(r["type"] == "query" for r in queries)
    assert all(r["result"]["points"][0]["id"] == r["index"] + 1 for r in queries)
    assert all(r["result"]["points"][0]["payload"]["scope"] == f'scope-{r["index"]:03d}' for r in queries)
    return {"name": name, "start_to_ready_ms": relevant["trial-ready"]["detail"]["start_to_ready_ms"],
            "startup": summarize_samples([s for s in samples if utc(s["utc"]) < ready]),
            "idle": summarize_samples(idle),
            "query_resources": summarize_samples([s for s in samples if utc(s["utc"]) >= query]),
            "query_count": len(queries), "query_p95_ms": percentile([r["elapsed_ms"] for r in queries], .95),
            "query_max_ms": max(r["elapsed_ms"] for r in queries)}


def main():
    events = rows("events.jsonl")
    assert events[-1]["type"] == "matrix-complete"
    names = [e["detail"]["trial"] for e in events if e["type"] == "trial-start"]
    assert names == ["trial-01-shared", "trial-02-many", "trial-03-many", "trial-04-shared", "trial-05-shared", "trial-06-many"]
    result = {"validation": "PASS", "release_approval": False, "preparation": validate_preparation(),
              "trials": [evaluate_trial(name, events) for name in names]}
    (ROOT / "summary.json").write_text(json.dumps(result, indent=2), encoding="utf-8")
    for trial in result["trials"]:
        idle = trial["idle"]
        print(trial["name"], "ready_ms", round(trial["start_to_ready_ms"], 1),
              "idle_memory_mib", round(idle["memory_last_mib"], 1),
              "swap_peak_mib_sec", round(max(trial["startup"]["swap_out_peak_mib_sec"] if trial["startup"]["samples"] else 0, idle["swap_out_peak_mib_sec"]), 2),
              "query_p95_ms", round(trial["query_p95_ms"], 3))


if __name__ == "__main__":
    main()
