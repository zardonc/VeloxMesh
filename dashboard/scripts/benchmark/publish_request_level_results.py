#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import socket
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import request_level_benchmark as benchmark


def build_snapshots(report_dirs: list[Path], generated_at: str | None = None) -> tuple[dict[str, Any], dict[str, Any]]:
    if not report_dirs:
        raise ValueError("at least one report directory is required")
    generated_at = generated_at or datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    aggregates: list[dict[str, Any]] = []
    requests: list[dict[str, Any]] = []
    for report_dir in report_dirs:
        summary = json.loads((report_dir / "summary.json").read_text(encoding="utf-8-sig"))
        snapshot = json.loads((report_dir / "request_snapshot.json").read_text(encoding="utf-8-sig"))
        run_requests = snapshot.get("requests") or []
        if not run_requests:
            raise ValueError(f"request_snapshot.json has no rows: {report_dir}")
        calculated = benchmark.recompute_summary(run_requests)
        for key in [
            "requestCount", "avgLatencyMs", "p50LatencyMs", "p95LatencyMs", "p99LatencyMs", "ttftMs", "throughputRps",
            "successRatePct", "errorRatePct", "timeoutRatePct",
        ]:
            summary[key] = calculated[key]
        benchmark.method_label(str(summary["methodId"]))
        aggregates.append(summary)
        requests.extend(run_requests)
    return (
        {"generatedAt": generated_at, "benchmarks": aggregates},
        {"generatedAt": generated_at, "requests": requests},
    )


def redis_set(redis_addr: str, key: str, value: dict[str, Any]) -> None:
    host, port_text = split_address(redis_addr)
    payload = json.dumps(value, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    command = f"*3\r\n$3\r\nSET\r\n${len(key.encode('utf-8'))}\r\n{key}\r\n${len(payload)}\r\n".encode("utf-8") + payload + b"\r\n"
    with socket.create_connection((host, int(port_text)), timeout=5) as connection:
        connection.sendall(command)
        response = connection.recv(128)
    if not response.startswith(b"+OK"):
        raise RuntimeError(f"Redis SET failed for {key}")


def split_address(value: str) -> tuple[str, str]:
    if ":" not in value:
        return value, "6379"
    return tuple(value.rsplit(":", 1))  # type: ignore[return-value]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Publish aggregate and request-level benchmark snapshots to Redis.")
    parser.add_argument("--report-dir", type=Path, action="append", required=True)
    parser.add_argument("--redis-addr", default="127.0.0.1:6379")
    parser.add_argument("--snapshot-output", type=Path)
    parser.add_argument("--dry-run", action="store_true")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    aggregate, requests = build_snapshots(args.report_dir)
    if not args.dry_run:
        redis_set(args.redis_addr, "veloxmesh:benchmarks", aggregate)
        redis_set(args.redis_addr, "veloxmesh:benchmark_requests", requests)
    if args.snapshot_output:
        args.snapshot_output.parent.mkdir(parents=True, exist_ok=True)
        args.snapshot_output.write_text(json.dumps({"aggregate": aggregate, "requestLevel": requests}, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps({"runs": len(aggregate["benchmarks"]), "requests": len(requests["requests"]), "published": not args.dry_run}))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
