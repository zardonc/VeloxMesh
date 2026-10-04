"""One diagnostic pair, never a replacement for balanced release validation."""
from pathlib import Path
import importlib.util
import json
import sys

ROOT = Path(__file__).resolve().parent
REPO = next(parent for parent in ROOT.parents if (parent / "go.mod").exists())
sys.path.insert(0, str(REPO / "scripts"))
from phase29_metrics import comparison_gate, stats, summarize_windows

SPEC = importlib.util.spec_from_file_location("phase29_evaluation", ROOT.parent / "policy-execution-20261003/evaluate.py")
EVALUATOR = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(EVALUATOR)
MODEL = "LOCAL-qwen2.5-0.5b-instruct"
NAMES = {"off": "TestLiveCacheLoadOff", "on": "TestLiveCacheLoadOnWithoutMemo"}
SERIAL = ("routing", "admission", "health_provider_sync", "health_model_sync", "circuit_result", "request_rules", "response_rules", "settlement")


def describe(rows):
    summary = summarize_windows([rows])
    totals = {**summary, "health_sync_sum": stats([sum(row["stages"].get(name, 0) for name in ("health_provider_sync", "health_model_sync")) for row in rows]),
              "unattributed_application": stats([row["application_ms"] - sum(row["stages"].get(name, 0) for name in SERIAL) for row in rows])}
    return totals


def main():
    rows = {mode: EVALUATOR.samples(ROOT / "workload" / MODEL / f"{name}.log") for mode, name in NAMES.items()}
    if any(len(window) != 100 for window in rows.values()):
        raise ValueError("complete 100-request diagnostic windows required")
    summaries = {mode: describe(window) for mode, window in rows.items()}
    positions = {row["planned_ms"] for row in rows["on"] if not row["hit"]}
    matched_off = [row for row in rows["off"] if row["planned_ms"] in positions]
    result = {"diagnostic_only": True, "blocks": 1, "order": ["off", "on"], "summaries": summaries,
              "matched_off_miss": describe(matched_off),
              "point_gate": comparison_gate(summaries["on"]["latency"]["p95_ms"], summaries["off"]["latency"]["p95_ms"], sum(summary["failed"] for summary in summaries.values())),
              "limitations": ["one temporal block", "dependency startup paging was observed", "no fresh six-block release validation", "no memo condition"]}
    (ROOT / "workload-readout.json").write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps({mode: {key: summary[key] for key in ("n", "failed", "hits", "actual_rps", "actual_max_concurrency", "latency", "application", "health_sync_sum", "unattributed_application")} for mode, summary in summaries.items()}, indent=2))


if __name__ == "__main__":
    main()
