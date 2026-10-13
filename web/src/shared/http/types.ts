import type { AppLocale } from '@shared/preferences/locale'

export interface SuccessEnvelope<T> {
  code: 0
  message: string
  data?: T
}

export interface ErrorEnvelope<T = unknown> {
  code: string
  message: string
  data?: T
}

export type AuthPrincipalType = 'admin' | 'access_key'

export type AccessKeyCostLimitRuleStatus = {
  id: number
  kind: 'total' | 'periodic'
  limit_usd: string
  used_usd: string
  remaining_usd: string
  status: 'available' | 'inactive' | 'exhausted'
  period_seconds?: number
  window_started_at_ms?: number
  window_ends_at_ms?: number
}

export interface AccessKeyCostLimitStatus {
  observed_at_ms: number
  allowed: boolean
  recoverable: boolean
  next_available_at_ms?: number
  rules: AccessKeyCostLimitRuleStatus[]
}

export interface AuthSessionPayload {
  authenticated: boolean
  principal_type: AuthPrincipalType
  // 仅当登录主体是访问密钥时返回，标识该成员的身份。
  access_key?: { id: number; name: string }
  // 仅当登录主体是访问密钥且设有成本上限时返回。
  cost_limit_status?: AccessKeyCostLimitStatus
  // 仅当登录主体是访问密钥且该密钥开启了请求内容记录时返回。
  request_body_recording?: { retention_days: number }
}

export interface AuthLockedPayload {
  retry_after_seconds: number
}

export type { AppLocale }
