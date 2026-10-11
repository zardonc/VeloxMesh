"""Window-aware diagnostics; candidates do not replace release contracts."""
import math

ORIGINAL_RATIO_LIMIT = 1.05
CANDIDATE_RATIO_LIMIT = 1.25
CANDIDATE_DELTA_MS = 40


def stats(values):
    if not values:
        return None
    ordered = sorted(values)
    return {"n": len(values), **{f"p{q}_ms": ordered[math.ceil(len(values)*q/100)-1]
                               for q in (50, 95, 99)}, "max_ms": max(values)}


def summarize(samples, elapsed_ms):
    if elapsed_ms <= 0:
        raise ValueError("positive observed window duration required")
    return {"n": len(samples), "failed": sum(not row["ok"] for row in samples),
            "hits": sum(row["hit"] for row in samples),
            "measurement_elapsed_ms": elapsed_ms,
            "actual_rps": len(samples)*1000/elapsed_ms,
            "actual_max_concurrency": max((row["concurrent"] for row in samples), default=0),
            "latency": stats([row["elapsed_ms"] for row in samples]),
            "application": stats([row["application_ms"] for row in samples if "application_ms" in row]),
            "send_lag": stats([max(0, row["start_ms"]-row["planned_ms"]) for row in samples]),
            "stages": {name: stats([row["stages"][name] for row in samples if name in row["stages"]])
                       for name in sorted({name for row in samples for name in row["stages"]})}}


def summarize_windows(windows):
    if not windows or any(not window for window in windows):
        raise ValueError("nonempty windows required; missing evidence cannot pass")
    elapsed_ms = sum(max(row["start_ms"]+row["elapsed_ms"] for row in window) for window in windows)
    samples = [row for window in windows for row in window]
    result = summarize(samples, elapsed_ms)
    return {**result, "hit_only": summarize([row for row in samples if row["hit"]], elapsed_ms),
            "miss_only": summarize([row for row in samples if not row["hit"]], elapsed_ms)}


def comparison_gate(on_p95, off_p95, failed=0):
    if not math.isfinite(on_p95) or not math.isfinite(off_p95) or on_p95 <= 0 or off_p95 <= 0:
        raise ValueError("positive finite matched P95 evidence required")
    ratio = on_p95/off_p95
    limit = min(CANDIDATE_RATIO_LIMIT*off_p95, off_p95+CANDIDATE_DELTA_MS)
    return {"ratio": ratio, "delta_ms": on_p95-off_p95, "original_limit": ORIGINAL_RATIO_LIMIT,
            "original_passed": failed == 0 and ratio <= ORIGINAL_RATIO_LIMIT,
            "candidate_ratio_limit": CANDIDATE_RATIO_LIMIT, "candidate_delta_limit_ms": CANDIDATE_DELTA_MS,
            "candidate_operator": "AND", "candidate_limit_ms": limit,
            "candidate_passed": failed == 0 and on_p95 <= limit, "proposal": True}
