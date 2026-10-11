from collections import Counter, defaultdict
from pathlib import Path
import hashlib
import json
import math

ROOT = Path(__file__).resolve().parent


def distribution(values):
    if not values:
        return None
    ordered = sorted(values)
    return {"n": len(values), **{f"p{q}_ms": ordered[math.ceil(len(values)*q/100)-1] for q in (50, 95, 99)}, "max_ms": max(values)}


def summarize(path):
    content = path.read_text()
    raw = [json.loads(line.split("SAMPLE ", 1)[1]) for line in content.splitlines() if "SAMPLE " in line]
    components = defaultdict(list)
    for record in raw:
        if record["type"] == "component_client":
            components[(record["name"], record["concurrent"])].append(record)
    result = {"log": path.relative_to(ROOT).as_posix(), "sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
              "passed": f"--- PASS: {path.stem} (" in content,
              "failures": [line.strip() for line in content.splitlines() if "--- FAIL:" in line],
              "components": [{"name": name, "concurrency": concurrency, "failed": sum(not r["ok"] for r in samples),
                              "errors": dict(Counter(r.get("error") for r in samples if not r["ok"])),
                              "latency": distribution([r["elapsed_ms"] for r in samples])}
                             for (name, concurrency), samples in components.items()]}
    stages = [r for r in raw if r["type"] == "stage"]
    result["stages"] = {name: distribution([r["elapsed_ms"] for r in stages if r["name"] == name])
                        for name in sorted(set(r["name"] for r in stages)) if not name.startswith("net_")}
    result["outcomes"] = dict(Counter(r["name"] for r in raw if r["type"] == "outcome"))
    result["special"] = [r for r in raw if r["type"] not in ("stage", "client", "outcome", "operation", "component_client")]
    result["requests"] = summarize_requests(raw, stages)
    return result


def summarize_requests(raw, stages):
    by_id = defaultdict(dict)
    for stage in stages:
        by_id[stage["request_id"]][stage["name"]] = stage
    requests = []
    for client in [r for r in raw if r["type"] == "client" and r.get("request_id")]:
        local = by_id[client["request_id"]]
        request = {"ok": client["ok"], "hit": client["hit"], "elapsed_ms": client["elapsed_ms"]}
        if "http_handler" in local:
            request["gateway_ms"] = local["http_handler"]["elapsed_ms"] - sum(local.get(name, {}).get("elapsed_ms", 0) for name in ("provider_complete", "cache_read"))
        if "response_rules" in local and "provider_complete" in local:
            request["post_provider_ms"] = local["response_rules"]["start_ms"] - local["provider_complete"]["start_ms"] - local["provider_complete"]["elapsed_ms"]
        requests.append(request)
    return {"count": len(requests), "failed": sum(not r["ok"] for r in requests), "hits": sum(r["hit"] for r in requests),
            **{name: distribution([r[name] for r in requests if name in r]) for name in ("elapsed_ms", "gateway_ms", "post_provider_ms")}}


logs = [summarize(path) for path in sorted(ROOT.rglob("Test*.log")) if "-vm-" not in path.stem]
summary = {"runs": len(logs), "passed": sum(r["passed"] for r in logs), "failed": sum(not r["passed"] for r in logs), "logs": logs}
(ROOT / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
print(json.dumps({k: v for k, v in summary.items() if k != "logs"}))
for result in logs:
    if result["components"]:
        print(json.dumps({"log": result["log"], "components": result["components"]}))
