import {
  Activity,
  BarChart3,
  Download,
  FileText,
  Gauge,
  KeyRound,
  Lock,
  Network,
  RefreshCw,
  Server,
  ShieldAlert,
  UserRound
} from "lucide-react";
import { FormEvent, useEffect, useMemo, useState } from "react";
import {
  BENCHMARK_COLUMNS,
  COMPARED_METHODS,
  AdminOverview,
  BenchmarkRun,
	CustomerApiKeysResponse,
	CustomerDashboardData,
	CustomerRequestQuery,
	CustomerRequestsResponse,
	CustomerSummaryResponse,
	CustomerUsageResponse,
  MvpSession,
  MvpView,
  ProviderHealth,
  RequestLog,
  SystemManagementTab,
  UserRole,
  benchmarkChartKey,
  buildBenchmarkComparisonGroups,
  calculateBenchmarkImprovements,
  dashboardHashFor,
  filterBenchmarkRows,
	fetchBenchmarkRawCSVExport,
	fetchBenchmarkReportZIPExport,
  getNavigationForRole,
  maskApiKey,
	mapBffRequestLogs,
  mockApi,
  parseDashboardHash,
  roleCanAccessView
} from "./api";
import "./styles.css";
import { AccountRole, AuthMode, authCopy, canRegisterRole, portalRoleForPathname, shouldHandlePortalClick } from "./authCopy";
import { SystemManagement } from "./SystemManagement";

type AppState =
  | { status: "loading" }
  | { status: "signed-out" }
  | { status: "ready"; session: MvpSession }
  | { status: "error"; message: string };

type DashboardData = {
  adminOverview?: AdminOverview;
  providerHealth: ProviderHealth[];
  requestLogs: RequestLog[];
  benchmarks: BenchmarkRun[];
	customer?: CustomerDashboardData;
};

const emptyData: DashboardData = {
  providerHealth: [],
  requestLogs: [],
  benchmarks: []
};

const viewIcons: Record<string, typeof Gauge> = {
  "admin-home": Gauge,
  "system-management": Network,
  benchmarks: BarChart3,
  "provider-health": Server,
  "request-logs": FileText,
  "customer-home": Gauge,
  "customer-usage": BarChart3,
  "customer-requests": FileText,
  "customer-api-keys": KeyRound,
  "customer-account": UserRound
};

export default function App() {
  const [appState, setAppState] = useState<AppState>({ status: "loading" });
  const [portalRole, setPortalRole] = useState<AccountRole>(() => portalRoleForPathname(window.location.pathname));
  const [activeView, setActiveView] = useState<MvpView>("admin-home");
  const [activeManagementTab, setActiveManagementTab] = useState<SystemManagementTab>("routing");
  const [data, setData] = useState<DashboardData>(emptyData);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [errorMode, setErrorMode] = useState(false);

  useEffect(() => {
    void boot();
  }, []);

  useEffect(() => {
    function syncPortalRole() {
      setPortalRole(portalRoleForPathname(window.location.pathname));
    }

    window.addEventListener("popstate", syncPortalRole);
    return () => window.removeEventListener("popstate", syncPortalRole);
  }, []);

  useEffect(() => {
    function syncHashView() {
      const location = parseDashboardHash(window.location.hash);
      if (location) {
        setActiveView(location.view);
        if (location.managementTab) {
          setActiveManagementTab(location.managementTab);
        }
      }
    }

    syncHashView();
    window.addEventListener("hashchange", syncHashView);
    return () => window.removeEventListener("hashchange", syncHashView);
  }, []);

  async function boot() {
    setAppState({ status: "loading" });
    try {
      const session = await mockApi.getSession();
      if (!session) {
        setAppState({ status: "signed-out" });
        return;
      }
      applyInitialLocation(session.role);
      setAppState({ status: "ready", session });
      await loadRoleData(session.role);
    } catch (error) {
      setAppState({ status: "error", message: errorMessage(error) });
    }
  }

  async function loadRoleData(role: UserRole) {
    setIsRefreshing(true);
    try {
      if (errorMode) {
        throw new Error("Simulated BFF error state");
      }
      if (role === "Admin") {
        const [adminOverview, providerHealth, requestLogs, benchmarks] = await Promise.all([
          mockApi.getAdminOverview(),
          mockApi.getProviderHealth(),
          mockApi.getRequestLogs(),
          mockApi.getBenchmarks()
        ]);
        setData({ adminOverview, providerHealth, requestLogs, benchmarks });
      } else {
		const customer = await mockApi.getCustomerDashboard();
        setData({
          ...emptyData,
			customer,
			requestLogs: mapBffRequestLogs({ logs: customer.requests.requests })
        });
      }
    } finally {
      setIsRefreshing(false);
    }
  }

  async function completeLogin(session: MvpSession) {
    applyInitialLocation(session.role);
    setAppState({ status: "ready", session });
    await loadRoleData(session.role);
  }

  async function logout() {
    await mockApi.logout();
    setData(emptyData);
    setAppState({ status: "signed-out" });
  }

  function navigateToPortal(role: AccountRole) {
    const pathname = role === "Admin" ? "/admin/login" : "/customer/login";
    window.history.pushState({}, "", pathname);
    setPortalRole(role);
  }

  async function refresh(session: MvpSession) {
    try {
      await loadRoleData(session.role);
    } catch (error) {
      setAppState({ status: "error", message: errorMessage(error) });
    }
  }

  function applyInitialLocation(role: UserRole) {
    const location = parseDashboardHash(window.location.hash);
    setActiveView(location?.view ?? (role === "Admin" ? "admin-home" : "customer-home"));
    if (location?.managementTab) {
      setActiveManagementTab(location.managementTab);
    }
  }

  if (appState.status === "loading") {
    return <LoadingState />;
  }

  if (appState.status === "signed-out") {
    return (
      <LoginScreen
        key={portalRole}
        role={portalRole}
        onAuthenticated={completeLogin}
        onPortalChange={navigateToPortal}
      />
    );
  }

  if (appState.status === "error") {
    return <ErrorState message={appState.message} onRetry={boot} />;
  }

  const session = appState.session;
  const canAccess = roleCanAccessView(session.role, activeView);

  return (
    <div className="app-shell">
      <Sidebar
        session={session}
        activeView={activeView}
        onNavigate={(view) => navigateToView(view, activeManagementTab, setActiveView)}
        onLogout={logout}
      />
      <main className="workspace" aria-busy={isRefreshing}>
        <Topbar
          session={session}
          activeView={activeView}
          isRefreshing={isRefreshing}
          errorMode={errorMode}
          onToggleError={() => setErrorMode((current) => !current)}
          onRefresh={() => refresh(session)}
        />
        {!canAccess ? (
          <NoPermissionState role={session.role} view={activeView} />
        ) : (
			<ViewRouter
              session={session}
              view={activeView}
              data={data}
              managementTab={activeManagementTab}
              onManagementTabChange={(tab) => navigateToManagementTab(tab, setActiveManagementTab)}
              onCustomerRefresh={() => loadRoleData("Customer")}
            />
        )}
      </main>
    </div>
  );
}

function LoginScreen({
  role,
  onAuthenticated,
  onPortalChange
}: {
  role: AccountRole;
  onAuthenticated: (session: MvpSession) => Promise<void>;
  onPortalChange: (role: AccountRole) => void;
}) {
  const [mode, setMode] = useState<AuthMode>("login");
  const [identifier, setIdentifier] = useState("");
  const [email, setEmail] = useState("");
  const [username, setUsername] = useState("");
	const [organization, setOrganization] = useState("");
  const [password, setPassword] = useState("");
	const [confirmPassword, setConfirmPassword] = useState("");
  const [challengeId, setChallengeId] = useState("");
  const [verificationCode, setVerificationCode] = useState("");
  const [devCode, setDevCode] = useState("");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const isVerifying = Boolean(challengeId);
  const copy = authCopy(mode, role, isVerifying);
  const otherPortalRole: AccountRole = role === "Admin" ? "Customer" : "Admin";
  const otherPortalPath = otherPortalRole === "Admin" ? "/admin/login" : "/customer/login";

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setError("");
    setMessage("");
    try {
      if (isVerifying) {
        const session = await mockApi.verifyLogin({ challengeId, code: verificationCode });
        await onAuthenticated(session);
        return;
      }
      if (mode === "register") {
		if (password !== confirmPassword) {
			throw new Error("Password confirmation does not match");
		}
		const response = await mockApi.register({ email, username, organization, password, confirmPassword });
		setChallengeId(response.challengeId);
		setDevCode(response.devCode ?? "");
        setMessage(response.message);
        return;
      }
      const challenge = await mockApi.login({ identifier, password, role });
      setChallengeId(challenge.challengeId);
      setDevCode(challenge.devCode ?? "");
      setMessage(challenge.message);
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setSubmitting(false);
    }
  }

  function resetAuth(nextMode: AuthMode) {
    setMode(nextMode);
    setChallengeId("");
    setVerificationCode("");
    setDevCode("");
    setError("");
    setMessage("");
  }

  return (
    <main className="login-shell">
      <section className="login-panel" aria-label="VeloxMesh login">
        <div className="brand-mark">
          <Network aria-hidden="true" size={30} />
          <div>
            <strong>VeloxMesh</strong>
            <span>AI Gateway Dashboard</span>
          </div>
        </div>
        <span className="auth-eyebrow">{copy.brandLabel}</span>
        <h1>{copy.title}</h1>
        <p>{copy.description}</p>
        <form className="auth-form" onSubmit={submit}>
          {mode === "register" && !isVerifying && (
            <>
              <label>Email<input type="email" value={email} onChange={(event) => setEmail(event.target.value)} required /></label>
              <label>Username<input value={username} onChange={(event) => setUsername(event.target.value)} minLength={4} required /></label>
			  <label>Organization<input value={organization} onChange={(event) => setOrganization(event.target.value)} required /></label>
            </>
          )}
          {mode === "login" && !isVerifying && (
            <label>Username or email<input value={identifier} onChange={(event) => setIdentifier(event.target.value)} required autoFocus /></label>
          )}
          {!isVerifying && (
			<label>Password<input type="password" value={password} onChange={(event) => setPassword(event.target.value)} minLength={8} required /></label>
          )}
		  {mode === "register" && !isVerifying && (
			<label>Confirm password<input type="password" value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} minLength={8} required /></label>
		  )}
          {isVerifying && (
            <label>Verification code<input inputMode="numeric" pattern="[0-9]{6}" maxLength={6} value={verificationCode} onChange={(event) => setVerificationCode(event.target.value)} required autoFocus /></label>
          )}
          {devCode && <div className="dev-code">Local verification code: <strong>{devCode}</strong></div>}
          {message && <div className="auth-message" role="status">{message}</div>}
          {error && <div className="auth-error" role="alert">{error}</div>}
          <button className="auth-submit" type="submit" disabled={submitting}>
            <Lock size={17} aria-hidden="true" />
            {submitting ? "Please wait..." : isVerifying ? "Verify and sign in" : mode === "login" ? "Sign in" : "Create account"}
          </button>
        </form>
        {!isVerifying && canRegisterRole(role) && (
          <button className="auth-switch" type="button" onClick={() => resetAuth(mode === "login" ? "register" : "login")}>
			{mode === "login" ? "Create Customer Account" : "Back to sign in"}
          </button>
        )}
        {isVerifying && <button className="auth-switch" type="button" onClick={() => resetAuth("login")}>Back to sign in</button>}
        {!isVerifying && mode === "login" && (
          <a
            className="auth-switch auth-portal-switch"
            href={otherPortalPath}
            onClick={(event) => {
              if (!shouldHandlePortalClick(event)) {
                return;
              }
              event.preventDefault();
              onPortalChange(otherPortalRole);
            }}
          >
            {otherPortalRole} portal
          </a>
        )}
      </section>
    </main>
  );
}

function Sidebar({
  session,
  activeView,
  onNavigate,
  onLogout
}: {
  session: MvpSession;
  activeView: MvpView;
  onNavigate: (view: MvpView) => void;
  onLogout: () => void;
}) {
  return (
    <aside className="sidebar">
      <div className="brand-mark sidebar-brand">
        <Network aria-hidden="true" size={24} />
        <div>
          <strong>VeloxMesh</strong>
          <span>{session.role} Dashboard</span>
        </div>
      </div>
      <section className="session-card">
        <span>{session.user}</span>
        <strong>{session.role}</strong>
        <small>{maskApiKey(session.apiKey)}</small>
        <button onClick={onLogout}>Sign out</button>
      </section>
      <nav aria-label={`${session.role} navigation`}>
        {getNavigationForRole(session.role).map((item) => {
          const Icon = viewIcons[item.view] ?? Gauge;
          return (
            <button
              key={item.view}
              className={item.view === activeView ? "active" : ""}
              onClick={() => onNavigate(item.view)}
            >
              <Icon size={17} aria-hidden="true" />
              <span>{item.label}</span>
            </button>
          );
        })}
      </nav>
    </aside>
  );
}

function Topbar({
  session,
  activeView,
  isRefreshing,
  errorMode,
  onToggleError,
  onRefresh
}: {
  session: MvpSession;
  activeView: MvpView;
  isRefreshing: boolean;
  errorMode: boolean;
  onToggleError: () => void;
  onRefresh: () => void;
}) {
  const title = getNavigationForRole(session.role).find((item) => item.view === activeView)?.label ?? "No Permission";
  return (
    <header className="topbar">
      <div>
        <h1>{title}</h1>
        <p>{session.role === "Admin" ? "Manage provider routing, benchmarks, and gateway operations." : "View your gateway usage, request status, and API access."}</p>
      </div>
      <div className="toolbar-actions">
        <button className={errorMode ? "danger-outline" : "secondary-button"} onClick={onToggleError}>
          {errorMode ? "Error Mode On" : "Simulate Error"}
        </button>
        <button className="primary-button" onClick={onRefresh} disabled={isRefreshing}>
          <RefreshCw size={17} aria-hidden="true" />
          {isRefreshing ? "Refreshing" : "Refresh"}
        </button>
      </div>
    </header>
  );
}

function ViewRouter({
  session,
  view,
  data,
	managementTab,
	onManagementTabChange,
	onCustomerRefresh
}: {
  session: MvpSession;
  view: MvpView;
  data: DashboardData;
	managementTab: SystemManagementTab;
	onManagementTabChange: (tab: SystemManagementTab) => void;
	onCustomerRefresh: () => Promise<void>;
}) {
  if (session.role === "Admin") {
    if (view === "admin-home") {
	  return <AdminHome overview={data.adminOverview} providerHealth={data.providerHealth} />;
    }
    if (view === "system-management") {
      return <SystemManagement activeTab={managementTab} onTabChange={onManagementTabChange} />;
    }
    if (view === "benchmarks") {
      return <BenchmarksPage rows={data.benchmarks} />;
    }
    if (view === "provider-health") {
      return <ProviderHealthPage rows={data.providerHealth} />;
    }
    if (view === "request-logs") {
      return <RequestLogsPage rows={data.requestLogs} />;
    }
    return <PlaceholderPage title={viewLabel(view)} />;
  }

  if (view === "customer-home") {
	return <CustomerHome session={session} summary={data.customer?.summary} requests={data.requestLogs} />;
	}
	if (view === "customer-usage") {
	return <CustomerUsage usage={data.customer?.usage} />;
  }
  if (view === "customer-requests") {
    return (
		<CustomerRequestsPage
			initial={data.customer?.requests}
			models={data.customer?.usage.models.map((row) => row.model) ?? []}
		/>
	);
  }
  if (view === "customer-api-keys") {
	return <CustomerApiKeys keys={data.customer?.apiKeys} onChanged={onCustomerRefresh} />;
	}
	if (view === "customer-account") {
	return <CustomerAccount session={session} />;
  }
  return <PlaceholderPage title={viewLabel(view)} />;
}

function AdminHome({
  overview,
  providerHealth
}: {
  overview?: AdminOverview;
  providerHealth: ProviderHealth[];
}) {
  if (!overview) {
    return <LoadingState compact />;
  }
	const liveProviderHealth = overview.providerHealth.length > 0 ? overview.providerHealth : providerHealth;
	const latest = overview.latestBenchmark;
  return (
    <>
	  <PartialDataBanner show={overview.partial} warnings={overview.warnings} />
      <section className="metric-grid">
        <Metric label="Gateway Status" value={overview.gatewayStatus} detail="Overall control-plane status" />
		<Metric label="Requests Today" value={formatOptionalNumber(overview.requestsToday)} detail={overview.activeTenants === null ? "Active tenants unavailable" : `${overview.activeTenants} active tenants`} />
		<Metric label="Avg Latency" value={formatOptionalMetric(overview.avgLatencyMs, "ms")} detail={`P95 ${formatOptionalMetric(overview.p95LatencyMs, "ms")}`} />
		<Metric label="Success Rate" value={formatOptionalMetric(overview.successRate, "%")} detail={overview.activeProviders === null ? "Active providers unavailable" : `${overview.activeProviders} providers active`} />
      </section>
      <section className="dashboard-grid">
        <article className="panel">
          <div className="panel-heading">
            <h2>Latest Benchmark</h2>
            <BarChart3 size={18} aria-hidden="true" />
          </div>
		  <strong className="large-value">{latest?.method ?? "Unavailable"}</strong>
		  <p>{latest ? `${latest.runId} · ${formatMs(latest.avgLatencyMs)} average latency, ${formatRps(latest.throughputRps)} throughput.` : "No valid benchmark data yet."}</p>
        </article>
        <article className="panel">
          <div className="panel-heading">
            <h2>Provider Snapshot</h2>
            <Activity size={18} aria-hidden="true" />
          </div>
          <div className="compact-list">
			{liveProviderHealth.slice(0, 3).map((provider) => (
              <div key={provider.provider}>
                <span>{provider.provider}</span>
                <strong>{provider.status}</strong>
              </div>
            ))}
			{liveProviderHealth.length === 0 ? <p>Provider health unavailable.</p> : null}
          </div>
        </article>
      </section>
	  <section className="summary-provenance" aria-label="Admin summary data sources">
		<div>
		  <strong>Generated</strong>
		  <span>{formatSummaryTimestamp(overview.generatedAt)}</span>
		</div>
		<div>
		  <strong>Sources</strong>
		  <span>{overview.dataSources.map((source) => `${source.name}: ${source.status}`).join(" · ") || "Unavailable"}</span>
		</div>
	  </section>
    </>
  );
}

function formatOptionalNumber(value: number | null): string {
	return value === null ? "Unavailable" : value.toLocaleString();
}

function formatOptionalMetric(value: number | null, unit: string): string {
	return value === null ? "Unavailable" : unit === "%" ? `${value}%` : `${value} ${unit}`;
}

function formatSummaryTimestamp(value: string): string {
	const timestamp = new Date(value);
	return Number.isNaN(timestamp.getTime()) ? "Unavailable" : timestamp.toLocaleString();
}

function CustomerHome({ session, summary, requests }: { session: MvpSession; summary?: CustomerSummaryResponse; requests: RequestLog[] }) {
  if (!summary) {
    return <LoadingState compact />;
  }
  return (
    <>
	  <PartialDataBanner show={summary.partialData} />
      <section className="metric-grid">
		<Metric label="Requests" value={summary.requests.toLocaleString()} detail="Your tenant gateway calls" />
		<Metric label="Total Tokens" value={summary.totalTokens.toLocaleString()} detail="Input and output tokens" />
		<Metric label="Avg / P95 Latency" value={`${summary.avgLatencyMs} / ${summary.p95LatencyMs} ms`} detail="Your real request latency" />
		<Metric label="Success Rate" value={`${summary.successRate}%`} detail={`${summary.errorRate}% errors · ${summary.timeoutRate}% timeouts`} />
      </section>
      <section className="dashboard-grid">
        <article className="panel">
          <div className="panel-heading">
			<h2>Tenant Identity</h2>
			<UserRound size={18} aria-hidden="true" />
          </div>
		  <strong className="large-value">{session.user}</strong>
		  <p>{summary.tenantId}. Data source: {summary.source}.</p>
        </article>
        <article className="panel">
          <div className="panel-heading">
            <h2>Recent Activity</h2>
            <FileText size={18} aria-hidden="true" />
          </div>
          <div className="compact-list">
			{requests.slice(0, 5).map((request) => (
              <div key={request.requestId}>
                <span>{request.requestId}</span>
                <strong>{request.status}</strong>
              </div>
			))}
			{requests.length === 0 && <p>No requests have been recorded for this tenant.</p>}
          </div>
        </article>
      </section>
    </>
  );
}

type UsageRange = "24h" | "7d" | "30d" | "custom";

function CustomerUsage({ usage: initial }: { usage?: CustomerUsageResponse }) {
	const [usage, setUsage] = useState<CustomerUsageResponse | undefined>(initial);
	const [range, setRange] = useState<UsageRange>("30d");
	const [customFrom, setCustomFrom] = useState("");
	const [customTo, setCustomTo] = useState("");
	const [loading, setLoading] = useState(false);
	const [error, setError] = useState("");

	useEffect(() => {
		void loadPreset("30d");
	}, []);

	async function load(query: { from?: string; to?: string }) {
		setLoading(true);
		setError("");
		try {
			setUsage(await mockApi.getCustomerUsage(query));
		} catch (reason) {
			setUsage(undefined);
			setError(errorMessage(reason));
		} finally {
			setLoading(false);
		}
	}

	async function loadPreset(next: Exclude<UsageRange, "custom">) {
		setRange(next);
		const hours = next === "24h" ? 24 : next === "7d" ? 24 * 7 : 24 * 30;
		const to = new Date();
		const from = new Date(to.getTime() - hours * 60 * 60 * 1000);
		await load({ from: from.toISOString(), to: to.toISOString() });
	}

	function applyCustomRange() {
		if (!customFrom || !customTo) {
			setError("Choose both From and To before applying a custom range.");
			return;
		}
		if (new Date(customFrom) > new Date(customTo)) {
			setError("From must be earlier than To.");
			return;
		}
		void load({ from: toApiTimestamp(customFrom), to: toApiTimestamp(customTo) });
	}

	const summary = usage?.summary;
	return (
		<>
			<section className="usage-toolbar">
				<div className="usage-range-control" aria-label="Usage time range">
					<button className={range === "24h" ? "active" : ""} onClick={() => void loadPreset("24h")} disabled={loading}>Last 24 hours</button>
					<button className={range === "7d" ? "active" : ""} onClick={() => void loadPreset("7d")} disabled={loading}>Last 7 days</button>
					<button className={range === "30d" ? "active" : ""} onClick={() => void loadPreset("30d")} disabled={loading}>Last 30 days</button>
					<button className={range === "custom" ? "active" : ""} onClick={() => setRange("custom")} disabled={loading}>Custom range</button>
				</div>
				{loading && <span className="inline-loading" role="status">Loading usage</span>}
			</section>
			{range === "custom" && (
				<section className="custom-range" aria-label="Custom usage range">
					<label className="select-field"><span>From</span><input type="datetime-local" value={customFrom} onChange={(event) => setCustomFrom(event.target.value)} /></label>
					<label className="select-field"><span>To</span><input type="datetime-local" value={customTo} onChange={(event) => setCustomTo(event.target.value)} /></label>
					<button className="primary-button" onClick={applyCustomRange} disabled={loading}>Apply range</button>
				</section>
			)}
			{error && <div className="auth-error" role="alert">{error}</div>}
			{!error && !usage && <LoadingState compact />}
			{usage && summary && (
				<>
					<PartialDataBanner show={usage.partialData} />
					<section className="metric-grid">
						<Metric label="Requests" value={summary.requests.toLocaleString()} detail="Calls in selected range" />
						<Metric label="Input / Output Tokens" value={`${summary.inputTokens.toLocaleString()} / ${summary.outputTokens.toLocaleString()}`} detail={`${summary.totalTokens.toLocaleString()} total tokens`} />
						<Metric label="Avg Latency" value={`${summary.avgLatencyMs} ms`} detail="Mean end-to-end latency" />
						<Metric label="P95 Latency" value={`${summary.p95LatencyMs} ms`} detail="95th percentile latency" />
					</section>
					{usage.series.length === 0 && usage.models.length === 0 ? (
						<EmptyState title="No usage data" detail="Gateway usage will appear after this tenant sends requests." />
					) : (
						<>
							<section className="panel">
								<div className="panel-heading"><h2>Usage Trend</h2><BarChart3 size={18} aria-hidden="true" /></div>
								<ResponsiveTable
									columns={["Date", "Requests", "Tokens", "Avg latency"]}
									rows={usage.series.map((row) => ({ Date: row.date, Requests: row.requests.toLocaleString(), Tokens: row.totalTokens.toLocaleString(), "Avg latency": `${row.avgLatencyMs} ms` }))}
								/>
							</section>
							<section className="panel section-gap">
								<div className="panel-heading"><h2>Model Distribution</h2><Network size={18} aria-hidden="true" /></div>
								<ResponsiveTable
									columns={["Model", "Requests", "Tokens"]}
									rows={usage.models.map((row) => ({ Model: row.model, Requests: row.requests.toLocaleString(), Tokens: row.totalTokens.toLocaleString() }))}
								/>
							</section>
						</>
					)}
				</>
			)}
		</>
	);
}

function BenchmarksPage({ rows }: { rows: BenchmarkRun[] }) {
  const [query, setQuery] = useState("");
  const [datasetFilter, setDatasetFilter] = useState("All");
  const [methodFilter, setMethodFilter] = useState("All");
	const [exportState, setExportState] = useState<{ kind: "idle" | "busy" | "error"; message?: string }>({ kind: "idle" });
  const sourceLabel = rows.some((row) => row.source.toLowerCase().includes("demo"))
    ? "Demo data"
    : "BFF / Redis live data";
  const calculatedRows = useMemo(() => calculateBenchmarkImprovements(rows), [rows]);
  const datasets = useMemo(() => ["All", ...Array.from(new Set(rows.map((row) => row.dataset)))], [rows]);
  const filteredRows = useMemo(
    () => filterBenchmarkRows(calculatedRows, { dataset: datasetFilter, method: methodFilter, query }),
    [calculatedRows, datasetFilter, methodFilter, query]
  );
  const comparisonGroups = useMemo(
    () => buildBenchmarkComparisonGroups(calculatedRows.filter((row) => datasetFilter === "All" || row.dataset === datasetFilter)),
    [calculatedRows, datasetFilter]
  );

  async function exportCsv() {
	await exportArtifact(fetchBenchmarkRawCSVExport);
  }

  async function exportReport() {
	await exportArtifact(fetchBenchmarkReportZIPExport);
  }

	async function exportArtifact(load: () => Promise<{ blob: Blob; filename: string }>) {
		setExportState({ kind: "busy" });
		try {
			const artifact = await load();
			downloadBlob(artifact.filename, artifact.blob);
			setExportState({ kind: "idle" });
		} catch (error) {
			setExportState({ kind: "error", message: errorMessage(error) });
		}
	}

  if (rows.length === 0) {
    return <EmptyState title="No benchmark rows" detail="Run a benchmark or connect the BFF benchmark endpoint." />;
  }

  return (
    <>
      <section className="benchmark-actions">
        <div className="benchmark-filters">
          <label className="search-field">
            <span>Search benchmarks</span>
            <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search provider, model, run ID..." />
          </label>
          <label className="select-field">
            <span>Dataset</span>
            <select value={datasetFilter} onChange={(event) => setDatasetFilter(event.target.value)}>
              {datasets.map((dataset) => <option key={dataset}>{dataset}</option>)}
            </select>
          </label>
          <label className="select-field">
            <span>Method</span>
            <select value={methodFilter} onChange={(event) => setMethodFilter(event.target.value)}>
              <option>All</option>
              {COMPARED_METHODS.map((method) => <option key={method}>{method}</option>)}
            </select>
          </label>
        </div>
        <div className="toolbar-actions">
          <button className="secondary-button" onClick={exportCsv} disabled={exportState.kind === "busy"}>
            <Download size={17} aria-hidden="true" />
            Export CSV
          </button>
          <button className="primary-button" onClick={exportReport} disabled={exportState.kind === "busy"}>
            <FileText size={17} aria-hidden="true" />
            Export Report
          </button>
        </div>
      </section>
	  {exportState.kind === "error" ? <div className="operation-notice error" role="alert">Export failed: {exportState.message}</div> : null}
      <PartialDataBanner show={rows.some((row) => row.partialData || row.status !== "passed")} />
      <section className="panel comparison-readiness">
        <div className="panel-heading">
          <h2>Four-Method Comparison Readiness</h2>
          <span className="source-pill">{comparisonGroups.filter((group) => group.complete).length}/{comparisonGroups.length} complete</span>
        </div>
        <div className="comparison-groups">
          {comparisonGroups.map((group) => (
            <div className="comparison-group" key={group.key}>
              <strong>{group.dataset}</strong>
              <span className="setup-summary">{group.rows[0].requestCount} requests · concurrency {group.rows[0].concurrency} · timeout {group.rows[0].timeoutSettingSeconds}s</span>
              <div className="method-statuses">
                {COMPARED_METHODS.map((method) => (
                  <span className={group.presentMethods.includes(method) ? "present" : "missing"} key={method}>
                    {method}: {group.presentMethods.includes(method) ? "Ready" : "Missing"}
                  </span>
                ))}
              </div>
            </div>
          ))}
        </div>
      </section>
      <section className="panel">
        <div className="panel-heading">
          <h2>Benchmark Result Summary</h2>
          <span className="source-pill">{sourceLabel}</span>
        </div>
        <ResponsiveTable
          columns={BENCHMARK_COLUMNS}
          rows={filteredRows.map(benchmarkToRow)}
          variant="benchmark"
        />
        {filteredRows.length === 0 && <EmptyState title="No matching benchmark" detail="Clear the search field to see all methods." compact />}
      </section>
      <section className="chart-grid">
        <MiniBarChart title="Avg Latency Comparison" rows={filteredRows} metric="avgLatencyMs" />
        <MiniBarChart title="Throughput Comparison" rows={filteredRows} metric="throughputRps" />
      </section>
    </>
  );
}

function ProviderHealthPage({ rows }: { rows: ProviderHealth[] }) {
  if (rows.length === 0) {
    return <EmptyState title="No provider health data" detail="Publish a benchmark operational snapshot or check the BFF Redis connection." />;
  }
  return (
    <>
      <PartialDataBanner show={rows.some((row) => row.status !== "Healthy")} />
      <section className="panel">
        <div className="panel-heading">
          <h2>Provider Health</h2>
          <Server size={18} aria-hidden="true" />
        </div>
        <ResponsiveTable
          columns={["Provider", "Target model", "Status", "Avg latency", "Error rate", "Timeout rate", "Last checked"]}
          rows={rows.map((row) => ({
            Provider: row.provider,
            "Target model": row.targetModel,
            Status: row.status,
            "Avg latency": `${row.avgLatencyMs} ms`,
            "Error rate": `${row.errorRate}%`,
            "Timeout rate": `${row.timeoutRate}%`,
            "Last checked": row.lastChecked
          }))}
        />
      </section>
    </>
  );
}

function RequestLogsPage({ rows, customerMode = false }: { rows: RequestLog[]; customerMode?: boolean }) {
  if (rows.length === 0) {
    return <EmptyState title="No request logs" detail="There are no requests for this view yet." />;
  }
  return (
    <section className="panel">
      <div className="panel-heading">
        <h2>{customerMode ? "My Requests" : "Requests / Logs"}</h2>
        <FileText size={18} aria-hidden="true" />
      </div>
      <ResponsiveTable
        columns={["Request ID", "Tenant", "Provider", "Model", "Method", "Latency", "TTFT", "Status", "Error", "Timestamp"]}
        rows={rows.map((row) => ({
          "Request ID": row.requestId,
          Tenant: row.tenant,
          Provider: row.provider,
          Model: row.model,
          Method: row.method,
          Latency: `${row.latencyMs} ms`,
          TTFT: `${row.ttftMs} ms`,
          Status: row.status,
          Error: row.errorMessage || "-",
          Timestamp: row.timestamp
        }))}
      />
    </section>
  );
}

type CustomerRequestFilters = CustomerRequestQuery & {
	status: string;
	model: string;
	from: string;
	to: string;
};

function CustomerRequestsPage({ initial, models }: { initial?: CustomerRequestsResponse; models: string[] }) {
	const [result, setResult] = useState<CustomerRequestsResponse | undefined>(initial);
	const [filters, setFilters] = useState<CustomerRequestFilters>({
		page: initial?.page ?? 1,
		pageSize: initial?.pageSize ?? 25,
		status: "",
		model: "",
		from: "",
		to: ""
	});
	const [loading, setLoading] = useState(false);
	const [error, setError] = useState("");

	useEffect(() => {
		setResult(initial);
	}, [initial]);

	async function load(next: CustomerRequestFilters) {
		setFilters(next);
		setLoading(true);
		setError("");
		try {
			setResult(await mockApi.getCustomerRequests({
				...next,
				from: toApiTimestamp(next.from),
				to: toApiTimestamp(next.to)
			}));
		} catch (reason) {
			setError(errorMessage(reason));
		} finally {
			setLoading(false);
		}
	}

	function update(patch: Partial<CustomerRequestFilters>) {
		void load({ ...filters, ...patch, page: patch.page ?? 1 });
	}

	const rows = result ? mapBffRequestLogs({ logs: result.requests }) : [];
	const total = result?.total ?? 0;
	const page = result?.page ?? filters.page;
	const pageSize = result?.pageSize ?? filters.pageSize;
	const pageCount = Math.max(1, Math.ceil(total / pageSize));
	const firstRow = total === 0 ? 0 : (page - 1) * pageSize + 1;
	const lastRow = Math.min(page * pageSize, total);
	const modelOptions = [...new Set(models)].sort();

	return (
		<>
			<section className="request-page-heading">
				<div><h2>My Requests</h2><p>Tenant-scoped gateway requests with server-side filtering and pagination.</p></div>
				{loading && <span className="inline-loading" role="status">Loading requests</span>}
			</section>
			<section className="request-filters" aria-label="Request filters">
				<label className="select-field">
					<span>Status</span>
					<select value={filters.status} onChange={(event) => update({ status: event.target.value })} disabled={loading}>
						<option value="">All statuses</option>
						<option value="Success">Success</option>
						<option value="Error">Error</option>
						<option value="Timeout">Timeout</option>
					</select>
				</label>
				<label className="select-field">
					<span>Model</span>
					<select value={filters.model} onChange={(event) => update({ model: event.target.value })} disabled={loading}>
						<option value="">All models</option>
						{modelOptions.map((model) => <option value={model} key={model}>{model}</option>)}
					</select>
				</label>
				<label className="select-field">
					<span>From</span>
					<input type="datetime-local" value={filters.from} onChange={(event) => update({ from: event.target.value })} disabled={loading} />
				</label>
				<label className="select-field">
					<span>To</span>
					<input type="datetime-local" value={filters.to} onChange={(event) => update({ to: event.target.value })} disabled={loading} />
				</label>
				<label className="select-field page-size-field">
					<span>Page size</span>
					<select value={filters.pageSize} onChange={(event) => update({ pageSize: Number(event.target.value) })} disabled={loading}>
						<option value="25">25</option>
						<option value="50">50</option>
						<option value="100">100</option>
					</select>
				</label>
				<button className="secondary-button" onClick={() => void load({ page: 1, pageSize: 25, status: "", model: "", from: "", to: "" })} disabled={loading}>Clear filters</button>
			</section>
			{error && <div className="auth-error" role="alert">{error}</div>}
			<PartialDataBanner show={Boolean(result?.partialData)} />
			<section className="panel">
				{!result ? (
					<LoadingState compact />
				) : rows.length === 0 ? (
					<EmptyState title="No request logs" detail="There are no requests matching these filters." compact />
				) : (
					<ResponsiveTable
						columns={["Request ID", "Tenant", "Provider", "Model", "Method", "Latency", "TTFT", "Status", "Error", "Timestamp"]}
						rows={rows.map((row) => ({
							"Request ID": row.requestId,
							Tenant: row.tenant,
							Provider: row.provider,
							Model: row.model,
							Method: row.method,
							Latency: `${row.latencyMs} ms`,
							TTFT: `${row.ttftMs} ms`,
							Status: row.status,
							Error: row.errorMessage || "-",
							Timestamp: row.timestamp
						}))}
					/>
				)}
			</section>
			<nav className="request-pagination" aria-label="Request pages">
				<span>{total === 0 ? "0 requests" : `${firstRow}-${lastRow} of ${total} requests`}</span>
				<div>
					<button className="secondary-button" aria-label="Previous page" onClick={() => update({ page: page - 1 })} disabled={loading || page <= 1}>Previous</button>
					<strong>Page {page} of {pageCount}</strong>
					<button className="secondary-button" aria-label="Next page" onClick={() => update({ page: page + 1 })} disabled={loading || page >= pageCount}>Next</button>
				</div>
			</nav>
		</>
	);
}

function toApiTimestamp(value: string): string {
	return value ? new Date(value).toISOString() : "";
}

function CustomerApiKeys({ keys, onChanged }: { keys?: CustomerApiKeysResponse; onChanged: () => Promise<void> }) {
	const [createdSecret, setCreatedSecret] = useState("");
	const [busy, setBusy] = useState(false);
	const [error, setError] = useState("");

	async function createKey() {
		setBusy(true);
		setError("");
		try {
			const created = await mockApi.createCustomerApiKey();
			setCreatedSecret(created.key);
			await onChanged();
		} catch (reason) {
			setError(errorMessage(reason));
		} finally {
			setBusy(false);
		}
	}

	async function revokeKey(id: string) {
		setBusy(true);
		setError("");
		try {
			await mockApi.revokeCustomerApiKey(id);
			await onChanged();
		} catch (reason) {
			setError(errorMessage(reason));
		} finally {
			setBusy(false);
		}
	}

	if (!keys) {
		return <LoadingState compact />;
	}
  return (
		<>
			<section className="benchmark-actions">
				<div><h2>My API Keys</h2><p>Keys are limited to this authenticated tenant.</p></div>
				<button className="primary-button" onClick={createKey} disabled={busy}><KeyRound size={17} aria-hidden="true" />Create API Key</button>
			</section>
			{createdSecret && (
				<section className="secret-callout" role="status">
					<strong>Copy this key now. It will not be shown again.</strong>
					<code>{createdSecret}</code>
					<button className="secondary-button" onClick={() => setCreatedSecret("")}>Dismiss</button>
				</section>
			)}
			{error && <div className="auth-error" role="alert">{error}</div>}
			<section className="panel">
				{keys.keys.length === 0 ? (
					<EmptyState title="No API keys" detail="Create a key to call the gateway as this tenant." compact />
				) : (
					<div className="table-wrap">
						<table>
							<thead><tr><th>Key</th><th>Scope</th><th>Status</th><th>Created</th><th>Last used</th><th>Action</th></tr></thead>
							<tbody>{keys.keys.map((key) => (
								<tr key={key.id}><td>{key.maskedKey}</td><td>{key.scope}</td><td>{key.status}</td><td>{key.createdAt || "-"}</td><td>{key.lastUsed}</td><td><button className="danger-outline" onClick={() => revokeKey(key.id)} disabled={busy}>Revoke</button></td></tr>
							))}</tbody>
						</table>
					</div>
				)}
			</section>
		</>
  );
}

function CustomerAccount({ session }: { session: MvpSession }) {
	return (
		<section className="panel">
			<div className="panel-heading"><h2>Account</h2><UserRound size={18} aria-hidden="true" /></div>
			<div className="account-grid">
				<div><span>Username</span><strong>{session.user}</strong></div>
				<div><span>Role</span><strong>{session.role}</strong></div>
				<div><span>User ID</span><strong>{session.userId}</strong></div>
				<div><span>Tenant ID</span><strong>{session.tenantId}</strong></div>
			</div>
		</section>
	);
}

function PlaceholderPage({ title }: { title: string }) {
  return (
    <section className="panel placeholder-panel">
      <Lock size={28} aria-hidden="true" />
      <h2>{title}</h2>
      <p>This page is kept as MVP navigation placeholder. It can be connected after the core dashboard, benchmark, export, provider, and request log pages are complete.</p>
    </section>
  );
}

function Metric({ label, value, detail }: { label: string; value: string; detail: string }) {
  return (
    <article className="metric-card">
      <span>{label}</span>
      <strong>{value}</strong>
      <small>{detail}</small>
    </article>
  );
}

function ResponsiveTable({ columns, rows, variant }: { columns: readonly string[]; rows: Array<Record<string, string>>; variant?: "benchmark" }) {
  return (
    <div className={`table-wrap${variant ? ` ${variant}-table` : ""}`}>
      <table>
        <thead>
          <tr>
            {columns.map((column) => (
              <th key={column}>{column}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={Object.values(row).join("|")}>
              {columns.map((column) => (
                <td key={column} data-label={column}>
                  {row[column]}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function MiniBarChart({ title, rows, metric }: { title: string; rows: BenchmarkRun[]; metric: "avgLatencyMs" | "throughputRps" }) {
  const values = rows.map((row) => row[metric] ?? 0);
  const max = Math.max(...values, 0);
  return (
    <section className="panel mini-chart">
      <h2>{title}</h2>
      {rows.map((row, index) => {
        const value = values[index];
        const width = max > 0 ? Math.max(12, (value / max) * 100) : 0;
        return (
          <div className="bar-row" key={benchmarkChartKey(title, row)}>
            <span>{row.method}</span>
            <div>
              <i style={{ width: `${width}%` }} />
            </div>
            <strong>{metric === "avgLatencyMs" ? formatMs(row[metric]) : formatRps(row[metric])}</strong>
          </div>
        );
      })}
    </section>
  );
}

function PartialDataBanner({ show, warnings = [] }: { show: boolean; warnings?: string[] }) {
  if (!show) {
    return null;
  }
  return (
    <div className="partial-banner" role="status">
	  <strong>Partial data:</strong> Available measurements are shown without filling missing values.
	  {warnings.length > 0 ? <span>{warnings.join(" · ")}</span> : null}
    </div>
  );
}

function LoadingState({ compact = false }: { compact?: boolean }) {
  return (
    <div className={compact ? "state-card compact" : "state-card full"}>
      <div className="spinner" />
      <strong>Loading dashboard data</strong>
      <span>Loading data through the authenticated BFF service layer.</span>
    </div>
  );
}

function EmptyState({ title, detail, compact = false }: { title: string; detail: string; compact?: boolean }) {
  return (
    <div className={compact ? "empty-state compact" : "empty-state"}>
      <strong>{title}</strong>
      <span>{detail}</span>
    </div>
  );
}

function ErrorState({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <main className="login-shell">
      <section className="state-card error">
        <ShieldAlert size={34} aria-hidden="true" />
        <strong>Dashboard error state</strong>
        <span>{message}</span>
        <button className="primary-button" onClick={onRetry}>Retry</button>
      </section>
    </main>
  );
}

function NoPermissionState({ role, view }: { role: UserRole; view: MvpView }) {
  return (
    <section className="state-card error">
      <Lock size={34} aria-hidden="true" />
      <strong>No permission</strong>
      <span>{role} cannot access {viewLabel(view)}. Customer accounts are blocked from Admin pages.</span>
    </section>
  );
}

function benchmarkToRow(row: BenchmarkRun): Record<string, string> {
  return {
    "Run ID": row.runId,
    Method: row.method,
    Dataset: row.dataset,
    "Request count": row.requestCount.toLocaleString(),
    Concurrency: row.concurrency.toLocaleString(),
    "Request rate": row.requestRate === null ? "-" : `${row.requestRate} req/s`,
    "Warm-up": `${row.warmUp} requests`,
    "Repeated runs": row.repeatedRuns.toLocaleString(),
    "Timeout setting": `${row.timeoutSettingSeconds} s`,
    Provider: row.provider,
    "Target model": row.targetModel,
    "Gateway version": row.gatewayVersion,
    "Avg latency": formatMs(row.avgLatencyMs),
    "P50 latency": formatMs(row.p50LatencyMs),
    "P95 latency": formatMs(row.p95LatencyMs),
    "P99 latency": formatMs(row.p99LatencyMs),
    TTFT: formatMs(row.ttftMs),
    Throughput: formatRps(row.throughputRps),
    "Success rate": `${row.successRatePct}%`,
    "Error rate": `${row.errorRatePct}%`,
    "Timeout rate": `${row.timeoutRatePct}%`,
    Improvement: row.improvementPct === null ? "-" : `${row.improvementPct}%`,
    "Test date": row.testDate,
    Source: row.source,
    "Raw file path": row.rawFilePath,
    "Export ID": row.exportId,
    Status: row.status,
    "Partial data": row.partialData ? "Yes" : "No"
  };
}

function formatMs(value: number | null): string {
  return value === null ? "-" : `${value.toLocaleString()} ms`;
}

function formatRps(value: number | null): string {
  return value === null ? "-" : `${value.toLocaleString()} req/s`;
}

function downloadBlob(filename: string, blob: Blob) {
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  URL.revokeObjectURL(url);
}

function viewLabel(view: MvpView): string {
  return view
    .split("-")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Unknown dashboard error";
}

function navigateToView(
  view: MvpView,
  managementTab: SystemManagementTab,
  setActiveView: (view: MvpView) => void
) {
  setActiveView(view);
  window.location.hash = dashboardHashFor(view, managementTab);
}

function navigateToManagementTab(
  tab: SystemManagementTab,
  setActiveManagementTab: (tab: SystemManagementTab) => void
) {
  setActiveManagementTab(tab);
  window.location.hash = dashboardHashFor("system-management", tab);
}
