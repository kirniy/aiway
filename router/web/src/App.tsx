import { useEffect, useMemo, useState } from 'react'
import { api } from './api'
import type { Config, OverviewResponse, Profile, ProfileStatus, UpdateInfo } from './types'

type RouteKey = '/routing' | '/safety' | '/diagnostics' | '/settings'

const routes: { path: RouteKey; label: string; short: string }[] = [
  { path: '/routing', label: 'Маршрутизация', short: 'Route' },
  { path: '/safety', label: 'Фейлсейф', short: 'Safe' },
  { path: '/diagnostics', label: 'Диагностика', short: 'Diag' },
  { path: '/settings', label: 'Настройки', short: 'Cfg' },
]

const initialRoute = (): RouteKey => {
  const current = window.location.pathname as RouteKey
  return routes.some((route) => route.path === current) ? current : '/routing'
}

function App() {
  const [route, setRoute] = useState<RouteKey>(initialRoute)
  const [overview, setOverview] = useState<OverviewResponse | null>(null)
  const [draftConfig, setDraftConfig] = useState<Config | null>(null)
  const [error, setError] = useState('')
  const [newDomain, setNewDomain] = useState('')
  const [updateInfo, setUpdateInfo] = useState<UpdateInfo | null>(null)

  const refresh = async () => {
    try {
      setError('')
      const payload = await api.overview()
      setOverview(payload)
      setDraftConfig(payload.config)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Не удалось загрузить данные')
    }
  }

  useEffect(() => {
    void refresh()
    const onPopState = () => setRoute(initialRoute())
    window.addEventListener('popstate', onPopState)
    return () => window.removeEventListener('popstate', onPopState)
  }, [])

  const activeProfile = useMemo(() => {
    if (!overview) return undefined
    return overview.profiles.find((profile) => profile.id === overview.config.activeId)
  }, [overview])

  const activeStatus: ProfileStatus | undefined = activeProfile
    ? overview?.statuses[activeProfile.id]
    : undefined

	const navigate = (path: RouteKey) => {
    window.history.pushState({}, '', path)
    setRoute(path)
  }

  const run = async (_label: string, action: () => Promise<unknown>) => {
    try {
      setError('')
      await action()
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Операция завершилась с ошибкой')
    }
  }

	if (!overview || !draftConfig) {
		return <div className="screen-loading">Загружаю AIWAY Manager...</div>
	}

	const customDomains = activeStatus?.customDomains?.length ? activeStatus.customDomains : (draftConfig.routing.customDomains ?? [])
	const logs = overview.logs ?? []
	const dnsEndpoint = draftConfig.routing.upstreamAddress || activeProfile?.host || 'Не задан'
	const dnsSni = draftConfig.routing.upstreamSni || activeProfile?.domain || 'Не задан'
	const actualDnsOn = Boolean(overview.routerDns?.active)
	const actualDnsEndpoint = overview.routerDns?.address || dnsEndpoint
	const actualDnsSni = overview.routerDns?.sni || dnsSni
	const enabledDomainCount = activeStatus?.serviceCount || Array.from(
		new Set([
			...draftConfig.routing.services.filter((service) => service.enabled).flatMap((service) => service.domains),
			...customDomains,
		]),
	).length

  const failsafe = draftConfig.routing.failsafeActive
	const managedProfile = Boolean(activeProfile?.host?.trim())
	const canManageVps = Boolean(managedProfile && activeStatus?.reachable)
	const canManageDomains = Boolean(canManageVps)
	const canMutateManagedVps = Boolean(canManageVps && activeStatus?.installState !== 'legacy')

  return (
    <div className="shell">
      <header className="topbar">
        <div className="topbar-inner">
          <div className="logo-group">
            <a className="logo-mark" href="/routing" onClick={(event) => { event.preventDefault(); navigate('/routing') }}>
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <circle cx="6" cy="7" r="2" />
                <circle cx="18" cy="7" r="2" />
                <circle cx="12" cy="17" r="2" />
                <path d="M8 7h8" />
                <path d="M7.5 8.5l3.2 5.2" />
                <path d="M16.5 8.5l-3.2 5.2" />
              </svg>
              <span className="logo-text">AIWAY Manager</span>
            </a>
            <span className="version-badge">v{overview.version}</span>
          </div>

          <nav className="topbar-nav">
            {routes.map((item) => (
              <a
                key={item.path}
                href={item.path}
                className={`topbar-link ${route === item.path ? 'active' : ''}`}
                onClick={(event) => {
                  event.preventDefault()
                  navigate(item.path)
                }}
              >
                {item.label}
              </a>
            ))}
          </nav>

          <div className="topbar-actions">
            <span className={`status-pill ${activeStatus?.reachable ? 'ok' : 'bad'}`}>
              {managedProfile ? (activeStatus?.reachable ? 'SSH OK' : 'SSH ERR') : 'DNS'}
            </span>
            {failsafe && <span className="status-pill warn">SAFE</span>}
            <button className="icon-button" title="Проверить сейчас" onClick={() => void run('check', () => api.checkNow())}>
              ↻
            </button>
            <button
              className={`dns-toggle-inline ${actualDnsOn && !failsafe ? 'on' : 'off'}`}
              title={actualDnsOn ? 'Отключить DNS-трюк' : 'Включить DNS-трюк'}
              onClick={() => void run('dns', () => api.toggleDns(!actualDnsOn))}
              aria-pressed={actualDnsOn && !failsafe}
            >
              <span className="dns-toggle-label">AIWAY DNS</span>
              <span className="dns-toggle-switch" aria-hidden="true">
                <span className="dns-toggle-thumb" />
              </span>
            </button>
          </div>
        </div>
      </header>
      <main className="page-container has-padding">
        {error && <div className="alert alert-error">{error}</div>}

        <section className="overview-strip">
          <OverviewCard
            label="Режим"
            value={actualDnsOn ? 'AIWAY DNS активен' : 'AIWAY DNS выключен'}
            detail={failsafe ? 'Фейлсейф ограничивает трафик' : 'Маршрутится через основной WAN'}
          />
          <OverviewCard label="Endpoint" value={actualDnsEndpoint} detail={`SNI: ${actualDnsSni}`} />
          <OverviewCard
            label="Проверка"
            value={activeStatus?.lastSuccessAt ? formatDate(activeStatus.lastSuccessAt) : 'Еще не было'}
            detail={`Ошибок подряд: ${activeStatus?.consecutiveFailures ?? 0}`}
          />
          <OverviewCard
            label="Домены"
            value={`${enabledDomainCount}`}
            detail="сейчас идут через aiway"
          />
        </section>

        {route === '/routing' && (
          <>
            <SectionHeader
              title="Маршрутизация"
              description="Один экран для включения AIWAY DNS, выбора сервисов и контроля пользовательских доменов."
            />

            <section className="content-grid">
              <article className="panel panel-main">
                <div className="dns-hero">
              <div>
                <span className="panel-kicker">Главный контур</span>
                <h2>AIWAY DNS</h2>
                <p className="dns-hero-copy">Панель направляет DNS на внешний или управляемый aiway endpoint, но всегда закрепляет маршрут до него через основной интернет-канал роутера.</p>
              </div>
                  <button
                    className={`btn dns-hero-button ${actualDnsOn && !failsafe ? 'btn-danger' : 'btn-primary'}`}
                    onClick={() => void run('dns', () => api.toggleDns(!actualDnsOn))}
                  >
                    {actualDnsOn && !failsafe ? 'Отключить AIWAY DNS' : 'Включить AIWAY DNS'}
                  </button>
                </div>

                <div className="detail-grid">
                  <div className="detail-box">
                    <span>Endpoint</span>
                    <strong>{actualDnsEndpoint}</strong>
                  </div>
                  <div className="detail-box">
                    <span>SNI / DoT</span>
                    <strong>{actualDnsSni}</strong>
                  </div>
                  <div className="detail-box">
                    <span>Состояние</span>
                    <strong>{actualDnsOn ? 'Активно' : 'Неактивно'}</strong>
                  </div>
                </div>

                <div className="panel-section-head">
                  <span className="panel-kicker">Сервисы</span>
                  <h3>Что отправлять через aiway</h3>
                </div>

                <div className="service-list">
                  {draftConfig.routing.services.map((service) => (
                    <label key={service.id} className="service-card">
                      <div className="service-body">
                        <div className="service-head">
                          <strong>{service.name}</strong>
                          <span className={`toggle-badge ${service.enabled ? 'on' : 'off'}`}>{service.enabled ? 'ON' : 'OFF'}</span>
                        </div>
                        <p>{service.description}</p>
                        <div className="domain-cloud">
                          {service.domains.map((domain) => (
                            <span key={domain}>{domain}</span>
                          ))}
                        </div>
                      </div>
                      <input
                        type="checkbox"
                        checked={service.enabled}
                        onChange={(event) => {
                          setDraftConfig({
                            ...draftConfig,
                            routing: {
                              ...draftConfig.routing,
                              services: draftConfig.routing.services.map((item) =>
                                item.id === service.id ? { ...item, enabled: event.target.checked } : item,
                              ),
                            },
                          })
                        }}
                      />
                    </label>
                  ))}
                </div>
              </article>

              <aside className="sidebar-stack">
                {canManageDomains ? (
                  <article className="panel panel-side">
                    <div className="panel-head compact">
                      <div>
                        <span className="panel-kicker">Кастомные домены</span>
                        <h2>{activeStatus?.installState === 'legacy' ? 'Свои сервисы (legacy)' : 'Свои сервисы'}</h2>
                      </div>
                    </div>
                    <div className="inline-form">
                      <input value={newDomain} onChange={(event) => setNewDomain(event.target.value)} placeholder="example-ai.com" />
                      <button
                        className="btn btn-primary"
                        onClick={() => {
                          if (!newDomain.trim()) return
                          void run('add-domain', () => api.addDomain(newDomain.trim()))
                          setNewDomain('')
                        }}
                      >
                        Добавить
                      </button>
                    </div>
                    {activeStatus?.installState === 'legacy' && <p className="muted">Для этого VPS панель меняет существующие legacy-конфиги Angie и Blocky точечно, без полной переустановки.</p>}
                    <div className="chips-stack">
                      {customDomains.length === 0 && <p className="muted">Кастомных доменов пока нет.</p>}
                      {customDomains.map((domain) => (
                        <div key={domain} className="chip-row">
                          <span>{domain}</span>
                          <button className="ghost-link" onClick={() => void run('remove-domain', () => api.removeDomain(domain))}>
                            Удалить
                          </button>
                        </div>
                      ))}
                    </div>
                    <button className="btn btn-secondary btn-full" onClick={() => void run('save-config', () => api.saveConfig(draftConfig))}>
                      Сохранить маршрутизацию
                    </button>
                  </article>
                ) : (
                  <article className="panel panel-side">
                    <div className="panel-head compact">
                      <div>
                        <span className="panel-kicker">Кастомные домены</span>
                        <h2>{canManageVps ? 'Только чтение' : 'Нужен доступный VPS'}</h2>
                      </div>
                    </div>
                    <p className="muted">{canManageVps ? 'Этот VPS подключен в режиме бережного legacy-управления. Статус и проверки доступны, но изменение доменов скрыто, чтобы не ломать существующую ручную конфигурацию.' : 'Добавление и удаление собственных доменов работает только когда активный VPS-профиль доступен по SSH. Для DNS-only режима эта секция скрыта специально, чтобы не вводить в заблуждение.'}</p>
                  </article>
                )}

                <article className="panel panel-side">
                  <div className="panel-head compact">
                      <div>
                        <span className="panel-kicker">Быстрый статус</span>
                        <h2>Что важно сейчас</h2>
                    </div>
                  </div>
                  <div className="health-matrix compact-health">
                    <HealthRow label="VPS / endpoint" ok={Boolean(activeStatus?.reachable)} detail={activeStatus?.lastError || 'Доступен'} />
                    <HealthRow label="Фейлсейф" ok={!failsafe} detail={failsafe ? 'Ограничивает маршрут' : 'Не активирован'} />
                    <HealthRow label="DNS режим" ok={actualDnsOn} detail={actualDnsOn ? 'Роутер реально использует aiway DNS' : 'Используется обычный DNS'} />
                  </div>
                </article>
              </aside>
            </section>
          </>
        )}

        {route === '/safety' && (
          <>
            <SectionHeader
              title="Фейлсейф"
              description="Минимальный набор правил, который отключает AIWAY DNS, если внешний endpoint перестает отвечать или начинает мешать трафику."
            />
            <section className="content-grid two-up">
              <article className="panel panel-main">
                <div className="panel-head">
                  <div>
                    <span className="panel-kicker">Политика защиты</span>
                    <h2>Когда отключать AIWAY DNS</h2>
                  </div>
                  <span className={`status-pill ${draftConfig.safety.enabled ? 'ok' : 'muted'}`}>
                    {draftConfig.safety.enabled ? 'включен' : 'отключен'}
                  </span>
                </div>

                <div className="form-grid">
                  <label>
                    <span>Интервал проверки, сек</span>
                    <input
                      type="number"
                      value={draftConfig.safety.intervalSeconds}
                      onChange={(event) =>
                        setDraftConfig({
                          ...draftConfig,
                          safety: { ...draftConfig.safety, intervalSeconds: Number(event.target.value) },
                        })
                      }
                    />
                  </label>
                  <label>
                    <span>Порог ошибок подряд</span>
                    <input
                      type="number"
                      value={draftConfig.safety.failThreshold}
                      onChange={(event) =>
                        setDraftConfig({
                          ...draftConfig,
                          safety: { ...draftConfig.safety, failThreshold: Number(event.target.value) },
                        })
                      }
                    />
                  </label>
                  <label className="checkbox-row">
                    <input
                      type="checkbox"
                      checked={draftConfig.safety.enabled}
                      onChange={(event) =>
                        setDraftConfig({
                          ...draftConfig,
                          safety: { ...draftConfig.safety, enabled: event.target.checked },
                        })
                      }
                    />
                    <span>Включить периодические проверки endpoint</span>
                  </label>
                  <label className="checkbox-row">
                    <input
                      type="checkbox"
                      checked={draftConfig.safety.disableDnsOnFailure}
                      onChange={(event) =>
                        setDraftConfig({
                          ...draftConfig,
                          safety: { ...draftConfig.safety, disableDnsOnFailure: event.target.checked },
                        })
                      }
                    />
                    <span>Отключать AIWAY DNS при деградации</span>
                  </label>
                  <label className="checkbox-row">
                    <input
                      type="checkbox"
                      checked={draftConfig.safety.autoRecover}
                      onChange={(event) =>
                        setDraftConfig({
                          ...draftConfig,
                          safety: { ...draftConfig.safety, autoRecover: event.target.checked },
                        })
                      }
                    />
                    <span>Автоматически возвращать AIWAY DNS после восстановления</span>
                  </label>
                </div>

                <button className="btn btn-primary" onClick={() => void run('save-safety', () => api.saveConfig(draftConfig))}>
                  Сохранить политику защиты
                </button>
              </article>

              <article className="panel panel-side">
                <div className="panel-head compact">
                  <div>
                    <span className="panel-kicker">Текущая картина</span>
                    <h2>Последний health-check</h2>
                  </div>
                </div>
                <div className="health-matrix">
                  <HealthRow label="Angie" ok={Boolean(activeStatus?.lastDoctor.angie)} />
                  <HealthRow label="Blocky" ok={Boolean(activeStatus?.lastDoctor.blocky)} />
                  <HealthRow label="DNS ответ" ok={Boolean(activeStatus?.lastDoctor.dns)} detail={activeStatus?.lastDoctor.dnsResult} />
                </div>
                <div className="metric-box">
                  <span>Последняя проверка</span>
                  <strong>{activeStatus?.lastCheckAt ? formatDate(activeStatus.lastCheckAt) : '—'}</strong>
                </div>
                <div className="metric-box">
                  <span>Текущая причина тревоги</span>
                  <strong>{activeStatus?.lastError || 'Ошибок нет'}</strong>
                </div>
              </article>
            </section>
          </>
        )}

        {route === '/diagnostics' && (
          <>
            <SectionHeader
              title="Диагностика"
              description="Команды управления aiway и сжатый журнал действий без лишнего визуального шума."
            />
            <section className="content-grid two-up">
              <article className="panel panel-main">
                <div className="panel-head">
                  <div>
                    <span className="panel-kicker">Операции</span>
                    <h2>Жизненный цикл aiway</h2>
                  </div>
                </div>
                <div className="action-grid">
                  <ActionButton label="Установить на VPS" accent="primary" disabled={!canMutateManagedVps} onClick={() => activeProfile && void run('install', () => api.profileInstall(activeProfile.id))} />
                  <ActionButton label="Пере применить конфиг" accent="secondary" disabled={!canMutateManagedVps} onClick={() => activeProfile && void run('sync', () => api.profileSync(activeProfile.id))} />
                  <ActionButton label="Сбросить кастомные домены" accent="secondary" disabled={!canMutateManagedVps} onClick={() => activeProfile && void run('reset', () => api.profileReset(activeProfile.id))} />
                  <ActionButton label="Полностью удалить aiway" accent="danger" disabled={!canMutateManagedVps} onClick={() => activeProfile && void run('uninstall', () => api.profileUninstall(activeProfile.id))} />
                </div>
                {!canMutateManagedVps && <p className="muted">{canManageVps ? 'Для этого VPS включен безопасный legacy-режим: управление статусом доступно, но install/sync/reset/uninstall скрыты до полной миграции конфигурации.' : 'Сейчас активен DNS-only режим. Для install/sync/reset/uninstall нужен профиль с SSH-доступом к VPS.'}</p>}
              </article>

              <article className="panel panel-side">
                <div className="panel-head compact">
                  <div>
                    <span className="panel-kicker">Журнал</span>
                    <h2>Последние события</h2>
                  </div>
                  <button className="ghost-link" onClick={() => void run('clear-logs', () => api.clearLogs())}>
                    Очистить
                  </button>
                </div>
                <div className="log-list">
                  {logs.length === 0 && <p className="muted">Пока пусто. Как только начнешь операции с VPS или доменами, события появятся здесь.</p>}
                  {logs.map((entry) => (
                    <article key={`${entry.timestamp}-${entry.message}`} className={`log-item log-${entry.level}`}>
                      <span>{formatDate(entry.timestamp)}</span>
                      <strong>{entry.level.toUpperCase()}</strong>
                      <p>{entry.message}</p>
                    </article>
                  ))}
                </div>
              </article>
            </section>
          </>
        )}

        {route === '/settings' && (
          <>
            <SectionHeader
              title="Настройки"
              description="Сначала укажи DNS-only endpoint, если aiway уже где-то работает. SSH-профили нужны только для полноценного управления VPS из панели."
            />
            <section className="content-grid two-up">
              <article className="panel panel-main">
                <div className="panel-head compact">
                  <div>
                    <span className="panel-kicker">DNS-only</span>
                    <h2>Внешний AIWAY endpoint</h2>
                  </div>
                </div>
                <div className="form-grid compact-grid">
                  <label>
                    <span>IP или домен DNS endpoint</span>
                    <input
                      value={draftConfig.routing.upstreamAddress}
                      onChange={(event) =>
                        setDraftConfig({
                          ...draftConfig,
                          routing: { ...draftConfig.routing, upstreamAddress: event.target.value },
                        })
                      }
                    />
                  </label>
                  <label>
                    <span>SNI / домен для DoT</span>
                    <input
                      value={draftConfig.routing.upstreamSni}
                      onChange={(event) =>
                        setDraftConfig({
                          ...draftConfig,
                          routing: { ...draftConfig.routing, upstreamSni: event.target.value },
                        })
                      }
                    />
                  </label>
                </div>
                <p className="muted">Подходит для уже существующей установки aiway: роутер использует этот endpoint как DNS, даже если SSH-доступ к серверу не настроен.</p>
              </article>

              <article className="panel panel-side">
                <div className="panel-head compact">
                  <div>
                    <span className="panel-kicker">CLI / API</span>
                    <h2>Для людей и агентов</h2>
                  </div>
                </div>
                <CodeBlock code={`aiway-manager status --endpoint http://192.168.1.1:2233\naiway-manager check --endpoint http://192.168.1.1:2233\naiway-manager dns on --endpoint http://192.168.1.1:2233\naiway-manager domains add perplexity.ai --endpoint http://192.168.1.1:2233\naiway-manager profiles install --profile ${activeProfile?.id || 'primary-vps'} --endpoint http://192.168.1.1:2233`} />
              </article>
            </section>

            <section className="content-grid single-column">
              <article className="panel panel-main">
                <div className="panel-head">
                  <div>
                    <span className="panel-kicker">SSH-профили</span>
                    <h2>Серверы под полное управление</h2>
                  </div>
                </div>

                <div className="profile-stack">
                  {draftConfig.profiles.map((profile, index) => (
                    <ProfileEditor
                      key={profile.id}
                      profile={profile}
                      active={draftConfig.activeId === profile.id}
                      onChange={(nextProfile) => {
                        const profiles = draftConfig.profiles.slice()
                        profiles[index] = nextProfile
                        setDraftConfig({ ...draftConfig, profiles })
                      }}
                      onActivate={() => setDraftConfig({ ...draftConfig, activeId: profile.id })}
                    />
                  ))}
                </div>

                <div className="toolbar-row">
                  <button
                    className="btn btn-secondary"
                    onClick={() =>
                      setDraftConfig({
                        ...draftConfig,
                        profiles: [
                          ...draftConfig.profiles,
                          {
                            id: `vps-${Date.now()}`,
                            name: 'Новый VPS',
                            host: '',
                            port: 22,
                            username: 'root',
                            authMethod: 'key',
                            privateKey: '/opt/etc/aiway-manager/id_ed25519',
                            useSudo: false,
                            sudoPassword: '',
                            domain: '',
                            email: '',
                            repoRef: 'main',
                            customDomains: [],
                            installOnConnect: false,
                          },
                        ],
                      })
                    }
                  >
                    Добавить сервер
                  </button>
                  <button className="btn btn-primary" onClick={() => void run('save-settings', () => api.saveConfig(draftConfig))}>
                    Сохранить настройки
                  </button>
                </div>
              </article>
            </section>
          </>
        )}
      </main>

      <footer className="app-footer">
        <div className="app-footer-inner">
          <div className="footer-meta">
            <span>AIWAY Manager v{overview.version}</span>
            <span>Keenetic / Entware</span>
          </div>

          <div className="footer-actions">
            <button className="footer-link-button" onClick={() => void run('update-check', async () => setUpdateInfo((await api.checkUpdate()) as UpdateInfo))}>
              <span className="footer-link-icon">↻</span>
              <span>{updateInfo?.available ? `Обновить до ${updateInfo.latest}` : 'Проверить обновления'}</span>
            </button>
            {updateInfo?.available && (
              <button className="footer-link-button primary" onClick={() => void run('update-apply', async () => setUpdateInfo((await api.applyUpdate()) as UpdateInfo))}>
                <span className="footer-link-icon">⬆</span>
                <span>Установить обновление</span>
              </button>
            )}
            <a className="footer-link-button" href="https://github.com/kirniy/aiway" target="_blank" rel="noreferrer">
              <span className="footer-link-icon" aria-hidden="true">
                <svg viewBox="0 0 24 24" fill="currentColor">
                  <path d="M12 2C6.477 2 2 6.484 2 12.017c0 4.426 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.866-.014-1.699-2.782.605-3.37-1.344-3.37-1.344-.455-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.004.071 1.532 1.033 1.532 1.033.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0 1 12 6.844a9.57 9.57 0 0 1 2.504.337c1.909-1.296 2.747-1.026 2.747-1.026.546 1.378.202 2.398.1 2.65.64.7 1.028 1.595 1.028 2.688 0 3.848-2.338 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.481A10.019 10.019 0 0 0 22 12.017C22 6.484 17.523 2 12 2Z"/>
                </svg>
              </span>
              <span>GitHub</span>
            </a>
            <a className="footer-link-button" href="https://t.me/kirniy" target="_blank" rel="noreferrer">
              <span className="footer-link-icon" aria-hidden="true">
                <svg viewBox="0 0 24 24" fill="currentColor">
                  <path d="M9.78 18.65 9.93 14l8.47-7.64c.37-.33-.08-.49-.57-.16L7.37 12.8l-4.5-1.4c-.98-.3-.99-.98.2-1.45L20.64 3.1c.82-.3 1.53.2 1.27 1.45l-3.01 14.18c-.21 1.01-.82 1.26-1.66.78l-4.6-3.39-2.22 2.14c-.24.24-.45.45-.91.39Z"/>
                </svg>
              </span>
              <span>Telegram</span>
            </a>
          </div>
        </div>
      </footer>
    </div>
  )
}

function SectionHeader({ title, description }: { title: string; description: string }) {
  return (
    <header className="page-header">
      <div>
        <h1>{title}</h1>
        <p className="description">{description}</p>
      </div>
    </header>
  )
}

function OverviewCard({ label, value, detail }: { label: string; value: string; detail: string }) {
  return (
    <article className="overview-card">
      <span className="summary-label">{label}</span>
      <strong>{value}</strong>
      <p>{detail}</p>
    </article>
  )
}

function HealthRow({ label, ok, detail }: { label: string; ok: boolean; detail?: string }) {
  return (
    <div className="health-row">
      <span>{label}</span>
      <strong className={ok ? 'good' : 'bad'}>{ok ? 'OK' : 'FAIL'}</strong>
      <small>{detail || '—'}</small>
    </div>
  )
}

function ActionButton({ label, accent, onClick, disabled = false }: { label: string; accent: 'primary' | 'secondary' | 'danger'; onClick: () => void; disabled?: boolean }) {
  return (
    <button className={`action-card ${accent} ${disabled ? 'disabled' : ''}`} onClick={onClick} disabled={disabled}>
      <strong>{label}</strong>
      <span>Выполнить без входа в стандартную админку Keenetic</span>
    </button>
  )
}

function ProfileEditor({
  profile,
  active,
  onChange,
  onActivate,
}: {
  profile: Profile
  active: boolean
  onChange: (profile: Profile) => void
  onActivate: () => void
}) {
  const inlineKeyUploaded = Boolean(profile.privateKey && (profile.privateKey.includes('BEGIN ') || profile.privateKey.includes('\n')))

  return (
    <article className={`profile-card ${active ? 'active' : ''}`}>
      <div className="profile-header">
        <label className="checkbox-row compact">
          <input type="radio" checked={active} onChange={onActivate} />
          <span>Активный профиль</span>
        </label>
        <span className="status-pill muted">{profile.id}</span>
      </div>
      <div className="form-grid compact-grid">
        <label>
          <span>Название</span>
          <input value={profile.name} onChange={(event) => onChange({ ...profile, name: event.target.value })} />
        </label>
        <label>
          <span>Хост / IP</span>
          <input value={profile.host} onChange={(event) => onChange({ ...profile, host: event.target.value })} />
        </label>
        <label>
          <span>SSH-порт</span>
          <input type="number" value={profile.port} onChange={(event) => onChange({ ...profile, port: Number(event.target.value) })} />
        </label>
        <label>
          <span>Пользователь</span>
          <input value={profile.username} onChange={(event) => onChange({ ...profile, username: event.target.value })} />
        </label>
        <label>
          <span>Авторизация</span>
          <select value={profile.authMethod} onChange={(event) => onChange({ ...profile, authMethod: event.target.value as Profile['authMethod'] })}>
            <option value="key">SSH key</option>
            <option value="password">Username + password</option>
          </select>
        </label>
        {profile.authMethod === 'key' ? (
          <label>
            <span>Путь к приватному ключу</span>
            <input
              value={inlineKeyUploaded ? '' : profile.privateKey || ''}
              placeholder={inlineKeyUploaded ? 'Загружен приватный ключ из файла' : '/opt/etc/aiway-manager/id_ed25519'}
              onChange={(event) => onChange({ ...profile, privateKey: event.target.value })}
            />
            <div className="key-upload-row">
              <input
                type="file"
                accept=".pem,.key,.txt,*/*"
                onChange={(event) => {
                  const file = event.target.files?.[0]
                  if (!file) return
                  const reader = new FileReader()
                  reader.onload = () => {
                    const value = typeof reader.result === 'string' ? reader.result : ''
                    onChange({ ...profile, privateKey: value })
                  }
                  reader.readAsText(file)
                }}
              />
              {inlineKeyUploaded && (
                <button className="ghost-link" type="button" onClick={() => onChange({ ...profile, privateKey: '' })}>
                  Удалить загруженный ключ
                </button>
              )}
            </div>
            {inlineKeyUploaded && <span className="muted">Ключ загружен в панель и будет использоваться без отдельного пути на роутере.</span>}
          </label>
        ) : (
          <label>
            <span>Пароль SSH</span>
            <input type="password" value={profile.password || ''} onChange={(event) => onChange({ ...profile, password: event.target.value })} />
          </label>
        )}
        <label>
          <span>Домен для DoT/DoH</span>
          <input value={profile.domain} onChange={(event) => onChange({ ...profile, domain: event.target.value })} />
        </label>
        <label>
          <span>Email для ACME</span>
          <input value={profile.email} onChange={(event) => onChange({ ...profile, email: event.target.value })} />
        </label>
        <label>
          <span>Git ref</span>
          <input value={profile.repoRef} onChange={(event) => onChange({ ...profile, repoRef: event.target.value })} />
        </label>
        <label className="checkbox-row compact">
          <input type="checkbox" checked={profile.useSudo} onChange={(event) => onChange({ ...profile, useSudo: event.target.checked })} />
          <span>Выполнять через sudo</span>
        </label>
      </div>
    </article>
  )
}

function CodeBlock({ code }: { code: string }) {
  return <pre className="code-block">{code}</pre>
}

function formatDate(value: string) {
  return new Date(value).toLocaleString('ru-RU', {
    day: '2-digit',
    month: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

export default App
