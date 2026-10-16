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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { Table } from '@tanstack/react-table'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { batchDeleteLogs } from '../../api'

interface BatchDeleteLogsDialogProps<TData> {
  table: Table<TData>
}

/**
 * Admin bulk deletion of whole usage-log entries: the log row and its
 * recorded bodies are removed together (docs/adr/0001-request-bodies-separate-table.md).
 */
export function BatchDeleteLogsDialog<TData>({
  table,
}: BatchDeleteLogsDialogProps<TData>) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)

  const selectedRows = table.getFilteredSelectedRowModel().rows
  const deleteLogs = useMutation({
    mutationFn: async (ids: number[]) => {
      const res = await batchDeleteLogs(ids)
      return res.data
    },
    onSuccess: (result) => {
      if (result?.success) {
        toast.success(t('Deleted {{count}} log(s)', { count: result.data?.deleted ?? 0 }))
        table.resetRowSelection()
      } else {
        toast.error(result?.message || t('Failed to delete logs'))
      }
      setOpen(false)
      queryClient.invalidateQueries({ queryKey: ['logs'] })
    },
    onError: () => {
      toast.error(t('Failed to delete logs'))
      setOpen(false)
    },
  })

  if (selectedRows.length === 0) return null

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <Button
        variant='destructive'
        size='sm'
        className='h-8'
        onClick={() => setOpen(true)}
      >
        {t('Delete Selected')}
      </Button>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('Delete Selected')}</DialogTitle>
        </DialogHeader>
        <p className='text-sm'>
          {t(
            'Deleting removes the log entries and any recorded request/response bodies. This cannot be undone.'
          )}
        </p>
        <DialogFooter>
          <Button variant='outline' onClick={() => setOpen(false)}>
            {t('Cancel')}
          </Button>
          <Button
            variant='destructive'
            disabled={deleteLogs.isPending}
            onClick={() => {
              const ids = selectedRows.map(
                (row) => (row.original as { id: number }).id
              )
              if (ids.length > 0) deleteLogs.mutate(ids)
            }}
          >
            {t('Delete')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
