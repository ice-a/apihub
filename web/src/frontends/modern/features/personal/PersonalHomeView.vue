<script setup lang="ts">
import { keepPreviousData, useQuery } from '@tanstack/vue-query'
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { getUsage, type UsageReport } from '@modern/api/usage'
import { getLogs, type LogPage } from '@modern/api/logs'
import { useAuthSession } from '@modern/features/auth/auth-session'
import { useApiClient } from '@shared/http/client-context'
import { usePageRefresh } from '@modern/app/page-refresh'
import { useMessageSource } from '@modern/app/messages'
import { resolveTimeRange, type TimeRangeState } from '@modern/app/time-range'
import {
  AppButton,
  AppCollectionState,
  AppDateTimeRangePicker,
  AppPanel,
  AppProgressBar,
  AppSegmentedControl,
} from '@modern/components/ui'
import { formatLocalDateTime, parseLocalDateTime, type DateRangePreset } from '@modern/components/ui/date-time'
import { formatUsageCost } from '@modern/features/usage/usage-display'
import { trendMetrics, type TrendMetric } from '@modern/features/usage/usage-state'
import UsageMetrics from '@modern/features/usage/UsageMetrics.vue'
import UsageTrend from '@modern/features/usage/UsageTrend.vue'

const { t, locale } = useI18n()
const router = useRouter()
const client = useApiClient()
const session = useAuthSession()
const me = computed(() => session.state.accessKey)

// 时间范围：访问密钥登录后，后端按 access_key_id 自限定用量与日志，这里只传时间窗。
const range = ref<TimeRangeState>({ preset: '24h' })
const from = ref('')
const to = ref('')
const preset = ref<DateRangePreset>('24h')
watch(
  () => [range.value, range.value.preset],
  () => {
    const resolved = resolveTimeRange(range.value)
    from.value = formatLocalDateTime(Number(resolved.from_ms))
    to.value = formatLocalDateTime(Number(resolved.to_ms))
    preset.value = range.value.preset ?? '24h'
  },
  { immediate: true },
)
function applyDate(): void {
  const start = parseLocalDateTime(from.value)
  const end = parseLocalDateTime(to.value)
  if (!start || !end || start >= end) return
  const next: TimeRangeState = preset.value
    ? { preset: preset.value }
    : { from_ms: String(start.getTime()), to_ms: String(end.getTime()) }
  if (JSON.stringify(next) === JSON.stringify(range.value)) {
    void usageQuery.refetch()
    void logsQuery.refetch()
  } else {
    range.value = next
  }
}

const usageQuery = useQuery(
  computed(() => ({
    queryKey: ['modern', 'personal-usage', range.value],
    queryFn: ({ signal }: { signal: AbortSignal }) =>
      getUsage(client, resolveTimeRange(range.value), signal),
    placeholderData: keepPreviousData,
  })),
)
const report = computed<UsageReport | undefined>(() => usageQuery.data.value)

const logsQuery = useQuery(
  computed(() => ({
    queryKey: ['modern', 'personal-logs', range.value],
    queryFn: ({ signal }: { signal: AbortSignal }) =>
      getLogs(client, { ...resolveTimeRange(range.value), limit: '5' }, 1, signal),
    placeholderData: keepPreviousData,
  })),
)
const logItems = computed<LogPage['items']>(() => logsQuery.data.value?.items ?? [])

const trend = ref<TrendMetric>('requests')
const trendOptions = computed(() => trendMetrics.map((value) => ({ value, label: t('usage.' + value) })))

// 预算：仅当该成员密钥设有成本上限时展示；剩余百分比决定进度条色调。
const budget = computed(() => session.state.costLimitStatus)
function remainingPercent(remaining: string, limit: string): number | undefined {
  const r = Number.parseFloat(remaining)
  const l = Number.parseFloat(limit)
  if (!Number.isFinite(r) || !Number.isFinite(l) || l <= 0) return undefined
  return Math.max(0, Math.min(100, (r / l) * 100))
}
function budgetTone(percent: number | undefined) {
  if (percent === undefined) return 'success' as const
  if (percent < 10) return 'danger' as const
  if (percent < 25) return 'warning' as const
  return 'success' as const
}

function viewAllLogs(): void {
  void router.push({ name: 'modern-logs', query: resolveTimeRange(range.value) })
}
function setTrend(value: TrendMetric): void {
  trend.value = value
}

const fetching = computed(() => usageQuery.isFetching.value || logsQuery.isFetching.value)
async function refresh(): Promise<void> {
  await Promise.all([usageQuery.refetch(), logsQuery.refetch()])
}
usePageRefresh({ refresh, pending: () => fetching.value, updatedAt: () => report.value?.observed_at_ms })
useMessageSource(() =>
  usageQuery.isError.value || logsQuery.isError.value
    ? { tone: 'warning', text: t('logs.stale'), action: { label: t('ui.retry'), run: refresh } }
    : undefined,
)

function formatCost(value: string): string {
  return formatUsageCost(value, locale.value)
}
function formatTime(value: number): string {
  return formatLocalDateTime(value)
}
</script>

<template>
  <div class="modern-personal-workspace">
    <header class="modern-personal-header">
      <h1 class="modern-personal-greeting">
        {{ me ? t('personal.greeting', { name: me.name }) : t('personal.greetingGeneric') }}
      </h1>
      <p class="modern-personal-subtitle">{{ t('personal.title') }}</p>
    </header>

    <form class="modern-personal-filters" role="search" :aria-label="t('logs.timeRange')" @submit.prevent>
      <AppDateTimeRangePicker
        v-model:from="from"
        v-model:to="to"
        :preset="preset"
        :label="t('logs.timeRange')"
        size="md"
        label-hidden
        class="modern-personal-date"
        @update:preset="preset = $event"
        @apply="applyDate"
      />
    </form>

    <AppCollectionState
      v-if="!report"
      :loading="usageQuery.isPending.value"
      :error="usageQuery.isError.value"
      :title="t(usageQuery.isError.value ? 'usage.failed' : 'usage.loading')"
    >
      <AppButton v-if="usageQuery.isError.value" @click="refresh">{{ t('ui.retry') }}</AppButton>
    </AppCollectionState>

    <div v-else class="modern-personal-report" :class="{ 'is-updating': usageQuery.isPlaceholderData.value }">
      <UsageMetrics :report="report" />

      <div class="modern-personal-grid">
        <AppPanel :title="t('personal.trend')" compact>
          <template #actions>
            <AppSegmentedControl
              :model-value="trend"
              :options="trendOptions"
              :label="t('personal.trend')"
              size="sm"
              appearance="field"
              @update:model-value="setTrend($event as TrendMetric)"
            />
          </template>
          <UsageTrend :report="report" :metric="trend" />
        </AppPanel>

        <AppPanel v-if="budget && budget.rules.length" :title="t('personal.budget')" compact>
          <div class="modern-personal-budget">
            <div
              v-for="rule in budget.rules"
              :key="rule.id"
              class="modern-personal-budget-rule"
            >
              <AppProgressBar
                :label="t('personal.budgetRemaining', { remaining: rule.remaining_usd, limit: rule.limit_usd })"
                :value="remainingPercent(rule.remaining_usd, rule.limit_usd)"
                :tone="budgetTone(remainingPercent(rule.remaining_usd, rule.limit_usd))"
                size="md"
              />
              <p class="modern-personal-budget-meta">
                {{ t('personal.budgetRemaining', { remaining: rule.remaining_usd, limit: rule.limit_usd }) }}
              </p>
            </div>
          </div>
        </AppPanel>
      </div>

      <AppPanel :title="t('personal.recentRequests')" compact>
        <template #actions>
          <AppButton variant="text" size="sm" @click="viewAllLogs">{{ t('personal.viewAllLogs') }}</AppButton>
        </template>
        <div v-if="logItems.length" class="modern-personal-logs">
          <div v-for="item in logItems" :key="item.request_id" class="modern-personal-log-row">
            <span class="modern-personal-log-time">{{ formatTime(item.completed_at_ms) }}</span>
            <span class="modern-personal-log-model">{{ item.upstream_model ?? item.client_model ?? '—' }}</span>
            <span class="modern-personal-log-status" :data-status="item.status">{{ item.status }}</span>
            <span class="modern-personal-log-tokens">{{ item.input_tokens }}/{{ item.output_tokens }}</span>
            <span class="modern-personal-log-cost">{{ formatCost(item.estimated_cost_nano_usd) }}</span>
          </div>
        </div>
        <p v-else class="modern-personal-logs-empty">{{ t('usage.loading') }}</p>
      </AppPanel>
    </div>
  </div>
</template>

<style scoped>
.modern-personal-workspace {
  display: flex;
  flex: 1;
  min-height: 0;
  min-width: 0;
  flex-direction: column;
  gap: var(--modern-space-5);
}
.modern-personal-header {
  display: flex;
  flex-direction: column;
  gap: var(--modern-space-1);
}
.modern-personal-greeting {
  margin: 0;
  font-size: var(--modern-font-size-title);
  font-weight: var(--modern-weight-semibold);
}
.modern-personal-subtitle {
  margin: 0;
  color: var(--modern-muted);
}
.modern-personal-filters {
  display: flex;
  flex: none;
  align-items: center;
  gap: var(--modern-space-3);
}
.modern-personal-date {
  flex: 1 1 240px;
  min-width: 0;
  max-width: 420px;
}
.modern-personal-report {
  display: grid;
  flex: 1;
  align-content: start;
  gap: var(--modern-page-gap);
  min-height: 0;
}
.modern-personal-report.is-updating {
  opacity: var(--modern-opacity-quiet);
}
.modern-personal-grid {
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(300px, 1fr);
  gap: var(--modern-space-4);
  align-items: stretch;
}
.modern-personal-budget {
  display: flex;
  flex-direction: column;
  gap: var(--modern-space-4);
}
.modern-personal-budget-meta {
  margin: var(--modern-space-2) 0 0;
  color: var(--modern-muted);
  font-size: var(--modern-font-size-small);
  font-variant-numeric: tabular-nums;
}
.modern-personal-logs {
  display: flex;
  flex-direction: column;
}
.modern-personal-log-row {
  display: grid;
  grid-template-columns: minmax(0, 1.4fr) minmax(0, 1.6fr) minmax(0, 0.8fr) minmax(0, 1fr) minmax(0, 1fr);
  gap: var(--modern-space-3);
  padding: var(--modern-space-3) 0;
  border-bottom: var(--modern-line-width) solid var(--modern-border);
  font-size: var(--modern-font-size-small);
  font-variant-numeric: tabular-nums;
}
.modern-personal-log-row:last-child {
  border-bottom: 0;
}
.modern-personal-log-model {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.modern-personal-log-status {
  text-transform: capitalize;
}
.modern-personal-log-status[data-status='failure'],
.modern-personal-log-status[data-status='error'] {
  color: var(--modern-tone-danger);
}
.modern-personal-logs-empty {
  margin: 0;
  color: var(--modern-muted);
}
@media (max-width: 1150px) {
  .modern-personal-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
