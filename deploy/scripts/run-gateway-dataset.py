#!/usr/bin/env python3
from __future__ import annotations

import argparse
import concurrent.futures
import csv
import json
import os
import time
import urllib.error
import urllib.request
from pathlib import Path


def main() -> int:
    args = parse_args()
    load_env_file(args.env_file)
    api_key = args.api_key or os.environ.get("DEV_API_KEY", "")
    if not api_key:
        raise SystemExit("DEV_API_KEY is required; pass --api-key or --env-file")

    gateway_url = args.gateway_url.rstrip("/")
    wait_for_health(gateway_url, args.wait_seconds)
    samples = load_samples(args.dataset, args.model or os.environ.get("VELOXMESH_TEST_MODEL", "example-model"))
    args.report_dir.mkdir(parents=True, exist_ok=True)

    with concurrent.futures.ThreadPoolExecutor(max_workers=args.concurrency) as pool:
        rows = list(pool.map(lambda item: call_gateway(gateway_url, api_key, item), enumerate(samples, start=1)))

    write_reports(args.report_dir, args.dataset, args.concurrency, rows)
    failed = sum(1 for row in rows if not ok_status(row["status"]))
    return 1 if failed else 0


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run a JSONL chat dataset against VeloxMesh Gateway.")
    parser.add_argument("--dataset", type=Path, required=True, help="JSONL file; each line is one chat request.")
    parser.add_argument("--report-dir", type=Path, required=True, help="Directory for summary.json and CSV/JSONL details.")
    parser.add_argument("--env-file", type=Path, help="Optional deploy env file containing DEV_API_KEY.")
    parser.add_argument("--gateway-url", default=os.environ.get("GATEWAY_URL", "http://localhost:8080"))
    parser.add_argument("--api-key", default="")
    parser.add_argument("--model", default="", help="Replaces {{MODEL}} in dataset rows.")
    parser.add_argument("--concurrency", type=int, default=1)
    parser.add_argument("--wait-seconds", type=int, default=90)
    return parser.parse_args()


def load_env_file(path: Path | None) -> None:
    if not path:
        return
    for line in path.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith("#") or "=" not in stripped:
            continue
        key, value = stripped.split("=", 1)
        os.environ.setdefault(key, value.strip().strip('"').strip("'"))


def wait_for_health(gateway_url: str, wait_seconds: int) -> None:
    deadline = time.time() + wait_seconds
    health_url = f"{gateway_url}/healthz"
    while True:
        try:
            with urllib.request.urlopen(health_url, timeout=3) as response:
                if ok_status(response.status):
                    return
        except Exception:
            if time.time() >= deadline:
                raise SystemExit(f"Gateway health check failed: {health_url}")
            time.sleep(2)


def load_samples(path: Path, model: str) -> list[dict]:
    rows: list[dict] = []
    for line in path.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if stripped:
            rows.append(json.loads(stripped.replace("{{MODEL}}", model)))
    if not rows:
        raise SystemExit(f"Dataset is empty: {path}")
    return rows


def call_gateway(gateway_url: str, api_key: str, item: tuple[int, dict]) -> dict:
    index, sample = item
    body = dict(sample)
    sample_id = body.pop("id", f"case-{index:03d}")
    request = urllib.request.Request(
        f"{gateway_url}/v1/chat/completions",
        data=json.dumps(body).encode("utf-8"),
        headers={"Authorization": f"Bearer {api_key}", "Content-Type": "application/json"},
        method="POST",
    )
    start = time.perf_counter()
    try:
        with urllib.request.urlopen(request, timeout=120) as response:
            payload = response.read().decode("utf-8", errors="replace")
            status = response.status
            error = ""
    except urllib.error.HTTPError as exc:
        payload = exc.read().decode("utf-8", errors="replace")
        status = exc.code
        error = payload
    except Exception as exc:
        payload = ""
        status = 0
        error = str(exc)
    return {
        "id": sample_id,
        "status": status,
        "latency_ms": round((time.perf_counter() - start) * 1000, 2),
        "error": error,
        "response_preview": payload[:500],
    }


def write_reports(report_dir: Path, dataset: Path, concurrency: int, rows: list[dict]) -> None:
    with (report_dir / "latency.csv").open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=["id", "status", "latency_ms"])
        writer.writeheader()
        writer.writerows({key: row[key] for key in ["id", "status", "latency_ms"]} for row in rows)

    with (report_dir / "responses.jsonl").open("w", encoding="utf-8") as handle:
        for row in rows:
            handle.write(json.dumps(row, ensure_ascii=True) + "\n")

    with (report_dir / "samples.failed.jsonl").open("w", encoding="utf-8") as handle:
        for row in rows:
            if not ok_status(row["status"]):
                handle.write(json.dumps(row, ensure_ascii=True) + "\n")

    latencies = [row["latency_ms"] for row in rows]
    summary = {
        "dataset": str(dataset),
        "concurrency": concurrency,
        "total": len(rows),
        "ok": sum(1 for row in rows if ok_status(row["status"])),
        "failed": sum(1 for row in rows if not ok_status(row["status"])),
        "avg_latency_ms": round(sum(latencies) / len(latencies), 2) if latencies else 0,
        "max_latency_ms": max(latencies) if latencies else 0,
    }
    (report_dir / "summary.json").write_text(json.dumps(summary, indent=2), encoding="utf-8")
    print(json.dumps(summary, indent=2))
    print(f"report_dir: {report_dir}")


def ok_status(status: int) -> bool:
    return 200 <= status < 300


if __name__ == "__main__":
    raise SystemExit(main())
