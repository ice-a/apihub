import type { ApiClient } from '@shared/http/client'
import { InvalidResponseError } from '@shared/http/errors'
import type {
  AccessKeyCostLimitRuleStatus,
  AccessKeyCostLimitStatus,
  AuthSessionPayload,
} from '@shared/http/types'
import { integer, record } from './response'

function readRequestBodyRecording(
  value: unknown,
): AuthSessionPayload['request_body_recording'] | undefined {
  if (value == null) return undefined
  return { retention_days: integer(record(value).retention_days, 1) }
}

function readAccessKey(
  value: unknown,
): AuthSessionPayload['access_key'] | undefined {
  if (value == null) return undefined
  const r = record(value)
  return { id: integer(r.id, 1), name: String(r.name ?? '') }
}

function readCostLimitStatus(
  value: unknown,
): AccessKeyCostLimitStatus | undefined {
  if (value == null) return undefined
  const r = record(value)
  const rules: AccessKeyCostLimitRuleStatus[] = Array.isArray(r.rules)
    ? r.rules
        .map((item: unknown) => {
          const rule = record(item)
          return {
            id: integer(rule.id, 1),
            kind: String(rule.kind ?? '') as AccessKeyCostLimitRuleStatus['kind'],
            limit_usd: String(rule.limit_usd ?? ''),
            used_usd: String(rule.used_usd ?? ''),
            remaining_usd: String(rule.remaining_usd ?? ''),
            status: String(rule.status ?? '') as AccessKeyCostLimitRuleStatus['status'],
            period_seconds: 'period_seconds' in rule ? integer(rule.period_seconds, 0) : undefined,
            window_started_at_ms:
              'window_started_at_ms' in rule ? integer(rule.window_started_at_ms, 0) : undefined,
            window_ends_at_ms:
              'window_ends_at_ms' in rule ? integer(rule.window_ends_at_ms, 0) : undefined,
          }
        })
        .filter((rule) => rule.kind === 'total' || rule.kind === 'periodic')
    : []
  return {
    observed_at_ms: integer(r.observed_at_ms, 0),
    allowed: Boolean(r.allowed),
    recoverable: Boolean(r.recoverable),
    next_available_at_ms: 'next_available_at_ms' in r ? integer(r.next_available_at_ms, 0) : undefined,
    rules,
  }
}

export async function getAuthSession(
  client: ApiClient,
  credential: string,
  signal: AbortSignal,
  handleUnauthorized = false,
): Promise<AuthSessionPayload> {
  const data = await client.request<unknown>('/api/auth/session', {
    authKey: credential,
    cache: 'no-store',
    signal,
    // 候选密钥验证失败不清除已有会话；恢复已保存身份时继续执行全局 401 清理。
    handleUnauthorized,
  })
  if (
    typeof data !== 'object' ||
    data === null ||
    !('authenticated' in data) ||
    data.authenticated !== true ||
    !('principal_type' in data) ||
    (data.principal_type !== 'admin' && data.principal_type !== 'access_key')
  ) {
    throw new InvalidResponseError()
  }
  const recording = readRequestBodyRecording(
    'request_body_recording' in data ? data.request_body_recording : undefined,
  )
  const accessKey = readAccessKey('access_key' in data ? data.access_key : undefined)
  const costLimitStatus = readCostLimitStatus(
    'cost_limit_status' in data ? data.cost_limit_status : undefined,
  )
  return {
    authenticated: true,
    principal_type: data.principal_type,
    ...(accessKey ? { access_key: accessKey } : {}),
    ...(costLimitStatus ? { cost_limit_status: costLimitStatus } : {}),
    ...(recording ? { request_body_recording: recording } : {}),
  }
}
