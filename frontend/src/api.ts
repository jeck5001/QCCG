/**
 * HTTP API adapter for the headless web UI.
 *
 * Mirrors all Wails Go bindings used by the frontend, routing calls through
 * a REST API served at the same origin instead of the Wails IPC layer.
 *
 * Import types from the existing Wails v3 binding models so that consumers
 * can swap this module in without any type changes.
 */

// ── Re-export types from existing Wails v3 bindings ──────────────────────
export type {
  Account,
  OAuthSession,
  QuotaBucket,
  QuotaInfo,
  Settings,
  Status,
} from "../bindings/qccg/account/models";

export type {
  ClientConfig,
  ClientConfigFile,
  QoderModel,
} from "../bindings/qccg/models";

export type { Entry, Level, LogPage } from "../bindings/qccg/logger/models";

export type { UpdateInfo } from "../bindings/qccg/internal/updater/models";

// ── Helpers ──────────────────────────────────────────────────────────────

const API_BASE = "/api/v1";

/** Returns `true` when running in a browser (not inside a Wails desktop shell). */
export function isWebMode(): boolean {
  return typeof window !== "undefined" && !(window as any).__WAILS__;
}

/** Returns the API base URL used by all requests. */
export function getApiBaseUrl(): string {
  return API_BASE;
}

/**
 * Thin wrapper around `fetch` that:
 *  - prefixes the URL with the API base
 *  - sets `Content-Type: application/json` for requests with a body
 *  - rejects on non-2xx responses with a readable error message
 */
async function request<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const url = `${API_BASE}${path}`;
  const headers: Record<string, string> = {
    ...(options.headers as Record<string, string> | undefined),
  };

  if (options.body !== undefined && !headers["Content-Type"]) {
    headers["Content-Type"] = "application/json";
  }

  const res = await fetch(url, {
    ...options,
    headers,
  });

  if (!res.ok) {
    let message = `API error ${res.status}`;
    try {
      const body = await res.json();
      if (body.error || body.message) {
        message = body.error || body.message;
      }
    } catch {
      // response body was not JSON – keep the generic message
    }
    throw new Error(message);
  }

  // 204 No Content or DELETE with no body
  if (res.status === 204 || res.headers.get("content-length") === "0") {
    return undefined as unknown as T;
  }

  return res.json() as Promise<T>;
}

// Convenience helpers
const get = <T>(path: string) => request<T>(path);
const del = <T>(path: string) => request<T>(path, { method: "DELETE" });
const post = <T>(path: string, body?: unknown) =>
  request<T>(path, { method: "POST", body: body !== undefined ? JSON.stringify(body) : undefined });
const put = <T>(path: string, body?: unknown) =>
  request<T>(path, { method: "PUT", body: body !== undefined ? JSON.stringify(body) : undefined });

// ── Account types (inline for request bodies) ────────────────────────────

interface AddAccountRequest {
  pat: string;
  region: string;
}

interface ReorderAccountsRequest {
  ids: string[];
}

// ── Accounts ─────────────────────────────────────────────────────────────

import type { Account } from "../bindings/qccg/account/models";
import type { QuotaInfo } from "../bindings/qccg/account/models";
import type { OAuthSession } from "../bindings/qccg/account/models";
import type { Settings } from "../bindings/qccg/account/models";
import type { Status } from "../bindings/qccg/account/models";
import type { ClientConfig } from "../bindings/qccg/models";
import type { ClientConfigFile } from "../bindings/qccg/models";
import type { QoderModel } from "../bindings/qccg/models";
import type { LogPage } from "../bindings/qccg/logger/models";
import type { UpdateInfo } from "../bindings/qccg/internal/updater/models";

export function listAccounts(): Promise<Account[]> {
  return get<Account[]>("/accounts");
}

export function addAccountByPAT(pat: string, region: string): Promise<Account> {
  return post<Account>("/accounts", { pat, region });
}

export function deleteAccount(id: string): Promise<void> {
  return del(`/accounts/${encodeURIComponent(id)}`);
}

export function setActiveAccount(id: string): Promise<void> {
  return put(`/accounts/${encodeURIComponent(id)}/active`);
}

export function reorderAccounts(ids: string[]): Promise<void> {
  return put("/accounts/reorder", { ids });
}

export function getAccountQuota(accountID: string): Promise<QuotaInfo | null> {
  return get<QuotaInfo | null>(`/accounts/${encodeURIComponent(accountID)}/quota`);
}

// ── OAuth ────────────────────────────────────────────────────────────────

export function startOAuthLogin(region: string): Promise<OAuthSession | null> {
  return post<OAuthSession | null>("/oauth/start", { region });
}

export function waitOAuthLogin(loginID: string): Promise<Account> {
  return get<Account>(`/oauth/wait/${encodeURIComponent(loginID)}`);
}

export function cancelOAuthLogin(loginID: string): Promise<void> {
  return post(`/oauth/cancel/${encodeURIComponent(loginID)}`);
}

// ── Bridge ───────────────────────────────────────────────────────────────

export function getStatus(): Promise<Status> {
  return get<Status>("/status");
}

export function startBridge(): Promise<Status> {
  return post<Status>("/bridge/start");
}

export function stopBridge(): Promise<Status> {
  return post<Status>("/bridge/stop");
}

// ── Client Configs ───────────────────────────────────────────────────────

interface SaveAdditionalConfigRequest {
  path: string;
  format: string;
  content: string;
}

interface ApplyConfigRequest {
  model: string;
}

export function getClientConfigs(): Promise<ClientConfig[]> {
  return get<ClientConfig[]>("/client-configs");
}

export function readClientConfigFile(clientType: string): Promise<ClientConfigFile> {
  return get<ClientConfigFile>(`/client-configs/${encodeURIComponent(clientType)}`);
}

export function saveClientConfigFile(clientType: string, content: string): Promise<void> {
  return put(`/client-configs/${encodeURIComponent(clientType)}`, { content });
}

export function saveAdditionalClientConfigFile(
  clientType: string,
  path: string,
  format: string,
  content: string,
): Promise<void> {
  return put(`/client-configs/${encodeURIComponent(clientType)}/additional`, { path, format, content });
}

export function applyClientConfig(clientType: string, model: string): Promise<void> {
  return post(`/client-configs/${encodeURIComponent(clientType)}/apply`, { model });
}

export function removeClientConfig(clientType: string): Promise<void> {
  return del(`/client-configs/${encodeURIComponent(clientType)}`);
}

export function backupClientConfigFile(clientType: string): Promise<void> {
  return post(`/client-configs/${encodeURIComponent(clientType)}/backup`);
}

export function hasClientConfigBackup(clientType: string): Promise<boolean> {
  return get<{ has_backup: boolean }>(`/client-configs/${encodeURIComponent(clientType)}/backup`)
    .then((r) => r.has_backup);
}

export function restoreClientConfigFile(clientType: string): Promise<void> {
  return post(`/client-configs/${encodeURIComponent(clientType)}/restore`);
}

// ── Settings ─────────────────────────────────────────────────────────────

export function getSettings(): Promise<Settings | null> {
  return get<Settings | null>("/settings");
}

export function saveSettings(s: Settings | null): Promise<void> {
  return put("/settings", s);
}

// ── Logs ─────────────────────────────────────────────────────────────────

export function getLogsSince(afterSeq: number, limit: number): Promise<LogPage> {
  const params = new URLSearchParams({
    after_seq: String(afterSeq),
    limit: String(limit),
  });
  return get<LogPage>(`/logs?${params.toString()}`);
}

export function clearLogs(): Promise<void> {
  return del("/logs");
}

// ── Models ───────────────────────────────────────────────────────────────

export function listQoderModels(): Promise<QoderModel[]> {
  return get<QoderModel[]>("/models");
}

// ── Update ───────────────────────────────────────────────────────────────

export function getVersion(): Promise<string> {
  return get<{ version: string }>("/version").then((r) => r.version);
}

export function checkUpdate(): Promise<UpdateInfo | null> {
  return get<UpdateInfo | null>("/update/check");
}

export function applyUpdate(): Promise<void> {
  return post("/update/apply");
}

// ── Cleanup ──────────────────────────────────────────────────────────────

export function cleanupAllData(): Promise<void> {
  return post("/cleanup");
}
