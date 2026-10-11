"""Recovery failures and component-only diagnostics; no performance pass inferred."""
from collections import Counter
from datetime import datetime
from pathlib import Path
import importlib.util
import json
import sys

ROOT = Path(__file__).resolve().parent
REPO = next(p for p in ROOT.parents if (p / "go.mod").exists())
sys.path.insert(0, str(REPO / "scripts"))
from phase29_metrics import stats


def records(path):
    return [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines() if line]


def utc(value):
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def vm_rows(path):
    result, columns, boot = [], [], True
    for row in records(path):
        fields = row["line"].split()
        if fields and fields[0] == "r":
            columns = fields[:fields.index("UTC")]
        elif fields and fields[0].isdigit() and columns:
            if boot:
                boot = False
                continue
            result.append({"utc": row["utc"], **dict(zip(columns, map(int, fields[:len(columns)])))})
    return result


def resource_summary(rows):
    return {"samples": len(rows), "swap_seconds": sum(r["si"] > 0 or r["so"] > 0 for r in rows),
            "max_swap_in_kib_s": max((r["si"] for r in rows), default=0),
            "max_swap_out_kib_s": max((r["so"] for r in rows), default=0),
            "sum_sampled_swap_in_kib": sum(r["si"] for r in rows),
            "max_block_read_kib_s": max((r["bi"] for r in rows), default=0),
            "max_iowait_pct": max((r["wa"] for r in rows), default=0),
            "max_system_cpu_pct": max((r["sy"] for r in rows), default=0)}


def parse_memory(row):
    text = row["output"]
    host = text.split("CGROUP:", 1)[0]
    fields = {line.split()[0].rstrip(":"): int(line.split()[1]) for line in host.splitlines()
              if len(line.split()) >= 2 and line.split()[1].isdigit()}
    cgroup = {}
    for section in text.split("CGROUP:")[1:]:
        name, _, content = section.partition("\n")
        value = content.split("PROCESS:", 1)[0].strip()
        cgroup[name] = int(value) if value.isdigit() else counter_fields(value)
        if name == "memory.pressure":
            cgroup[name] = pressure_fields(value)
    processes = []
    for section in text.split("PROCESS:")[1:]:
        name, _, content = section.partition("\n")
        values = {line.split()[0].rstrip(":"): int(line.split()[1]) for line in content.splitlines()
                  if len(line.split()) >= 2 and line.split()[1].isdigit()}
        processes.append({"name": name, **{key: values.get(key) for key in ("Pid", "VmRSS", "VmSwap")}})
    return {"utc": row["utc"], "success": row["success"], "host": fields, "host_pressure": pressure_fields(host),
            "cgroup": cgroup, "processes": processes}


def counter_fields(text):
    return {line.split()[0]: int(line.split()[1]) for line in text.splitlines()
            if len(line.split()) == 2 and line.split()[1].isdigit()}


def pressure_fields(text):
    return {line.split()[0]: {k: float(v) for field in line.split()[1:] for k, v in [field.split("=")]}
            for line in text.splitlines() if line.startswith(("some avg10=", "full avg10="))}


def idle_summary():
    root = ROOT / "idle"
    vm = vm_rows(root / "vmstat.jsonl")
    host = records(root / "host-resources.jsonl")
    result = {}
    for condition in json.loads((root / "idle-plan.json").read_text()):
        label = condition["label"]
        period = json.loads((root / f"{label}-period.json").read_text())
        snapshots = [parse_memory(r) for r in records(root / f"{label}-memory.jsonl")]
        start, end = utc(period["start"]), utc(snapshots[-1]["utc"])
        rows = [r for r in vm if start <= utc(r["utc"]) <= end]
        tail = [r for r in rows if (end-utc(r["utc"])).total_seconds() <= 30]
        if not snapshots or any(not r["success"] for r in snapshots):
            raise ValueError(f"incomplete memory observations: {label}")
        result[label] = {"period": {**period, "observation_end": snapshots[-1]["utc"],
                                   "shutdown_excluded": True}, "resources": resource_summary(rows),
                         "last_30_seconds": resource_summary(tail), "memory": snapshots,
                         "host_available_min_bytes": min(r["memory_available_bytes"] for r in host
                                                         if start <= utc(r["utc"]) <= end)}
    return result


def previous_spikes():
    spec = importlib.util.spec_from_file_location("old_evaluator", ROOT.parent / "policy-execution-20261003/evaluate.py")
    old = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(old)
    baseline = ROOT.parent / "memory-upgrade-controlled-20261004/final"
    slow = []
    paths = [baseline / f"{scenario}-{index}/LOCAL-qwen2.5-0.5b-instruct/{name}.log"
             for scenario in ("low-hit", "pure-miss") for index in range(1, 7)
             for name in ("TestLiveControlledOff", "TestLiveControlledOffAfter")]
    for path in paths:
        if path.stem not in ("TestLiveControlledOff", "TestLiveControlledOffAfter"):
            continue
        rows = old.samples(path)
        if len(rows) != 100:
            raise ValueError(f"incomplete previous formal window: {path}")
        for row in rows:
            if row["elapsed_ms"] <= 250:
                continue
            slow.append({"path": str(path.relative_to(REPO)), "index": row["index"],
                         "request_id": row["request_id"], "elapsed_ms": row["elapsed_ms"],
                         "stages": row["stages"], "application_ms": row["application_ms"]})
    return {"count": len(slow), "indices": dict(Counter(r["index"] for r in slow)), "samples": slow,
            "threshold_ms": 250, "threshold_is_diagnostic": True}


def main():
    recovery = records(ROOT / "recovery.jsonl")
    result = {"formal_performance_windows": 0, "formal_performance_requests": 0,
              "recovery": {"result": json.loads((ROOT / "run-result.json").read_text()),
                           "samples": len(recovery), "quiet_seconds_max": max(r["consecutive"] for r in recovery),
                           "swap_seconds": sum(int(r["line"].split()[6]) > 0 or int(r["line"].split()[7]) > 0
                                               for r in recovery),
                           "required_quiet_seconds": 10, "timeout_seconds": 120},
              "idle_conditions": idle_summary(), "previous_baseline_spikes": previous_spikes(),
              "limits": ["idle conditions run once in fixed order; no causal estimate for collection count",
                         "cgroup major faults include file-backed faults as well as swap",
                         "cache-off warmup comparison not run because recovery failed",
                         "no change to P95 release criteria or semantic safety conclusion"]}
    (ROOT / "evaluation.json").write_text(json.dumps(result, indent=2)+"\n", encoding="utf-8")
    print(json.dumps({"recovery": result["recovery"], "previous_spikes": result["previous_baseline_spikes"]["count"],
                      "idle": {k: {"all": v["resources"], "tail": v["last_30_seconds"],
                                    "available_kib": v["memory"][-1]["host"]["MemAvailable"],
                                    "cgroup_bytes": v["memory"][-1]["cgroup"].get("memory.current"),
                                    "cgroup_swap_bytes": v["memory"][-1]["cgroup"].get("memory.swap.current"),
                                    "host_pressure": v["memory"][-1]["host_pressure"]}
                               for k, v in result["idle_conditions"].items()}}, indent=2))


if __name__ == "__main__":
    main()
