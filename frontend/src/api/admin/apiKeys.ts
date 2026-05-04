/**
 * Admin API Keys API endpoints
 * Handles API key management for administrators
 */

import { apiClient } from '../client'
import type { ApiKey } from '@/types'

export interface UpdateApiKeyGroupResult {
  api_key: ApiKey
  auto_granted_group_access: boolean
  granted_group_id?: number
  granted_group_name?: string
}

/**
 * Update an API key's group binding
 * @param id - API Key ID
 * @param groupId - Group ID (0 to unbind, positive to bind, null/undefined to skip)
 * @returns Updated API key with auto-grant info
 */
export async function updateApiKeyGroup(
  id: number,
  options: {
    groupId?: number | null
    quotaWeight?: number
    quotaShareOverflowGroupId?: number | null
    resetRateLimitUsage?: boolean
  }
): Promise<UpdateApiKeyGroupResult> {
  const payload: Record<string, unknown> = {}

  if (options.groupId !== undefined) {
    payload.group_id = options.groupId === null ? 0 : options.groupId
  }
  if (options.quotaWeight !== undefined) {
    payload.quota_weight = options.quotaWeight
  }
  if (options.quotaShareOverflowGroupId !== undefined) {
    payload.quota_share_overflow_group_id = options.quotaShareOverflowGroupId === null ? 0 : options.quotaShareOverflowGroupId
  }
  if (options.resetRateLimitUsage !== undefined) {
    payload.reset_rate_limit_usage = options.resetRateLimitUsage
  }

  const { data } = await apiClient.put<UpdateApiKeyGroupResult>(`/admin/api-keys/${id}`, payload)
  return data
}

export const apiKeysAPI = {
  updateApiKeyGroup
}

export default apiKeysAPI
