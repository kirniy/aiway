export type ServiceToggle = {
  id: string
  name: string
  description: string
  domains: string[]
  enabled: boolean
}

export type Profile = {
  id: string
  name: string
  host: string
  port: number
  username: string
  authMethod: 'password' | 'key'
  password?: string
  privateKey?: string
  useSudo: boolean
  sudoPassword?: string
  domain: string
  email: string
  repoRef: string
  customDomains: string[]
  installOnConnect: boolean
}

export type Config = {
  dashboard: {
    port: number
    listenAddress: string
    authEnabled: boolean
    themePreference: string
  }
  safety: {
    enabled: boolean
    intervalSeconds: number
    failThreshold: number
    autoRecover: boolean
    disableDnsOnFailure: boolean
    canaryDomains: string[]
  }
  routing: {
    desiredDnsOn: boolean
    customDomains: string[]
    services: ServiceToggle[]
    lastAppliedAt: string
    failsafeActive: boolean
  }
  profiles: Profile[]
  activeId: string
}

export type DoctorResult = {
  angie: boolean
  blocky: boolean
  dns: boolean
  dnsResult: string
}

export type ProfileStatus = {
  profileId: string
  reachable: boolean
  installed: boolean
  angie: string
  blocky: string
  lastError?: string
  lastCheckAt?: string
  lastSuccessAt?: string
  consecutiveFailures: number
  desiredDnsOn: boolean
  effectiveDnsOn: boolean
  lastDoctor: DoctorResult
  installState: string
  installOutputPreview: string
  customDomains: string[]
  serviceCount: number
}

export type LogEntry = {
  timestamp: string
  level: string
  message: string
}

export type OverviewResponse = {
  config: Config
  profiles: Profile[]
  statuses: Record<string, ProfileStatus>
  logs: LogEntry[]
  activeProfile?: Profile
  generatedAt: string
}
