from __future__ import annotations

import csv
import json
import os
import subprocess
import tempfile
import threading
import unittest
import zipfile
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

import publish_request_level_results as publisher
import request_level_benchmark as benchmark


class FakeGatewayHandler(BaseHTTPRequestHandler):
    requests: list[dict] = []
    providers: list[dict] = []

    def do_GET(self) -> None:
        if self.path == "/healthz":
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"ok")
            return
        if self.path == "/v1/models":
            encoded = json.dumps({"data": [{"id": "improved-model"}]}).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)
            return
        self.send_error(404)

    def do_POST(self) -> None:
        if self.path == "/admin/v1/providers":
            length = int(self.headers.get("Content-Length", "0"))
            body = json.loads(self.rfile.read(length))
            FakeGatewayHandler.providers.append(body)
            encoded = json.dumps({**body, "revision": 1, "secret": {"configured": True}}).encode("utf-8")
            self.send_response(201)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)
            return
        if self.path != "/v1/chat/completions":
            self.send_error(404)
            return
        length = int(self.headers.get("Content-Length", "0"))
        body = json.loads(self.rfile.read(length))
        FakeGatewayHandler.requests.append({
            "path": self.path,
            "authorization": self.headers.get("Authorization"),
            "request_id": self.headers.get("X-Request-ID"),
            "body": body,
        })
        response = {
            "id": self.headers.get("X-Request-ID"),
            "model": body["model"],
            "choices": [{"message": {"role": "assistant", "content": "answer"}}],
            "usage": {"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18},
        }
        encoded = json.dumps(response).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.send_header("X-Request-ID", self.headers.get("X-Request-ID", ""))
        self.send_header("X-Provider", "improved-provider")
        self.send_header("X-Routing-Strategy", "default-provider")
        self.send_header("X-Cache", "hit" if self.headers.get("X-Request-ID", "").endswith("mmlu-1") else "miss")
        self.send_header("X-Retry-Count", "1")
        self.end_headers()
        self.wfile.write(encoded)

    def log_message(self, _format: str, *_args: object) -> None:
        return


class RequestLevelBenchmarkTests(unittest.TestCase):
    def setUp(self) -> None:
        FakeGatewayHandler.requests = []
        FakeGatewayHandler.providers = []

    def test_method_ids_are_stable_and_complete(self) -> None:
        self.assertEqual(benchmark.METHOD_LABELS, {
            "local_baseline": "Local Baseline",
            "gateway": "Our Gateway Method",
            "improved_model": "Improved Model",
            "gateway_improved_model": "Our Gateway + Improved Model",
        })
        with self.assertRaises(ValueError):
            benchmark.method_label("custom-untracked-method")

    def test_gateway_run_writes_request_rows_recomputable_summary_and_complete_package(self) -> None:
        server = ThreadingHTTPServer(("127.0.0.1", 0), FakeGatewayHandler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)

        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            dataset = root / "mmlu.jsonl"
            dataset.write_text(
                "\n".join([
                    json.dumps({"id": "mmlu-0", "messages": [{"role": "user", "content": "Question 0"}]}),
                    json.dumps({"id": "mmlu-1", "messages": [{"role": "user", "content": "Question 1"}]}),
                ]),
                encoding="utf-8",
            )
            report_dir = root / "report"
            secret = "provider-secret-value"
            config = benchmark.RunConfig(
                dataset=dataset,
                report_dir=report_dir,
                gateway_url=f"http://127.0.0.1:{server.server_port}",
                api_key=secret,
                run_id="run-improved-1",
                method_id="gateway_improved_model",
                provider="improved-provider",
                model="improved-model",
                model_version="v2.1",
                gateway_version="test",
                concurrency=2,
                request_rate=None,
                timeout_seconds=5,
            )

            result = benchmark.run(config)

            self.assertEqual(len(result.requests), 2)
            self.assertEqual(len(FakeGatewayHandler.requests), 2)
            self.assertTrue(all(item["path"] == "/v1/chat/completions" for item in FakeGatewayHandler.requests))
            self.assertTrue(all(item["authorization"] == f"Bearer {secret}" for item in FakeGatewayHandler.requests))
            self.assertTrue(all(row["runId"] == "run-improved-1" for row in result.requests))
            self.assertTrue(all(row["methodId"] == "gateway_improved_model" for row in result.requests))
            self.assertTrue(all(row["provider"] == "improved-provider" for row in result.requests))
            self.assertEqual([row["rowIndex"] for row in result.requests], [0, 1])
            self.assertEqual(result.requests[0]["totalTokens"], 18)
            self.assertEqual(result.requests[0]["route"], "default-provider")
            self.assertEqual(result.requests[0]["retryCount"], 1)
            self.assertFalse(result.requests[0]["cacheHit"])
            self.assertTrue(result.requests[1]["cacheHit"])

            with (report_dir / "raw_requests.csv").open(encoding="utf-8", newline="") as handle:
                raw_rows = list(csv.DictReader(handle))
            self.assertEqual(len(raw_rows), 2)
            recomputed = benchmark.recompute_summary_from_csv(report_dir / "raw_requests.csv")
            summary = json.loads((report_dir / "summary.json").read_text(encoding="utf-8"))
            self.assertEqual(recomputed["requestCount"], summary["requestCount"])
            self.assertEqual(recomputed["avgLatencyMs"], summary["avgLatencyMs"])
            self.assertEqual(recomputed["successRatePct"], summary["successRatePct"])

            with zipfile.ZipFile(report_dir / "veloxmesh-benchmark-report.zip") as archive:
                self.assertTrue({
                    "report.html", "metadata.json", "summary.csv", "raw_requests.csv", "errors_and_timeouts.csv",
                    "charts/latency.svg", "charts/tail-latency.svg", "charts/throughput.svg", "charts/error-timeout-rate.svg",
                }.issubset(set(archive.namelist())))
            for path in report_dir.rglob("*"):
                if path.is_file() and path.suffix != ".zip":
                    self.assertNotIn(secret, path.read_text(encoding="utf-8"))

    def test_publisher_merges_runs_into_aggregate_and_request_snapshots(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            report_a = self._minimal_report(root / "a", "run-a", "gateway")
            report_b = self._minimal_report(root / "b", "run-b", "improved_model")

            aggregate, requests = publisher.build_snapshots([report_a, report_b], "2026-07-18T12:00:00Z")

            self.assertEqual([row["runId"] for row in aggregate["benchmarks"]], ["run-a", "run-b"])
            self.assertEqual(len(requests["requests"]), 2)
            self.assertEqual({row["methodId"] for row in requests["requests"]}, {"gateway", "improved_model"})

    def test_improved_model_registration_uses_gateway_and_does_not_print_secrets(self) -> None:
        server = ThreadingHTTPServer(("127.0.0.1", 0), FakeGatewayHandler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        secret = "provider-secret-value"
        environment = {
            **os.environ,
            "VELOXMESH_ADMIN_API_KEY": "admin-test-key",
            "VELOXMESH_DATA_API_KEY": "data-test-key",
            "IMPROVED_MODEL_API_KEY": secret,
        }
        script = Path(__file__).with_name("register-improved-model.ps1")
        completed = subprocess.run([
            "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", str(script),
            "-GatewayUrl", f"http://127.0.0.1:{server.server_port}",
            "-ProviderId", "improved-provider", "-BaseUrl", "https://model.example/v1",
            "-ModelId", "improved-model", "-ModelVersion", "v2.1",
        ], check=True, capture_output=True, text=True, env=environment)
        result = json.loads(completed.stdout)
        self.assertTrue(result["verified"])
        self.assertEqual(result["provider_id"], "improved-provider")
        self.assertEqual(result["model_id"], "improved-model")
        self.assertEqual(result["model_version"], "v2.1")
        self.assertEqual(FakeGatewayHandler.providers[0]["api_key"], secret)
        self.assertNotIn(secret, completed.stdout)

    def _minimal_report(self, path: Path, run_id: str, method_id: str) -> Path:
        path.mkdir(parents=True)
        label = benchmark.method_label(method_id)
        summary = {
            "runId": run_id, "methodId": method_id, "method": label, "dataset": "tiny", "requestCount": 1,
            "provider": "provider", "targetModel": "model", "modelVersion": "v1", "avgLatencyMs": 10,
        }
        request = {
            "runId": run_id, "requestId": f"{run_id}:0", "dataset": "tiny", "rowIndex": 0,
            "methodId": method_id, "method": label, "provider": "provider", "model": "model", "modelVersion": "v1",
            "route": "default-provider", "startedAt": "2026-07-18T12:00:00Z", "endedAt": "2026-07-18T12:00:00.010Z",
            "latencyMs": 10, "ttftMs": 5, "inputTokens": 1, "outputTokens": 1, "totalTokens": 2,
            "status": "success", "httpStatus": 200, "errorType": "", "timeout": False, "retryCount": 0, "cacheHit": False,
        }
        (path / "summary.json").write_text(json.dumps(summary), encoding="utf-8")
        (path / "request_snapshot.json").write_text(json.dumps({"generatedAt": "2026-07-18T12:00:00Z", "requests": [request]}), encoding="utf-8")
        return path


if __name__ == "__main__":
    unittest.main()
