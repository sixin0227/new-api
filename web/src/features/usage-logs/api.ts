/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { api, getFreshAuthHeaders, type ApiRequestConfig } from '@/lib/api'
import { createServerError } from '@/lib/server-error-message'

import type { UsageLog } from './data/schema'
import { buildQueryParams } from './lib/query-params'
import { parseTaskArtifactsResponse } from './lib/task-artifacts'
import type {
  GetLogsParams,
  GetLogsResponse,
  GetLogStatsParams,
  GetLogStatsResponse,
  GetMidjourneyLogsParams,
  GetTaskLogsParams,
  TaskArtifactsResponse,
  UserInfo,
} from './types'

// ============================================================================
// Generic API Helpers
// ============================================================================

function buildApiPath(endpoint: string, isAdmin: boolean): string {
  return isAdmin ? endpoint : `${endpoint}/self`
}

async function fetchLogs<T>(
  endpoint: string,
  params: T,
  isAdmin: boolean
): Promise<GetLogsResponse> {
  const paramRecord = params as unknown as Record<string, unknown>
  const queryParams = buildQueryParams({
    p: paramRecord.p || 1,
    page_size: paramRecord.page_size || 20,
    ...params,
  })
  const path = buildApiPath(endpoint, isAdmin)
  const res = await api.get(`${path}?${queryParams}`)
  return res.data
}

async function fetchLogStats<T>(
  endpoint: string,
  params: T,
  isAdmin: boolean
): Promise<GetLogStatsResponse> {
  const queryParams = buildQueryParams(
    params as unknown as Record<string, unknown>
  )
  const path = buildApiPath(endpoint, isAdmin)
  const res = await api.get(`${path}/stat?${queryParams}`)
  return res.data
}

// ============================================================================
// Common Log APIs
// ============================================================================

export const getAllLogs = (params: GetLogsParams = {}) =>
  fetchLogs('/api/log', params, true)

export const getUserLogs = (
  params: Omit<GetLogsParams, 'username' | 'channel'> = {}
) => fetchLogs('/api/log', params, false)

export const getLogStats = (params: GetLogStatsParams = {}) =>
  fetchLogStats('/api/log', params, true)

export const getUserLogStats = (
  params: Omit<GetLogStatsParams, 'username' | 'channel'> = {}
) => fetchLogStats('/api/log', params, false)

/**
 * Stream the logs matching params, newest first. The server sends one JSON
 * log per line and ends with an API response object; a stream that ends
 * without a successful one is a failed export. Resolves with the log count.
 */
export async function exportLogs(
  params: GetLogsParams,
  isAdmin: boolean,
  options: { signal: AbortSignal; onLog: (log: UsageLog) => void }
): Promise<number> {
  const queryParams = buildQueryParams(
    params as unknown as Record<string, unknown>
  )
  const response = await fetch(
    `${buildApiPath('/api/log', isAdmin)}/export?${queryParams}`,
    {
      credentials: 'include',
      headers: await getFreshAuthHeaders(),
      signal: options.signal,
    }
  )
  if (!response.ok || !response.body) {
    const body: unknown = await response.json().catch(() => null)
    throw createServerError(body ?? { status: response.status })
  }

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let result: { success?: unknown; data?: { count?: number } } | null = null
  for (;;) {
    const { done, value } = await reader.read()
    buffer += done ? decoder.decode() : decoder.decode(value, { stream: true })
    const lines = buffer.split('\n')
    buffer = done ? '' : (lines.pop() ?? '')
    for (const line of lines) {
      if (!line.trim()) continue
      let parsed
      try {
        parsed = JSON.parse(line)
      } catch {
        throw createServerError(null)
      }
      if (typeof parsed.success === 'boolean') result = parsed
      else options.onLog(parsed as UsageLog)
    }
    if (done) break
  }
  if (result?.success !== true) {
    throw createServerError(result)
  }
  return result.data?.count ?? 0
}

export async function getUserInfo(
  userId: number
): Promise<{ success: boolean; message?: string; data?: UserInfo }> {
  const res = await api.get(`/api/user/${userId}`)
  return res.data
}

export interface RequestBodiesData {
  request_body: string
  response_body: string
  request_skipped: boolean
  response_skipped: boolean
}

export interface RequestBodiesResponse {
  success: boolean
  message?: string
  data?: RequestBodiesData
}

/** Recorded request/response bodies of one relay exchange (admin only). */
export const getRequestBody = (requestId: string) =>
  api.get<RequestBodiesResponse>(
    `/api/log/body?request_id=${encodeURIComponent(requestId)}`
  )

/** Bulk-delete whole usage-log entries (metadata and recorded bodies). */
export const batchDeleteLogs = (ids: number[]) =>
  api.post<{ success: boolean; message?: string; data?: { deleted: number } }>(
    '/api/log/batch-delete',
    { ids }
  )

// ============================================================================
// Request Body Browser APIs (admin only)
// ============================================================================

export interface RequestBodyListItem {
  id: number
  request_id: string
  user_id: number
  username: string
  created_at: number
  model: string
  has_log_metadata: boolean
  channel_id: number
  channel_name: string
  token_name: string
  is_stream: boolean
  prompt_tokens: number
  completion_tokens: number
  use_time: number
  request_preview: string
  response_preview: string
  request_body: string
  response_body: string
  request_skipped: boolean
  response_skipped: boolean
}

export interface GetRequestBodiesParams {
  p?: number
  page_size?: number
  start_timestamp?: number
  end_timestamp?: number
  contains?: string
  user_id?: number
}

export interface RequestBodiesListResponse {
  success: boolean
  message?: string
  data?: {
    items: RequestBodyListItem[]
    total: number
    page: number
    page_size: number
  }
}

/** List recorded bodies with their summaries and full payloads (admin only). */
export const getRequestBodiesList = (params: GetRequestBodiesParams = {}) =>
  api.get<RequestBodiesListResponse>(
    `/api/log/bodies?${buildQueryParams(params as unknown as Record<string, unknown>)}`
  )

/** Load the full recorded bodies for the selected exchanges (admin only). */
export const exportRequestBodies = (requestIds: string[]) =>
  api.post<{ success: boolean; message?: string; data?: { items: unknown[] } }>(
    '/api/log/bodies/export',
    { request_ids: requestIds }
  )

/** Delete the selected exchanges' bodies and their usage-log rows. */
export const deleteRequestBodies = (requestIds: string[]) =>
  api.post<{ success: boolean; message?: string; data?: { deleted: number } }>(
    '/api/log/bodies/delete',
    { request_ids: requestIds }
  )

export interface DeleteAllRequestBodiesFilters {
  contains?: string
  user_id?: number
  start_timestamp?: number
  end_timestamp?: number
}

/** Delete every recorded exchange matching the browse filters (admin only). */
export const deleteAllRequestBodies = (
  filters: DeleteAllRequestBodiesFilters
) =>
  api.post<{ success: boolean; message?: string; data?: { deleted: number } }>(
    '/api/log/bodies/delete-all',
    filters
  )

// ============================================================================
// MjProxy (Drawing) Logs API
// ============================================================================

export const getAllMidjourneyLogs = (params: GetMidjourneyLogsParams) =>
  fetchLogs('/api/mj', params, true)

export const getUserMidjourneyLogs = (params: GetMidjourneyLogsParams) =>
  fetchLogs('/api/mj', params, false)

// ============================================================================
// Task Logs API
// ============================================================================

export const getAllTaskLogs = (params: GetTaskLogsParams) =>
  fetchLogs('/api/task', params, true)

export const getUserTaskLogs = (params: GetTaskLogsParams) =>
  fetchLogs('/api/task', params, false)

const taskArtifactRequestConfig = {
  skipBusinessError: true,
  skipErrorHandler: true,
} satisfies ApiRequestConfig

export async function getTaskArtifacts(taskId: string) {
  const response = await api.get<TaskArtifactsResponse>(
    `/api/task/${encodeURIComponent(taskId)}/artifacts`,
    taskArtifactRequestConfig
  )
  return parseTaskArtifactsResponse(response.data)
}
