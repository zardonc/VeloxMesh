import { expect, Page, test } from "playwright/test";
import { execFileSync } from "node:child_process";

function seedRedisDocument(key: string, value: unknown) {
	const container = process.env.E2E_REDIS_CONTAINER;
	expect(container, "E2E_REDIS_CONTAINER must identify the isolated test Redis container").toBeTruthy();
	execFileSync("docker", ["exec", "-i", container!, "redis-cli", "-x", "SET", key], {
		input: JSON.stringify(value),
		stdio: ["pipe", "pipe", "pipe"]
	});
}

function readRedisDocument<T>(key: string): T {
	const container = process.env.E2E_REDIS_CONTAINER;
	expect(container).toBeTruthy();
	const value = execFileSync("docker", ["exec", container!, "redis-cli", "--raw", "GET", key], { encoding: "utf8" });
	return JSON.parse(value) as T;
}

function seedAdminOperationalData() {
	const generatedAt = new Date().toISOString();
	seedRedisDocument("veloxmesh:provider_health", {
		generatedAt,
		providers: [{ provider: "e2e-provider", targetModel: "e2e/model", status: "Healthy", avgLatencyMs: 120, errorRate: 0, timeoutRate: 0, lastChecked: generatedAt }]
	});
	seedRedisDocument("veloxmesh:request_logs", {
		generatedAt,
		logs: [{ requestId: "e2e-admin-request", tenant: "e2e-admin-tenant", provider: "e2e-provider", model: "e2e/model", method: "Our Gateway Method", inputTokens: 10, outputTokens: 20, status: "Success", latencyMs: 120, ttftMs: 35, errorMessage: "", timestamp: generatedAt }]
	});
	seedRedisDocument("veloxmesh:benchmarks", {
		generatedAt,
		benchmarks: [{ runId: "e2e-benchmark-run", method: "Our Gateway Method", dataset: "e2e-dataset", requestCount: 1, concurrency: 1, requestRate: 1, warmUp: 0, repeatedRuns: 1, timeoutSettingSeconds: 30, provider: "e2e-provider", targetModel: "e2e/model", gatewayVersion: "e2e", avgLatencyMs: 120, p50LatencyMs: 120, p95LatencyMs: 120, p99LatencyMs: 120, ttftMs: 35, throughputRps: 1, successRatePct: 100, errorRatePct: 0, timeoutRatePct: 0, improvementPct: 10, testDate: generatedAt, source: "e2e Redis", rawFilePath: "e2e/raw.json", exportId: "e2e-export", status: "passed", partialData: false }]
	});
	seedRedisDocument("veloxmesh:benchmark_requests", {
		generatedAt,
		requests: [{
			runId: "e2e-benchmark-run", requestId: "e2e-benchmark-request", dataset: "e2e-dataset", rowIndex: 0,
			methodId: "gateway", method: "Our Gateway Method", provider: "e2e-provider", model: "e2e/model", modelVersion: "e2e-v1",
			route: "default-provider", startedAt: generatedAt, endedAt: generatedAt, latencyMs: 120, ttftMs: 35,
			inputTokens: 10, outputTokens: 20, totalTokens: 30, status: "success", httpStatus: 200, errorType: "",
			timeout: false, retryCount: 0, cacheHit: false
		}]
	});
}

async function finishVerification(page: Page) {
  const codeText = await page.locator(".dev-code").innerText();
  const code = codeText.match(/\d{6}/)?.[0];
  expect(code).toBeTruthy();
  await page.getByLabel("Verification code").fill(code!);
  await page.getByRole("button", { name: "Verify and sign in" }).click();
}

async function registerCustomer(page: Page, suffix: string) {
  const username = `customer_${suffix}`;
  const password = "DashboardPass1234";
  await page.goto("/");
	await page.getByRole("button", { name: "Create Customer Account" }).click();
  await page.getByLabel("Email").fill(`${username}@example.test`);
  await page.getByLabel("Username").fill(username);
	await page.getByLabel("Organization").fill(`Organization ${suffix}`);
  await page.getByLabel("Password", { exact: true }).fill(password);
	await page.getByLabel("Confirm password").fill(password);
  await page.getByRole("button", { name: "Create account" }).click();
	await finishVerification(page);
	await expect(page.getByRole("heading", { name: "Customer Home" })).toBeVisible();
}

async function loginAdmin(page: Page) {
	const password = process.env.E2E_ADMIN_PASSWORD ?? "E2E-Admin-Password-1234";
	await page.goto("/");
	await page.getByRole("button", { name: "Admin Sign In" }).click();
	await page.getByLabel("Username or email").fill(process.env.E2E_ADMIN_USERNAME ?? "e2e_admin");
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
	await finishVerification(page);
	await expect(page.getByRole("heading", { name: "Admin Home" })).toBeVisible();
}

test("admin sees live operational data and exports benchmark files", async ({ page }) => {
	seedAdminOperationalData();
	await loginAdmin(page);
	const summaryResponse = await page.request.get("/bff/admin/summary");
	expect(summaryResponse.ok()).toBe(true);
	const summaryPayload = await summaryResponse.json();
	await expect(page.locator(".metric-card").filter({ hasText: "Requests Today" }).locator("strong"))
		.toHaveText(summaryPayload.requestVolume === null ? "Unavailable" : Number(summaryPayload.requestVolume).toLocaleString());
	await expect(page.locator(".metric-card").filter({ hasText: "Success Rate" }).locator("strong"))
		.toHaveText(summaryPayload.successRate === null ? "Unavailable" : `${summaryPayload.successRate}%`);
	await expect(page.getByText("Generated", { exact: true })).toBeVisible();
	await expect(page.getByText("Sources", { exact: true })).toBeVisible();
	await Promise.all([
		page.waitForResponse((response) => response.url().includes("/bff/admin/summary") && response.status() === 200),
		page.getByRole("button", { name: "Refresh" }).click()
	]);

  const benchmarksResponse = await page.request.get("/bff/admin/benchmarks");
  expect(benchmarksResponse.ok()).toBe(true);
  const benchmarkPayload = await benchmarksResponse.json();
  const currentRunId = benchmarkPayload.benchmarks?.[0]?.runId as string | undefined;
  const currentProvider = benchmarkPayload.benchmarks?.[0]?.provider as string | undefined;
  expect(currentRunId).toBeTruthy();
  expect(currentProvider).toBeTruthy();

  const logsResponse = await page.request.get("/bff/admin/request-logs");
  expect(logsResponse.ok()).toBe(true);
  const logPayload = await logsResponse.json();
  const currentRequestId = logPayload.logs?.[0]?.requestId as string | undefined;
  expect(currentRequestId).toBeTruthy();

  await page.getByRole("button", { name: "Benchmarks" }).click();
  await expect(page.getByRole("heading", { name: "Four-Method Comparison Readiness" })).toBeVisible();
  await expect(page.getByRole("cell", { name: currentRunId! }).first()).toBeVisible();
  await expect(page.getByText("Improved Model: Missing", { exact: true }).first()).toBeVisible();

  const csvDownload = page.waitForEvent("download");
  await page.getByRole("button", { name: "Export CSV" }).click();
  expect((await csvDownload).suggestedFilename()).toBe("veloxmesh-benchmark-raw-requests.csv");
  const reportDownload = page.waitForEvent("download");
  await page.getByRole("button", { name: "Export Report" }).click();
  expect((await reportDownload).suggestedFilename()).toBe("veloxmesh-benchmark-report.zip");

  await page.getByRole("button", { name: "Provider Health" }).click();
  await expect(page.getByText(currentProvider!, { exact: true }).first()).toBeVisible();
  await page.getByRole("button", { name: "Requests / Logs" }).click();
  await expect(page.getByText(currentRequestId!, { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Benchmarks" }).click();
  const desktopLayout = await page.evaluate(() => {
    const wrapper = document.querySelector(".benchmark-table") as HTMLElement;
    return {
      bodyOverflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
      tableScrollable: wrapper.scrollWidth > wrapper.clientWidth
    };
  });
  expect(desktopLayout.bodyOverflow).toBeLessThanOrEqual(1);
  expect(desktopLayout.tableScrollable).toBe(true);

  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.getByRole("heading", { name: "Benchmark Result Summary" })).toBeVisible();
  const mobileLayout = await page.evaluate(() => ({
    bodyOverflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
    firstCellDisplay: getComputedStyle(document.querySelector(".benchmark-table td")!).display
  }));
  expect(mobileLayout.bodyOverflow).toBeLessThanOrEqual(1);
  expect(mobileLayout.firstCellDisplay).toBe("grid");
});

test("admin system management tabs preserve their deep link", async ({ page }) => {
	await loginAdmin(page);

	const adminNavigation = page.getByRole("navigation", { name: "Admin navigation" });
	await expect(adminNavigation.getByRole("button", { name: "System Management" })).toBeVisible();
	await expect(adminNavigation.getByRole("button", { name: "Routing", exact: true })).toHaveCount(0);
	await expect(adminNavigation.getByRole("button", { name: "Tenants", exact: true })).toHaveCount(0);

	await adminNavigation.getByRole("button", { name: "System Management" }).click();
	await expect(page.getByRole("heading", { name: "System Management", level: 2 })).toBeVisible();
	await expect(page).toHaveURL(/#system-management\/routing$/);

	await page.getByRole("tab", { name: "Audit" }).click();
	await expect(page.getByRole("tab", { name: "Audit" })).toHaveAttribute("aria-selected", "true");
	await expect(page).toHaveURL(/#system-management\/audit$/);

	await page.reload();
	await expect(page.getByRole("tab", { name: "Audit" })).toHaveAttribute("aria-selected", "true");
	await expect(page.getByRole("tabpanel", { name: "Audit" })).toBeVisible();

	for (const viewport of [
		{ width: 1440, height: 900 },
		{ width: 1024, height: 768 },
		{ width: 390, height: 844 }
	]) {
		await page.setViewportSize(viewport);
		const layout = await page.evaluate(() => ({
			bodyOverflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
			tabsScrollable: document.querySelector(".management-tabs")!.scrollWidth >= document.querySelector(".management-tabs")!.clientWidth
		}));
		expect(layout.bodyOverflow).toBeLessThanOrEqual(1);
		expect(layout.tabsScrollable).toBe(true);
	}
});

test("admin manages local routing tenants API keys audit and settings", async ({ page }) => {
	await loginAdmin(page);
	await page.getByRole("navigation", { name: "Admin navigation" }).getByRole("button", { name: "System Management" }).click();
	await expect(page.getByText("VeloxMesh Admin API is not connected", { exact: true })).toBeVisible();

	const suffix = Date.now().toString();
	const policy = `E2E route ${suffix}`;
	await page.getByRole("button", { name: "Add routing rule" }).click();
	await page.getByLabel("Policy").fill(policy);
	await page.getByLabel("Selector").fill("latency-aware");
	await page.getByLabel("Target").fill("e2e-provider");
	await page.getByLabel("Routing status").selectOption("Draft");
	await page.getByRole("button", { name: "Save routing rule" }).click();
	let routingRow = page.getByRole("row", { name: new RegExp(policy) });
	await expect(routingRow).toBeVisible();
	await routingRow.getByRole("button", { name: `Edit ${policy}` }).click();
	await page.getByLabel("Target").fill("e2e-backup-provider");
	await page.getByRole("button", { name: "Save routing rule" }).click();
	routingRow = page.getByRole("row", { name: new RegExp(policy) });
	await expect(routingRow).toContainText("e2e-backup-provider");

	await page.getByRole("tab", { name: "Tenants" }).click();
	const tenant = `e2e-tenant-${suffix}`;
	await page.getByRole("button", { name: "Add tenant" }).click();
	await page.getByLabel("Tenant ID").fill(tenant);
	await page.getByLabel("Owner").fill("E2E Owner");
	await page.getByLabel("Daily quota").fill("2000");
	await page.getByRole("button", { name: "Save tenant" }).click();
	const tenantRow = page.getByRole("row", { name: new RegExp(tenant) });
	await expect(tenantRow).toBeVisible();
	await tenantRow.getByRole("button", { name: `Edit ${tenant}` }).click();
	await page.getByLabel("Tenant status").selectOption("Inactive");
	await page.getByRole("button", { name: "Save tenant" }).click();
	await expect(page.getByRole("row", { name: new RegExp(`${tenant}.*Inactive`) })).toBeVisible();

	await page.getByRole("tab", { name: "API Keys" }).click();
	await page.getByRole("button", { name: "Issue API key" }).click();
	await page.getByLabel("API key tenant").fill(tenant);
	await page.getByLabel("Scope").fill("gateway:invoke");
	await page.getByRole("button", { name: "Create API key" }).click();
	await expect(page.getByText("Copy this key now. It will not be shown again.")).toBeVisible();
	const adminSecret = await page.locator(".management-secret code").innerText();
	expect(adminSecret.startsWith("vx_admin_")).toBe(true);
	await page.getByRole("tab", { name: "Audit" }).click();
	await page.getByRole("tab", { name: "API Keys" }).click();
	await expect(page.getByText(adminSecret, { exact: true })).toHaveCount(0);
	const apiKeyRow = page.getByRole("row", { name: new RegExp(tenant) });
	await expect(apiKeyRow).toContainText("...");
	page.once("dialog", (dialog) => dialog.accept());
	await apiKeyRow.getByRole("button", { name: /Revoke key-/ }).click();
	await expect(page.getByRole("row", { name: new RegExp(tenant) })).toHaveCount(0);

	await page.getByRole("tab", { name: "Audit" }).click();
	await page.getByLabel("Search audit events").fill(tenant);
	await expect(page.getByRole("cell", { name: new RegExp(tenant) }).first()).toBeVisible();
	const auditDownload = page.waitForEvent("download");
	await page.getByRole("button", { name: "Export audit CSV" }).click();
	expect((await auditDownload).suggestedFilename()).toBe("veloxmesh-audit.csv");

	await page.getByRole("tab", { name: "Routing" }).click();
	routingRow = page.getByRole("row", { name: new RegExp(policy) });
	page.once("dialog", (dialog) => dialog.accept());
	await routingRow.getByRole("button", { name: `Delete ${policy}` }).click();
	await expect(page.getByRole("row", { name: new RegExp(policy) })).toHaveCount(0);

	await page.getByRole("tab", { name: "Settings" }).click();
	await expect(page.getByText("Settings are local to the Dashboard BFF", { exact: true })).toBeVisible();
	await page.getByLabel("Default provider").fill("e2e-provider");
	await page.getByLabel("Default model").fill("e2e/model");
	await page.getByLabel("Request timeout seconds").fill("45");
	await page.getByLabel("Data retention days").fill("60");
	await page.getByRole("button", { name: "Save settings" }).click();
	await expect(page.locator(".operation-notice")).toContainText("Settings saved");
	await page.reload();
	await expect(page.getByLabel("Default provider")).toHaveValue("e2e-provider");
	await expect(page.getByLabel("Data retention days")).toHaveValue("60");
});

test("system management exposes loading error retry empty and responsive states", async ({ page }) => {
	await loginAdmin(page);
	let routingAttempts = 0;
	let releaseFailure!: () => void;
	let signalIntercepted!: () => void;
	const failureGate = new Promise<void>((resolve) => { releaseFailure = resolve; });
	const intercepted = new Promise<void>((resolve) => { signalIntercepted = resolve; });
	await page.route("**/bff/admin/routing", async (route) => {
		routingAttempts += 1;
		if (routingAttempts <= 2) {
			signalIntercepted();
			await failureGate;
			await route.fulfill({ status: 500, contentType: "application/json", body: JSON.stringify({ error: "management data unavailable" }) });
			return;
		}
		await route.continue();
	});

	await page.getByRole("navigation", { name: "Admin navigation" }).getByRole("button", { name: "System Management" }).click();
	await intercepted;
	await expect(page.getByText("Loading configuration", { exact: true })).toBeVisible();
	releaseFailure();
	await expect(page.getByRole("alert")).toContainText("management data unavailable");
	await page.getByRole("button", { name: "Retry" }).click();
	await expect(page.getByText("VeloxMesh Admin API is not connected", { exact: true })).toBeVisible();
	await page.route("**/bff/admin/routing", async (route) => {
		if (route.request().method() === "POST") {
			await route.fulfill({ status: 500, contentType: "application/json", body: JSON.stringify({ error: "routing save failed" }) });
			return;
		}
		await route.continue();
	});
	await page.getByRole("button", { name: "Add routing rule" }).click();
	await page.getByLabel("Policy").fill("unsaved-route");
	await page.getByLabel("Selector").fill("latency-aware");
	await page.getByLabel("Target").fill("provider-a");
	await page.getByRole("button", { name: "Save routing rule" }).click();
	await expect(page.getByRole("alert")).toContainText("routing save failed");
	await expect(page.getByLabel("Policy")).toHaveValue("unsaved-route");
	await page.getByRole("button", { name: "Cancel create" }).click();

	await page.getByPlaceholder("Filter rows").fill("no-such-routing-rule");
	await expect(page.getByText("No routing rules", { exact: true })).toBeVisible();

	await page.setViewportSize({ width: 390, height: 844 });
	const mobileLayout = await page.evaluate(() => ({
		bodyOverflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
		panelWidth: document.querySelector(".management-panel")?.getBoundingClientRect().width ?? 0
	}));
	expect(mobileLayout.bodyOverflow).toBeLessThanOrEqual(1);
	expect(mobileLayout.panelWidth).toBeLessThanOrEqual(390);
});

test("customer cannot open admin UI or API", async ({ page }) => {
	await registerCustomer(page, `${Date.now()}`);
	await expect(page.getByText("No requests have been recorded for this tenant.")).toBeVisible();
	await page.getByRole("button", { name: "My Requests" }).click();
	await expect(page.getByRole("heading", { name: "My Requests", level: 2 })).toBeVisible();
	await expect(page.getByLabel("Status")).toBeVisible();
	await expect(page.getByLabel("Model")).toBeVisible();
	await expect(page.getByLabel("From")).toBeVisible();
	await expect(page.getByLabel("To", { exact: true })).toBeVisible();
	await expect(page.getByLabel("Page size")).toHaveValue("25");
	await expect(page.getByRole("button", { name: "Previous page" })).toBeDisabled();
	await expect(page.getByRole("button", { name: "Next page" })).toBeDisabled();
	await expect(page.getByText("0 requests", { exact: true })).toBeVisible();
	await page.getByRole("button", { name: "Usage", exact: true }).click();
	await expect(page.getByRole("button", { name: "Last 24 hours" })).toBeVisible();
	await expect(page.getByRole("button", { name: "Last 7 days" })).toBeVisible();
	await expect(page.getByRole("button", { name: "Last 30 days" })).toBeVisible();
	await expect(page.getByRole("button", { name: "Custom range" })).toBeVisible();
	await expect(page.getByText("No usage data", { exact: true })).toBeVisible();
	await page.getByRole("button", { name: "My API Keys" }).click();
	await page.getByRole("button", { name: "Create API Key" }).click();
	await expect(page.getByText("Copy this key now. It will not be shown again.")).toBeVisible();
	const secret = await page.locator(".secret-callout code").innerText();
	expect(secret.startsWith("vx_live_")).toBe(true);
	await page.getByRole("button", { name: "Dismiss" }).click();
	await expect(page.getByRole("cell", { name: /vx_live_\.\.\./ })).toBeVisible();
	await expect(page.getByText(secret, { exact: true })).toHaveCount(0);
  await page.goto("/#benchmarks");
  await expect(page.getByText("No permission", { exact: true })).toBeVisible();
  const response = await page.request.get("/bff/admin/benchmarks");
  expect(response.status()).toBe(403);
});

test("Customer A and B remain isolated across UI, API, and API keys", async ({ browser, request }, testInfo) => {
	const baseURL = String(testInfo.project.use.baseURL);
	const contextA = await browser.newContext({ baseURL });
	const contextB = await browser.newContext({ baseURL });
	const pageA = await contextA.newPage();
	const pageB = await contextB.newPage();
	try {
		await registerCustomer(pageA, `a_${Date.now()}`);
		await registerCustomer(pageB, `b_${Date.now()}`);
		const sessionA = await (await contextA.request.get("/bff/session")).json() as { tenantId: string };
		const sessionB = await (await contextB.request.get("/bff/session")).json() as { tenantId: string };
		expect(sessionA.tenantId).toBeTruthy();
		expect(sessionB.tenantId).toBeTruthy();
		expect(sessionA.tenantId).not.toBe(sessionB.tenantId);

		const timestamp = new Date().toISOString();
		seedRedisDocument("veloxmesh:request_logs", {
			generatedAt: timestamp,
			logs: [
				{ requestId: "tenant-a-request", tenant: sessionA.tenantId, provider: "provider-a", model: "model-a", method: "Our Gateway Method", inputTokens: 11, outputTokens: 21, status: "Success", latencyMs: 101, ttftMs: 31, errorMessage: "", timestamp },
				{ requestId: "tenant-b-request", tenant: sessionB.tenantId, provider: "provider-b", model: "model-b", method: "Our Gateway Method", inputTokens: 12, outputTokens: 22, status: "Timeout", latencyMs: 502, ttftMs: 302, errorMessage: "upstream timeout", timestamp }
			]
		});
		const storedLogs = readRedisDocument<{ logs: Array<{ requestId: string; tenant: string }> }>("veloxmesh:request_logs");
		expect(storedLogs.logs).toEqual(expect.arrayContaining([
			expect.objectContaining({ requestId: "tenant-a-request", tenant: sessionA.tenantId }),
			expect.objectContaining({ requestId: "tenant-b-request", tenant: sessionB.tenantId })
		]));

		const injectedA = await contextA.request.get(`/bff/customer/requests?tenant_id=${encodeURIComponent(sessionB.tenantId)}`, {
			headers: { "X-Tenant-ID": sessionB.tenantId }
		});
		expect(injectedA.ok()).toBe(true);
		const injectedPayload = await injectedA.json();
		expect(injectedPayload.tenantId).toBe(sessionA.tenantId);
		expect(injectedPayload.requests.map((row: { requestId: string }) => row.requestId), JSON.stringify(injectedPayload)).toEqual(["tenant-a-request"]);

		await pageA.goto("/#customer-requests");
		await pageA.getByRole("button", { name: "Refresh" }).click();
		await expect(pageA.getByText("tenant-a-request", { exact: true })).toBeVisible();
		await expect(pageA.getByText("tenant-b-request", { exact: true })).toHaveCount(0);
		await pageB.goto("/#customer-requests");
		await pageB.getByRole("button", { name: "Refresh" }).click();
		await expect(pageB.getByText("tenant-b-request", { exact: true })).toBeVisible();
		await expect(pageB.getByText("tenant-a-request", { exact: true })).toHaveCount(0);

		await pageA.getByRole("button", { name: "Usage", exact: true }).click();
		await expect(pageA.getByText("model-a", { exact: true })).toBeVisible();
		await expect(pageA.getByText("model-b", { exact: true })).toHaveCount(0);
		await pageB.getByRole("button", { name: "Usage", exact: true }).click();
		await expect(pageB.getByText("model-b", { exact: true })).toBeVisible();
		await expect(pageB.getByText("model-a", { exact: true })).toHaveCount(0);

		const createdAResponse = await contextA.request.post("/bff/customer/api-keys", {
			data: { scope: "gateway:invoke", tenantId: sessionB.tenantId }
		});
		expect(createdAResponse.status()).toBe(201);
		const keyA = await createdAResponse.json() as { id: string; key: string; maskedKey: string };
		const listA = await (await contextA.request.get("/bff/customer/api-keys")).json();
		const listB = await (await contextB.request.get("/bff/customer/api-keys")).json();
		expect(listA.tenantId).toBe(sessionA.tenantId);
		expect(listA.keys.map((key: { id: string }) => key.id)).toContain(keyA.id);
		expect(JSON.stringify(listA)).not.toContain(keyA.key);
		expect(listB.keys.map((key: { id: string }) => key.id)).not.toContain(keyA.id);
		expect((await contextB.request.delete(`/bff/customer/api-keys/${keyA.id}`)).status()).toBe(404);

		expect((await request.get("/bff/customer/summary")).status()).toBe(401);
		expect((await contextA.request.get("/bff/admin/benchmarks")).status()).toBe(403);
	} finally {
		await contextA.close();
		await contextB.close();
	}
});

test("Customer states, responsive layouts, refresh, and logout meet acceptance criteria", async ({ page, request }, testInfo) => {
	await registerCustomer(page, `acceptance_${Date.now()}`);
	const session = await (await page.request.get("/bff/session")).json() as { tenantId: string };
	const sessionCookie = (await page.context().cookies()).find((cookie) => cookie.name === "veloxmesh_session");
	expect(sessionCookie).toBeTruthy();

	let releaseSummaryRequest!: () => void;
	let markSummaryIntercepted!: () => void;
	const summaryRequestGate = new Promise<void>((resolve) => {
		releaseSummaryRequest = resolve;
	});
	const summaryIntercepted = new Promise<void>((resolve) => {
		markSummaryIntercepted = resolve;
	});
	await page.route("**/bff/customer/summary", async (route) => {
		markSummaryIntercepted();
		await summaryRequestGate;
		await route.continue();
	}, { times: 1 });
	const delayedSummary = page.waitForResponse((response) => response.url().includes("/bff/customer/summary") && response.status() === 200);
	await page.getByRole("button", { name: "Refresh" }).click();
	await summaryIntercepted;
	await expect(page.locator("main.workspace[aria-busy='true']")).toBeVisible();
	releaseSummaryRequest();
	await delayedSummary;
	await page.unroute("**/bff/customer/summary");
	await expect(page.getByRole("heading", { name: "Customer Home" })).toBeVisible();

	const timestamp = new Date().toISOString();
	seedRedisDocument("veloxmesh:request_logs", {
		generatedAt: timestamp,
		logs: [{ requestId: "acceptance-request", tenant: session.tenantId, provider: "acceptance-provider", model: "acceptance/model-with-a-long-name", method: "Our Gateway Method", inputTokens: 100, outputTokens: 50, status: "Success", latencyMs: 150, ttftMs: 45, errorMessage: "", timestamp }]
	});
	await page.getByRole("button", { name: "Refresh" }).click();
	await expect(page.getByText("acceptance-request", { exact: true })).toBeVisible();

	await page.route("**/bff/customer/usage**", async (route) => {
		const response = await route.fetch();
		const payload = await response.json();
		payload.partialData = true;
		payload.summary.partialData = true;
		await route.fulfill({ response, json: payload });
	});
	await page.getByRole("button", { name: "Usage", exact: true }).click();
	await expect(page.getByText(/Partial data:/)).toBeVisible();
	await page.unroute("**/bff/customer/usage**");

	await page.route("**/bff/customer/usage**", async (route) => {
		await route.fulfill({ status: 500, contentType: "application/json", body: JSON.stringify({ error: "simulated usage failure" }) });
	});
	await expect(page.getByRole("button", { name: "Last 24 hours" })).toBeEnabled();
	await page.getByRole("button", { name: "Last 24 hours" }).click();
	await expect(page.getByRole("alert")).toContainText("simulated usage failure");
	await page.unroute("**/bff/customer/usage**");

	await page.getByRole("button", { name: "My Requests" }).click();
	await page.getByRole("button", { name: "Refresh" }).click();
	await expect(page.getByText("acceptance-request", { exact: true })).toBeVisible();
	for (const viewport of [
		{ width: 1440, height: 900, name: "desktop" },
		{ width: 1024, height: 768, name: "tablet" },
		{ width: 390, height: 844, name: "mobile" }
	]) {
		await page.setViewportSize({ width: viewport.width, height: viewport.height });
		await expect(page.getByRole("button", { name: "Clear filters" })).toBeVisible();
		const layout = await page.evaluate(() => ({
			bodyOverflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
			buttonFits: Array.from(document.querySelectorAll("button")).every((button) => button.scrollWidth <= button.clientWidth + 1)
		}));
		expect(layout.bodyOverflow).toBeLessThanOrEqual(1);
		expect(layout.buttonFits).toBe(true);
		const screenshotPath = testInfo.outputPath(`customer-requests-${viewport.name}-${viewport.width}x${viewport.height}.png`);
		await page.screenshot({ path: screenshotPath, fullPage: true });
		await testInfo.attach(`Customer Requests ${viewport.name}`, { path: screenshotPath, contentType: "image/png" });
	}

	await page.reload();
	await expect(page.getByRole("heading", { name: "My Requests", level: 1 })).toBeVisible();
	await expect(page.getByText("acceptance-request", { exact: true })).toBeVisible();
	await page.getByRole("button", { name: "Sign out" }).click();
	await expect(page.getByRole("heading", { name: "Customer sign in" })).toBeVisible();
	const oldSession = await request.get("/bff/session", {
		headers: { Cookie: `${sessionCookie!.name}=${sessionCookie!.value}` }
	});
	expect(oldSession.status()).toBe(401);
});
