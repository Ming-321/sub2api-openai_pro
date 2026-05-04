<template>
  <BaseDialog
    :show="showDialog"
    title="quota_share 校准提醒"
    width="normal"
    :close-on-click-outside="false"
    @close="snooze"
  >
    <div class="space-y-4">
      <div>
        <p class="text-sm font-medium text-gray-900 dark:text-white">
          {{ hasPending ? '有新的配额校准建议' : '暂时无法更新配额' }}
        </p>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
          {{ summaryText }}
        </p>
      </div>

      <div class="max-h-64 space-y-2 overflow-y-auto">
        <div
          v-for="group in reminder?.groups || []"
          :key="group.group_id"
          class="rounded-lg border border-gray-200 p-3 dark:border-dark-700"
        >
          <div class="flex items-center justify-between gap-3">
            <span class="text-sm font-medium text-gray-900 dark:text-white">{{ group.group_name }}</span>
            <span
              :class="[
                'inline-flex rounded-full px-2.5 py-1 text-xs font-medium',
                group.has_pending
                  ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
                  : 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-300',
              ]"
            >
              {{ group.has_pending ? '建议更新' : '数据不足' }}
            </span>
          </div>
          <p class="mt-2 text-xs text-gray-500 dark:text-dark-400">
            {{ group.reason || '等待更多采样数据' }}
          </p>
        </div>
      </div>
    </div>

    <template #footer>
      <button class="btn btn-secondary" :disabled="submitting" @click="ignoreToday">
        忽略
      </button>
      <button class="btn btn-secondary" :disabled="submitting" @click="snooze">
        下次再通知
      </button>
      <button class="btn btn-primary" :disabled="submitting || !hasPending" @click="applyPending">
        立即更新
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import type { QuotaShareCalibrationReminderResponse } from '@/types'

const appStore = useAppStore()
const authStore = useAuthStore()

const reminder = ref<QuotaShareCalibrationReminderResponse | null>(null)
const showDialog = ref(false)
const submitting = ref(false)
const checkedForUser = ref<number | null>(null)

const todayKey = () => {
  const now = new Date()
  const month = `${now.getMonth() + 1}`.padStart(2, '0')
  const day = `${now.getDate()}`.padStart(2, '0')
  return `${now.getFullYear()}-${month}-${day}`
}
const storagePrefix = computed(() => `quota_share_calibration:${authStore.user?.id || 'anonymous'}:${todayKey()}`)
const ignoredKey = computed(() => `${storagePrefix.value}:ignored`)
const snoozedKey = computed(() => `${storagePrefix.value}:snoozed`)
const hasPending = computed(() => (reminder.value?.pending_count || 0) > 0)
const summaryText = computed(() => {
  if (!reminder.value?.has_quota_share) return '当前没有 quota_share 分组。'
  if (hasPending.value) return `${reminder.value.pending_count} 个分组有可应用建议。`
  return '已有 quota_share 分组，但当前采样数据不足。'
})

const shouldSkip = () => {
  return localStorage.getItem(ignoredKey.value) === '1' || sessionStorage.getItem(snoozedKey.value) === '1'
}

const loadReminder = async () => {
  if (!authStore.isAdmin || !authStore.user?.id || shouldSkip()) return
  checkedForUser.value = authStore.user.id
  try {
    const data = await adminAPI.groups.getQuotaShareCalibrationReminder()
    reminder.value = data
    showDialog.value = data.has_quota_share && !shouldSkip()
  } catch (error) {
    console.error('Failed to load quota_share calibration reminder:', error)
  }
}

const ignoreToday = () => {
  localStorage.setItem(ignoredKey.value, '1')
  showDialog.value = false
}

const snooze = () => {
  sessionStorage.setItem(snoozedKey.value, '1')
  showDialog.value = false
}

const applyPending = async () => {
  const pendingGroups = (reminder.value?.groups || []).filter((group) => group.has_pending)
  if (!pendingGroups.length) return
  submitting.value = true
  try {
    for (const group of pendingGroups) {
      await adminAPI.groups.applyQuotaShareCalibration(group.group_id, 'all')
    }
    appStore.showSuccess('quota_share 校准建议已应用')
    ignoreToday()
  } catch (error) {
    console.error('Failed to apply quota_share calibration reminder:', error)
    appStore.showError('应用 quota_share 校准建议失败')
  } finally {
    submitting.value = false
  }
}

onMounted(loadReminder)

watch(
  () => authStore.user?.id,
  (userID) => {
    if (userID && checkedForUser.value !== userID) {
      loadReminder()
    }
  }
)
</script>
