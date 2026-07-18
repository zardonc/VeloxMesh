import { Download, KeyRound, Pencil, Plus, RefreshCw, Save, Trash2, X } from "lucide-react";
import { FormEvent, ReactNode, useEffect, useMemo, useState } from "react";
import {
  AdminSettings,
  AdminSettingsResponse,
  ApiKeysResponse,
  AuditResponse,
  CreatedAdminApiKey,
	ConfigurationApplication,
  RoutingInput,
  RoutingResponse,
  SYSTEM_MANAGEMENT_TABS,
  SystemManagementTab,
  TenantInput,
  TenantsResponse,
  createApiKey,
  createRoutingRule,
  createTenant,
  deleteApiKey,
  deleteRoutingRule,
  deleteTenant,
  exportAuditCSV,
  fetchAdminApiKeys,
  fetchAdminAudit,
  fetchAdminRouting,
  fetchAdminSettings,
  fetchAdminTenants,
  updateAdminSettings,
  updateRoutingRule,
  updateTenant
} from "./api";

type LoadState = "loading" | "ready" | "error";
type Notice = { tone: "success" | "warning" | "error"; text: string } | null;

const emptyRouting: RoutingInput = { policy: "", selector: "", target: "", status: "Draft" };
const emptyTenant: TenantInput = { tenant: "", owner: "", dailyQuota: "", status: "Healthy" };

export function SystemManagement({
  activeTab,
  onTabChange
}: {
  activeTab: SystemManagementTab;
  onTabChange: (tab: SystemManagementTab) => void;
}) {
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState<Notice>(null);
  const [saving, setSaving] = useState(false);
  const [routing, setRouting] = useState<RoutingResponse | null>(null);
  const [tenants, setTenants] = useState<TenantsResponse | null>(null);
  const [apiKeys, setApiKeys] = useState<ApiKeysResponse | null>(null);
  const [audit, setAudit] = useState<AuditResponse | null>(null);
  const [settings, setSettings] = useState<AdminSettingsResponse | null>(null);
  const [routingForm, setRoutingForm] = useState<RoutingInput | null>(null);
  const [routingOriginal, setRoutingOriginal] = useState("");
  const [tenantForm, setTenantForm] = useState<TenantInput | null>(null);
  const [tenantOriginal, setTenantOriginal] = useState("");
  const [apiKeyForm, setApiKeyForm] = useState<{ tenant: string; scope: string } | null>(null);
  const [createdKey, setCreatedKey] = useState<CreatedAdminApiKey | null>(null);
  const [query, setQuery] = useState("");
  const [auditResult, setAuditResult] = useState("All");

  async function load(tab: SystemManagementTab = activeTab) {
    setLoadState("loading");
    setError("");
    setNotice(null);
    try {
      if (tab === "routing") setRouting(await fetchAdminRouting());
      if (tab === "tenants") setTenants(await fetchAdminTenants());
      if (tab === "api-keys") setApiKeys(await fetchAdminApiKeys());
      if (tab === "audit") setAudit(await fetchAdminAudit());
      if (tab === "settings") setSettings(await fetchAdminSettings());
      setLoadState("ready");
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Unable to load management data.");
      setLoadState("error");
    }
  }

  useEffect(() => {
    setQuery("");
    setAuditResult("All");
    setRoutingForm(null);
    setTenantForm(null);
    setApiKeyForm(null);
    setCreatedKey(null);
    void load(activeTab);
  }, [activeTab]);

  const activeResponse = activeTab === "routing" ? routing
    : activeTab === "tenants" ? tenants
      : activeTab === "api-keys" ? apiKeys
        : activeTab === "audit" ? audit
          : settings;

  async function runMutation(action: () => Promise<unknown>, success: string) {
    setSaving(true);
    setNotice(null);
    try {
			const result = await action();
			const application = configurationApplication(result);
			if (application?.state === "failed" || (application && !application.applied && application.state !== "warning")) {
				setNotice({ tone: "error", text: application.message || "Gateway did not apply the saved configuration." });
				return false;
			}
			const finalNotice = applicationNotice(application, success);
      await load(activeTab);
			setNotice(finalNotice);
      return true;
    } catch (caught) {
      setNotice({ tone: "error", text: caught instanceof Error ? caught.message : "Operation failed." });
      return false;
    } finally {
      setSaving(false);
    }
  }

  return (
    <section className="panel management-panel">
      <div className="panel-heading management-heading">
        <div>
          <h2>System Management</h2>
          <span className="management-source">Configuration and access controls</span>
        </div>
        <button className="icon-button" type="button" title="Refresh current tab" aria-label="Refresh current tab" onClick={() => void load()} disabled={loadState === "loading"}>
          <RefreshCw size={17} aria-hidden="true" />
        </button>
      </div>
      <div className="management-tabs" role="tablist" aria-label="System management sections">
        {SYSTEM_MANAGEMENT_TABS.map((tab) => (
          <button
            id={`management-tab-${tab.id}`}
            key={tab.id}
            type="button"
            role="tab"
            aria-selected={tab.id === activeTab}
            aria-controls={`management-panel-${tab.id}`}
            tabIndex={tab.id === activeTab ? 0 : -1}
            onClick={() => onTabChange(tab.id)}
          >
            {tab.label}
          </button>
        ))}
      </div>
      <div id={`management-panel-${activeTab}`} className="management-tab-panel" role="tabpanel" aria-label={SYSTEM_MANAGEMENT_TABS.find((tab) => tab.id === activeTab)?.label} aria-labelledby={`management-tab-${activeTab}`}>
        {loadState === "loading" && <ManagementLoading />}
        {loadState === "error" && <ManagementError message={error} onRetry={() => void load()} />}
        {loadState === "ready" && (
          <>
            {activeResponse?.partialData && <PartialDataNotice warnings={activeResponse.warnings} source={activeResponse.source} />}
			{activeResponse?.source && !activeResponse.partialData && <div className="management-live-source" role="status">Source: {activeResponse.source}</div>}
            {notice && <div className={`operation-notice ${notice.tone}`} role={notice.tone === "error" ? "alert" : "status"}>{notice.text}</div>}
            {activeTab === "routing" && routing && (
              <RoutingTab
                data={routing}
                form={routingForm}
                original={routingOriginal}
                saving={saving}
                onAdd={() => { setRoutingOriginal(""); setRoutingForm({ ...emptyRouting }); }}
                onEdit={(rule) => { setRoutingOriginal(rule.policy); setRoutingForm({ ...rule }); }}
                onCancel={() => setRoutingForm(null)}
                onFormChange={setRoutingForm}
                onSave={(event) => {
                  event.preventDefault();
                  if (!routingForm) return;
                  void runMutation(
                    () => routingOriginal ? updateRoutingRule(routingOriginal, routingForm) : createRoutingRule(routingForm),
                    routingOriginal ? "Routing rule updated." : "Routing rule created."
                  ).then((succeeded) => { if (succeeded) setRoutingForm(null); });
                }}
                onDelete={(policy) => {
                  if (window.confirm(`Delete routing rule ${policy}?`)) void runMutation(() => deleteRoutingRule(policy), "Routing rule deleted.");
                }}
              />
            )}
            {activeTab === "tenants" && tenants && (
              <TenantsTab
                data={tenants}
                form={tenantForm}
                original={tenantOriginal}
                saving={saving}
                onAdd={() => { setTenantOriginal(""); setTenantForm({ ...emptyTenant }); }}
                onEdit={(tenant) => { setTenantOriginal(tenant.tenant); setTenantForm({ ...tenant }); }}
                onCancel={() => setTenantForm(null)}
                onFormChange={setTenantForm}
                onSave={(event) => {
                  event.preventDefault();
                  if (!tenantForm) return;
                  void runMutation(
                    () => tenantOriginal ? updateTenant(tenantOriginal, tenantForm) : createTenant(tenantForm),
                    tenantOriginal ? "Tenant updated." : "Tenant created."
                  ).then((succeeded) => { if (succeeded) setTenantForm(null); });
                }}
                onDelete={(tenant) => {
                  if (window.confirm(`Delete tenant ${tenant}?`)) void runMutation(() => deleteTenant(tenant), "Tenant deleted.");
                }}
              />
            )}
            {activeTab === "api-keys" && apiKeys && (
              <ApiKeysTab
                data={apiKeys}
                form={apiKeyForm}
                createdKey={createdKey}
                saving={saving}
                onAdd={() => setApiKeyForm({ tenant: "", scope: "gateway:invoke" })}
                onCancel={() => setApiKeyForm(null)}
                onDismissSecret={() => setCreatedKey(null)}
                onFormChange={setApiKeyForm}
                onSave={async (event) => {
                  event.preventDefault();
                  if (!apiKeyForm) return;
                  setSaving(true);
                  setNotice(null);
                  try {
                    const issued = await createApiKey(apiKeyForm);
                    setCreatedKey(issued);
                    setApiKeyForm(null);
                    setApiKeys(await fetchAdminApiKeys());
                    setNotice({ tone: "success", text: "API key issued." });
                  } catch (caught) {
                    setNotice({ tone: "error", text: caught instanceof Error ? caught.message : "Unable to issue API key." });
                  } finally {
                    setSaving(false);
                  }
                }}
                onDelete={(id) => {
                  if (window.confirm(`Revoke API key ${id}?`)) void runMutation(() => deleteApiKey(id), "API key revoked.");
                }}
              />
            )}
            {activeTab === "audit" && audit && <AuditTab data={audit} query={query} result={auditResult} onQueryChange={setQuery} onResultChange={setAuditResult} />}
            {activeTab === "settings" && settings && (
              <SettingsTab
                data={settings}
                saving={saving}
                onSave={async (next) => {
                  setSaving(true);
                  setNotice(null);
                  try {
                    const updated = await updateAdminSettings(next);
                    setSettings(updated);
                    setNotice({ tone: "success", text: "Settings saved." });
                  } catch (caught) {
                    setNotice({ tone: "error", text: caught instanceof Error ? caught.message : "Unable to save settings." });
                  } finally {
                    setSaving(false);
                  }
                }}
              />
            )}
          </>
        )}
      </div>
    </section>
  );
}

function configurationApplication(value: unknown): ConfigurationApplication | undefined {
	if (!value || typeof value !== "object" || !("application" in value)) return undefined;
	return (value as { application?: ConfigurationApplication }).application;
}

export function applicationNotice(application: ConfigurationApplication | undefined, fallback: string): Exclude<Notice, null> {
	if (!application) return { tone: "success", text: fallback };
	const evidence = [
		`revision ${application.revision}`,
		application.providerId ? `provider ${application.providerId}` : "",
		application.route ? `route ${application.route}` : "",
		application.requestId ? `request ${application.requestId}` : ""
	].filter(Boolean).join("; ");
	if (application.state === "warning") {
		return { tone: "warning", text: `${application.message || "Configuration applied, but live verification is incomplete."}${evidence ? ` (${evidence})` : ""}` };
	}
	if (application.state === "failed") {
		return { tone: "error", text: application.message || "Gateway did not apply the saved configuration." };
	}
	if (application.state === "verified") {
		return { tone: "success", text: `Configuration applied and verified${evidence ? ` (${evidence})` : ""}.` };
	}
	return { tone: "success", text: `Configuration applied${evidence ? ` (${evidence})` : ""}.` };
}

function PartialDataNotice({ warnings, source }: { warnings?: string[]; source?: string }) {
  return (
    <div className="partial-banner management-partial" role="status">
      <strong>Partial data</strong>
      <span>{warnings?.[0] ?? "This data is not yet connected to the production control plane."}</span>
      <small>Source: {source ?? "unknown"}</small>
    </div>
  );
}

function ManagementLoading() {
  return <div className="state-card"><span className="spinner" aria-hidden="true" /><strong>Loading configuration</strong></div>;
}

function ManagementError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return <div className="state-card error" role="alert"><strong>Unable to load configuration</strong><span>{message}</span><button className="secondary-button" type="button" onClick={onRetry}>Retry</button></div>;
}

function ManagementToolbar({ query, onQueryChange, action, actionLabel, actionDisabled = false }: { query: string; onQueryChange: (value: string) => void; action: () => void; actionLabel: string; actionDisabled?: boolean }) {
  return (
    <div className="management-toolbar">
      <label className="search-field"><span>Search</span><input value={query} onChange={(event) => onQueryChange(event.target.value)} placeholder="Filter rows" /></label>
      <button className="primary-button" type="button" onClick={action} disabled={actionDisabled}><Plus size={16} aria-hidden="true" />{actionLabel}</button>
    </div>
  );
}

function RoutingTab({ data, form, original, saving, onAdd, onEdit, onCancel, onFormChange, onSave, onDelete }: {
  data: RoutingResponse;
  form: RoutingInput | null;
  original: string;
  saving: boolean;
  onAdd: () => void;
  onEdit: (rule: RoutingInput) => void;
  onCancel: () => void;
  onFormChange: (value: RoutingInput) => void;
  onSave: (event: FormEvent) => void;
  onDelete: (policy: string) => void;
}) {
  const [query, setQuery] = useState("");
  const rows = useMemo(() => data.rules.filter((rule) => Object.values(rule).some((value) => String(value).toLowerCase().includes(query.toLowerCase()))), [data, query]);
  return <>
	<ManagementToolbar query={query} onQueryChange={setQuery} action={onAdd} actionLabel={data.singleton ? "Global routing only" : "Add routing rule"} actionDisabled={data.singleton} />
	{data.singleton && <div className="management-contract-note">Gateway routing is a single global configuration. Edit it to apply changes. Revision {data.revision ?? "unknown"}.</div>}
    {form && <RoutingForm value={form} editing={Boolean(original)} saving={saving} onChange={onFormChange} onSubmit={onSave} onCancel={onCancel} />}
    {rows.length === 0 ? <EmptyRows label="routing rules" /> : <ManagementTable columns={["Policy", "Selector", "Target", "Status", "Actions"]} rows={rows.map((rule) => ({
      key: rule.policy,
      cells: [rule.policy, rule.selector, rule.target, rule.status],
      actions: <><IconAction label={`Edit ${rule.policy}`} title="Edit routing rule" onClick={() => onEdit(rule)} icon={<Pencil size={15} />} />{!data.singleton && <IconAction label={`Delete ${rule.policy}`} title="Delete routing rule" danger onClick={() => onDelete(rule.policy)} icon={<Trash2 size={15} />} />}</>
    }))} />}
  </>;
}

function RoutingForm({ value, editing, saving, onChange, onSubmit, onCancel }: { value: RoutingInput; editing: boolean; saving: boolean; onChange: (value: RoutingInput) => void; onSubmit: (event: FormEvent) => void; onCancel: () => void }) {
  return <form className="management-form" onSubmit={onSubmit}>
    <label>Policy<input value={value.policy} onChange={(event) => onChange({ ...value, policy: event.target.value })} required /></label>
    <label>Selector<input value={value.selector} onChange={(event) => onChange({ ...value, selector: event.target.value })} required /></label>
    <label>Target<input value={value.target} onChange={(event) => onChange({ ...value, target: event.target.value })} required /></label>
    <label>Routing status<select value={value.status} onChange={(event) => onChange({ ...value, status: event.target.value })}><option>Active</option><option>Draft</option><option>Enforced</option><option>Inactive</option></select></label>
    <FormActions saveLabel="Save routing rule" editing={editing} saving={saving} onCancel={onCancel} />
  </form>;
}

function TenantsTab({ data, form, original, saving, onAdd, onEdit, onCancel, onFormChange, onSave, onDelete }: {
  data: TenantsResponse;
  form: TenantInput | null;
  original: string;
  saving: boolean;
  onAdd: () => void;
  onEdit: (tenant: TenantInput) => void;
  onCancel: () => void;
  onFormChange: (value: TenantInput) => void;
  onSave: (event: FormEvent) => void;
  onDelete: (tenant: string) => void;
}) {
  const [query, setQuery] = useState("");
  const rows = useMemo(() => data.tenants.filter((tenant) => Object.values(tenant).some((value) => String(value).toLowerCase().includes(query.toLowerCase()))), [data, query]);
  return <>
    <ManagementToolbar query={query} onQueryChange={setQuery} action={onAdd} actionLabel="Add tenant" />
    {form && <form className="management-form" onSubmit={onSave}>
      <label>Tenant ID<input value={form.tenant} onChange={(event) => onFormChange({ ...form, tenant: event.target.value })} required /></label>
      <label>Owner<input value={form.owner} onChange={(event) => onFormChange({ ...form, owner: event.target.value })} required /></label>
      <label>Daily quota<input value={form.dailyQuota} onChange={(event) => onFormChange({ ...form, dailyQuota: event.target.value })} required /></label>
      <label>Tenant status<select value={form.status} onChange={(event) => onFormChange({ ...form, status: event.target.value })}><option>Healthy</option><option>Rate Limited</option><option>Inactive</option></select></label>
      <FormActions saveLabel="Save tenant" editing={Boolean(original)} saving={saving} onCancel={onCancel} />
    </form>}
    {rows.length === 0 ? <EmptyRows label="tenants" /> : <ManagementTable columns={["Tenant", "Owner", "Daily Quota", "Status", "Actions"]} rows={rows.map((tenant) => ({
      key: tenant.tenant,
      cells: [tenant.tenant, tenant.owner, tenant.dailyQuota, tenant.status],
      actions: <><IconAction label={`Edit ${tenant.tenant}`} title="Edit tenant" onClick={() => onEdit(tenant)} icon={<Pencil size={15} />} /><IconAction label={`Delete ${tenant.tenant}`} title="Delete tenant" danger onClick={() => onDelete(tenant.tenant)} icon={<Trash2 size={15} />} /></>
    }))} />}
  </>;
}

function ApiKeysTab({ data, form, createdKey, saving, onAdd, onCancel, onDismissSecret, onFormChange, onSave, onDelete }: {
  data: ApiKeysResponse;
  form: { tenant: string; scope: string } | null;
  createdKey: CreatedAdminApiKey | null;
  saving: boolean;
  onAdd: () => void;
  onCancel: () => void;
  onDismissSecret: () => void;
  onFormChange: (value: { tenant: string; scope: string }) => void;
  onSave: (event: FormEvent) => void;
  onDelete: (id: string) => void;
}) {
  const [query, setQuery] = useState("");
  const rows = useMemo(() => data.keys.filter((key) => [key.key, key.tenant, key.scope, key.status ?? ""].some((value) => value.toLowerCase().includes(query.toLowerCase()))), [data, query]);
  return <>
    <ManagementToolbar query={query} onQueryChange={setQuery} action={onAdd} actionLabel="Issue API key" />
    {createdKey && <div className="secret-callout management-secret" role="status"><strong>Copy this key now. It will not be shown again.</strong><code>{createdKey.key}</code><button className="secondary-button" type="button" onClick={onDismissSecret}>Dismiss API key</button></div>}
    {form && <form className="management-form compact-form" onSubmit={onSave}>
      <label>API key tenant<input value={form.tenant} onChange={(event) => onFormChange({ ...form, tenant: event.target.value })} required /></label>
      <label>Scope<input value={form.scope} onChange={(event) => onFormChange({ ...form, scope: event.target.value })} required /></label>
      <FormActions saveLabel="Create API key" editing={false} saving={saving} onCancel={onCancel} />
    </form>}
    {rows.length === 0 ? <EmptyRows label="API keys" /> : <ManagementTable columns={["Key", "Tenant", "Scope", "Status", "Created", "Last Used", "Actions"]} rows={rows.map((key) => ({
      key: key.id ?? key.key,
      cells: [key.key, key.tenant, key.scope, key.status ?? "Unknown", formatDate(key.createdAt), key.lastUsed],
      actions: <IconAction label={`Revoke ${key.id ?? key.key}`} title="Revoke API key" danger onClick={() => onDelete(key.id ?? key.key)} icon={<Trash2 size={15} />} />
    }))} />}
  </>;
}

function AuditTab({ data, query, result, onQueryChange, onResultChange }: { data: AuditResponse; query: string; result: string; onQueryChange: (value: string) => void; onResultChange: (value: string) => void }) {
  const rows = useMemo(() => data.events.filter((event) => {
    const matchesQuery = Object.values(event).some((value) => value.toLowerCase().includes(query.toLowerCase()));
    return matchesQuery && (result === "All" || event.result === result);
  }), [data, query, result]);

  async function downloadAudit() {
    const csv = await exportAuditCSV();
    const url = URL.createObjectURL(new Blob([csv], { type: "text/csv;charset=utf-8" }));
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = "veloxmesh-audit.csv";
    anchor.click();
    URL.revokeObjectURL(url);
  }

  return <>
    <div className="management-toolbar audit-toolbar">
      <label className="search-field"><span>Search</span><input aria-label="Search audit events" value={query} onChange={(event) => onQueryChange(event.target.value)} placeholder="Actor or action" /></label>
      <label className="select-field"><span>Result</span><select value={result} onChange={(event) => onResultChange(event.target.value)}><option>All</option><option>Success</option><option>Failed</option><option>Rate Limited</option></select></label>
      <button className="secondary-button" type="button" onClick={() => void downloadAudit()}><Download size={16} aria-hidden="true" />Export audit CSV</button>
    </div>
    {rows.length === 0 ? <EmptyRows label="audit events" /> : <ManagementTable columns={["Time", "Actor", "Action", "Result"]} rows={rows.map((event, index) => ({ key: `${event.time}-${event.action}-${index}`, cells: [event.time, event.actor, event.action, event.result] }))} />}
  </>;
}

function SettingsTab({ data, saving, onSave }: { data: AdminSettingsResponse; saving: boolean; onSave: (settings: AdminSettings) => Promise<void> }) {
  const [form, setForm] = useState<AdminSettings>(data.settings);
  useEffect(() => setForm(data.settings), [data]);
  return <div className="settings-layout">
    <form className="settings-form" onSubmit={(event) => { event.preventDefault(); void onSave(form); }}>
      <label>Default provider<input value={form.defaultProvider} onChange={(event) => setForm({ ...form, defaultProvider: event.target.value })} required /></label>
      <label>Default model<input value={form.defaultModel} onChange={(event) => setForm({ ...form, defaultModel: event.target.value })} required /></label>
      <label>Request timeout seconds<input type="number" min="1" max="600" value={form.requestTimeoutSeconds} onChange={(event) => setForm({ ...form, requestTimeoutSeconds: Number(event.target.value) })} required /></label>
      <label>Data retention days<input type="number" min="1" max="3650" value={form.dataRetentionDays} onChange={(event) => setForm({ ...form, dataRetentionDays: Number(event.target.value) })} required /></label>
      <button className="primary-button" type="submit" disabled={saving}><Save size={16} aria-hidden="true" />{saving ? "Saving" : "Save settings"}</button>
    </form>
    <section className="integration-status" aria-label="Integration status">
      <h3>Integration status</h3>
      {Object.entries(data.integrations).map(([name, status]) => <div key={name}><span>{name}</span><strong className={status === "Configured" ? "configured" : "not-configured"}>{status}</strong></div>)}
    </section>
  </div>;
}

function FormActions({ saveLabel, editing, saving, onCancel }: { saveLabel: string; editing: boolean; saving: boolean; onCancel: () => void }) {
  return <div className="form-actions"><button className="primary-button" type="submit" disabled={saving}><Save size={16} aria-hidden="true" />{saving ? "Saving" : saveLabel}</button><button className="secondary-button" type="button" onClick={onCancel}><X size={16} aria-hidden="true" />Cancel {editing ? "edit" : "create"}</button></div>;
}

function ManagementTable({ columns, rows }: { columns: string[]; rows: Array<{ key: string; cells: string[]; actions?: ReactNode }> }) {
  return <div className="table-wrap management-table"><table><thead><tr>{columns.map((column) => <th key={column}>{column}</th>)}</tr></thead><tbody>{rows.map((row) => <tr key={row.key}>{row.cells.map((cell, index) => <td key={`${row.key}-${columns[index]}`} data-label={columns[index]}>{cell}</td>)}{row.actions && <td data-label="Actions"><div className="row-actions">{row.actions}</div></td>}</tr>)}</tbody></table></div>;
}

function IconAction({ label, title, danger, onClick, icon }: { label: string; title: string; danger?: boolean; onClick: () => void; icon: ReactNode }) {
  return <button className={`icon-button${danger ? " danger" : ""}`} type="button" aria-label={label} title={title} onClick={onClick}>{icon}</button>;
}

function EmptyRows({ label }: { label: string }) {
  return <div className="empty-state compact"><KeyRound size={22} aria-hidden="true" /><strong>No {label}</strong><span>Adjust the filter or add a new item.</span></div>;
}

function formatDate(value?: string) {
  if (!value) return "Unknown";
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString();
}
