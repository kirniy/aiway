import type { Config, OverviewResponse } from './types'

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
  })

  const payload = await response.json().catch(() => ({}))
  if (!response.ok) {
    throw new Error(payload.error || `HTTP ${response.status}`)
  }
  return payload as T
}

export const api = {
  overview: () => request<OverviewResponse>('/api/overview'),
  saveConfig: (config: Config) =>
    request<Config>('/api/config', {
      method: 'PUT',
      body: JSON.stringify(config),
    }),
  checkNow: () =>
    request('/api/actions/check-now', {
      method: 'POST',
      body: JSON.stringify({}),
    }),
  toggleDns: (enabled: boolean) =>
    request('/api/actions/toggle-dns', {
      method: 'POST',
      body: JSON.stringify({ enabled }),
    }),
  addDomain: (domain: string) =>
    request('/api/actions/domain/add', {
      method: 'POST',
      body: JSON.stringify({ domain }),
    }),
  removeDomain: (domain: string) =>
    request('/api/actions/domain/remove', {
      method: 'POST',
      body: JSON.stringify({ domain }),
    }),
  profileInstall: (profileId: string) =>
    request('/api/actions/profile/install', {
      method: 'POST',
      body: JSON.stringify({ profileId }),
    }),
  profileUninstall: (profileId: string) =>
    request('/api/actions/profile/uninstall', {
      method: 'POST',
      body: JSON.stringify({ profileId }),
    }),
  profileReset: (profileId: string) =>
    request('/api/actions/profile/reset', {
      method: 'POST',
      body: JSON.stringify({ profileId }),
    }),
  profileSync: (profileId: string) =>
    request('/api/actions/profile/sync', {
      method: 'POST',
      body: JSON.stringify({ profileId }),
    }),
  clearLogs: () =>
    request('/api/logs', {
      method: 'DELETE',
    }),
  checkUpdate: () =>
    request('/api/actions/update/check', {
      method: 'POST',
      body: JSON.stringify({}),
    }),
  applyUpdate: () =>
    request('/api/actions/update/apply', {
      method: 'POST',
      body: JSON.stringify({}),
    }),
}
