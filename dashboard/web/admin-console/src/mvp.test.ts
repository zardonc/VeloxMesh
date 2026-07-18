import { describe, expect, it } from "vitest";
import {
  BENCHMARK_COLUMNS,
  COMPARED_METHODS,
  benchmarkChartKey,
  buildBenchmarkCsv,
  buildBenchmarkReportHtml,
  buildBenchmarkComparisonGroups,
  calculateBenchmarkImprovements,
  demoBenchmarks,
  filterBenchmarkRows,
  getNavigationForRole,
  mapBffBenchmarksToMvpRuns,
  mapBffProviderHealth,
  mapBffRequestLogs,
  mvpSessionFromBff,
  maskApiKey,
  roleCanAccessView
} from "./api";

describe("MVP benchmark data contract", () => {
  it("exposes every required benchmark column", async () => {
    const benchmarks = demoBenchmarks;

    expect(BENCHMARK_COLUMNS).toEqual([
      "Run ID",
      "Method",
      "Dataset",
      "Request count",
      "Concurrency",
      "Request rate",
      "Warm-up",
      "Repeated runs",
      "Timeout setting",
      "Provider",
      "Target model",
      "Gateway version",
      "Avg latency",
      "P50 latency",
      "P95 latency",
      "P99 latency",
      "TTFT",
      "Throughput",
      "Success rate",
      "Error rate",
      "Timeout rate",
      "Improvement",
      "Test date",
      "Source",
      "Raw file path",
      "Export ID",
      "Status",
      "Partial data"
    ]);
    expect(benchmarks).toHaveLength(4);
    expect(benchmarks.map((row) => row.method)).toEqual([
      "Local Baseline",
      "Our Gateway Method",
      "Improved Model",
      "Our Gateway + Improved Model"
    ]);
  });

  it("builds CSV with benchmark values and headers", async () => {
    const csv = buildBenchmarkCsv(demoBenchmarks);

    expect(csv.split("\n")[0]).toContain("Run ID,Method,Dataset,Request count");
    expect(csv).toContain("Our Gateway + Improved Model");
    expect(csv).toContain("P99 latency");
    expect(csv).toContain("Source,Raw file path,Export ID,Status,Partial data");
    expect(csv).toContain("bm-gateway-model-001");
  });

  it("builds report content with the required sections", async () => {
    const report = buildBenchmarkReportHtml(demoBenchmarks);

    expect(report).toContain("Report Metadata");
    expect(report).toContain("Benchmark Setup");
    expect(report).toContain("Compared Methods");
    expect(report).toContain("Result Summary");
    expect(report).toContain("Charts");
    expect(report).toContain("Analysis");
    expect(report).toContain("Data Source");
    expect(report).toContain("Limitations");
    expect(report).toContain("Appendix");
    expect(report).toContain("bm-gateway-model-001");
    expect(report).toContain("outputs/benchmarks/gateway_plus_model.json");
    expect(report).not.toContain("mock report");
  });

  it("does not invent a comparison conclusion from one run", () => {
    const report = buildBenchmarkReportHtml([demoBenchmarks[0]]);

    expect(report).toContain("Insufficient Data");
    expect(report).not.toContain("performs best overall");
  });

  it("maps BFF benchmark rows into the control panel table model", () => {
    const rows = mapBffBenchmarksToMvpRuns({
      source: "redis",
      generatedAt: "2026-07-16T00:05:00Z",
      storage: {
        redis: { status: "connected", detail: "loaded veloxmesh:benchmarks" },
        qdrant: { status: "connected", detail: "collection ready" }
      },
      benchmarks: [
        {
          runId: "run-mmlu-20",
          method: "Our Gateway Method",
          dataset: "mmlu_20",
          requestCount: 20,
          concurrency: 1,
          requestRate: null,
          warmUp: 0,
          repeatedRuns: 1,
          timeoutSettingSeconds: 120,
          provider: "openai-compatible",
          targetModel: "model-a",
          gatewayVersion: "VeloxMesh",
          avgLatencyMs: 600,
          p50LatencyMs: 500,
          p95LatencyMs: 842,
          p99LatencyMs: 900,
          ttftMs: 180,
          throughputRps: 1.2,
          successRatePct: 95,
          errorRatePct: 5,
          timeoutRatePct: 0,
          improvementPct: null,
          testDate: "2026-07-16T00:00:00Z",
          source: "gateway-runner",
          rawFilePath: "reports/run-mmlu-20",
          exportId: "run-mmlu-20",
          status: "passed",
          partialData: false
        }
      ]
    });

    expect(rows).toHaveLength(1);
    expect(rows[0]).toMatchObject({
      runId: "run-mmlu-20",
      method: "Our Gateway Method",
      dataset: "mmlu_20",
      provider: "openai-compatible",
      requestCount: 20,
      p95LatencyMs: 842,
      source: "gateway-runner"
    });
  });

  it("keeps an empty live response empty instead of substituting mock rows", () => {
    expect(mapBffBenchmarksToMvpRuns({ source: "empty", benchmarks: [] })).toEqual([]);
  });

  it("uses run IDs to keep chart keys unique when methods repeat", () => {
    const first = { ...demoBenchmarks[0], runId: "run-a", method: "Our Gateway Method" };
    const second = { ...demoBenchmarks[1], runId: "run-b", method: "Our Gateway Method" };

    expect(benchmarkChartKey("Latency", first)).not.toBe(benchmarkChartKey("Latency", second));
  });

  it("groups only comparable setups and reports missing methods", () => {
    const groups = buildBenchmarkComparisonGroups(demoBenchmarks.slice(0, 2));

    expect(groups).toHaveLength(1);
    expect(groups[0].presentMethods).toEqual(COMPARED_METHODS.slice(0, 2));
    expect(groups[0].missingMethods).toEqual(["Improved Model", "Our Gateway + Improved Model"]);
    expect(groups[0].complete).toBe(false);
  });

  it("calculates improvement only against a matching Local Baseline", () => {
    const calculated = calculateBenchmarkImprovements([
      { ...demoBenchmarks[0], improvementPct: null, avgLatencyMs: 1000 },
      { ...demoBenchmarks[1], improvementPct: null, avgLatencyMs: 750 }
    ]);
    expect(calculated[0].improvementPct).toBe(0);
    expect(calculated[1].improvementPct).toBe(25);
    expect(calculateBenchmarkImprovements([{ ...demoBenchmarks[1], improvementPct: null }])[0].improvementPct).toBeNull();
  });

  it("filters benchmark rows by dataset, method, and free text", () => {
    expect(filterBenchmarkRows(demoBenchmarks, {
      dataset: demoBenchmarks[0].dataset,
      method: "Our Gateway Method",
      query: "llama-3.1"
    })).toHaveLength(1);
    expect(filterBenchmarkRows(demoBenchmarks, { dataset: "All", method: "All", query: "missing-value" })).toEqual([]);
  });
});

describe("MVP role and security behavior", () => {
	it("builds the UI session from the authenticated BFF identity", () => {
		expect(mvpSessionFromBff({ user: "admin_user", role: "Admin", scopes: ["admin:write"] })).toEqual({
			user: "admin_user",
			userId: "",
			tenantId: "",
			role: "Admin",
			apiKey: ""
		});
		expect(() => mvpSessionFromBff({ user: "bad", role: "Owner", scopes: [] })).toThrow("Unsupported account role");
	});

  it("masks API keys without exposing the secret", () => {
    expect(maskApiKey("vx_live_1234567890abcdef")).toBe("vx_l...cdef");
    expect(maskApiKey("short")).toBe("••••");
  });

  it("prevents customers from accessing admin views", () => {
    expect(roleCanAccessView("Customer", "benchmarks")).toBe(false);
    expect(roleCanAccessView("Customer", "customer-home")).toBe(true);
    expect(roleCanAccessView("Admin", "benchmarks")).toBe(true);
  });

  it("returns separate sidebars for admin and customer roles", () => {
    expect(getNavigationForRole("Admin").map((item) => item.view)).toContain("provider-health");
    expect(getNavigationForRole("Admin").map((item) => item.view)).toContain("request-logs");
    expect(getNavigationForRole("Customer").map((item) => item.view)).toEqual([
      "customer-home",
      "customer-usage",
      "customer-requests",
      "customer-api-keys",
      "customer-account"
    ]);
  });
});

describe("live operational data mapping", () => {
  it("maps BFF provider health rows without mock substitution", () => {
    expect(mapBffProviderHealth({
      source: "redis",
      providers: [{
        provider: "sans-openai-compatible",
        targetModel: "nvidia/z-ai/glm-5.2",
        status: "Healthy",
        avgLatencyMs: 3552.54,
        errorRate: 0,
        timeoutRate: 0,
        lastChecked: "2026-07-16T18:12:00Z"
      }]
    })).toEqual([expect.objectContaining({ provider: "sans-openai-compatible", avgLatencyMs: 3552.54 })]);
    expect(mapBffProviderHealth({ source: "empty", providers: [] })).toEqual([]);
  });

  it("maps real request IDs, methods, TTFT, and errors", () => {
    expect(mapBffRequestLogs({
      source: "redis",
      logs: [{
        requestId: "run-live:mmlu-0",
        tenant: "benchmark",
        provider: "sans-openai-compatible",
        model: "nvidia/z-ai/glm-5.2",
        method: "Our Gateway Method",
        inputTokens: 90,
        outputTokens: 54,
        status: "Success",
        latencyMs: 4761.94,
        ttftMs: 4761.69,
        errorMessage: "",
        timestamp: "2026-07-16T18:10:33Z"
      }]
    })[0]).toMatchObject({ requestId: "run-live:mmlu-0", method: "Our Gateway Method", ttftMs: 4761.69 });
  });
});
