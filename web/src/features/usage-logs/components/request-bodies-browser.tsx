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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ChevronLeft,
  ChevronRight,
  Download,
  Loader2,
  MessageSquareText,
  Search,
  ShieldAlert,
  Trash2,
} from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { EmptyState } from '@/components/empty-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  deleteAllRequestBodies,
  deleteRequestBodies,
  exportRequestBodies,
  getRequestBodiesList,
} from '../api'
import { downloadJsonFile } from '../lib/request-body-view'
import { BodiesRow, type BodyViewMode } from './bodies-row'
import { useLogsViewScope } from './usage-logs-provider'

const BODIES_PAGE_SIZE = 10

/**
 * Admin browser for recorded request/response bodies: every row renders the
 * recorded exchange directly — chat payloads as bubbles, other payloads as
 * JSON — with a per-row and a page-wide switch to the raw JSON view.
 */
export function RequestBodiesBrowser() {
  const { t } = useTranslation()
  const { isAdminView } = useLogsViewScope()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [containsInput, setContainsInput] = useState('')
  const [userIdInput, setUserIdInput] = useState('')
  const [filters, setFilters] = useState({ contains: '', userId: '' })
  const [listMode, setListMode] = useState<BodyViewMode>('conversation')
  const [modeOverrides, setModeOverrides] = useState<Map<string, BodyViewMode>>(
    new Map()
  )
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [deleteIds, setDeleteIds] = useState<string[] | null>(null)
  const [deleteAllOpen, setDeleteAllOpen] = useState(false)

  const { data, isLoading, isFetching, refetch } = useQuery({
    queryKey: [
      'request-bodies-browser',
      page,
      filters.contains,
      filters.userId,
    ],
    enabled: isAdminView,
    queryFn: async () => {
      const response = requireServerSuccess(
        (
          await getRequestBodiesList({
            p: page,
            page_size: BODIES_PAGE_SIZE,
            contains: filters.contains || undefined,
            user_id: filters.userId ? Number(filters.userId) : undefined,
          })
        ).data
      )
      return response.data
    },
  })

  const items = useMemo(() => data?.items ?? [], [data])
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / BODIES_PAGE_SIZE))
  const selectedIds = useMemo(() => [...selected], [selected])

  // Deleting can empty the last page; fall back to the last page that exists.
  useEffect(() => {
    if (page > totalPages) setPage(totalPages)
  }, [page, totalPages])

  const clearRowState = () => {
    setSelected(new Set())
    setModeOverrides(new Map())
  }

  const exportMutation = useMutation({
    mutationFn: async (requestIds: string[]) =>
      requireServerSuccess((await exportRequestBodies(requestIds)).data).data,
    onSuccess: (result) => {
      downloadJsonFile(`request-bodies-export-${Date.now()}.json`, {
        items: result?.items ?? [],
      })
      toast.success(
        t('Exported {{count}} record(s)', {
          count: result?.items?.length ?? 0,
        })
      )
    },
  })

  const deleteMutation = useMutation({
    mutationFn: async (requestIds: string[]) =>
      requireServerSuccess((await deleteRequestBodies(requestIds)).data).data,
    onSuccess: (result) => {
      toast.success(
        t('Deleted {{count}} record(s)', { count: result?.deleted ?? 0 })
      )
      clearRowState()
      setDeleteIds(null)
      queryClient.invalidateQueries({ queryKey: ['request-bodies-browser'] })
    },
  })

  const deleteAllMutation = useMutation({
    mutationFn: async () =>
      requireServerSuccess(
        (
          await deleteAllRequestBodies({
            contains: filters.contains || undefined,
            user_id: filters.userId ? Number(filters.userId) : undefined,
          })
        ).data
      ).data,
    onSuccess: (result) => {
      toast.success(
        t('Deleted {{count}} record(s)', { count: result?.deleted ?? 0 })
      )
      clearRowState()
      setDeleteAllOpen(false)
      setPage(1)
      queryClient.invalidateQueries({ queryKey: ['request-bodies-browser'] })
    },
  })

  if (!isAdminView) {
    return (
      <EmptyState
        icon={ShieldAlert}
        title={t('Administrator access required')}
        description={t(
          'Recorded request and response bodies are only visible to administrators.'
        )}
      />
    )
  }

  const handleSearch = () => {
    setPage(1)
    setFilters({ contains: containsInput.trim(), userId: userIdInput.trim() })
    clearRowState()
  }

  const handleReset = () => {
    setContainsInput('')
    setUserIdInput('')
    setPage(1)
    setFilters({ contains: '', userId: '' })
    clearRowState()
  }

  const goToPage = (next: number) => {
    setPage(next)
    clearRowState()
  }

  const handleToggleSelected = (requestId: string, isSelected: boolean) => {
    setSelected((previous) => {
      const next = new Set(previous)
      if (isSelected) next.add(requestId)
      else next.delete(requestId)
      return next
    })
  }

  const handleToggleMode = (requestId: string) => {
    setModeOverrides((previous) => {
      const next = new Map(previous)
      const current = next.get(requestId) ?? listMode
      next.set(requestId, current === 'conversation' ? 'raw' : 'conversation')
      return next
    })
  }

  // The page-wide switch sets every row at once and drops the per-row
  // overrides, so the whole list shows a single consistent mode.
  const handleListModeChange = (value: string[]) => {
    const next = value.find((entry) => entry !== listMode) ?? value.at(-1)
    if (next !== 'conversation' && next !== 'raw') return
    if (next === listMode) return
    setListMode(next)
    setModeOverrides(new Map())
  }

  const allSelected =
    items.length > 0 && items.every((item) => selected.has(item.request_id))

  let listContent = null
  if (isLoading) {
    listContent = <LoadingState size='sm' />
  } else if (items.length === 0) {
    listContent = (
      <EmptyState
        icon={MessageSquareText}
        title={t('No recorded bodies')}
        description={t(
          'Bodies are recorded only when body recording is enabled and the request used a token with the per-token switch on.'
        )}
        className='min-h-48'
      />
    )
  } else {
    listContent = (
      <div className='divide-y overflow-hidden rounded-lg border'>
        <label className='flex items-center gap-2 px-3 py-2 text-xs'>
          <Checkbox
            checked={allSelected}
            onCheckedChange={(value) => {
              setSelected(
                value
                  ? new Set(items.map((item) => item.request_id))
                  : new Set()
              )
            }}
          />
          {t('Select all')}
        </label>
        {items.map((item) => (
          <BodiesRow
            key={item.request_id}
            item={item}
            mode={modeOverrides.get(item.request_id) ?? listMode}
            selected={selected.has(item.request_id)}
            onToggleSelected={handleToggleSelected}
            onToggleMode={handleToggleMode}
            onDelete={(target) => setDeleteIds([target.request_id])}
          />
        ))}
      </div>
    )
  }

  return (
    <div className='flex h-full min-h-0 flex-col gap-3'>
      <div className='flex flex-wrap items-center gap-2'>
        <Input
          className='h-8 w-full sm:w-64'
          placeholder={t('Search in bodies')}
          aria-label={t('Search in bodies')}
          value={containsInput}
          onChange={(event) => setContainsInput(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') handleSearch()
          }}
        />
        <Input
          className='h-8 w-full sm:w-40'
          placeholder={t('User ID')}
          aria-label={t('User ID')}
          inputMode='numeric'
          value={userIdInput}
          onChange={(event) => setUserIdInput(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') handleSearch()
          }}
        />
        <Button size='sm' className='h-8' onClick={handleSearch}>
          <Search className='size-3.5' aria-hidden='true' />
          {t('Search')}
        </Button>
        <Button
          variant='outline'
          size='sm'
          className='h-8'
          onClick={handleReset}
        >
          {t('Reset')}
        </Button>
        <div className='flex flex-wrap items-center gap-2 sm:ml-auto'>
          <ToggleGroup
            value={[listMode]}
            onValueChange={handleListModeChange}
            variant='outline'
            aria-label={t('Display mode')}
          >
            <ToggleGroupItem value='conversation'>
              {t('Conversation')}
            </ToggleGroupItem>
            <ToggleGroupItem value='raw'>{t('Raw JSON')}</ToggleGroupItem>
          </ToggleGroup>
          {selectedIds.length > 0 && (
            <div className='flex items-center gap-2'>
              <span className='text-muted-foreground text-xs'>
                {t('{{count}} selected', { count: selectedIds.length })}
              </span>
              <Button
                variant='outline'
                size='sm'
                className='h-8'
                disabled={exportMutation.isPending}
                onClick={() => exportMutation.mutate(selectedIds)}
              >
                {exportMutation.isPending ? (
                  <Loader2 className='size-3.5 animate-spin' />
                ) : (
                  <Download className='size-3.5' aria-hidden='true' />
                )}
                {t('Export Selected')}
              </Button>
              <Button
                variant='destructive'
                size='sm'
                className='h-8'
                onClick={() => setDeleteIds(selectedIds)}
              >
                <Trash2 className='size-3.5' aria-hidden='true' />
                {t('Delete Selected')}
              </Button>
            </div>
          )}
          <Button
            variant='destructive'
            size='sm'
            className='h-8'
            disabled={total === 0 || isLoading}
            onClick={() => setDeleteAllOpen(true)}
          >
            <Trash2 className='size-3.5' aria-hidden='true' />
            {t('Delete All')}
          </Button>
        </div>
      </div>

      <div className='min-h-0 flex-1 overflow-y-auto'>
        <div className={isFetching && !isLoading ? 'opacity-60' : undefined}>
          {listContent}
        </div>
      </div>

      <div className='flex items-center justify-between'>
        <span className='text-muted-foreground text-xs'>
          {t('Total:')} {total}
        </span>
        <div className='flex items-center gap-2'>
          <Button
            variant='outline'
            size='icon'
            className='size-8'
            aria-label={t('Go to previous page')}
            onClick={() => goToPage(Math.max(1, page - 1))}
            disabled={page <= 1}
          >
            <ChevronLeft className='size-4' />
          </Button>
          <span className='text-sm tabular-nums' aria-live='polite'>
            {page} / {totalPages}
          </span>
          <Button
            variant='outline'
            size='icon'
            className='size-8'
            aria-label={t('Go to next page')}
            onClick={() => goToPage(Math.min(totalPages, page + 1))}
            disabled={page >= totalPages}
          >
            <ChevronRight className='size-4' />
          </Button>
        </div>
      </div>

      <ConfirmDialog
        open={deleteIds !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteIds(null)
        }}
        title={
          deleteIds?.length === 1
            ? t('Delete this record')
            : t('Delete Selected')
        }
        desc={
          deleteIds?.length === 1
            ? t(
                'Deleting removes this record and its usage-log entry. This cannot be undone.'
              )
            : t(
                'Deleting removes the selected records and their usage-log entries. This cannot be undone.'
              )
        }
        destructive
        isLoading={deleteMutation.isPending}
        handleConfirm={() => {
          if (deleteIds !== null) deleteMutation.mutate(deleteIds)
        }}
      />

      <ConfirmDialog
        open={deleteAllOpen}
        onOpenChange={(open) => {
          setDeleteAllOpen(open)
          if (open) void refetch()
        }}
        title={t('Delete All')}
        desc={t(
          'Deleting removes all {{count}} record(s) matching the current filters and their usage-log entries. This cannot be undone.',
          { count: total }
        )}
        destructive
        disabled={total === 0}
        isLoading={deleteAllMutation.isPending}
        handleConfirm={() => deleteAllMutation.mutate()}
      />
    </div>
  )
}
