"""Real embedding endpoint check; each invocation has a 60-second deadline."""

import argparse
import concurrent.futures
import json
import math
import threading
import time
import urllib.error
import urllib.request
from pathlib import Path

TEST_SECONDS = 60
REQUEST_SECONDS = 10
MAX_CONCURRENCY = 16
MAX_REQUESTS = 4096


def request(options, index):
    started = time.perf_counter()
    payload = json.dumps({"model": options.model, "input": [
        f"Verification document {index}: the trial lasts fourteen days.",
        f"Verification document {index}: the refund window lasts seven days.",
    ]}).encode()
    req = urllib.request.Request(options.url.rstrip("/") + "/v1/embeddings", payload,
                                 {"Content-Type": "application/json"})
    sample = {"type": "client", "index": index, "ok": False}
    try:
        with urllib.request.urlopen(req, timeout=REQUEST_SECONDS) as response:
            body = json.load(response)
            sample["status"] = response.status
        data = body.get("data", [])
        assert len(data) == 2, "batch output count mismatch"
        assert sorted(item["index"] for item in data) == [0, 1], "invalid output indexes"
        vectors = [item["embedding"] for item in sorted(data, key=lambda item: item["index"])]
        assert len(vectors[0]) > 0 and len(vectors[0]) == len(vectors[1]), "dimension mismatch"
        assert all(isinstance(n, (int, float)) and math.isfinite(n)
                   for vector in vectors for n in vector), "invalid vector values"
        assert all(any(n != 0 for n in vector) for vector in vectors), "zero vector"
        assert vectors[0] != vectors[1], "different inputs returned identical vectors"
        sample.update(ok=True, dimension=len(vectors[0]))
    except (urllib.error.URLError, TimeoutError, ValueError, KeyError, AssertionError) as error:
        sample["error"] = str(error)
    sample["elapsed_ms"] = (time.perf_counter() - started) * 1000
    return sample


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--url", required=True)
    parser.add_argument("--model", required=True)
    parser.add_argument("--concurrency", type=int, required=True)
    parser.add_argument("--count", type=int, required=True)
    parser.add_argument("--output", type=Path, required=True)
    options = parser.parse_args()
    assert 1 <= options.concurrency <= MAX_CONCURRENCY
    assert 1 <= options.count <= MAX_REQUESTS
    started = time.perf_counter()
    deadline = threading.Timer(TEST_SECONDS, lambda: __import__("os")._exit(124))
    deadline.daemon = True
    deadline.start()
    options.output.parent.mkdir(parents=True, exist_ok=True)
    samples = []
    with options.output.open("w", encoding="utf-8") as output:
        with concurrent.futures.ThreadPoolExecutor(max_workers=options.concurrency) as pool:
            for sample in pool.map(lambda index: request(options, index), range(options.count)):
                output.write(json.dumps(sample) + "\n")
                output.flush()
                samples.append(sample)
        elapsed = time.perf_counter() - started
        durations = sorted(sample["elapsed_ms"] for sample in samples)
        quantile = lambda fraction: durations[math.ceil(len(durations) * fraction) - 1]
        summary = {"type": "metadata", "model": options.model, "concurrency": options.concurrency,
                   "count": options.count, "batch_size": 2, "elapsed_ms": elapsed * 1000,
                   "ok": sum(sample["ok"] for sample in samples),
                   "dimension": sorted({sample["dimension"] for sample in samples if sample["ok"]}),
                   "rps": len(samples) / elapsed, "p50_ms": quantile(0.5),
                   "p95_ms": quantile(0.95), "p99_ms": quantile(0.99)}
        output.write(json.dumps(summary) + "\n")
    deadline.cancel()
    print(json.dumps(summary))
    if summary["ok"] != options.count or len(summary["dimension"]) != 1:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
