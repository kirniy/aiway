import { useEffect, useMemo, useState } from 'react'
import { api } from './api'
import type { Config, OverviewResponse, Profile, ProfileStatus } from './types'

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
  const [pending, setPending] = useState('')
  const [error, setError] = useState('')
  const [newDomain, setNewDomain] = useState('')

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

  const run = async (label: string, action: () => Promise<unknown>) => {
    try {
      setPending(label)
      setError('')
      await action()
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Операция завершилась с ошибкой')
    } finally {
      setPending('')
    }
  }

  if (!overview || !draftConfig) {
    return <div className="screen-loading">Загружаю AIWAY Manager...</div>
  }

  const dnsWanted = draftConfig.routing.desiredDnsOn
  const failsafe = draftConfig.routing.failsafeActive

  return (
    <div className="shell">
      <div className="shell-glow shell-glow-left" />
      <div className="shell-glow shell-glow-right" />

      <header className="topbar">
        <div>
          <div className="eyebrow">AIWAY / KEENETIC CONTROL SURFACE</div>
          <div className="brand-row">
            <h1>AIWAY Manager</h1>
            <span className={`status-pill ${activeStatus?.reachable ? 'ok' : 'bad'}`}>
              {activeStatus?.reachable ? 'VPS на связи' : 'VPS недоступен'}
            </span>
            <span className={`status-pill ${failsafe ? 'warn' : 'ok'}`}>
              {failsafe ? 'Фейлсейф активен' : 'Фейлсейф готов'}
            </span>
          </div>
          <p className="topbar-copy">
            Полное управление aiway с роутера: установка на VPS, обновления, кастомные домены, проверки здоровья и
            работа без входа в стандартную админку Keenetic.
          </p>
        </div>

        <div className="hero-actions">
          <button className="btn btn-secondary" onClick={() => void run('check', () => api.checkNow())}>
            {pending === 'check' ? 'Проверяю...' : 'Проверить сейчас'}
          </button>
          <button
            className={`btn ${dnsWanted && !failsafe ? 'btn-danger' : 'btn-primary'}`}
            onClick={() => void run('dns', () => api.toggleDns(!dnsWanted))}
          >
            {pending === 'dns' ? 'Применяю...' : dnsWanted ? 'Отключить DNS-трюк' : 'Включить DNS-трюк'}
          </button>
        </div>
      </header>

      <nav className="tabs">
        {routes.map((item) => (
          <button
            key={item.path}
            className={`tab ${route === item.path ? 'active' : ''}`}
            onClick={() => navigate(item.path)}
          >
            <span>{item.short}</span>
            <strong>{item.label}</strong>
          </button>
        ))}
      </nav>

      {error && <div className="alert alert-error">{error}</div>}

      <section className="summary-grid">
        <article className="summary-card accent-cyan">
          <span className="summary-label">Активный профиль</span>
          <strong>{activeProfile?.name || 'Не выбран'}</strong>
          <p>
            {activeProfile?.host || 'Укажи IP или домен VPS'}:{activeProfile?.port ?? 22}
          </p>
        </article>
        <article className="summary-card accent-green">
          <span className="summary-label">Сервисы под проксирование</span>
          <strong>
            {draftConfig.routing.services.filter((service) => service.enabled).length + draftConfig.routing.customDomains.length}
          </strong>
          <p>Штатные сервисы + кастомные домены, которыми управляешь прямо из панели.</p>
        </article>
        <article className="summary-card accent-amber">
          <span className="summary-label">Последняя успешная проверка</span>
          <strong>{activeStatus?.lastSuccessAt ? formatDate(activeStatus.lastSuccessAt) : 'Ещё не было'}</strong>
          <p>Ошибок подряд: {activeStatus?.consecutiveFailures ?? 0}</p>
        </article>
        <article className="summary-card accent-pink">
          <span className="summary-label">CLI для людей и агентов</span>
          <strong>`aiway-manager status --endpoint http://router:2222`</strong>
          <p>Один и тот же API обслуживает GUI, CLI и автоматизации в локальной сети.</p>
        </article>
      </section>

      {route === '/routing' && (
        <section className="content-grid">
          <article className="panel panel-main">
            <div className="panel-head">
              <div>
                <span className="panel-kicker">Живой контур</span>
                <h2>Что сейчас делает aiway</h2>
              </div>
              <span className={`status-pill ${activeStatus?.effectiveDnsOn ? 'ok' : 'muted'}`}>
                {activeStatus?.effectiveDnsOn ? 'DNS активен' : 'DNS неактивен'}
              </span>
            </div>

            <div className="service-list">
              {draftConfig.routing.services.map((service) => (
                <label key={service.id} className="service-card">
                  <div>
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

          <aside className="panel panel-side">
            <div className="panel-head compact">
              <div>
                <span className="panel-kicker">Кастомные сервисы</span>
                <h2>Свои домены</h2>
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
            <div className="chips-stack">
              {draftConfig.routing.customDomains.length === 0 && <p className="muted">Кастомных доменов пока нет.</p>}
              {draftConfig.routing.customDomains.map((domain) => (
                <div key={domain} className="chip-row">
                  <span>{domain}</span>
                  <button className="ghost-link" onClick={() => void run('remove-domain', () => api.removeDomain(domain))}>
                    Удалить
                  </button>
                </div>
              ))}
            </div>
            <button className="btn btn-secondary btn-full" onClick={() => void run('save-config', () => api.saveConfig(draftConfig))}>
              Сохранить пресеты и структуру маршрутизации
            </button>
          </aside>
        </section>
      )}

      {route === '/safety' && (
        <section className="content-grid two-up">
          <article className="panel panel-main">
            <div className="panel-head">
              <div>
                <span className="panel-kicker">Фейлсейф</span>
                <h2>Автоматический защитный контур</h2>
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
                <span>Периодически проверять aiway и реагировать на деградацию</span>
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
                <span>Отключать DNS-трюк, если проверки последовательно падают</span>
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
                <span>Автоматически возвращать рабочий режим после успешного восстановления</span>
              </label>
            </div>

            <button className="btn btn-primary" onClick={() => void run('save-safety', () => api.saveConfig(draftConfig))}>
              Сохранить параметры защиты
            </button>
          </article>

          <article className="panel panel-side">
            <div className="panel-head compact">
              <div>
                <span className="panel-kicker">Состояние</span>
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
      )}

      {route === '/diagnostics' && (
        <section className="content-grid two-up">
          <article className="panel panel-main">
            <div className="panel-head">
              <div>
                <span className="panel-kicker">Операции</span>
                <h2>Жизненный цикл aiway на сервере</h2>
              </div>
            </div>

            <div className="action-grid">
              <ActionButton label="Установить на VPS" accent="primary" onClick={() => activeProfile && void run('install', () => api.profileInstall(activeProfile.id))} />
              <ActionButton label="Пере применить конфиг" accent="secondary" onClick={() => activeProfile && void run('sync', () => api.profileSync(activeProfile.id))} />
              <ActionButton label="Сбросить кастомные домены" accent="secondary" onClick={() => activeProfile && void run('reset', () => api.profileReset(activeProfile.id))} />
              <ActionButton label="Полностью удалить aiway" accent="danger" onClick={() => activeProfile && void run('uninstall', () => api.profileUninstall(activeProfile.id))} />
            </div>
          </article>

          <article className="panel panel-side">
            <div className="panel-head compact">
              <div>
                <span className="panel-kicker">Журнал</span>
                <h2>Системные события</h2>
              </div>
              <button className="ghost-link" onClick={() => void run('clear-logs', () => api.clearLogs())}>
                Очистить
              </button>
            </div>
            <div className="log-list">
              {overview.logs.length === 0 && <p className="muted">Пока пусто. Как только начнешь операции с VPS или доменами, события появятся здесь.</p>}
              {overview.logs.map((entry) => (
                <article key={`${entry.timestamp}-${entry.message}`} className={`log-item log-${entry.level}`}>
                  <span>{formatDate(entry.timestamp)}</span>
                  <strong>{entry.level.toUpperCase()}</strong>
                  <p>{entry.message}</p>
                </article>
              ))}
            </div>
          </article>
        </section>
      )}

      {route === '/settings' && (
        <section className="content-grid two-up">
          <article className="panel panel-main">
            <div className="panel-head">
              <div>
                <span className="panel-kicker">VPS-профили</span>
                <h2>Серверы, которыми управляет роутер</h2>
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
                Сохранить конфигурацию панели
              </button>
            </div>
          </article>

          <article className="panel panel-side">
            <div className="panel-head compact">
              <div>
                <span className="panel-kicker">Командная работа</span>
                <h2>API и CLI</h2>
              </div>
            </div>
            <CodeBlock code={`aiway-manager status --endpoint http://192.168.1.1:2222\naiway-manager check --endpoint http://192.168.1.1:2222\naiway-manager dns on --endpoint http://192.168.1.1:2222\naiway-manager domains add perplexity.ai --endpoint http://192.168.1.1:2222\naiway-manager profiles install --profile ${activeProfile?.id || 'primary-vps'} --endpoint http://192.168.1.1:2222`} />
            <p className="muted">
              Этот же интерфейс удобен для агентов: везде одинаковые JSON-ответы, один endpoint и предсказуемые действия.
            </p>
          </article>
        </section>
      )}
    </div>
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

function ActionButton({ label, accent, onClick }: { label: string; accent: 'primary' | 'secondary' | 'danger'; onClick: () => void }) {
  return (
    <button className={`action-card ${accent}`} onClick={onClick}>
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
            <input value={profile.privateKey || ''} onChange={(event) => onChange({ ...profile, privateKey: event.target.value })} />
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
