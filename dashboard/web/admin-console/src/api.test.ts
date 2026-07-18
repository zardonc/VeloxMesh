import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  buildApiKeysPageViewModel,
  buildAuditPageViewModel,
  buildBenchmarksPageViewModel,
  buildDashboardViewModel,
  buildProvidersPageViewModel,
  buildRequestLogsPageViewModel,
  buildRoutingPageViewModel,
  buildTenantsPageViewModel,
  createApiKey,
	createCustomerApiKey,
  createProvider,
  createRoutingRule,
  createTenant,
  deleteApiKey,
  deleteRoutingRule,
  deleteTenant,
	exportAuditCSV,
	fetchInitialAppData,
	fetchCustomerDashboardData,
	fetchCustomerRequests,
	fetchCustomerUsage,
  fetchSession,
  filterManagementRows,
  loginAccount,
  logoutAccount,
  registerAccount,
	registerCustomerAccount,
	revokeCustomerApiKey,
  updateProvider,
  updateRoutingRule,
  updateTenant,
  verifyLoginCode
} from "./api";

beforeEach(() => {
  vi.restoreAllMocks();
});

describe("buildDashboardViewModel", () => {
  it("maps BFF responses into dashboard sections", () => {
    const viewModel = buildDashboardViewModel({
      summary: {
        defaultProvider: "sans-primary",
        defaultModel: "oc/deepseek-v4-flash-free",
        modelCount: 3,
        activeTenants: 4,
        requestVolume: 18420,
        successRate: 99.2,
        p95LatencyMs: 842,
        queueDepth: 17,
        updatedAt: "2026-07-06T14:41:04Z"
      },
      providers: {
        providers: [
          {
            name: "sans-primary",
            baseUrl: "https://example.test/v1",
            defaultModel: "oc/deepseek-v4-flash-free",
            models: ["a", "b", "c"],
            status: "healthy",
            p95LatencyMs: 842,
            successRate: 99.2,
            requestsToday: 18420
          }
        ]
      },
      requests: {
        requests: [
          {
            id: "req_1",
            tenant: "capstone-demo",
            provider: "sans-primary",
            model: "oc/deepseek-v4-flash-free",
            status: "success",
            latencyMs: 714,
            route: "latency-aware"
          }
        ]
      },
      routing: { rules: [] },
      tenants: { tenants: [] },
      apiKeys: { keys: [] },
      audit: { events: [] },
      requestLogs: { logs: [] },
      benchmarks: { benchmarks: [] },
      session: { user: "local-admin", role: "Admin", scopes: ["admin:write"] }
    });

    expect(viewModel.kpis).toHaveLength(4);
    expect(viewModel.kpis[0]).toMatchObject({ label: "Requests", value: "18,420" });
    expect(viewModel.providers[0].modelCount).toBe(3);
    expect(viewModel.recentRequests[0].statusLabel).toBe("Success");
  });
});

describe("buildProvidersPageViewModel", () => {
  it("creates a dedicated Providers page model from BFF provider data", () => {
    const viewModel = buildProvidersPageViewModel({
      providers: [
        {
          name: "sans-primary",
          baseUrl: "https://example.test/v1",
          defaultModel: "oc/deepseek-v4-flash-free",
          models: ["model-a", "model-b"],
          status: "healthy",
          p95LatencyMs: 842,
          successRate: 99.2,
          requestsToday: 18420
        }
      ]
    });

    expect(viewModel.title).toBe("Providers");
    expect(viewModel.actionLabel).toBe("Add Provider");
    expect(viewModel.rows[0]).toMatchObject({
      name: "sans-primary",
      status: "Healthy",
      modelCount: 2,
      traffic: "18,420"
    });
  });
});

describe("secondary page view models", () => {
  it("creates dedicated page models for every navigation item", () => {
    const pages = [
      buildRoutingPageViewModel(),
      buildTenantsPageViewModel(),
      buildApiKeysPageViewModel(),
      buildAuditPageViewModel()
    ];

    expect(pages.map((page) => page.title)).toEqual([
      "Routing",
      "Tenants",
      "API Keys",
      "Audit"
    ]);
    expect(pages.every((page) => page.actionLabel.length > 0)).toBe(true);
    expect(pages.every((page) => page.rows.length > 0)).toBe(true);
  });

  it("creates request log and benchmark page models from BFF payloads", () => {
    const requests = buildRequestLogsPageViewModel({
      logs: [
        {
          id: "req_10291",
          tenant: "coursework-lab",
          provider: "sans-primary",
          model: "model-a",
          inputTokens: 812,
          outputTokens: 248,
          status: "success",
          latencyMs: 714,
          error: ""
        }
      ]
    });
    const benchmarks = buildBenchmarksPageViewModel({
      source: "redis",
      storage: {
        redis: { status: "connected", detail: "loaded veloxmesh:benchmarks" },
        qdrant: { status: "connected", detail: "healthz ok" }
      },
      benchmarks: [
        {
          runId: "run-1",
          method: "Our Gateway Method",
          dataset: "lmsys_20",
          requestCount: 20,
          concurrency: 1,
          requestRate: null,
          warmUp: 0,
          repeatedRuns: 1,
          timeoutSettingSeconds: 120,
          provider: "openai-compatible",
          targetModel: "model-a",
          gatewayVersion: "VeloxMesh",
          avgLatencyMs: 700,
          p50LatencyMs: 600,
          p95LatencyMs: 842,
          p99LatencyMs: 900,
          ttftMs: 214,
          throughputRps: 2.13,
          successRatePct: 100,
          errorRatePct: 0,
          timeoutRatePct: 0,
          improvementPct: null,
          testDate: "2026-07-16T00:00:00Z",
          source: "gateway-runner",
          rawFilePath: "reports/run-1",
          exportId: "run-1",
          status: "passed",
          partialData: false
        }
      ]
    });

    expect(requests.title).toBe("Requests");
    expect(requests.rows[0]).toMatchObject({ Request: "req_10291", "Input Tokens": "812" });
    expect(benchmarks.title).toBe("Benchmarks");
    expect(benchmarks.metrics).toEqual([
      { label: "Source", value: "Redis", detail: "1 evaluation scenarios" },
      { label: "Redis", value: "Connected", detail: "loaded veloxmesh:benchmarks" },
      { label: "Qdrant", value: "Connected", detail: "healthz ok" }
    ]);
    expect(benchmarks.rows[0]).toMatchObject({ "Run ID": "run-1", Method: "Our Gateway Method", Dataset: "lmsys_20" });
  });

  it("uses BFF supplied rows for secondary page models", () => {
    expect(
      buildRoutingPageViewModel({
        rules: [{ policy: "Cost cap", selector: "cost-aware", target: "backup", status: "Draft" }]
      }).rows[0]
    ).toMatchObject({ Policy: "Cost cap", Target: "backup" });

    expect(
      buildTenantsPageViewModel({
        tenants: [{ tenant: "new-team", owner: "Capstone", dailyQuota: "2,500", status: "Healthy" }]
      }).rows[0]
    ).toMatchObject({ Tenant: "new-team", Owner: "Capstone" });

    expect(
      buildApiKeysPageViewModel({
        keys: [{ key: "vx-new-team", tenant: "new-team", scope: "gateway:invoke", lastUsed: "never" }]
      }).rows[0]
    ).toMatchObject({ Key: "vx-new-team", Scope: "gateway:invoke" });

    expect(
      buildAuditPageViewModel({
        events: [{ time: "20:00", actor: "admin", action: "Created tenant new-team", result: "Success" }]
      }).rows[0]
    ).toMatchObject({ Action: "Created tenant new-team", Result: "Success" });
  });
});

describe("admin write APIs", () => {
  it("posts create operations to BFF endpoints", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ ok: true })
    });
    vi.stubGlobal("fetch", fetchMock);

    await createProvider({
      name: "backup",
      baseUrl: "https://backup.example/v1",
      defaultModel: "backup/model",
      models: ["backup/model"]
    });
    await createRoutingRule({
      policy: "Cost cap",
      selector: "cost-aware",
      target: "backup",
      status: "Draft"
    });
    await createTenant({
      tenant: "new-team",
      owner: "Capstone",
      dailyQuota: "2,500",
      status: "Healthy"
    });
    await createApiKey({
      tenant: "new-team",
      scope: "gateway:invoke"
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "/bff/admin/providers",
      expect.objectContaining({ method: "POST" })
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/bff/admin/routing",
      expect.objectContaining({ method: "POST" })
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/bff/admin/tenants",
      expect.objectContaining({ method: "POST" })
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/bff/admin/api-keys",
      expect.objectContaining({ method: "POST" })
    );
  });

  it("downloads audit CSV text from the BFF", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        text: async () => "time,actor,action,result\n20:00,admin,Exported,Success"
      })
    );

    await expect(exportAuditCSV()).resolves.toContain("time,actor,action,result");
  });

  it("updates and deletes resources through BFF endpoints", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ ok: true })
    });
    vi.stubGlobal("fetch", fetchMock);

    await updateProvider("sans-primary", {
      baseUrl: "https://new.example/v1",
      defaultModel: "new/model",
      models: ["new/model"],
      status: "healthy"
    });
    await updateRoutingRule("Primary route", {
      policy: "Primary route",
      selector: "latency-aware",
      target: "sans-primary",
      status: "Active"
    });
    await updateTenant("capstone-demo", {
      tenant: "capstone-demo",
      owner: "Demo",
      dailyQuota: "6,000",
      status: "Healthy"
    });
    await deleteRoutingRule("Primary route");
    await deleteTenant("capstone-demo");
    await deleteApiKey("vx-dev");

    expect(fetchMock).toHaveBeenCalledWith(
      "/bff/admin/providers/sans-primary",
      expect.objectContaining({ method: "PUT" })
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/bff/admin/routing/Primary%20route",
      expect.objectContaining({ method: "PUT" })
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/bff/admin/tenants/capstone-demo",
      expect.objectContaining({ method: "PUT" })
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/bff/admin/routing/Primary%20route",
      expect.objectContaining({ method: "DELETE" })
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/bff/admin/tenants/capstone-demo",
      expect.objectContaining({ method: "DELETE" })
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/bff/admin/api-keys/vx-dev",
      expect.objectContaining({ method: "DELETE" })
    );
  });

  it("loads session context and surfaces BFF error messages", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn()
        .mockResolvedValueOnce({
          ok: true,
          json: async () => ({ user: "local-admin", role: "Admin", scopes: ["admin:write"] })
        })
        .mockResolvedValueOnce({
          ok: false,
          status: 404,
          statusText: "Not Found",
          json: async () => ({ error: "tenant not found" })
        })
    );

    await expect(fetchSession()).resolves.toMatchObject({ role: "Admin" });
    await expect(deleteTenant("missing")).rejects.toThrow("tenant not found");
    expect(fetch).toHaveBeenCalledWith("/bff/session");
  });

  it("calls auth endpoints for register, login, verification, and logout", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ user: "capstone_owner", role: "Admin", scopes: ["admin:write"] })
    });
    vi.stubGlobal("fetch", fetchMock);

    await registerAccount({
      email: "owner@example.test",
      username: "capstone_owner",
      password: "correct-horse",
      role: "Customer"
    });
    await loginAccount({ identifier: "capstone_owner", password: "correct-horse" });
    await verifyLoginCode({ challengeId: "challenge-1", code: "123456" });
    await logoutAccount();

    expect(fetchMock).toHaveBeenCalledWith(
      "/bff/auth/register",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          email: "owner@example.test",
          username: "capstone_owner",
          password: "correct-horse",
          role: "Customer"
        })
      })
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/bff/auth/login",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ identifier: "capstone_owner", password: "correct-horse" })
      })
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/bff/auth/verify",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ challengeId: "challenge-1", code: "123456" })
      })
    );
    expect(fetchMock).toHaveBeenCalledWith("/bff/auth/logout", expect.objectContaining({ method: "POST" }));
  });

  it("loads customer startup data without calling admin endpoints", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ user: "customer_user", role: "Customer", scopes: ["gateway:invoke"] })
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(fetchInitialAppData()).resolves.toMatchObject({
      kind: "customer",
      session: { user: "customer_user", role: "Customer" }
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith("/bff/session");
    expect(fetchMock.mock.calls.some(([path]) => String(path).startsWith("/bff/admin/"))).toBe(false);
  });
});

describe("filterManagementRows", () => {
  it("filters rows across all visible column values", () => {
    const page = buildRequestLogsPageViewModel({
      logs: [
        {
          id: "req_1",
          tenant: "capstone-demo",
          provider: "sans-primary",
          model: "model-a",
          inputTokens: 10,
          outputTokens: 20,
          status: "success",
          latencyMs: 100,
          error: ""
        },
        {
          id: "req_2",
          tenant: "ops-sandbox",
          provider: "sans-primary",
          model: "model-b",
          inputTokens: 0,
          outputTokens: 0,
          status: "rate_limited",
          latencyMs: 0,
          error: "tenant quota exceeded"
        }
      ]
    });

    expect(filterManagementRows(page.rows, "quota")).toHaveLength(1);
    expect(filterManagementRows(page.rows, "CAPSTONE")[0].Tenant).toBe("capstone-demo");
    expect(filterManagementRows(page.rows, " ")).toHaveLength(2);
  });
});

describe("customer tenant API contract", () => {
	it("loads Customer usage for an explicit time range", async () => {
		const fetchMock = vi.fn().mockResolvedValue({
			ok: true,
			json: async () => ({ tenantId: "tenant-a", summary: {}, series: [], models: [] })
		});
		vi.stubGlobal("fetch", fetchMock);

		await fetchCustomerUsage({ from: "2026-07-10T00:00:00.000Z", to: "2026-07-17T00:00:00.000Z" });

		expect(fetchMock).toHaveBeenCalledWith(
			"/bff/customer/usage?from=2026-07-10T00%3A00%3A00.000Z&to=2026-07-17T00%3A00%3A00.000Z"
		);
	});

	it("loads unbounded Customer usage without an empty query string", async () => {
		const fetchMock = vi.fn().mockResolvedValue({
			ok: true,
			json: async () => ({ tenantId: "tenant-a", summary: {}, series: [], models: [] })
		});
		vi.stubGlobal("fetch", fetchMock);

		await fetchCustomerUsage({});

		expect(fetchMock).toHaveBeenCalledWith("/bff/customer/usage");
	});

	it("loads a filtered page of Customer requests from the tenant endpoint", async () => {
		const fetchMock = vi.fn().mockResolvedValue({
			ok: true,
			json: async () => ({
				tenantId: "tenant-a",
				requests: [],
				page: 2,
				pageSize: 25,
				total: 0,
				source: "redis",
				generatedAt: "now",
				partialData: false
			})
		});
		vi.stubGlobal("fetch", fetchMock);

		await fetchCustomerRequests({
			page: 2,
			pageSize: 25,
			status: "Timeout",
			model: "provider/model-a",
			from: "2026-07-01T00:00:00.000Z",
			to: "2026-07-17T23:59:59.000Z"
		});

		expect(fetchMock).toHaveBeenCalledWith(
			"/bff/customer/requests?page=2&pageSize=25&status=Timeout&model=provider%2Fmodel-a&from=2026-07-01T00%3A00%3A00.000Z&to=2026-07-17T23%3A59%3A59.000Z"
		);
	});

	it("omits empty Customer request filters", async () => {
		const fetchMock = vi.fn().mockResolvedValue({
			ok: true,
			json: async () => ({ tenantId: "tenant-a", requests: [], page: 1, pageSize: 50, total: 0 })
		});
		vi.stubGlobal("fetch", fetchMock);

		await fetchCustomerRequests({ page: 1, pageSize: 50, status: "", model: "" });

		expect(fetchMock).toHaveBeenCalledWith("/bff/customer/requests?page=1&pageSize=50");
	});

	it("registers Customers without sending a client-controlled role or tenant", async () => {
		const fetchMock = vi.fn().mockResolvedValue({
			ok: true,
			json: async () => ({
				status: "verification_required",
				role: "Customer",
				tenantId: "tenant-alice",
				challengeId: "challenge-alice",
				verificationRequired: true,
				message: "Verify your email"
			})
		});
		vi.stubGlobal("fetch", fetchMock);

		await registerCustomerAccount({
			email: "alice@example.test",
			username: "alice_customer",
			organization: "Alice Research",
			password: "correct-horse-battery-staple",
			confirmPassword: "correct-horse-battery-staple"
		});

		expect(fetchMock).toHaveBeenCalledWith(
			"/bff/auth/customer/register",
			expect.objectContaining({
				method: "POST",
				body: expect.not.stringContaining("role")
			})
		);
		expect(fetchMock.mock.calls[0][1].body).not.toContain("tenantId");
	});

	it("loads all Customer pages only from tenant-scoped endpoints", async () => {
		const payloads = [
			{ tenantId: "tenant-a", requests: 1, inputTokens: 10, outputTokens: 20, totalTokens: 30, avgLatencyMs: 100, p95LatencyMs: 100, successRate: 100, errorRate: 0, timeoutRate: 0, modelUsage: { "model-a": 1 }, source: "redis", generatedAt: "now", partialData: false },
			{ tenantId: "tenant-a", summary: {}, series: [], models: [], source: "redis", generatedAt: "now", partialData: false },
			{ tenantId: "tenant-a", requests: [], page: 1, pageSize: 25, total: 0, source: "redis", generatedAt: "now", partialData: false },
			{ tenantId: "tenant-a", keys: [] }
		];
		const fetchMock = vi.fn().mockImplementation(async () => ({ ok: true, json: async () => payloads.shift() }));
		vi.stubGlobal("fetch", fetchMock);

		const data = await fetchCustomerDashboardData();

		expect(data.summary.tenantId).toBe("tenant-a");
		expect(fetchMock.mock.calls.map(([path]) => path)).toEqual([
			"/bff/customer/summary",
			"/bff/customer/usage",
			"/bff/customer/requests?page=1&pageSize=25",
			"/bff/customer/api-keys"
		]);
		expect(fetchMock.mock.calls.some(([path]) => String(path).startsWith("/bff/admin/"))).toBe(false);
	});

	it("creates and revokes a Customer API key by id", async () => {
		const fetchMock = vi.fn().mockResolvedValue({
			ok: true,
			json: async () => ({ id: "key-a", key: "vx_live_secret", maskedKey: "vx_live...cret", scope: "gateway:invoke", status: "Active", createdAt: "now" })
		});
		vi.stubGlobal("fetch", fetchMock);

		await createCustomerApiKey();
		await revokeCustomerApiKey("key-a");

		expect(fetchMock).toHaveBeenNthCalledWith(1, "/bff/customer/api-keys", expect.objectContaining({ method: "POST" }));
		expect(fetchMock).toHaveBeenNthCalledWith(2, "/bff/customer/api-keys/key-a", expect.objectContaining({ method: "DELETE" }));
	});
});
