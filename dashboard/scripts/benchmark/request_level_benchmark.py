#!/usr/bin/env python3
from __future__ import annotations

import argparse
import concurrent.futures
import csv
import json
import math
import os
import socket
import time
import urllib.error
import urllib.request
import zipfile
from dataclasses import dataclass
from datetime import datetime, timezone
from html import escape
from pathlib import Path
from typing import Any


METHOD_LABELS = {
    "local_baseline": "Local Baseline",
    "gateway": "Our Gateway Method",
    "improved_model": "Improved Model",
    "gateway_improved_model": "Our Gateway + Improved Model",
}

RAW_FIELDS = [
    "run_id", "request_id", "dataset", "row_index", "method_id", "method", "provider", "model", "model_version", "route",
    "started_at", "ended_at", "latency_ms", "ttft_ms", "input_tokens", "output_tokens", "total_tokens", "status", "http_status",
    "error_type", "timeout", "retry_count", "cache_hit",
]

SUMMARY_FIELDS = [
    "run_id", "method_id", "method", "dataset", "provider", "model", "model_version", "request_count", "avg_latency_ms",
    "p50_latency_ms", "p95_latency_ms", "p99_latency_ms", "ttft_ms", "throughput_rps", "success_rate_pct", "error_rate_pct",
    "timeout_rate_pct", "started_at", "ended_at",
]


@dataclass(frozen=True)
class RunConfig:
    dataset: Path
    report_dir: Path
    gateway_url: str
    api_key: str
    run_id: str
    method_id: str
    provider: str
    model: str
    model_version: str
    gateway_version: str
    concurrency: int = 1
    request_rate: float | None = None
    timeout_seconds: int = 120
    warm_up: int = 0
    repeated_runs: int = 1


@dataclass(frozen=True)
class RunResult:
    requests: list[dict[str, Any]]
    summary: dict[str, Any]
    report_dir: Path


def method_label(method_id: str) -> str:
    try:
        return METHOD_LABELS[method_id]
    except KeyError as exc:
        raise ValueError(f"unsupported benchmark method ID: {method_id}") from exc


def run(config: RunConfig) -> RunResult:
    validate_config(config)
    wait_for_health(config.gateway_url, min(config.timeout_seconds, 30))
    samples = load_samples(config.dataset, config.model)
    config.report_dir.mkdir(parents=True, exist_ok=True)
    schedule_started = time.perf_counter()

    def execute(item: tuple[int, dict[str, Any]]) -> dict[str, Any]:
        index, _sample = item
        if config.request_rate and config.request_rate > 0:
            target = schedule_started + (index / config.request_rate)
            delay = target - time.perf_counter()
            if delay > 0:
                time.sleep(delay)
        return call_gateway(config, item)

    with concurrent.futures.ThreadPoolExecutor(max_workers=config.concurrency) as pool:
        requests = list(pool.map(execute, enumerate(samples)))
    requests.sort(key=lambda row: int(row["rowIndex"]))
    summary = build_benchmark_summary(config, recompute_summary(requests))
    write_artifacts(config, requests, summary)
    return RunResult(requests=requests, summary=summary, report_dir=config.report_dir)


def validate_config(config: RunConfig) -> None:
    method_label(config.method_id)
    if not config.api_key:
        raise ValueError("Gateway API key is required")
    if not config.gateway_url.lower().startswith(("http://", "https://")):
        raise ValueError("Gateway URL must use http or https")
    if not config.model or not config.model_version or not config.provider:
        raise ValueError("provider, model, and model version are required")
    if config.concurrency < 1 or config.timeout_seconds < 1:
        raise ValueError("concurrency and timeout must be positive")


def wait_for_health(gateway_url: str, wait_seconds: int) -> None:
    url = gateway_url.rstrip("/") + "/healthz"
    deadline = time.time() + wait_seconds
    while True:
        try:
            with urllib.request.urlopen(url, timeout=3) as response:
                if 200 <= response.status < 300:
                    return
        except Exception:
            if time.time() >= deadline:
                raise RuntimeError(f"Gateway health check failed: {url}")
            time.sleep(0.25)


def load_samples(path: Path, model: str) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    for line in path.read_text(encoding="utf-8-sig").splitlines():
        if not line.strip():
            continue
        sample = json.loads(line)
        sample["model"] = model
        rows.append(sample)
    if not rows:
        raise ValueError(f"dataset is empty: {path}")
    return rows


def call_gateway(config: RunConfig, item: tuple[int, dict[str, Any]]) -> dict[str, Any]:
    row_index, sample = item
    body = dict(sample)
    sample_id = str(body.pop("id", f"row-{row_index}"))
    requested_id = f"{config.run_id}:{sample_id}"
    request = urllib.request.Request(
        config.gateway_url.rstrip("/") + "/v1/chat/completions",
        data=json.dumps(body).encode("utf-8"),
        headers={
            "Authorization": f"Bearer {config.api_key}",
            "Content-Type": "application/json",
            "X-Request-ID": requested_id,
        },
        method="POST",
    )
    started_at = utc_now()
    started = time.perf_counter()
    response_headers: dict[str, str] = {}
    payload = ""
    http_status = 0
    error_type = ""
    timeout = False
    ttft_ms: float | None = None
    try:
        with urllib.request.urlopen(request, timeout=config.timeout_seconds) as response:
            ttft_ms = round((time.perf_counter() - started) * 1000, 2)
            http_status = response.status
            response_headers = {key.lower(): value for key, value in response.headers.items()}
            payload = response.read().decode("utf-8", errors="replace")
    except urllib.error.HTTPError as exc:
        http_status = exc.code
        response_headers = {key.lower(): value for key, value in exc.headers.items()}
        payload = exc.read().decode("utf-8", errors="replace")
        error_type = classify_error(http_status, payload)
    except (TimeoutError, socket.timeout) as exc:
        error_type = "timeout"
        timeout = True
        payload = str(exc)
    except urllib.error.URLError as exc:
        timeout = isinstance(exc.reason, (TimeoutError, socket.timeout))
        error_type = "timeout" if timeout else "gateway_unavailable"
        payload = str(exc)

    ended_at = utc_now()
    latency_ms = round((time.perf_counter() - started) * 1000, 2)
    parsed = parse_response(payload) if 200 <= http_status < 300 else {}
    if 200 <= http_status < 300 and not parsed.get("valid"):
        error_type = "invalid_model_response"
    success = 200 <= http_status < 300 and bool(parsed.get("valid")) and not timeout
    usage = parsed.get("usage", {})
    request_id = response_headers.get("x-request-id") or str(parsed.get("id") or requested_id)
    retry_count = parse_int(response_headers.get("x-retry-count"), 0)
    return {
        "runId": config.run_id,
        "requestId": request_id,
        "dataset": config.dataset.stem,
        "rowIndex": row_index,
        "methodId": config.method_id,
        "method": method_label(config.method_id),
        "provider": response_headers.get("x-provider") or config.provider,
        "model": str(parsed.get("model") or config.model),
        "modelVersion": config.model_version,
        "route": response_headers.get("x-routing-strategy", "unknown"),
        "startedAt": started_at,
        "endedAt": ended_at,
        "latencyMs": latency_ms,
        "ttftMs": ttft_ms,
        "inputTokens": parse_int(usage.get("prompt_tokens"), 0),
        "outputTokens": parse_int(usage.get("completion_tokens"), 0),
        "totalTokens": parse_int(usage.get("total_tokens"), 0),
        "status": "success" if success else "timeout" if timeout else "error",
        "httpStatus": http_status,
        "errorType": "" if success else error_type or "request_failed",
        "timeout": timeout,
        "retryCount": retry_count,
        "cacheHit": response_headers.get("x-cache", "").lower() in {"hit", "true", "1"},
    }


def parse_response(payload: str) -> dict[str, Any]:
    try:
        value = json.loads(payload)
        content = value["choices"][0]["message"]["content"]
        if not isinstance(content, str) or not content.strip():
            return {"valid": False}
        return {"valid": True, "id": value.get("id"), "model": value.get("model"), "usage": value.get("usage") or {}}
    except (json.JSONDecodeError, KeyError, IndexError, TypeError):
        return {"valid": False}


def classify_error(status: int, text: str) -> str:
    lowered = text.lower()
    if status in {408, 504} or "timeout" in lowered or "deadline" in lowered:
        return "timeout"
    if status in {401, 403}:
        return "authentication"
    if status == 429:
        return "rate_limit"
    if status >= 500:
        return "provider_error"
    if status >= 400:
        return "invalid_request"
    return "request_failed"


def recompute_summary(requests: list[dict[str, Any]]) -> dict[str, Any]:
    if not requests:
        raise ValueError("at least one raw request is required")
    latencies = [float(row["latencyMs"]) for row in requests]
    ttfts = [float(row["ttftMs"]) for row in requests if row.get("ttftMs") not in {None, ""}]
    successes = sum(1 for row in requests if row["status"] == "success" and 200 <= int(row["httpStatus"]) < 300)
    timeouts = sum(1 for row in requests if bool_value(row["timeout"]) or row["status"] == "timeout")
    errors = len(requests) - successes - timeouts
    starts = [parse_time(str(row["startedAt"])) for row in requests]
    ends = [parse_time(str(row["endedAt"])) for row in requests]
    duration = max((max(ends) - min(starts)).total_seconds(), 0.0)
    denominator = len(requests)
    return {
        "requestCount": denominator,
        "avgLatencyMs": rounded(sum(latencies) / denominator),
        "p50LatencyMs": percentile(latencies, 0.50),
        "p95LatencyMs": percentile(latencies, 0.95),
        "p99LatencyMs": percentile(latencies, 0.99),
        "ttftMs": rounded(sum(ttfts) / len(ttfts)) if ttfts else None,
        "throughputRps": rounded(successes / duration, 4) if duration > 0 else 0.0,
        "successRatePct": rounded(successes * 100 / denominator),
        "errorRatePct": rounded(errors * 100 / denominator),
        "timeoutRatePct": rounded(timeouts * 100 / denominator),
        "startedAt": min(starts).isoformat().replace("+00:00", "Z"),
        "endedAt": max(ends).isoformat().replace("+00:00", "Z"),
    }


def build_benchmark_summary(config: RunConfig, calculated: dict[str, Any]) -> dict[str, Any]:
    success_rate = float(calculated["successRatePct"])
    return {
        "runId": config.run_id,
        "methodId": config.method_id,
        "method": method_label(config.method_id),
        "dataset": config.dataset.stem,
        **{key: calculated[key] for key in [
            "requestCount", "avgLatencyMs", "p50LatencyMs", "p95LatencyMs", "p99LatencyMs", "ttftMs", "throughputRps",
            "successRatePct", "errorRatePct", "timeoutRatePct",
        ]},
        "concurrency": config.concurrency,
        "requestRate": config.request_rate,
        "warmUp": config.warm_up,
        "repeatedRuns": config.repeated_runs,
        "timeoutSettingSeconds": config.timeout_seconds,
        "provider": config.provider,
        "targetModel": config.model,
        "modelVersion": config.model_version,
        "gatewayVersion": config.gateway_version,
        "improvementPct": None,
        "testDate": calculated["startedAt"],
        "source": "request-level-gateway-runner",
        "rawFilePath": str(config.report_dir / "raw_requests.csv"),
        "exportId": config.run_id,
        "status": "passed" if success_rate >= 95 else "failed" if success_rate == 0 else "partial",
        "partialData": success_rate < 100 or calculated["ttftMs"] is None,
    }


def write_artifacts(config: RunConfig, requests: list[dict[str, Any]], summary: dict[str, Any]) -> None:
    config.report_dir.mkdir(parents=True, exist_ok=True)
    write_raw_csv(config.report_dir / "raw_requests.csv", requests)
    write_raw_csv(config.report_dir / "errors_and_timeouts.csv", [row for row in requests if row["status"] != "success"])
    write_summary_csv(config.report_dir / "summary.csv", requests)
    (config.report_dir / "summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2), encoding="utf-8")
    snapshot = {"generatedAt": utc_now(), "requests": requests}
    (config.report_dir / "request_snapshot.json").write_text(json.dumps(snapshot, ensure_ascii=False, indent=2), encoding="utf-8")
    metadata = {
        "project": "VeloxMesh AI Gateway",
        "generatedAt": snapshot["generatedAt"],
        "runId": config.run_id,
        "methodId": config.method_id,
        "method": method_label(config.method_id),
        "dataset": config.dataset.stem,
        "providerId": config.provider,
        "modelId": config.model,
        "modelVersion": config.model_version,
        "gatewayUrl": config.gateway_url,
        "requestCount": len(requests),
        "rawRequestFields": RAW_FIELDS,
    }
    (config.report_dir / "metadata.json").write_text(json.dumps(metadata, ensure_ascii=False, indent=2), encoding="utf-8")
    charts = config.report_dir / "charts"
    charts.mkdir(exist_ok=True)
    values = recompute_summary(requests)
    chart_specs = {
        "latency.svg": ("Average latency", float(values["avgLatencyMs"]), "ms"),
        "tail-latency.svg": ("P99 latency", float(values["p99LatencyMs"]), "ms"),
        "throughput.svg": ("Throughput", float(values["throughputRps"]), "req/s"),
        "error-timeout-rate.svg": ("Error and timeout rate", float(values["errorRatePct"]) + float(values["timeoutRatePct"]), "%"),
    }
    for name, (title, value, unit) in chart_specs.items():
        (charts / name).write_text(single_value_svg(title, method_label(config.method_id), value, unit), encoding="utf-8")
    (config.report_dir / "report.html").write_text(report_html(summary, requests), encoding="utf-8")
    package = config.report_dir / "veloxmesh-benchmark-report.zip"
    with zipfile.ZipFile(package, "w", compression=zipfile.ZIP_DEFLATED) as archive:
        for relative in [
            "report.html", "metadata.json", "summary.csv", "raw_requests.csv", "errors_and_timeouts.csv",
            "charts/latency.svg", "charts/tail-latency.svg", "charts/throughput.svg", "charts/error-timeout-rate.svg",
        ]:
            archive.write(config.report_dir / relative, relative)


def write_raw_csv(path: Path, requests: list[dict[str, Any]]) -> None:
    with path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=RAW_FIELDS)
        writer.writeheader()
        for row in requests:
            writer.writerow(to_csv_request(row))


def write_summary_csv(path: Path, requests: list[dict[str, Any]]) -> None:
    summary = recompute_summary(requests)
    first = requests[0]
    row = {
        "run_id": first["runId"], "method_id": first["methodId"], "method": first["method"], "dataset": first["dataset"],
        "provider": first["provider"], "model": first["model"], "model_version": first["modelVersion"],
        **{snake_name(key): value for key, value in summary.items()},
    }
    with path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=SUMMARY_FIELDS)
        writer.writeheader()
        writer.writerow({key: row.get(key, "") for key in SUMMARY_FIELDS})


def recompute_summary_from_csv(path: Path) -> dict[str, Any]:
    with path.open(encoding="utf-8", newline="") as handle:
        rows = [from_csv_request(row) for row in csv.DictReader(handle)]
    return recompute_summary(rows)


def to_csv_request(row: dict[str, Any]) -> dict[str, Any]:
    return {snake_name(key): value for key, value in row.items() if snake_name(key) in RAW_FIELDS}


def from_csv_request(row: dict[str, str]) -> dict[str, Any]:
    return {
        "runId": row["run_id"], "requestId": row["request_id"], "dataset": row["dataset"], "rowIndex": int(row["row_index"]),
        "methodId": row["method_id"], "method": row["method"], "provider": row["provider"], "model": row["model"],
        "modelVersion": row["model_version"], "route": row["route"], "startedAt": row["started_at"], "endedAt": row["ended_at"],
        "latencyMs": float(row["latency_ms"]), "ttftMs": float(row["ttft_ms"]) if row["ttft_ms"] else None,
        "inputTokens": int(row["input_tokens"]), "outputTokens": int(row["output_tokens"]), "totalTokens": int(row["total_tokens"]),
        "status": row["status"], "httpStatus": int(row["http_status"]), "errorType": row["error_type"],
        "timeout": bool_value(row["timeout"]), "retryCount": int(row["retry_count"]), "cacheHit": bool_value(row["cache_hit"]),
    }


def report_html(summary: dict[str, Any], requests: list[dict[str, Any]]) -> str:
    error_rows = "".join(
        f"<tr><td>{escape(str(row['requestId']))}</td><td>{escape(str(row['status']))}</td><td>{row['httpStatus']}</td><td>{escape(str(row['errorType']))}</td></tr>"
        for row in [item for item in requests if item["status"] != "success"][:20]
    )
    return f"""<!doctype html><html lang="en"><head><meta charset="utf-8"><title>VeloxMesh Benchmark Report</title></head><body>
<h1>VeloxMesh AI Gateway Benchmark Report</h1><p>Run: {escape(str(summary['runId']))}; Method: {escape(str(summary['method']))}</p>
<h2>Result Summary</h2><p>Requests: {summary['requestCount']}; Avg latency: {summary['avgLatencyMs']} ms; P95: {summary['p95LatencyMs']} ms; Success: {summary['successRatePct']}%.</p>
<h2>Charts</h2><p>See the four SVG files in <code>charts/</code>.</p>
<h2>Errors and Timeouts</h2><table><tr><th>Request ID</th><th>Status</th><th>HTTP</th><th>Error type</th></tr>{error_rows}</table>
<h2>Appendix</h2><p>Field definitions are in <code>metadata.json</code>. Complete request evidence is in <code>raw_requests.csv</code>; prompts, responses, and credentials are excluded.</p>
</body></html>"""


def single_value_svg(title: str, label: str, value: float, unit: str) -> str:
    bar_width = 500 if value > 0 else 0
    return f'<svg xmlns="http://www.w3.org/2000/svg" width="900" height="120"><rect width="100%" height="100%" fill="white"/><text x="16" y="28" font-family="Arial" font-size="20">{escape(title)}</text><text x="16" y="72" font-family="Arial" font-size="14">{escape(label)}</text><rect x="260" y="52" width="{bar_width}" height="22" fill="#2563eb"/><text x="780" y="70" font-family="Arial" font-size="14">{value} {escape(unit)}</text></svg>'


def percentile(values: list[float], fraction: float) -> float:
    ordered = sorted(values)
    index = max(0, math.ceil(len(ordered) * fraction) - 1)
    return rounded(ordered[index])


def rounded(value: float, digits: int = 2) -> float:
    return round(value, digits)


def parse_time(value: str) -> datetime:
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def parse_int(value: Any, fallback: int) -> int:
    try:
        return int(value)
    except (TypeError, ValueError):
        return fallback


def bool_value(value: Any) -> bool:
    if isinstance(value, bool):
        return value
    return str(value).lower() in {"true", "1", "yes"}


def snake_name(value: str) -> str:
    result = []
    for character in value:
        if character.isupper():
            result.append("_")
            result.append(character.lower())
        else:
            result.append(character)
    return "".join(result)


def load_env_file(path: Path | None) -> None:
    if not path:
        return
    for line in path.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith("#") or "=" not in stripped:
            continue
        key, value = stripped.split("=", 1)
        os.environ.setdefault(key, value.strip().strip('"').strip("'"))


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run request-level benchmark evidence through VeloxMesh Gateway.")
    parser.add_argument("--dataset", type=Path, required=True)
    parser.add_argument("--report-dir", type=Path, required=True)
    parser.add_argument("--env-file", type=Path)
    parser.add_argument("--gateway-url", default=os.environ.get("GATEWAY_URL", "http://127.0.0.1:8080"))
    parser.add_argument("--api-key", default="")
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--method-id", choices=sorted(METHOD_LABELS), required=True)
    parser.add_argument("--provider", required=True)
    parser.add_argument("--model", required=True)
    parser.add_argument("--model-version", required=True)
    parser.add_argument("--gateway-version", default="VeloxMesh")
    parser.add_argument("--concurrency", type=int, default=1)
    parser.add_argument("--request-rate", type=float)
    parser.add_argument("--timeout-seconds", type=int, default=120)
    parser.add_argument("--warm-up", type=int, default=0)
    parser.add_argument("--repeated-runs", type=int, default=1)
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    load_env_file(args.env_file)
    api_key = args.api_key or os.environ.get("DEV_API_KEY", "")
    result = run(RunConfig(
        dataset=args.dataset, report_dir=args.report_dir, gateway_url=args.gateway_url, api_key=api_key, run_id=args.run_id,
        method_id=args.method_id, provider=args.provider, model=args.model, model_version=args.model_version,
        gateway_version=args.gateway_version, concurrency=args.concurrency, request_rate=args.request_rate,
        timeout_seconds=args.timeout_seconds, warm_up=args.warm_up, repeated_runs=args.repeated_runs,
    ))
    print(json.dumps(result.summary, ensure_ascii=False, indent=2))
    return 0 if result.summary["status"] == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
