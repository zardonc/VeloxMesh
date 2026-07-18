export type SummaryResponse = {
  defaultProvider: string;
  defaultModel: string;
  modelCount: number | null;
  activeProviders: number | null;
  activeTenants: number | null;
  requestVolume: number | null;
  avgLatencyMs: number | null;
  successRate: number | null;
  errorRate: number | null;
  timeoutRate: number | null;
  p95LatencyMs: number | null;
  queueDepth: number | null;
	gatewayStatus: "Healthy" | "Partial" | "Error";
	routingStrategy: string;
	topology: {
		node_id: string;
		role: string;
		leader_id: string;
		writable: boolean;
		wal_lag_elapsed: number;
		wal_lag_pending: number;
		degraded_reason?: string;
	} | null;
	latestBenchmark: BenchmarkRun | null;
	providerHealth: ProviderHealth[];
	recentErrors: RequestLogsResponse["logs"];
	generatedAt: string;
	dataSources: SummaryDataSource[];
	partial: boolean;
	partialData: boolean;
	warnings: string[];
};

export type SummaryDataSource = {
	name: string;
	source: string;
	status: "ok" | "empty" | "error";
	detail?: string;
	generatedAt?: string;
};

export type ProviderResponse = {
  providers: Array<{
    name: string;
    baseUrl: string;
    defaultModel: string;
    models: string[];
    status: string;
    p95LatencyMs: number;
    successRate: number;
    requestsToday: number;
		application?: ConfigurationApplication;
  }>;
};

export type ConfigurationApplication = {
	state: "applied" | "verified" | "warning" | "failed";
	applied: boolean;
	verified: boolean;
	revision: number;
	requestId?: string;
	providerId?: string;
	route?: string;
	message?: string;
};

export type RequestsResponse = {
  requests: Array<{
    id: string;
    tenant: string;
    provider: string;
    model: string;
    status: string;
    latencyMs: number;
    route: string;
  }>;
};

export type ManagementMetadata = {
  source?: string;
  partialData?: boolean;
  warnings?: string[];
};

export type RoutingResponse = ManagementMetadata & {
	singleton?: boolean;
	revision?: number;
  rules: Array<{
    policy: string;
    selector: string;
    target: string;
    status: string;
		revision?: number;
		application?: ConfigurationApplication;
  }>;
};

export type TenantsResponse = ManagementMetadata & {
  tenants: Array<{
    tenant: string;
    owner: string;
    dailyQuota: string;
    status: string;
		revision?: number;
  }>;
};

export type ApiKeysResponse = ManagementMetadata & {
  keys: Array<{
    id?: string;
    key: string;
    tenant: string;
    scope: string;
    status?: string;
    createdAt?: string;
    lastUsed: string;
  }>;
};

export type AuditResponse = ManagementMetadata & {
  events: Array<{
    time: string;
    actor: string;
    action: string;
    result: string;
  }>;
};

export type AdminSettings = {
  defaultProvider: string;
  defaultModel: string;
  requestTimeoutSeconds: number;
  dataRetentionDays: number;
	revision?: number;
};

export type AdminSettingsResponse = ManagementMetadata & {
  settings: AdminSettings;
  integrations: {
    gateway: string;
    redis: string;
    qdrant: string;
    smtp: string;
  };
};

export type CreatedAdminApiKey = {
  id: string;
  key: string;
  maskedKey: string;
  tenant: string;
  scope: string;
  status: string;
  createdAt: string;
};

export type RequestLogsResponse = {
	 source?: string;
  logs: Array<{
    id?: string;
    requestId?: string;
    tenant: string;
    provider: string;
    model: string;
    method?: string;
    inputTokens: number;
    outputTokens: number;
    status: string;
    latencyMs: number;
    ttftMs?: number;
    error?: string;
    errorMessage?: string;
    timestamp?: string;
  }>;
};

export type ProviderHealthResponse = {
  source?: string;
  providers: ProviderHealth[];
};

export type BenchmarksResponse = {
  source?: string;
  generatedAt?: string;
  storage?: {
    redis: {
      status: string;
      detail: string;
    };
    qdrant: {
      status: string;
      detail: string;
    };
  };
  benchmarks: BenchmarkRun[];
};

export type SessionResponse = {
  user: string;
  userId?: string;
  tenantId?: string;
  role: string;
  scopes: string[];
};

export type RegisterResponse = {
  status: string;
  message: string;
  user: string;
  role: "Customer";
  tenantId: string;
  scopes: string[];
  verificationRequired: boolean;
  challengeId: string;
  delivery: string;
  devCode?: string;
};

export type RegisterInput = {
  email: string;
  username: string;
  password: string;
  role: "Admin" | "Customer";
};

export type CustomerRegisterInput = {
  email: string;
  username: string;
  organization: string;
  password: string;
  confirmPassword: string;
};

export type LoginInput = {
  identifier: string;
  password: string;
};

export type LoginChallengeResponse = {
  verificationRequired: boolean;
  challengeId: string;
  delivery: string;
  message: string;
  devCode?: string;
};

export type VerifyLoginInput = {
  challengeId: string;
  code: string;
};

export type DashboardPayload = {
  summary: SummaryResponse;
  providers: ProviderResponse;
  requests: RequestsResponse;
  routing: RoutingResponse;
  tenants: TenantsResponse;
  apiKeys: ApiKeysResponse;
  audit: AuditResponse;
  requestLogs: RequestLogsResponse;
  benchmarks: BenchmarksResponse;
  session: SessionResponse;
};

export type InitialAppData =
  | { kind: "admin"; payload: DashboardPayload }
  | { kind: "customer"; session: SessionResponse };

export type DashboardViewModel = {
  kpis: Array<{
    label: string;
    value: string;
    detail: string;
    tone: "blue" | "green" | "amber" | "slate";
  }>;
  providers: Array<{
    name: string;
    baseUrl: string;
    defaultModel: string;
    modelCount: number;
    status: string;
    p95LatencyMs: number;
    successRate: string;
    requestsToday: string;
  }>;
  recentRequests: Array<{
    id: string;
    tenant: string;
    provider: string;
    model: string;
    status: string;
    statusLabel: string;
    latency: string;
    route: string;
  }>;
  updatedAt: string;
};

export type ProvidersPageViewModel = {
  title: string;
  actionLabel: string;
  rows: Array<{
    name: string;
    baseUrl: string;
    defaultModel: string;
    status: string;
    modelCount: number;
    modelsText: string;
    p95Latency: string;
    successRate: string;
    traffic: string;
  }>;
};

export type ManagementPageViewModel = {
  title: string;
  description: string;
  actionLabel: string;
  metrics: Array<{
    label: string;
    value: string;
    detail: string;
  }>;
  columns: string[];
  rows: Array<Record<string, string>>;
};

export async function fetchDashboardPayload(): Promise<DashboardPayload> {
  const [summary, providers, requests, routing, tenants, apiKeys, audit, requestLogs, benchmarks, session] = await Promise.all([
    getJSON<SummaryResponse>("/bff/admin/summary"),
    getJSON<ProviderResponse>("/bff/admin/providers"),
    getJSON<RequestsResponse>("/bff/admin/requests"),
    getJSON<RoutingResponse>("/bff/admin/routing"),
    getJSON<TenantsResponse>("/bff/admin/tenants"),
    getJSON<ApiKeysResponse>("/bff/admin/api-keys"),
    getJSON<AuditResponse>("/bff/admin/audit"),
    getJSON<RequestLogsResponse>("/bff/admin/request-logs"),
    getJSON<BenchmarksResponse>("/bff/admin/benchmarks"),
    getJSON<SessionResponse>("/bff/session")
  ]);

  return { summary, providers, requests, routing, tenants, apiKeys, audit, requestLogs, benchmarks, session };
}

export async function fetchInitialAppData(): Promise<InitialAppData> {
  const session = await fetchSession();
  if (session.role === "Customer") {
    return { kind: "customer", session };
  }
  const payload = await fetchDashboardPayload();
  return { kind: "admin", payload };
}

export function buildDashboardViewModel(payload: DashboardPayload): DashboardViewModel {
  const { summary } = payload;

  return {
    kpis: [
      {
        label: "Requests",
        value: formatLiveNumber(summary.requestVolume),
        detail: "Today across gateway routes",
        tone: "blue"
      },
      {
        label: "Success Rate",
        value: formatLivePercent(summary.successRate),
        detail: "Provider completion health",
        tone: "green"
      },
      {
        label: "P95 Latency",
        value: formatLiveMilliseconds(summary.p95LatencyMs),
        detail: "End-to-end request latency",
        tone: "amber"
      },
      {
        label: "Queue Depth",
        value: formatLiveNumber(summary.queueDepth),
		detail: summary.activeTenants === null ? "Active tenants unavailable" : `${summary.activeTenants} active tenants`,
        tone: "slate"
      }
    ],
    providers: payload.providers.providers.map((provider) => ({
      name: provider.name,
      baseUrl: provider.baseUrl,
      defaultModel: provider.defaultModel,
      modelCount: provider.models.length,
      status: titleCase(provider.status),
      p95LatencyMs: provider.p95LatencyMs,
      successRate: `${provider.successRate.toFixed(1)}%`,
      requestsToday: provider.requestsToday.toLocaleString()
    })),
    recentRequests: payload.requests.requests.map((request) => ({
      id: request.id,
      tenant: request.tenant,
      provider: request.provider,
      model: request.model,
      status: request.status,
      statusLabel: titleCase(request.status.replace("_", " ")),
      latency: request.latencyMs > 0 ? `${request.latencyMs} ms` : "Blocked",
      route: request.route
    })),
	updatedAt: new Date(summary.generatedAt).toLocaleString()
  };
}

function formatLiveNumber(value: number | null): string {
	return value === null ? "Unavailable" : value.toLocaleString();
}

function formatLivePercent(value: number | null): string {
	return value === null ? "Unavailable" : `${value}%`;
}

function formatLiveMilliseconds(value: number | null): string {
	return value === null ? "Unavailable" : `${value} ms`;
}

export function buildProvidersPageViewModel(payload: ProviderResponse): ProvidersPageViewModel {
  return {
    title: "Providers",
    actionLabel: "Add Provider",
    rows: payload.providers.map((provider) => ({
      name: provider.name,
      baseUrl: provider.baseUrl,
      defaultModel: provider.defaultModel,
      status: titleCase(provider.status),
      modelCount: provider.models.length,
      modelsText: provider.models.join(","),
      p95Latency: `${provider.p95LatencyMs} ms`,
      successRate: `${provider.successRate.toFixed(1)}%`,
      traffic: provider.requestsToday.toLocaleString()
    }))
  };
}

export function buildRoutingPageViewModel(payload: RoutingResponse = defaultRouting()): ManagementPageViewModel {
  return {
    title: "Routing",
    description: "Manage route selection, fallback order, and admission controls",
    actionLabel: "New Rule",
    metrics: [
      { label: "Active Rules", value: "3", detail: "Latency, quality, quota" },
      { label: "Fallbacks", value: "1", detail: "Enabled for provider drift" },
      { label: "Queue Guard", value: "17", detail: "Current request backlog" }
    ],
    columns: ["Policy", "Selector", "Target", "Status"],
    rows: payload.rules.map((rule) => ({
      Policy: rule.policy,
      Selector: rule.selector,
      Target: rule.target,
      Status: rule.status
    }))
  };
}

export function buildTenantsPageViewModel(payload: TenantsResponse = defaultTenants()): ManagementPageViewModel {
  return {
    title: "Tenants",
    description: "Review tenant quotas, traffic ownership, and access boundaries",
    actionLabel: "Add Tenant",
    metrics: [
      { label: "Active Tenants", value: "4", detail: "Using shared gateway" },
      { label: "Quota Alerts", value: "1", detail: "Ops sandbox throttled" },
      { label: "Owner Teams", value: "3", detail: "Coursework, capstone, ops" }
    ],
    columns: ["Tenant", "Owner", "Daily Quota", "Status"],
    rows: payload.tenants.map((tenant) => ({
      Tenant: tenant.tenant,
      Owner: tenant.owner,
      "Daily Quota": tenant.dailyQuota,
      Status: tenant.status
    }))
  };
}

export function buildApiKeysPageViewModel(payload: ApiKeysResponse = defaultApiKeys()): ManagementPageViewModel {
  return {
    title: "API Keys",
    description: "Inspect key scopes, last-used signals, and rotation state",
    actionLabel: "Issue Key",
    metrics: [
      { label: "Active Keys", value: "6", detail: "Across all tenants" },
      { label: "Expiring Soon", value: "1", detail: "Rotate within 7 days" },
      { label: "Admin Keys", value: "2", detail: "Privileged scopes" }
    ],
    columns: ["Key", "Tenant", "Scope", "Last Used"],
    rows: payload.keys.map((key) => ({
      Key: key.key,
      Tenant: key.tenant,
      Scope: key.scope,
      "Last Used": key.lastUsed
    }))
  };
}

export function buildAuditPageViewModel(payload: AuditResponse = defaultAudit()): ManagementPageViewModel {
  return {
    title: "Audit",
    description: "Track administrative changes and request-level decisions",
    actionLabel: "Export CSV",
    metrics: [
      { label: "Events Today", value: "24", detail: "Config and routing changes" },
      { label: "Policy Updates", value: "3", detail: "Reviewed by admin" },
      { label: "Blocked Calls", value: "1", detail: "Tenant quota enforcement" }
    ],
    columns: ["Time", "Actor", "Action", "Result"],
    rows: payload.events.map((event) => ({
      Time: event.time,
      Actor: event.actor,
      Action: event.action,
      Result: event.result
    }))
  };
}

export function buildRequestLogsPageViewModel(payload: RequestLogsResponse = defaultRequestLogs()): ManagementPageViewModel {
  const blocked = payload.logs.filter((log) => log.status.toLowerCase() !== "success").length;
  return {
    title: "Requests",
    description: "Inspect gateway request flow, token usage, latency, and failures",
    actionLabel: "Refresh",
    metrics: [
      { label: "Rows", value: payload.logs.length.toLocaleString(), detail: "Recent gateway requests" },
      { label: "Blocked", value: blocked.toLocaleString(), detail: "Quota or policy outcomes" },
      { label: "Token Columns", value: "2", detail: "Input and output tokens" }
    ],
    columns: ["Request", "Tenant", "Provider", "Model", "Input Tokens", "Output Tokens", "Status", "Latency", "Error"],
    rows: payload.logs.map((log) => ({
      Request: log.requestId ?? log.id ?? "unknown",
      Tenant: log.tenant,
      Provider: log.provider,
      Model: log.model,
      "Input Tokens": log.inputTokens.toLocaleString(),
      "Output Tokens": log.outputTokens.toLocaleString(),
      Status: titleCase(log.status.replace("_", " ")),
      Latency: log.latencyMs > 0 ? `${log.latencyMs} ms` : "Blocked",
      Error: log.errorMessage ?? log.error ?? "-"
    }))
  };
}

export function buildBenchmarksPageViewModel(payload: BenchmarksResponse = defaultBenchmarks()): ManagementPageViewModel {
  const source = payload.source ?? "fallback";
  const redis = payload.storage?.redis ?? { status: "unknown", detail: "No Redis status reported" };
  const qdrant = payload.storage?.qdrant ?? { status: "unknown", detail: "No Qdrant status reported" };
  return {
    title: "Benchmarks",
    description: "Compare scheduler behavior across representative gateway workloads",
    actionLabel: "Refresh",
    metrics: [
      {
        label: "Source",
        value: titleCase(source),
        detail: `${payload.benchmarks.length.toLocaleString()} evaluation scenarios`
      },
      { label: "Redis", value: titleCase(redis.status), detail: redis.detail },
      { label: "Qdrant", value: titleCase(qdrant.status), detail: qdrant.detail }
    ],
    columns: ["Run ID", "Method", "Dataset", "P95 Latency", "Success Rate", "Status"],
    rows: payload.benchmarks.map((benchmark) => ({
      "Run ID": benchmark.runId,
      Method: benchmark.method,
      Dataset: benchmark.dataset,
      "P95 Latency": formatMetric(benchmark.p95LatencyMs, "ms"),
      "Success Rate": `${benchmark.successRatePct}%`,
      Status: benchmark.status
    }))
  };
}

export function filterManagementRows(rows: Array<Record<string, string>>, query: string): Array<Record<string, string>> {
  const normalized = query.trim().toLowerCase();
  if (normalized === "") {
    return rows;
  }
  return rows.filter((row) =>
    Object.values(row).some((value) => value.toLowerCase().includes(normalized))
  );
}

export type ProviderInput = {
  name: string;
  baseUrl: string;
  defaultModel: string;
  models: string[];
};

export type ProviderMutationResult = ProviderInput & {
	status: string;
	application?: ConfigurationApplication;
};

export type RoutingMutationResult = RoutingInput & {
	revision?: number;
	application?: ConfigurationApplication;
};

export type ProviderUpdateInput = {
  baseUrl: string;
  defaultModel: string;
  models: string[];
  status: string;
};

export type RoutingInput = {
  policy: string;
  selector: string;
  target: string;
  status: string;
	revision?: number;
};

export type TenantInput = {
  tenant: string;
  owner: string;
  dailyQuota: string;
  status: string;
	revision?: number;
};

export type ApiKeyInput = {
  tenant: string;
  scope: string;
};

export async function fetchAdminRouting(): Promise<RoutingResponse> {
  return getJSON<RoutingResponse>("/bff/admin/routing");
}

export async function fetchAdminTenants(): Promise<TenantsResponse> {
  return getJSON<TenantsResponse>("/bff/admin/tenants");
}

export async function fetchAdminApiKeys(): Promise<ApiKeysResponse> {
  return getJSON<ApiKeysResponse>("/bff/admin/api-keys");
}

export async function fetchAdminAudit(): Promise<AuditResponse> {
  return getJSON<AuditResponse>("/bff/admin/audit");
}

export async function fetchAdminSettings(): Promise<AdminSettingsResponse> {
  return getJSON<AdminSettingsResponse>("/bff/admin/settings");
}

export async function updateAdminSettings(input: AdminSettings): Promise<AdminSettingsResponse> {
  return putJSONResponse<AdminSettingsResponse>("/bff/admin/settings", input);
}

export async function createProvider(input: ProviderInput): Promise<ProviderMutationResult> {
	return postJSONResponse<ProviderMutationResult>("/bff/admin/providers", input);
}

export async function updateProvider(name: string, input: ProviderUpdateInput): Promise<ProviderMutationResult> {
	return putJSONResponse<ProviderMutationResult>(`/bff/admin/providers/${encodeURIComponent(name)}`, input);
}

export async function deleteProvider(name: string): Promise<void> {
  await deleteJSON(`/bff/admin/providers/${encodeURIComponent(name)}`);
}

export async function createRoutingRule(input: RoutingInput): Promise<RoutingMutationResult> {
	return postJSONResponse<RoutingMutationResult>("/bff/admin/routing", input);
}

export async function updateRoutingRule(policy: string, input: RoutingInput): Promise<RoutingMutationResult> {
	return putJSONResponse<RoutingMutationResult>(`/bff/admin/routing/${encodeURIComponent(policy)}`, input);
}

export async function deleteRoutingRule(policy: string): Promise<void> {
  await deleteJSON(`/bff/admin/routing/${encodeURIComponent(policy)}`);
}

export async function createTenant(input: TenantInput): Promise<void> {
  await postJSON("/bff/admin/tenants", input);
}

export async function updateTenant(tenant: string, input: TenantInput): Promise<void> {
  await putJSON(`/bff/admin/tenants/${encodeURIComponent(tenant)}`, input);
}

export async function deleteTenant(tenant: string): Promise<void> {
  await deleteJSON(`/bff/admin/tenants/${encodeURIComponent(tenant)}`);
}

export async function createApiKey(input: ApiKeyInput): Promise<CreatedAdminApiKey> {
  return postJSONResponse<CreatedAdminApiKey>("/bff/admin/api-keys", input);
}

export async function deleteApiKey(key: string): Promise<void> {
  await deleteJSON(`/bff/admin/api-keys/${encodeURIComponent(key)}`);
}

export async function fetchSession(): Promise<SessionResponse> {
  return getJSON<SessionResponse>("/bff/session");
}

export async function registerAccount(input: RegisterInput): Promise<RegisterResponse> {
  return postJSONResponse<RegisterResponse>("/bff/auth/register", input);
}

export async function registerCustomerAccount(input: CustomerRegisterInput): Promise<RegisterResponse> {
  return postJSONResponse<RegisterResponse>("/bff/auth/customer/register", input);
}

export async function loginAccount(input: LoginInput): Promise<LoginChallengeResponse> {
  return postJSONResponse<LoginChallengeResponse>("/bff/auth/login", input);
}

export async function verifyLoginCode(input: VerifyLoginInput): Promise<SessionResponse> {
  return postJSONResponse<SessionResponse>("/bff/auth/verify", input);
}

export async function logoutAccount(): Promise<void> {
  await postJSON("/bff/auth/logout", {});
}

export async function exportAuditCSV(): Promise<string> {
  const response = await fetch("/bff/admin/audit.csv");
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response));
  }
  return response.text();
}

export type DownloadedArtifact = {
	blob: Blob;
	filename: string;
};

export async function fetchBenchmarkRawCSVExport(): Promise<DownloadedArtifact> {
	return fetchDownloadArtifact("/bff/admin/benchmarks/raw.csv", "veloxmesh-benchmark-raw-requests.csv");
}

export async function fetchBenchmarkReportZIPExport(): Promise<DownloadedArtifact> {
	return fetchDownloadArtifact("/bff/admin/benchmarks/export.zip", "veloxmesh-benchmark-report.zip");
}

async function fetchDownloadArtifact(path: string, fallbackFilename: string): Promise<DownloadedArtifact> {
	const response = await fetch(path, { credentials: "same-origin" });
	if (!response.ok) {
		throw new Error(await responseErrorMessage(response));
	}
	return {
		blob: await response.blob(),
		filename: attachmentFilename(response.headers.get("Content-Disposition"), fallbackFilename)
	};
}

function attachmentFilename(contentDisposition: string | null, fallback: string): string {
	const match = contentDisposition?.match(/filename\*?=(?:UTF-8''|\")?([^\";]+)/i);
	const candidate = match?.[1]?.trim();
	if (!candidate) {
		return fallback;
	}
	const decoded = decodeURIComponent(candidate);
	const basename = decoded.split(/[\\/]/).pop()?.replace(/[^A-Za-z0-9._-]/g, "-");
	return basename || fallback;
}

async function getJSON<T>(path: string): Promise<T> {
  const response = await fetch(path);
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response));
  }
  return response.json() as Promise<T>;
}

async function postJSON(path: string, value: unknown): Promise<void> {
  const response = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(value)
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response));
  }
}

async function postJSONResponse<T>(path: string, value: unknown): Promise<T> {
  const response = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(value)
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response));
  }
  return response.json() as Promise<T>;
}

async function putJSON(path: string, value: unknown): Promise<void> {
  const response = await fetch(path, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(value)
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response));
  }
}

async function putJSONResponse<T>(path: string, value: unknown): Promise<T> {
  const response = await fetch(path, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(value)
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response));
  }
  return response.json() as Promise<T>;
}

async function deleteJSON(path: string): Promise<void> {
  const response = await fetch(path, {
    method: "DELETE"
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response));
  }
}

async function responseErrorMessage(response: Response): Promise<string> {
  try {
    const body = await response.json() as { error?: string };
    if (body.error) {
      return body.error;
    }
  } catch {
    // Fall through to the generic HTTP message.
  }
  return `Request failed: ${response.status} ${response.statusText}`;
}

function titleCase(value: string): string {
  return value
    .split(" ")
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function defaultRouting(): RoutingResponse {
  return {
    rules: [
      { policy: "Primary route", selector: "latency-aware", target: "sans-primary", status: "Active" },
      { policy: "Fallback", selector: "quality-fallback", target: "sans-primary", status: "Active" },
      { policy: "Admission control", selector: "tenant-quota", target: "shared queue", status: "Enforced" }
    ]
  };
}

function defaultTenants(): TenantsResponse {
  return {
    tenants: [
      { tenant: "coursework-lab", owner: "Evaluation", dailyQuota: "8,000", status: "Healthy" },
      { tenant: "capstone-demo", owner: "Demo", dailyQuota: "5,000", status: "Healthy" },
      { tenant: "ops-sandbox", owner: "Operations", dailyQuota: "1,000", status: "Rate Limited" }
    ]
  };
}

function defaultApiKeys(): ApiKeysResponse {
  return {
    keys: [
      { key: "vx-dev", tenant: "capstone-demo", scope: "admin:read", lastUsed: "just now" },
      { key: "vx-coursework", tenant: "coursework-lab", scope: "gateway:invoke", lastUsed: "12 min ago" },
      { key: "vx-ops", tenant: "ops-sandbox", scope: "admin:write", lastUsed: "1 hour ago" }
    ]
  };
}

function defaultAudit(): AuditResponse {
  return {
    events: [
      { time: "20:26", actor: "admin", action: "Refreshed provider health", result: "Success" },
      { time: "20:18", actor: "gateway", action: "Applied tenant quota", result: "Rate Limited" },
      { time: "20:12", actor: "admin", action: "Viewed routing policy", result: "Success" }
    ]
  };
}

function defaultRequestLogs(): RequestLogsResponse {
  return {
    logs: [
      {
        id: "req_10291",
        tenant: "coursework-lab",
        provider: "sans-primary",
        model: "oc/deepseek-v4-flash-free",
        inputTokens: 812,
        outputTokens: 248,
        status: "success",
        latencyMs: 714,
        error: ""
      }
    ]
  };
}

function defaultBenchmarks(): BenchmarksResponse {
  return {
    source: "empty",
    benchmarks: []
  };
}

export type UserRole = "Admin" | "Customer";

export type MvpView =
  | "admin-home"
  | "system-management"
  | "benchmarks"
  | "provider-health"
  | "request-logs"
  | "customer-home"
  | "customer-usage"
  | "customer-requests"
  | "customer-api-keys"
  | "customer-account";

export type NavigationItem = {
  label: string;
  view: MvpView;
};

export type SystemManagementTab = "routing" | "tenants" | "api-keys" | "audit" | "settings";

export const SYSTEM_MANAGEMENT_TABS: Array<{ id: SystemManagementTab; label: string }> = [
  { id: "routing", label: "Routing" },
  { id: "tenants", label: "Tenants" },
  { id: "api-keys", label: "API Keys" },
  { id: "audit", label: "Audit" },
  { id: "settings", label: "Settings" }
];

export type DashboardLocation = {
  view: MvpView;
  managementTab?: SystemManagementTab;
};

export type MvpSession = {
  user: string;
  userId: string;
  tenantId: string;
  role: UserRole;
  apiKey: string;
};

export function mvpSessionFromBff(session: SessionResponse): MvpSession {
  if (session.role !== "Admin" && session.role !== "Customer") {
    throw new Error(`Unsupported account role: ${session.role}`);
  }
  return {
    user: session.user,
    userId: session.userId ?? "",
    tenantId: session.tenantId ?? "",
    role: session.role,
    apiKey: ""
  };
}

export type AdminOverview = {
  gatewayStatus: "Healthy" | "Partial" | "Error";
  requestsToday: number | null;
  avgLatencyMs: number | null;
	p95LatencyMs: number | null;
  successRate: number | null;
	errorRate: number | null;
	timeoutRate: number | null;
  activeProviders: number | null;
	activeTenants: number | null;
	queueDepth: number | null;
	defaultProvider: string;
	defaultModel: string;
	routingStrategy: string;
	latestBenchmark: BenchmarkRun | null;
	providerHealth: ProviderHealth[];
	recentErrors: RequestLog[];
	generatedAt: string;
	dataSources: SummaryDataSource[];
	partial: boolean;
	warnings: string[];
};

export function mapAdminSummaryToOverview(summary: SummaryResponse): AdminOverview {
	return {
		gatewayStatus: summary.gatewayStatus,
		requestsToday: summary.requestVolume,
		avgLatencyMs: summary.avgLatencyMs,
		p95LatencyMs: summary.p95LatencyMs,
		successRate: summary.successRate,
		errorRate: summary.errorRate,
		timeoutRate: summary.timeoutRate,
		activeProviders: summary.activeProviders,
		activeTenants: summary.activeTenants,
		queueDepth: summary.queueDepth,
		defaultProvider: summary.defaultProvider,
		defaultModel: summary.defaultModel,
		routingStrategy: summary.routingStrategy,
		latestBenchmark: summary.latestBenchmark,
		providerHealth: mapBffProviderHealth({ providers: summary.providerHealth }),
		recentErrors: mapBffRequestLogs({ logs: summary.recentErrors }),
		generatedAt: summary.generatedAt,
		dataSources: summary.dataSources.map((source) => ({ ...source })),
		partial: summary.partial,
		warnings: [...summary.warnings]
	};
}

export type ProviderHealth = {
  provider: string;
  targetModel: string;
  status: "Healthy" | "Degraded" | "Unavailable";
  avgLatencyMs: number;
  errorRate: number;
  timeoutRate: number;
  lastChecked: string;
};

export type RequestLog = {
  requestId: string;
  tenant: string;
  provider: string;
  model: string;
  method: string;
  latencyMs: number;
  ttftMs: number;
  status: "Success" | "Error" | "Timeout";
  errorMessage: string;
  timestamp: string;
};

export type BenchmarkRun = {
  runId: string;
	methodId?: "local_baseline" | "gateway" | "improved_model" | "gateway_improved_model";
  method: string;
  dataset: string;
  requestCount: number;
  concurrency: number;
  requestRate: number | null;
  warmUp: number;
  repeatedRuns: number;
  timeoutSettingSeconds: number;
  provider: string;
  targetModel: string;
	modelVersion?: string;
  gatewayVersion: string;
  avgLatencyMs: number | null;
  p50LatencyMs: number | null;
  p95LatencyMs: number | null;
  p99LatencyMs: number | null;
  ttftMs: number | null;
  throughputRps: number | null;
  successRatePct: number;
  errorRatePct: number;
  timeoutRatePct: number;
  improvementPct: number | null;
  testDate: string;
  source: string;
  rawFilePath: string;
  exportId: string;
  status: "passed" | "failed" | "partial";
  partialData: boolean;
};

export const COMPARED_METHODS = [
  "Local Baseline",
  "Our Gateway Method",
  "Improved Model",
  "Our Gateway + Improved Model"
] as const;

export type BenchmarkComparisonGroup = {
  key: string;
  dataset: string;
  rows: BenchmarkRun[];
  presentMethods: string[];
  missingMethods: string[];
  complete: boolean;
};

export function buildBenchmarkComparisonGroups(rows: BenchmarkRun[]): BenchmarkComparisonGroup[] {
  const grouped = new Map<string, BenchmarkRun[]>();
  rows.forEach((row) => {
    const key = benchmarkSetupKey(row);
    grouped.set(key, [...(grouped.get(key) ?? []), row]);
  });
  return Array.from(grouped, ([key, groupRows]) => {
    const presentMethods = COMPARED_METHODS.filter((method) => groupRows.some((row) => row.method === method));
    const missingMethods = COMPARED_METHODS.filter((method) => !presentMethods.includes(method));
    return {
      key,
      dataset: groupRows[0]?.dataset ?? "Unknown",
      rows: groupRows,
      presentMethods: [...presentMethods],
      missingMethods: [...missingMethods],
      complete: missingMethods.length === 0
    };
  });
}

export function calculateBenchmarkImprovements(rows: BenchmarkRun[]): BenchmarkRun[] {
  const baselines = new Map<string, number>();
  rows.forEach((row) => {
    if (row.method === "Local Baseline" && row.avgLatencyMs !== null && row.avgLatencyMs > 0) {
      baselines.set(benchmarkSetupKey(row), row.avgLatencyMs);
    }
  });
  return rows.map((row) => {
    if (row.improvementPct !== null) return row;
    const baseline = baselines.get(benchmarkSetupKey(row));
    if (baseline === undefined || row.avgLatencyMs === null) return row;
    const improvementPct = row.method === "Local Baseline" ? 0 : Number((((baseline - row.avgLatencyMs) / baseline) * 100).toFixed(2));
    return { ...row, improvementPct };
  });
}

function benchmarkSetupKey(row: BenchmarkRun): string {
  return [
    row.dataset,
    row.requestCount,
    row.concurrency,
    row.requestRate ?? "unlimited",
    row.warmUp,
    row.repeatedRuns,
    row.timeoutSettingSeconds
  ].join("|");
}

export function filterBenchmarkRows(
  rows: BenchmarkRun[],
  filters: { dataset: string; method: string; query: string }
): BenchmarkRun[] {
  const query = filters.query.trim().toLowerCase();
  return rows.filter((row) => {
    if (filters.dataset !== "All" && row.dataset !== filters.dataset) return false;
    if (filters.method !== "All" && row.method !== filters.method) return false;
    return !query || Object.values(row).some((value) => String(value).toLowerCase().includes(query));
  });
}

export function mapBffBenchmarksToMvpRuns(payload: BenchmarksResponse): BenchmarkRun[] {
  return payload.benchmarks.map((row) => ({ ...row }));
}

export function mapBffProviderHealth(payload: ProviderHealthResponse): ProviderHealth[] {
  return payload.providers.map((row) => ({ ...row }));
}

export function mapBffRequestLogs(payload: RequestLogsResponse): RequestLog[] {
  return payload.logs.map((row) => ({
    requestId: row.requestId ?? row.id ?? "unknown",
    tenant: row.tenant,
    provider: row.provider,
    model: row.model,
    method: row.method ?? "Unknown",
    latencyMs: row.latencyMs,
    ttftMs: row.ttftMs ?? 0,
    status: normalizeRequestStatus(row.status),
    errorMessage: row.errorMessage ?? row.error ?? "",
    timestamp: row.timestamp ?? ""
  }));
}

function normalizeRequestStatus(status: string): RequestLog["status"] {
  const normalized = status.toLowerCase();
  if (normalized === "success") return "Success";
  if (normalized === "timeout") return "Timeout";
  return "Error";
}

export function benchmarkChartKey(title: string, row: BenchmarkRun): string {
  return `${title}-${row.runId}`;
}

export type CustomerOverview = {
  customerName: string;
  tenantId: string;
  requestsToday: number;
  totalTokens: number;
  avgLatencyMs: number;
  p95LatencyMs: number;
  successRate: number;
  errorRate: number;
  timeoutRate: number;
  modelUsage: Record<string, number>;
  source: string;
  partialData: boolean;
  recentRequests: RequestLog[];
};

export type CustomerSummaryResponse = {
  tenantId: string;
  requests: number;
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
  avgLatencyMs: number;
  p95LatencyMs: number;
  successRate: number;
  errorRate: number;
  timeoutRate: number;
  modelUsage: Record<string, number>;
  source: string;
  generatedAt: string;
  partialData: boolean;
};

export type CustomerUsageResponse = {
  tenantId: string;
  summary: CustomerSummaryResponse;
  series: Array<{ date: string; requests: number; totalTokens: number; avgLatencyMs: number }>;
  models: Array<{ model: string; requests: number; totalTokens: number }>;
  source: string;
  generatedAt: string;
  partialData: boolean;
};

export type CustomerUsageQuery = {
  from?: string;
  to?: string;
};

export type CustomerRequestsResponse = {
  tenantId: string;
  requests: RequestLogsResponse["logs"];
  page: number;
  pageSize: number;
  total: number;
  source: string;
  generatedAt: string;
  partialData: boolean;
};

export type CustomerRequestQuery = {
  page: number;
  pageSize: number;
  status?: string;
  model?: string;
  from?: string;
  to?: string;
};

export type CustomerApiKey = {
  id: string;
  maskedKey: string;
  scope: string;
  status: string;
  createdAt: string;
  lastUsed: string;
};

export type CustomerApiKeysResponse = {
  tenantId: string;
  keys: CustomerApiKey[];
};

export type CreatedCustomerApiKey = CustomerApiKey & {
  key: string;
};

export type CustomerDashboardData = {
  summary: CustomerSummaryResponse;
  usage: CustomerUsageResponse;
  requests: CustomerRequestsResponse;
  apiKeys: CustomerApiKeysResponse;
};

export async function fetchCustomerRequests(query: CustomerRequestQuery): Promise<CustomerRequestsResponse> {
  const params = new URLSearchParams({
    page: String(query.page),
    pageSize: String(query.pageSize)
  });
  for (const key of ["status", "model", "from", "to"] as const) {
    const value = query[key]?.trim();
    if (value) {
      params.set(key, value);
    }
  }
  return getJSON<CustomerRequestsResponse>(`/bff/customer/requests?${params.toString()}`);
}

export async function fetchCustomerUsage(query: CustomerUsageQuery): Promise<CustomerUsageResponse> {
  const params = new URLSearchParams();
  if (query.from?.trim()) {
    params.set("from", query.from.trim());
  }
  if (query.to?.trim()) {
    params.set("to", query.to.trim());
  }
  const suffix = params.size > 0 ? `?${params.toString()}` : "";
  return getJSON<CustomerUsageResponse>(`/bff/customer/usage${suffix}`);
}

export async function fetchCustomerDashboardData(): Promise<CustomerDashboardData> {
  const [summary, usage, requests, apiKeys] = await Promise.all([
    getJSON<CustomerSummaryResponse>("/bff/customer/summary"),
    fetchCustomerUsage({}),
    fetchCustomerRequests({ page: 1, pageSize: 25 }),
    getJSON<CustomerApiKeysResponse>("/bff/customer/api-keys")
  ]);
  return { summary, usage, requests, apiKeys };
}

export async function createCustomerApiKey(): Promise<CreatedCustomerApiKey> {
  return postJSONResponse<CreatedCustomerApiKey>("/bff/customer/api-keys", { scope: "gateway:invoke" });
}

export async function revokeCustomerApiKey(id: string): Promise<void> {
  return deleteJSON(`/bff/customer/api-keys/${encodeURIComponent(id)}`);
}

export const BENCHMARK_COLUMNS = [
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
] as const;

export const adminNavigation: NavigationItem[] = [
  { label: "Admin Home", view: "admin-home" },
  { label: "System Management", view: "system-management" },
  { label: "Benchmarks", view: "benchmarks" },
  { label: "Provider Health", view: "provider-health" },
  { label: "Requests / Logs", view: "request-logs" }
];

export const customerNavigation: NavigationItem[] = [
  { label: "Customer Home", view: "customer-home" },
  { label: "Usage", view: "customer-usage" },
  { label: "My Requests", view: "customer-requests" },
  { label: "My API Keys", view: "customer-api-keys" },
  { label: "Account", view: "customer-account" }
];

export const demoBenchmarks: BenchmarkRun[] = [
  {
    runId: "bm-local-001",
    method: "Local Baseline",
    dataset: "LMSYS-Chat-1M-10K sample",
    requestCount: 1000,
    concurrency: 8,
    requestRate: 20,
    warmUp: 100,
    repeatedRuns: 3,
    timeoutSettingSeconds: 30,
    provider: "local-vllm",
    targetModel: "llama-3.1-8b-instruct",
    gatewayVersion: "none",
    avgLatencyMs: 920,
    p50LatencyMs: 780,
    p95LatencyMs: 1480,
    p99LatencyMs: 1920,
    ttftMs: 310,
    throughputRps: 1.0167,
    successRatePct: 97.6,
    errorRatePct: 1.4,
    timeoutRatePct: 1,
    improvementPct: 0,
    testDate: "2026-07-12",
    source: "mock CSV / local baseline",
    rawFilePath: "outputs/benchmarks/local_baseline.json",
    exportId: "bm-local-001",
    status: "passed",
    partialData: false
  },
  {
    runId: "bm-gateway-001",
    method: "Our Gateway Method",
    dataset: "LMSYS-Chat-1M-10K sample",
    requestCount: 1000,
    concurrency: 8,
    requestRate: 20,
    warmUp: 100,
    repeatedRuns: 3,
    timeoutSettingSeconds: 30,
    provider: "veloxmesh-gateway",
    targetModel: "llama-3.1-8b-instruct",
    gatewayVersion: "0.1.0",
    avgLatencyMs: 760,
    p50LatencyMs: 640,
    p95LatencyMs: 1210,
    p99LatencyMs: 1620,
    ttftMs: 260,
    throughputRps: 1.2333,
    successRatePct: 98.7,
    errorRatePct: 0.8,
    timeoutRatePct: 0.5,
    improvementPct: 17.4,
    testDate: "2026-07-12",
    source: "Redis veloxmesh:benchmarks",
    rawFilePath: "outputs/benchmarks/gateway_method.json",
    exportId: "bm-gateway-001",
    status: "passed",
    partialData: false
  },
  {
    runId: "bm-model-001",
    method: "Improved Model",
    dataset: "LMSYS-Chat-1M-10K sample",
    requestCount: 1000,
    concurrency: 8,
    requestRate: 20,
    warmUp: 100,
    repeatedRuns: 3,
    timeoutSettingSeconds: 30,
    provider: "local-vllm",
    targetModel: "improved-llama-3.1-8b",
    gatewayVersion: "none",
    avgLatencyMs: 710,
    p50LatencyMs: 600,
    p95LatencyMs: 1140,
    p99LatencyMs: 1540,
    ttftMs: 238,
    throughputRps: 1.3167,
    successRatePct: 98.5,
    errorRatePct: 0.9,
    timeoutRatePct: 0.6,
    improvementPct: 22.8,
    testDate: "2026-07-12",
    source: "mock JSON / improved model",
    rawFilePath: "outputs/benchmarks/improved_model.json",
    exportId: "bm-model-001",
    status: "passed",
    partialData: false
  },
  {
    runId: "bm-gateway-model-001",
    method: "Our Gateway + Improved Model",
    dataset: "LMSYS-Chat-1M-10K sample",
    requestCount: 1000,
    concurrency: 8,
    requestRate: 20,
    warmUp: 100,
    repeatedRuns: 3,
    timeoutSettingSeconds: 30,
    provider: "veloxmesh-gateway",
    targetModel: "improved-llama-3.1-8b",
    gatewayVersion: "0.1.0",
    avgLatencyMs: 610,
    p50LatencyMs: 520,
    p95LatencyMs: 980,
    p99LatencyMs: 1280,
    ttftMs: 205,
    throughputRps: 1.5333,
    successRatePct: 99.2,
    errorRatePct: 0.5,
    timeoutRatePct: 0.3,
    improvementPct: 33.7,
    testDate: "2026-07-12",
    source: "Redis / Qdrant export",
    rawFilePath: "outputs/benchmarks/gateway_plus_model.json",
    exportId: "bm-gateway-model-001",
    status: "passed",
    partialData: false
  }
];

const mockProviderHealth: ProviderHealth[] = [
  {
    provider: "veloxmesh-gateway",
    targetModel: "improved-llama-3.1-8b",
    status: "Healthy",
    avgLatencyMs: 610,
    errorRate: 0.5,
    timeoutRate: 0.3,
    lastChecked: "2026-07-12 14:30"
  },
  {
    provider: "local-vllm",
    targetModel: "llama-3.1-8b-instruct",
    status: "Degraded",
    avgLatencyMs: 920,
    errorRate: 1.4,
    timeoutRate: 1.0,
    lastChecked: "2026-07-12 14:28"
  },
  {
    provider: "fallback-provider",
    targetModel: "general-chat-fast",
    status: "Healthy",
    avgLatencyMs: 690,
    errorRate: 0.7,
    timeoutRate: 0.4,
    lastChecked: "2026-07-12 14:26"
  }
];

const mockRequestLogs: RequestLog[] = [
  {
    requestId: "req_7001",
    tenant: "capstone-demo",
    provider: "veloxmesh-gateway",
    model: "improved-llama-3.1-8b",
    method: "Our Gateway + Improved Model",
    latencyMs: 604,
    ttftMs: 202,
    status: "Success",
    errorMessage: "",
    timestamp: "2026-07-12 14:25:10"
  },
  {
    requestId: "req_7002",
    tenant: "coursework-lab",
    provider: "local-vllm",
    model: "llama-3.1-8b-instruct",
    method: "Local Baseline",
    latencyMs: 1510,
    ttftMs: 328,
    status: "Timeout",
    errorMessage: "request exceeded 30 s timeout",
    timestamp: "2026-07-12 14:24:48"
  },
  {
    requestId: "req_7003",
    tenant: "capstone-demo",
    provider: "veloxmesh-gateway",
    model: "llama-3.1-8b-instruct",
    method: "Our Gateway Method",
    latencyMs: 735,
    ttftMs: 252,
    status: "Success",
    errorMessage: "",
    timestamp: "2026-07-12 14:24:02"
  }
];

export function getNavigationForRole(role: UserRole): NavigationItem[] {
  return role === "Admin" ? adminNavigation : customerNavigation;
}

export function roleCanAccessView(role: UserRole, view: MvpView): boolean {
  const allowed = getNavigationForRole(role).map((item) => item.view);
  return allowed.includes(view);
}

export function parseDashboardHash(hash: string): DashboardLocation | null {
  const value = hash.replace(/^#/, "");
  const [viewValue, tabValue] = value.split("/");
  const validViews = [...adminNavigation, ...customerNavigation].map((item) => item.view);
  if (!validViews.includes(viewValue as MvpView)) {
    return null;
  }
  const view = viewValue as MvpView;
  if (view !== "system-management") {
    return { view };
  }
  const managementTab = SYSTEM_MANAGEMENT_TABS.some((tab) => tab.id === tabValue)
    ? tabValue as SystemManagementTab
    : "routing";
  return { view, managementTab };
}

export function dashboardHashFor(view: MvpView, managementTab: SystemManagementTab = "routing"): string {
  return view === "system-management" ? `${view}/${managementTab}` : view;
}

export function maskApiKey(value: string): string {
	if (!value) {
		return "Not issued";
	}
  if (value.length < 12) {
    return "••••";
  }
  return `${value.slice(0, 4)}...${value.slice(-4)}`;
}

export function buildBenchmarkCsv(rows: BenchmarkRun[]): string {
  const csvRows = rows.map(benchmarkValues);
  return [BENCHMARK_COLUMNS, ...csvRows]
    .map((values) => values.map(csvCell).join(","))
    .join("\n");
}

export function buildBenchmarkReportHtml(rows: BenchmarkRun[]): string {
  const generatedAt = new Date().toISOString();
  const methods = uniqueValues(rows.map((row) => row.method));
  const datasets = uniqueValues(rows.map((row) => row.dataset));
  const providers = uniqueValues(rows.map((row) => row.provider));
  const models = uniqueValues(rows.map((row) => row.targetModel));
  const gatewayVersions = uniqueValues(rows.map((row) => row.gatewayVersion));
  const sourceRows = rows.map((row) => `<tr><td>${escapeHtml(row.runId)}</td><td>${escapeHtml(row.source)}</td><td>${escapeHtml(row.rawFilePath)}</td><td>${escapeHtml(row.status)}</td></tr>`).join("");
  return `<!doctype html>
<html>
<head>
  <meta charset="utf-8" />
  <title>VeloxMesh AI Gateway Benchmark Report</title>
  <style>
    body { font-family: Arial, sans-serif; margin: 32px; color: #172033; line-height: 1.45; }
    h1, h2 { color: #0f172a; }
    table { border-collapse: collapse; width: 100%; margin: 12px 0 24px; font-size: 12px; }
    th, td { border: 1px solid #d7dee8; padding: 7px; text-align: left; vertical-align: top; }
    th { background: #eef2f7; }
    .table-wrap { overflow-x: auto; }
    .chart { margin: 12px 0 22px; }
    .bar-row { display: grid; grid-template-columns: minmax(180px, 1fr) 3fr 100px; gap: 10px; align-items: center; margin: 7px 0; }
    .bar-track { height: 14px; background: #e6eaf0; }
    .bar { display: block; height: 100%; background: #147d6f; }
    .warning { border-left: 4px solid #b45309; padding: 10px 12px; background: #fff7ed; }
    @media print { body { margin: 14mm; } .table-wrap { overflow: visible; } }
  </style>
</head>
<body>
  <h1>VeloxMesh AI Gateway Benchmark Report</h1>
  <h2>Report Metadata</h2>
  <p>Project name: VeloxMesh AI Gateway Dashboard<br />Export time: ${escapeHtml(generatedAt)}<br />Gateway version: ${escapeHtml(gatewayVersions.join(", ") || "Unavailable")}<br />Run IDs: ${escapeHtml(rows.map((row) => row.runId).join(", ") || "None")}</p>
  <h2>Benchmark Setup</h2>
  <p>Datasets: ${escapeHtml(datasets.join(", ") || "Unavailable")}<br />Providers: ${escapeHtml(providers.join(", ") || "Unavailable")}<br />Target models: ${escapeHtml(models.join(", ") || "Unavailable")}<br />Per-run request count, concurrency, request rate, warm-up, repeated runs, and timeout settings are listed in Result Summary.</p>
  <h2>Compared Methods</h2>
  <p>${escapeHtml(methods.join(", ") || "No methods available")}</p>
  <h2>Result Summary</h2>
  <div class="table-wrap">${benchmarkTableHtml(rows)}</div>
  <h2>Charts</h2>
  ${reportChartHtml("Average latency comparison", rows, (row) => row.avgLatencyMs, "ms")}
  ${reportChartHtml("P95 latency comparison", rows, (row) => row.p95LatencyMs, "ms")}
  ${reportChartHtml("P99 latency comparison", rows, (row) => row.p99LatencyMs, "ms")}
  ${reportChartHtml("Throughput comparison", rows, (row) => row.throughputRps, "req/s")}
  ${reportChartHtml("Error rate comparison", rows, (row) => row.errorRatePct, "%")}
  ${reportChartHtml("Timeout rate comparison", rows, (row) => row.timeoutRatePct, "%")}
  <h2>Analysis</h2>
  ${benchmarkAnalysisHtml(rows)}
  <h2>Data Source</h2>
  <p>BFF endpoint: <code>/bff/admin/benchmarks</code>; Redis key: <code>veloxmesh:benchmarks</code>.</p>
  <table><thead><tr><th>Run ID</th><th>Source</th><th>Raw file path</th><th>Status</th></tr></thead><tbody>${sourceRows || "<tr><td colspan=\"4\">No source rows</td></tr>"}</tbody></table>
  <h2>Limitations</h2>
  <p>This report reflects the listed local environment and Provider network conditions. It does not directly reproduce the teacher experiment because the original teacher model, hardware, and dataset are unavailable. Failed or partial runs are retained and must not be treated as proof of improvement.</p>
  <h2>Appendix</h2>
  <p>The complete canonical BenchmarkRun table above is the aggregate appendix. Request-level responses, latency rows, and error/timeout samples are stored under each listed raw file path.</p>
</body>
</html>`;
}

function demoAdminSummaryResponse(): SummaryResponse {
	const successful = mockRequestLogs.filter((row) => row.status === "Success");
	const errors = mockRequestLogs.filter((row) => row.status === "Error");
	const timeouts = mockRequestLogs.filter((row) => row.status === "Timeout");
	const denominator = mockRequestLogs.length || 1;
	const average = mockRequestLogs.reduce((total, row) => total + row.latencyMs, 0) / denominator;
	const activeTenants = new Set(mockRequestLogs.map((row) => row.tenant).filter(Boolean)).size;
	return {
		defaultProvider: mockProviderHealth[0]?.provider ?? "demo-provider",
		defaultModel: mockProviderHealth[0]?.targetModel ?? "demo-model",
		modelCount: new Set(mockProviderHealth.map((row) => row.targetModel)).size,
		activeProviders: mockProviderHealth.filter((row) => row.status !== "Unavailable").length,
		activeTenants,
		requestVolume: mockRequestLogs.length,
		avgLatencyMs: Number(average.toFixed(2)),
		p95LatencyMs: Math.max(...mockRequestLogs.map((row) => row.latencyMs)),
		successRate: Number(((successful.length * 100) / denominator).toFixed(2)),
		errorRate: Number(((errors.length * 100) / denominator).toFixed(2)),
		timeoutRate: Number(((timeouts.length * 100) / denominator).toFixed(2)),
		queueDepth: null,
		gatewayStatus: "Partial",
		routingStrategy: "demo",
		topology: null,
		latestBenchmark: demoBenchmarks.at(-1) ?? null,
		providerHealth: mockProviderHealth,
		recentErrors: [...errors, ...timeouts].map((row) => ({
			requestId: row.requestId,
			tenant: row.tenant,
			provider: row.provider,
			model: row.model,
			method: row.method,
			inputTokens: 0,
			outputTokens: 0,
			status: row.status,
			latencyMs: row.latencyMs,
			ttftMs: row.ttftMs,
			errorMessage: row.errorMessage,
			timestamp: row.timestamp
		})),
		generatedAt: new Date().toISOString(),
		dataSources: [{ name: "Demo data", source: "VITE_DASHBOARD_DEMO_MODE", status: "ok", detail: "explicit frontend demo mode" }],
		partial: true,
		partialData: true,
		warnings: ["Demo mode is enabled; values are not production evidence."]
	};
}

export const mockApi = {
  async login(input: LoginInput): Promise<LoginChallengeResponse> {
    return loginAccount(input);
  },

  async verifyLogin(input: VerifyLoginInput): Promise<MvpSession> {
    return mvpSessionFromBff(await verifyLoginCode(input));
  },

  async register(input: CustomerRegisterInput): Promise<RegisterResponse> {
	return registerCustomerAccount(input);
  },

  async logout(): Promise<void> {
    return logoutAccount();
  },

  async getSession(): Promise<MvpSession | null> {
    try {
      return mvpSessionFromBff(await fetchSession());
    } catch {
	  return null;
	}
  },

  async getAdminOverview(): Promise<AdminOverview> {
	try {
		return mapAdminSummaryToOverview(await getJSON<SummaryResponse>("/bff/admin/summary"));
	} catch (error) {
		if (import.meta.env.VITE_DASHBOARD_DEMO_MODE === "true") {
			return delayed(mapAdminSummaryToOverview(demoAdminSummaryResponse()));
		}
		throw error;
	}
  },

  async getProviderHealth(): Promise<ProviderHealth[]> {
    try {
      const payload = await getJSON<ProviderHealthResponse>("/bff/admin/provider-health");
      return mapBffProviderHealth(payload);
    } catch {
      return import.meta.env.VITE_DASHBOARD_DEMO_MODE === "true" ? delayed(mockProviderHealth) : [];
    }
  },

  async getRequestLogs(): Promise<RequestLog[]> {
    try {
      const payload = await getJSON<RequestLogsResponse>("/bff/admin/request-logs");
      return mapBffRequestLogs(payload);
    } catch {
      return import.meta.env.VITE_DASHBOARD_DEMO_MODE === "true" ? delayed(mockRequestLogs) : [];
    }
  },

  async getBenchmarks(): Promise<BenchmarkRun[]> {
    try {
      const response = await fetch("/bff/admin/benchmarks");
      if (!response.ok) {
        throw new Error(await responseErrorMessage(response));
      }
      const payload = await response.json() as BenchmarksResponse;
      const liveRows = mapBffBenchmarksToMvpRuns(payload);
      if (liveRows.length > 0) {
        return liveRows;
      }
    } catch {
      // Keep the control panel usable when the local BFF is not running.
    }
    if (import.meta.env.VITE_DASHBOARD_DEMO_MODE === "true") {
      return delayed(demoBenchmarks);
    }
    return [];
  },

  async getCustomerDashboard(): Promise<CustomerDashboardData> {
    return fetchCustomerDashboardData();
  },

  async getCustomerRequests(query: CustomerRequestQuery): Promise<CustomerRequestsResponse> {
    return fetchCustomerRequests(query);
  },

  async getCustomerUsage(query: CustomerUsageQuery): Promise<CustomerUsageResponse> {
    return fetchCustomerUsage(query);
  },

  async createCustomerApiKey(): Promise<CreatedCustomerApiKey> {
    return createCustomerApiKey();
  },

  async revokeCustomerApiKey(id: string): Promise<void> {
    return revokeCustomerApiKey(id);
  }
};

function csvCell(value: string): string {
  if (/[",\n]/.test(value)) {
    return `"${value.replace(/"/g, '""')}"`;
  }
  return value;
}

function benchmarkTableHtml(rows: BenchmarkRun[]): string {
  const header = BENCHMARK_COLUMNS.map((column) => `<th>${escapeHtml(column)}</th>`).join("");
  const body = rows.map((row) => `<tr>${benchmarkValues(row).map((value) => `<td>${escapeHtml(value)}</td>`).join("")}</tr>`).join("");
  return `<table><thead><tr>${header}</tr></thead><tbody>${body || `<tr><td colspan="${BENCHMARK_COLUMNS.length}">No benchmark data</td></tr>`}</tbody></table>`;
}

function benchmarkValues(row: BenchmarkRun): string[] {
  return [
    row.runId,
    row.method,
    row.dataset,
    String(row.requestCount),
    String(row.concurrency),
    nullableNumber(row.requestRate),
    String(row.warmUp),
    String(row.repeatedRuns),
    String(row.timeoutSettingSeconds),
    row.provider,
    row.targetModel,
    row.gatewayVersion,
    nullableNumber(row.avgLatencyMs),
    nullableNumber(row.p50LatencyMs),
    nullableNumber(row.p95LatencyMs),
    nullableNumber(row.p99LatencyMs),
    nullableNumber(row.ttftMs),
    nullableNumber(row.throughputRps),
    String(row.successRatePct),
    String(row.errorRatePct),
    String(row.timeoutRatePct),
    nullableNumber(row.improvementPct),
    row.testDate,
    row.source,
    row.rawFilePath,
    row.exportId,
    row.status,
    String(row.partialData)
  ];
}

function benchmarkAnalysisHtml(rows: BenchmarkRun[]): string {
  const valid = rows.filter((row) => row.status === "passed" && !row.partialData && row.avgLatencyMs !== null);
  if (valid.length < 2) {
    return `<p class="warning"><strong>Insufficient Data.</strong> At least two complete passed runs with average latency are required for a comparison. Available failed or partial runs remain visible in the summary.</p>`;
  }
  const bestLatency = valid.reduce((best, row) => (row.avgLatencyMs! < best.avgLatencyMs! ? row : best));
  const throughputRows = valid.filter((row) => row.throughputRps !== null);
  const bestThroughput = throughputRows.length > 0
    ? throughputRows.reduce((best, row) => (row.throughputRps! > best.throughputRps! ? row : best))
    : null;
  const baseline = valid.find((row) => row.method.toLowerCase() === "local baseline");
  const gateway = valid.find((row) => row.method.toLowerCase() === "our gateway method");
  const gatewayFinding = baseline && gateway
    ? `Our Gateway Method ${gateway.avgLatencyMs! < baseline.avgLatencyMs! ? "reduced" : "did not reduce"} average latency versus Local Baseline (${gateway.avgLatencyMs} ms vs ${baseline.avgLatencyMs} ms).`
    : "Gateway versus Local Baseline cannot be determined from the available complete runs.";
  return `<ul><li>${escapeHtml(bestLatency.method)} has the lowest measured average latency (${bestLatency.avgLatencyMs} ms).</li><li>${bestThroughput ? `${escapeHtml(bestThroughput.method)} has the highest measured throughput (${bestThroughput.throughputRps} req/s).` : "Throughput comparison is unavailable."}</li><li>${escapeHtml(gatewayFinding)}</li></ul>`;
}

function reportChartHtml(title: string, rows: BenchmarkRun[], metric: (row: BenchmarkRun) => number | null, unit: string): string {
  const points = rows.map((row) => ({ method: row.method, value: metric(row) })).filter((point): point is { method: string; value: number } => point.value !== null);
  if (points.length === 0) {
    return `<section class="chart"><h3>${escapeHtml(title)}</h3><p>No data available.</p></section>`;
  }
  const max = Math.max(...points.map((point) => point.value), 0);
  const bars = points.map((point) => {
    const width = max > 0 ? Math.max(1, (point.value / max) * 100) : 0;
    return `<div class="bar-row"><span>${escapeHtml(point.method)}</span><span class="bar-track"><i class="bar" style="width:${width.toFixed(2)}%"></i></span><strong>${point.value.toLocaleString()} ${escapeHtml(unit)}</strong></div>`;
  }).join("");
  return `<section class="chart"><h3>${escapeHtml(title)}</h3>${bars}</section>`;
}

function uniqueValues(values: string[]): string[] {
  return [...new Set(values.filter((value) => value.trim().length > 0))];
}

function escapeHtml(value: string): string {
  return value.replace(/[&<>"']/g, (character) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#039;" })[character]!);
}

function delayed<T>(value: T): Promise<T> {
  return new Promise((resolve) => globalThis.setTimeout(() => resolve(value), 80));
}

function nullableNumber(value: number | null): string {
  return value === null ? "" : String(value);
}

function formatMetric(value: number | null, unit: string): string {
  return value === null ? "-" : `${value.toLocaleString()} ${unit}`;
}

function browserStorage(): Storage | null {
  if (typeof globalThis.localStorage === "undefined") {
    return null;
  }
  return globalThis.localStorage;
}
