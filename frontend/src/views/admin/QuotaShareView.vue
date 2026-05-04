<template>
  <AppLayout>
    <div class="mx-auto flex w-full max-w-7xl flex-col gap-6 px-4 py-6 sm:px-6 lg:px-8">
      <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <p class="text-sm font-medium uppercase tracking-[0.18em] text-emerald-600 dark:text-emerald-400">
            Quota Share
          </p>
          <h1 class="mt-1 text-2xl font-semibold text-gray-900 dark:text-white">
            配额共享状态面板
          </h1>
          <p class="mt-2 max-w-2xl text-sm text-gray-500 dark:text-dark-400">
            查看 quota_share 分组的 5 小时与 7 天窗口、估算总额、校准状态，以及每个下游 Key 的实时权重和用量。
          </p>
        </div>

        <div class="flex flex-col gap-3 sm:flex-row">
          <a
            href="/quota-share"
            target="_blank"
            rel="noopener noreferrer"
            class="btn btn-secondary"
          >
            打开用户查询页
          </a>
          <button class="btn btn-primary" :disabled="loadingGroups || loadingStatus" @click="refreshAll">
            <Icon name="refresh" size="sm" class="mr-2" :class="loadingGroups || loadingStatus ? 'animate-spin' : ''" />
            刷新状态
          </button>
        </div>
      </div>

      <div class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <div class="grid gap-4 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-end">
          <div>
            <label class="input-label">选择分组</label>
            <Select
              v-model="selectedGroupId"
              :options="groupOptions"
              :placeholder="loadingGroups ? '加载中...' : '请选择 quota_share 分组'"
              :disabled="loadingGroups || groupOptions.length === 0"
            />
          </div>
          <div class="text-sm text-gray-500 dark:text-dark-400">
            <span v-if="selectedGroup">当前分组：{{ selectedGroup.name }} · {{ selectedGroup.platform }}</span>
            <span v-else>当前还没有 quota_share 分组</span>
          </div>
        </div>
      </div>

      <EmptyState
        v-if="!loadingGroups && groupOptions.length === 0"
        title="还没有 quota_share 分组"
        description="先去分组管理里创建 quota_share 分组，再回来查看共享配额状态。"
      />

      <div v-else-if="selectedGroup" class="space-y-6">
        <div class="grid gap-4 xl:grid-cols-4 md:grid-cols-2">
          <section class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
            <p class="text-xs font-medium uppercase tracking-[0.18em] text-gray-500 dark:text-dark-400">5 小时窗口</p>
            <p class="mt-3 text-3xl font-semibold text-gray-900 dark:text-white">
              {{ formatUSD(window5hEstimate) }}
            </p>
            <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
              上游已用 {{ formatPct(status?.group_state?.u5p) }}
            </p>
            <p class="mt-3 text-xs text-gray-500 dark:text-dark-400">
              {{ formatWindow(status?.group_state?.w5s, status?.group_state?.w5e) }}
            </p>
          </section>

          <section class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
            <p class="text-xs font-medium uppercase tracking-[0.18em] text-gray-500 dark:text-dark-400">7 天窗口</p>
            <p class="mt-3 text-3xl font-semibold text-gray-900 dark:text-white">
              {{ formatUSD(window7dEstimate) }}
            </p>
            <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
              上游已用 {{ formatPct(status?.group_state?.u7p) }}
            </p>
            <p class="mt-3 text-xs text-gray-500 dark:text-dark-400">
              {{ formatWindow(status?.group_state?.w7s, status?.group_state?.w7e) }}
            </p>
          </section>

          <section class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
            <p class="text-xs font-medium uppercase tracking-[0.18em] text-gray-500 dark:text-dark-400">总权重</p>
            <p class="mt-3 text-3xl font-semibold text-gray-900 dark:text-white">
              {{ status?.total_weight ?? 0 }}
            </p>
            <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
              {{ status?.keys?.length ?? 0 }} 个下游 Key
            </p>
            <p class="mt-3 text-xs text-gray-500 dark:text-dark-400">
              仅统计当前分组下已绑定的 API Key
            </p>
          </section>

          <section class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
            <p class="text-xs font-medium uppercase tracking-[0.18em] text-gray-500 dark:text-dark-400">校准状态</p>
            <div class="mt-3 space-y-2 text-sm text-gray-600 dark:text-dark-300">
              <div class="flex items-center justify-between gap-3">
                <span>5h 校准次数</span>
                <span class="font-medium text-gray-900 dark:text-white">{{ calibration5hCount }}</span>
              </div>
              <div class="flex items-center justify-between gap-3">
                <span>7d 校准次数</span>
                <span class="font-medium text-gray-900 dark:text-white">{{ calibration7dCount }}</span>
              </div>
            </div>
            <p class="mt-3 text-xs text-gray-500 dark:text-dark-400">
              5h 当前估算 {{ formatUSD(calibration5hEstimate) }} · 7d 当前估算 {{ formatUSD(calibration7dEstimate) }}
            </p>
          </section>
        </div>

        <section class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
          <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">半自动校准建议</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
                系统只生成建议，不会自动改正式配额；限流和用户查询页仍使用当前正式配额。
              </p>
            </div>
            <div class="flex gap-2">
              <button class="btn btn-secondary" :disabled="loadingCalibration || !calibrationStatus?.has_pending" @click="discardCalibration('all')">
                忽略建议
              </button>
              <button class="btn btn-primary" :disabled="loadingCalibration || !calibrationStatus?.has_pending" @click="applyCalibration('all')">
                立即更新
              </button>
            </div>
          </div>

          <div class="mt-5 grid gap-4 md:grid-cols-2">
            <div
              v-for="window in calibrationWindows"
              :key="window.window"
              class="rounded-lg border border-gray-200 p-4 dark:border-dark-700"
            >
              <div class="flex items-center justify-between gap-3">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ window.window }} 窗口</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
                    正式配额 {{ formatUSD(window.estimate) }}
                  </p>
                </div>
                <span
                  :class="[
                    'inline-flex rounded-full px-2.5 py-1 text-xs font-medium',
                    window.suggestion?.status === 'pending'
                      ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
                      : 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-300',
                  ]"
                >
                  {{ suggestionLabel(window.suggestion?.status) }}
                </span>
              </div>

              <div v-if="window.suggestion?.status === 'pending'" class="mt-4 space-y-2 text-sm text-gray-600 dark:text-dark-300">
                <div class="flex justify-between gap-4">
                  <span>建议配额</span>
                  <span class="font-medium text-gray-900 dark:text-white">{{ formatUSD(window.suggestion.suggested_estimate_usd) }}</span>
                </div>
                <div class="flex justify-between gap-4">
                  <span>变化比例</span>
                  <span :class="estimateDeltaClass(window.suggestion)">
                    {{ formatEstimateDelta(window.suggestion) }}
                  </span>
                </div>
                <div class="flex justify-between gap-4">
                  <span>本地采样</span>
                  <span>{{ formatUSD(window.suggestion.local_usd) }}</span>
                </div>
                <div class="flex justify-between gap-4">
                  <span>上游增量</span>
                  <span>{{ formatPct(window.suggestion.upstream_pct_delta) }}</span>
                </div>
                <div class="flex gap-2 pt-2">
                  <button class="btn btn-secondary btn-sm" :disabled="loadingCalibration" @click="discardCalibration(window.window as '5h' | '7d')">
                    忽略
                  </button>
                  <button class="btn btn-primary btn-sm" :disabled="loadingCalibration" @click="applyCalibration(window.window as '5h' | '7d')">
                    应用
                  </button>
                </div>
              </div>

              <p v-else class="mt-4 text-sm text-gray-500 dark:text-dark-400">
                {{ window.suggestion?.reason || '暂无可应用建议，等待更多上游百分比和本地用量采样。' }}
              </p>
            </div>
          </div>
        </section>

        <section class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
          <div class="flex items-center justify-between gap-3">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">下游 Key 分配明细</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
                所有 Key 都挂在管理员账号下，使用 Key 名称区分下游使用者。
              </p>
            </div>
            <div v-if="lastUpdatedLabel" class="text-xs text-gray-500 dark:text-dark-400">
              更新于 {{ lastUpdatedLabel }}
            </div>
          </div>

          <div class="mt-5 overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
              <thead>
                <tr class="text-left text-xs font-medium uppercase tracking-[0.18em] text-gray-500 dark:text-dark-400">
                  <th class="px-3 py-3">Key 名称</th>
                  <th class="px-3 py-3">状态</th>
                  <th class="px-3 py-3">权重</th>
                  <th class="px-3 py-3">5h 已用 / 限额</th>
                  <th class="px-3 py-3">7d 已用 / 限额</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
                <tr v-if="!status?.keys?.length">
                  <td colspan="5" class="px-3 py-8 text-center text-sm text-gray-500 dark:text-dark-400">
                    当前分组下还没有下游 Key。
                  </td>
                </tr>
                <tr v-for="key in status?.keys || []" :key="key.key_id">
                  <td class="px-3 py-4 align-top">
                    <div class="font-medium text-gray-900 dark:text-white">{{ key.key_name || `#${key.key_id}` }}</div>
                    <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">Key ID: {{ key.key_id }}</div>
                  </td>
                  <td class="px-3 py-4 align-top">
                    <span
                      :class="[
                        'inline-flex rounded-full px-2.5 py-1 text-xs font-medium',
                        key.status === 'active'
                          ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
                          : 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-300',
                      ]"
                    >
                      {{ key.status }}
                    </span>
                  </td>
                  <td class="px-3 py-4 align-top text-sm text-gray-900 dark:text-white">
                    {{ key.quota_weight }}
                    <span class="ml-2 text-xs text-gray-500 dark:text-dark-400">
                      {{ formatWeightPct(key.quota_weight) }}
                    </span>
                  </td>
                  <td class="px-3 py-4 align-top">
                    <div class="text-sm text-gray-900 dark:text-white">
                      {{ formatUSD(key.usage_5h) }} / {{ formatUSD(key.limit_5h) }}
                    </div>
                    <div class="mt-2 h-2 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                      <div class="h-full rounded-full bg-emerald-500 transition-all" :style="{ width: `${clampPct(key.usage_5h, key.limit_5h)}%` }" />
                    </div>
                  </td>
                  <td class="px-3 py-4 align-top">
                    <div class="text-sm text-gray-900 dark:text-white">
                      {{ formatUSD(key.usage_7d) }} / {{ formatUSD(key.limit_7d) }}
                    </div>
                    <div class="mt-2 h-2 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                      <div class="h-full rounded-full bg-indigo-500 transition-all" :style="{ width: `${clampPct(key.usage_7d, key.limit_7d)}%` }" />
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import type {
  AdminGroup,
  QuotaShareCalibrationStatusResponse,
  QuotaShareCalibrationSuggestion,
  QuotaShareStatusResponse
} from '@/types'
import { useAppStore } from '@/stores/app'

const appStore = useAppStore()
const route = useRoute()
const router = useRouter()

const groups = ref<AdminGroup[]>([])
const selectedGroupId = ref<number | null>(null)
const selectedGroup = ref<AdminGroup | null>(null)
const status = ref<QuotaShareStatusResponse | null>(null)
const calibrationStatus = ref<QuotaShareCalibrationStatusResponse | null>(null)
const loadingGroups = ref(false)
const loadingStatus = ref(false)
const loadingCalibration = ref(false)

const groupOptions = computed(() =>
  groups.value.map((group) => ({
    value: group.id,
    label: `${group.name} (ID:${group.id})`,
  }))
)

const window5hEstimate = computed(
  () => status.value?.group_state?.e5 ?? selectedGroup.value?.estimated_5h_limit_usd ?? 0
)
const window7dEstimate = computed(
  () => status.value?.group_state?.e7 ?? selectedGroup.value?.estimated_7d_limit_usd ?? 0
)

const calibration5h = computed(() => selectedGroup.value?.calibration_state?.['5h'] || selectedGroup.value?.calibration_state?.window_5h)
const calibration7d = computed(() => selectedGroup.value?.calibration_state?.['7d'] || selectedGroup.value?.calibration_state?.window_7d)
const calibration5hCount = computed(() => calibration5h.value?.calibration_count ?? 0)
const calibration7dCount = computed(() => calibration7d.value?.calibration_count ?? 0)
const calibration5hEstimate = computed(() => calibration5h.value?.current_estimate_usd ?? 0)
const calibration7dEstimate = computed(() => calibration7d.value?.current_estimate_usd ?? 0)
const calibrationWindows = computed(() => calibrationStatus.value?.windows || [])
const lastUpdatedLabel = computed(() => {
  const unix = status.value?.group_state?.uat
  return unix ? new Date(unix * 1000).toLocaleString() : ''
})

const formatUSD = (value?: number | null) => `$${Number(value || 0).toFixed(2)}`
const formatPct = (value?: number | null) => `${Number(value || 0).toFixed(1)}%`

const suggestionLabel = (status?: string | null) => {
  switch (status) {
    case 'pending':
      return '建议更新'
    case 'insufficient_data':
      return '数据不足'
    case 'rejected':
      return '已拒绝'
    case 'applied':
      return '已应用'
    case 'discarded':
      return '已忽略'
    default:
      return '暂无建议'
  }
}

const formatEstimateDelta = (suggestion?: QuotaShareCalibrationSuggestion | null) => {
  if (!suggestion?.suggested_estimate_usd || !suggestion.current_estimate_usd) return '—'
  const pct = ((suggestion.suggested_estimate_usd - suggestion.current_estimate_usd) / suggestion.current_estimate_usd) * 100
  return `${pct >= 0 ? '+' : ''}${pct.toFixed(1)}%`
}

const estimateDeltaClass = (suggestion?: QuotaShareCalibrationSuggestion | null) => {
  if (!suggestion?.suggested_estimate_usd || !suggestion.current_estimate_usd) return 'text-gray-500 dark:text-dark-400'
  return suggestion.suggested_estimate_usd >= suggestion.current_estimate_usd
    ? 'font-medium text-emerald-600 dark:text-emerald-400'
    : 'font-medium text-amber-600 dark:text-amber-400'
}

const formatWindow = (start?: number, end?: number) => {
  if (!start && !end) return '窗口尚未初始化'
  const startLabel = start ? new Date(start * 1000).toLocaleString() : '—'
  const endLabel = end ? new Date(end * 1000).toLocaleString() : '—'
  return `${startLabel} → ${endLabel}`
}

const clampPct = (used: number, limit: number) => {
  if (!limit || limit <= 0) return 0
  return Math.min((used / limit) * 100, 100)
}

const formatWeightPct = (weight: number) => {
  const total = status.value?.total_weight ?? 0
  if (!total) return '(0%)'
  return `(${((weight / total) * 100).toFixed(0)}%)`
}

const syncGroupQuery = (groupId: number | null) => {
  const nextQuery = { ...route.query }
  if (groupId) {
    nextQuery.group_id = String(groupId)
  } else {
    delete nextQuery.group_id
  }
  router.replace({ query: nextQuery })
}

const loadGroups = async () => {
  loadingGroups.value = true
  try {
    const response = await adminAPI.groups.list(1, 200, {
      sort_by: 'sort_order',
      sort_order: 'asc',
    })
    groups.value = (response.items || []).filter((group) => group.subscription_type === 'quota_share')

    const queryGroupId = Number(route.query.group_id || 0)
    const preferredGroup =
      groups.value.find((group) => group.id === queryGroupId) ||
      groups.value.find((group) => group.id === selectedGroupId.value) ||
      groups.value[0] ||
      null

    selectedGroupId.value = preferredGroup?.id ?? null
    selectedGroup.value = preferredGroup
    syncGroupQuery(selectedGroupId.value)
  } catch (error) {
    console.error('Failed to load quota share groups:', error)
    appStore.showError('加载 quota_share 分组失败')
  } finally {
    loadingGroups.value = false
  }
}

const loadGroupStatus = async (groupId: number) => {
  loadingStatus.value = true
  try {
    const [group, groupStatus, groupCalibration] = await Promise.all([
      adminAPI.groups.getById(groupId),
      adminAPI.groups.getQuotaShareStatus(groupId),
      adminAPI.groups.getQuotaShareCalibrationStatus(groupId),
    ])
    selectedGroup.value = group
    status.value = groupStatus
    calibrationStatus.value = groupCalibration
  } catch (error) {
    console.error('Failed to load quota share status:', error)
    appStore.showError('加载 quota_share 状态失败')
  } finally {
    loadingStatus.value = false
  }
}

const applyCalibration = async (window: '5h' | '7d' | 'all') => {
  if (!selectedGroupId.value) return
  loadingCalibration.value = true
  try {
    await adminAPI.groups.applyQuotaShareCalibration(selectedGroupId.value, window)
    appStore.showSuccess('quota_share 校准建议已应用')
    await loadGroupStatus(selectedGroupId.value)
  } catch (error) {
    console.error('Failed to apply quota share calibration:', error)
    appStore.showError('应用 quota_share 校准建议失败')
  } finally {
    loadingCalibration.value = false
  }
}

const discardCalibration = async (window: '5h' | '7d' | 'all') => {
  if (!selectedGroupId.value) return
  loadingCalibration.value = true
  try {
    await adminAPI.groups.discardQuotaShareCalibration(selectedGroupId.value, window)
    appStore.showSuccess('quota_share 校准建议已忽略')
    await loadGroupStatus(selectedGroupId.value)
  } catch (error) {
    console.error('Failed to discard quota share calibration:', error)
    appStore.showError('忽略 quota_share 校准建议失败')
  } finally {
    loadingCalibration.value = false
  }
}

const refreshAll = async () => {
  await loadGroups()
  if (selectedGroupId.value) {
    await loadGroupStatus(selectedGroupId.value)
  }
}

watch(selectedGroupId, async (groupId) => {
  syncGroupQuery(groupId)
  if (!groupId) {
    selectedGroup.value = null
    status.value = null
    calibrationStatus.value = null
    return
  }
  await loadGroupStatus(groupId)
})

onMounted(async () => {
  await refreshAll()
})
</script>
