<template>
  <BaseDialog :show="show" :title="t('admin.users.userApiKeys')" width="wide" @close="handleClose">
    <div v-if="user" class="space-y-4">
      <div class="flex items-center gap-3 rounded-xl bg-gray-50 p-4 dark:bg-dark-700">
        <div class="flex h-10 w-10 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30">
          <span class="text-lg font-medium text-primary-700 dark:text-primary-300">{{ user.email.charAt(0).toUpperCase() }}</span>
        </div>
        <div>
          <p class="font-medium text-gray-900 dark:text-white">{{ user.email }}</p>
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ user.username }}</p>
        </div>
      </div>

      <div
        v-if="canManageDistributionKeys"
        class="rounded-xl border border-cyan-200 bg-cyan-50/70 p-4 dark:border-cyan-900/40 dark:bg-cyan-900/10"
      >
        <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <p class="font-medium text-gray-900 dark:text-white">下游 Key 分发</p>
            <p class="mt-1 text-sm text-gray-600 dark:text-dark-300">
              所有下游 Key 都创建在当前管理员账号下，通过 Key 名称区分不同使用者；如需配额共享，请绑定 quota_share 分组并设置权重。
            </p>
          </div>
          <div class="flex items-center gap-2">
            <a
              href="/quota-share"
              target="_blank"
              rel="noopener noreferrer"
              class="btn btn-secondary btn-sm"
            >
              用户查询页
            </a>
            <button type="button" class="btn btn-primary btn-sm" @click="showCreateForm = !showCreateForm">
              {{ showCreateForm ? '收起创建表单' : '创建分发 Key' }}
            </button>
          </div>
        </div>

        <div v-if="showCreateForm" class="mt-4 grid gap-4 md:grid-cols-2">
          <div class="md:col-span-2">
            <label class="input-label">Key 名称</label>
            <input
              v-model="createForm.name"
              type="text"
              class="input"
              placeholder="例如：张三 / team-1 / 下游用户A"
            />
          </div>

          <div>
            <label class="input-label">绑定分组</label>
            <Select
              v-model="createForm.group_id"
              :options="distributionGroupOptions"
              placeholder="先创建 Key 再绑定分组也可以"
            />
          </div>

          <div v-if="selectedCreateGroup?.subscription_type === 'quota_share'">
            <label class="input-label">权重</label>
            <input
              v-model.number="createForm.quota_weight"
              type="number"
              min="1"
              step="1"
              class="input"
              placeholder="1"
            />
          </div>

          <div v-if="selectedCreateGroup?.subscription_type === 'quota_share'" class="md:col-span-2">
            <label class="input-label">共享额度用完后的个人兜底分组</label>
            <Select
              v-model="createForm.quota_share_overflow_group_id"
              :options="overflowGroupOptions"
              placeholder="未配置则共享额度用完后返回 429"
            />
          </div>

          <div>
            <label class="input-label">自定义 Key（可选）</label>
            <input
              v-model="createForm.custom_key"
              type="text"
              class="input font-mono"
              placeholder="留空则自动生成"
            />
          </div>

          <div>
            <label class="input-label">有效期（天，可选）</label>
            <input
              v-model.number="createForm.expires_in_days"
              type="number"
              min="1"
              step="1"
              class="input"
              placeholder="留空表示不过期"
            />
          </div>

          <div class="md:col-span-2 flex justify-end">
            <button type="button" class="btn btn-primary" :disabled="createSubmitting" @click="handleCreateDistributionKey">
              <svg
                v-if="createSubmitting"
                class="-ml-1 mr-2 h-4 w-4 animate-spin"
                fill="none"
                viewBox="0 0 24 24"
              >
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              {{ createSubmitting ? '创建中...' : '创建并分发' }}
            </button>
          </div>
        </div>
      </div>

      <div v-if="loading" class="flex justify-center py-8">
        <svg class="h-8 w-8 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
        </svg>
      </div>

      <div v-else-if="apiKeys.length === 0" class="py-8 text-center">
        <p class="text-sm text-gray-500">{{ t('admin.users.noApiKeys') }}</p>
      </div>

      <div v-else ref="scrollContainerRef" class="max-h-96 space-y-3 overflow-y-auto" @scroll="closeGroupSelector">
        <div v-for="key in apiKeys" :key="key.id" class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800">
          <div class="flex items-start justify-between gap-4">
            <div class="min-w-0 flex-1">
              <div class="mb-1 flex flex-wrap items-center gap-2">
                <span class="font-medium text-gray-900 dark:text-white">{{ key.name }}</span>
                <span :class="['badge text-xs', key.status === 'active' ? 'badge-success' : 'badge-danger']">{{ key.status }}</span>
                <span
                  v-if="key.group?.subscription_type === 'quota_share'"
                  class="inline-flex rounded-full bg-cyan-100 px-2 py-0.5 text-xs font-medium text-cyan-700 dark:bg-cyan-900/30 dark:text-cyan-400"
                >
                  quota_share
                </span>
              </div>
              <p class="truncate font-mono text-sm text-gray-500">{{ key.key.substring(0, 20) }}...{{ key.key.substring(key.key.length - 8) }}</p>
            </div>

            <a
              href="/quota-share"
              target="_blank"
              rel="noopener noreferrer"
              class="rounded-lg border border-gray-200 px-3 py-1.5 text-xs font-medium text-gray-600 transition-colors hover:bg-gray-50 hover:text-gray-900 dark:border-dark-600 dark:text-dark-300 dark:hover:bg-dark-700 dark:hover:text-white"
            >
              打开查询页
            </a>
          </div>

          <div class="mt-3 flex flex-wrap gap-4 text-xs text-gray-500">
            <div class="flex items-center gap-1">
              <span>{{ t('admin.users.group') }}:</span>
              <button
                :ref="(el) => setGroupButtonRef(key.id, el)"
                @click="openGroupSelector(key)"
                class="-mx-1 -my-0.5 flex cursor-pointer items-center gap-1 rounded-md px-1 py-0.5 transition-colors hover:bg-gray-100 dark:hover:bg-dark-700"
                :disabled="updatingKeyIds.has(key.id)"
              >
                <GroupBadge
                  v-if="key.group_id && key.group"
                  :name="key.group.name"
                  :platform="key.group.platform"
                  :subscription-type="key.group.subscription_type"
                  :rate-multiplier="key.group.rate_multiplier"
                />
                <span v-else class="text-gray-400 italic">{{ t('admin.users.none') }}</span>
                <svg v-if="updatingKeyIds.has(key.id)" class="h-3 w-3 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg>
                <svg v-else class="h-3 w-3 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M8.25 15L12 18.75 15.75 15m-7.5-6L12 5.25 15.75 9" /></svg>
              </button>
            </div>

            <div v-if="key.group?.subscription_type === 'quota_share'" class="flex items-center gap-2">
              <span>权重:</span>
              <input
                v-model.number="weightDrafts[key.id]"
                type="number"
                min="1"
                step="1"
                class="h-8 w-20 rounded-lg border border-gray-200 bg-white px-2 text-xs text-gray-900 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
              />
              <button
                type="button"
                class="rounded-lg border border-cyan-200 px-2.5 py-1 text-xs font-medium text-cyan-700 transition-colors hover:bg-cyan-50 dark:border-cyan-900/40 dark:text-cyan-300 dark:hover:bg-cyan-900/20"
                :disabled="updatingKeyIds.has(key.id)"
                @click="updateQuotaWeight(key)"
              >
                保存权重
              </button>
            </div>

            <div v-if="key.group?.subscription_type === 'quota_share'" class="flex min-w-[260px] items-center gap-2">
              <span>兜底分组:</span>
              <div class="w-56">
                <Select
                  :model-value="key.quota_share_overflow_group_id ?? null"
                  :options="overflowGroupOptions"
                  :disabled="updatingKeyIds.has(key.id)"
                  placeholder="未配置"
                  @change="(value) => updateQuotaShareOverflowGroup(key, value as number | null)"
                />
              </div>
            </div>

            <div class="flex items-center gap-1">
              <span>{{ t('admin.users.columns.created') }}:</span>
              <span>{{ formatDateTime(key.created_at) }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </BaseDialog>

  <Teleport to="body">
    <div
      v-if="groupSelectorKeyId !== null && dropdownPosition"
      ref="dropdownRef"
      class="animate-in fade-in slide-in-from-top-2 fixed z-[100000020] w-64 overflow-hidden rounded-xl bg-white shadow-lg ring-1 ring-black/5 duration-200 dark:bg-dark-800 dark:ring-white/10"
      :style="{ top: dropdownPosition.top + 'px', left: dropdownPosition.left + 'px' }"
    >
      <div class="max-h-64 overflow-y-auto p-1.5">
        <button
          @click="changeGroup(selectedKeyForGroup!, null)"
          :class="[
            'flex w-full items-center rounded-lg px-3 py-2 text-sm transition-colors',
            !selectedKeyForGroup?.group_id
              ? 'bg-primary-50 dark:bg-primary-900/20'
              : 'hover:bg-gray-100 dark:hover:bg-dark-700'
          ]"
        >
          <span class="text-gray-500 italic">{{ t('admin.users.none') }}</span>
          <svg
            v-if="!selectedKeyForGroup?.group_id"
            class="ml-auto h-4 w-4 shrink-0 text-primary-600 dark:text-primary-400"
            fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2"
          ><path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" /></svg>
        </button>

        <button
          v-for="group in allGroups"
          :key="group.id"
          @click="changeGroup(selectedKeyForGroup!, group.id)"
          :class="[
            'flex w-full items-center justify-between rounded-lg px-3 py-2 text-sm transition-colors',
            selectedKeyForGroup?.group_id === group.id
              ? 'bg-primary-50 dark:bg-primary-900/20'
              : 'hover:bg-gray-100 dark:hover:bg-dark-700'
          ]"
        >
          <GroupOptionItem
            :name="group.name"
            :platform="group.platform"
            :subscription-type="group.subscription_type"
            :rate-multiplier="group.rate_multiplier"
            :description="group.description"
            :selected="selectedKeyForGroup?.group_id === group.id"
          />
        </button>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, type ComponentPublicInstance } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { adminAPI } from '@/api/admin'
import { keysAPI } from '@/api'
import { formatDateTime } from '@/utils/format'
import type { AdminUser, AdminGroup, ApiKey } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import GroupOptionItem from '@/components/common/GroupOptionItem.vue'
import Select from '@/components/common/Select.vue'

const props = defineProps<{ show: boolean; user: AdminUser | null }>()
const emit = defineEmits(['close'])
const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const apiKeys = ref<ApiKey[]>([])
const allGroups = ref<AdminGroup[]>([])
const loading = ref(false)
const createSubmitting = ref(false)
const updatingKeyIds = ref(new Set<number>())
const groupSelectorKeyId = ref<number | null>(null)
const dropdownPosition = ref<{ top: number; left: number } | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)
const scrollContainerRef = ref<HTMLElement | null>(null)
const groupButtonRefs = ref<Map<number, HTMLElement>>(new Map())
const showCreateForm = ref(false)
const weightDrafts = ref<Record<number, number>>({})

const createForm = ref({
  name: '',
  group_id: null as number | null,
  quota_share_overflow_group_id: null as number | null,
  quota_weight: 1,
  custom_key: '',
  expires_in_days: null as number | null,
})

const canManageDistributionKeys = computed(() => {
  if (!props.user || !authStore.user) return false
  return authStore.isAdmin && props.user.id === authStore.user.id
})

const distributionGroupOptions = computed(() => [
  { value: null, label: '暂不绑定分组' },
  ...allGroups.value.map((group) => ({
    value: group.id,
    label: group.subscription_type === 'quota_share' ? `${group.name} · quota_share` : group.name,
  })),
])

const overflowGroupOptions = computed(() => [
  { value: null, label: '不配置个人兜底' },
  ...allGroups.value
    .filter((group) => group.status === 'active' && group.platform === 'openai' && group.subscription_type === 'standard')
    .map((group) => ({
      value: group.id,
      label: group.name,
    })),
])

const selectedCreateGroup = computed(() => {
  if (createForm.value.group_id == null) return null
  return allGroups.value.find((group) => group.id === createForm.value.group_id) || null
})

const selectedKeyForGroup = computed(() => {
  if (groupSelectorKeyId.value === null) return null
  return apiKeys.value.find((k) => k.id === groupSelectorKeyId.value) || null
})

const setGroupButtonRef = (keyId: number, el: Element | ComponentPublicInstance | null) => {
  if (el instanceof HTMLElement) {
    groupButtonRefs.value.set(keyId, el)
  } else {
    groupButtonRefs.value.delete(keyId)
  }
}

const resetCreateForm = () => {
  createForm.value = {
    name: '',
    group_id: null,
    quota_share_overflow_group_id: null,
    quota_weight: 1,
    custom_key: '',
    expires_in_days: null,
  }
}

const syncWeightDrafts = (keys: ApiKey[]) => {
  const drafts: Record<number, number> = {}
  for (const key of keys) {
    drafts[key.id] = key.quota_weight && key.quota_weight > 0 ? key.quota_weight : 1
  }
  weightDrafts.value = drafts
}

watch(
  () => props.show,
  (visible) => {
    if (visible && props.user) {
      load()
      loadGroups()
    } else {
      closeGroupSelector()
      showCreateForm.value = false
      resetCreateForm()
    }
  }
)

const load = async () => {
  if (!props.user) return
  loading.value = true
  groupButtonRefs.value.clear()
  try {
    const res = await adminAPI.users.getUserApiKeys(props.user.id)
    apiKeys.value = res.items || []
    syncWeightDrafts(apiKeys.value)
  } catch (error) {
    console.error('Failed to load API keys:', error)
    appStore.showError('加载用户 API Key 失败')
  } finally {
    loading.value = false
  }
}

const loadGroups = async () => {
  try {
    const groups = await adminAPI.groups.getAll()
    allGroups.value = groups
  } catch (error) {
    console.error('Failed to load groups:', error)
    appStore.showError('加载分组失败')
  }
}

const handleCreateDistributionKey = async () => {
  if (!createForm.value.name.trim()) {
    appStore.showError('请先填写 Key 名称')
    return
  }

  const selectedGroup = selectedCreateGroup.value
  const shouldBindQuotaShare = selectedGroup?.subscription_type === 'quota_share'
  const quotaWeight = shouldBindQuotaShare ? Math.max(1, Number(createForm.value.quota_weight || 1)) : undefined

  createSubmitting.value = true
  try {
    const created = await keysAPI.create(
      createForm.value.name.trim(),
      undefined,
      createForm.value.custom_key.trim() || undefined,
      undefined,
      undefined,
      undefined,
      createForm.value.expires_in_days && createForm.value.expires_in_days > 0
        ? createForm.value.expires_in_days
        : undefined
    )

    if (createForm.value.group_id !== null) {
      await adminAPI.apiKeys.updateApiKeyGroup(created.id, {
        groupId: createForm.value.group_id,
        quotaWeight,
        quotaShareOverflowGroupId: shouldBindQuotaShare ? createForm.value.quota_share_overflow_group_id : null,
      })
    }

    appStore.showSuccess('分发 Key 创建成功')
    showCreateForm.value = false
    resetCreateForm()
    await load()
  } catch (error: any) {
    appStore.showError(error?.message || '创建分发 Key 失败')
  } finally {
    createSubmitting.value = false
  }
}

const DROPDOWN_HEIGHT = 272
const DROPDOWN_GAP = 4

const openGroupSelector = (key: ApiKey) => {
  if (groupSelectorKeyId.value === key.id) {
    closeGroupSelector()
    return
  }

  const buttonEl = groupButtonRefs.value.get(key.id)
  if (buttonEl) {
    const rect = buttonEl.getBoundingClientRect()
    const spaceBelow = window.innerHeight - rect.bottom
    const openUpward = spaceBelow < DROPDOWN_HEIGHT && rect.top > spaceBelow
    dropdownPosition.value = {
      top: openUpward ? rect.top - DROPDOWN_HEIGHT - DROPDOWN_GAP : rect.bottom + DROPDOWN_GAP,
      left: rect.left,
    }
  }
  groupSelectorKeyId.value = key.id
}

const closeGroupSelector = () => {
  groupSelectorKeyId.value = null
  dropdownPosition.value = null
}

const changeGroup = async (key: ApiKey, newGroupId: number | null) => {
  closeGroupSelector()
  if (key.group_id === newGroupId || (!key.group_id && newGroupId === null)) return

  const targetGroup = newGroupId == null ? null : allGroups.value.find((group) => group.id === newGroupId) || null
  const quotaWeight = targetGroup?.subscription_type === 'quota_share'
    ? Math.max(1, Number(weightDrafts.value[key.id] || key.quota_weight || 1))
    : undefined

  updatingKeyIds.value.add(key.id)
  try {
    const result = await adminAPI.apiKeys.updateApiKeyGroup(key.id, {
      groupId: newGroupId,
      quotaWeight,
    })
    const idx = apiKeys.value.findIndex((k) => k.id === key.id)
    if (idx !== -1) {
      apiKeys.value[idx] = result.api_key
      weightDrafts.value[key.id] = result.api_key.quota_weight && result.api_key.quota_weight > 0 ? result.api_key.quota_weight : 1
    }
    if (result.auto_granted_group_access && result.granted_group_name) {
      appStore.showSuccess(t('admin.users.groupChangedWithGrant', { group: result.granted_group_name }))
    } else {
      appStore.showSuccess(t('admin.users.groupChangedSuccess'))
    }
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.users.groupChangeFailed'))
  } finally {
    updatingKeyIds.value.delete(key.id)
  }
}

const updateQuotaWeight = async (key: ApiKey) => {
  const nextWeight = Math.max(1, Number(weightDrafts.value[key.id] || 1))
  updatingKeyIds.value.add(key.id)
  try {
    const result = await adminAPI.apiKeys.updateApiKeyGroup(key.id, {
      quotaWeight: nextWeight,
    })
    const idx = apiKeys.value.findIndex((k) => k.id === key.id)
    if (idx !== -1) {
      apiKeys.value[idx] = result.api_key
      weightDrafts.value[key.id] = result.api_key.quota_weight && result.api_key.quota_weight > 0 ? result.api_key.quota_weight : nextWeight
    }
    appStore.showSuccess('权重已更新')
  } catch (error: any) {
    appStore.showError(error?.message || '更新权重失败')
  } finally {
    updatingKeyIds.value.delete(key.id)
  }
}

const updateQuotaShareOverflowGroup = async (key: ApiKey, value: number | null) => {
  updatingKeyIds.value.add(key.id)
  try {
    const result = await adminAPI.apiKeys.updateApiKeyGroup(key.id, {
      quotaShareOverflowGroupId: value,
    })
    const idx = apiKeys.value.findIndex((k) => k.id === key.id)
    if (idx !== -1) {
      apiKeys.value[idx] = result.api_key
    }
    appStore.showSuccess(value ? '兜底分组已更新' : '兜底分组已清空')
  } catch (error: any) {
    appStore.showError(error?.message || '更新兜底分组失败')
  } finally {
    updatingKeyIds.value.delete(key.id)
  }
}

const handleKeyDown = (event: KeyboardEvent) => {
  if (event.key === 'Escape' && groupSelectorKeyId.value !== null) {
    event.stopPropagation()
    closeGroupSelector()
  }
}

const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  if (dropdownRef.value && !dropdownRef.value.contains(target)) {
    for (const el of groupButtonRefs.value.values()) {
      if (el.contains(target)) return
    }
    closeGroupSelector()
  }
}

const handleClose = () => {
  closeGroupSelector()
  emit('close')
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  document.addEventListener('keydown', handleKeyDown, true)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  document.removeEventListener('keydown', handleKeyDown, true)
})
</script>
