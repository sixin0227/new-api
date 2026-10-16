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
import { useQuery } from '@tanstack/react-query'
import { Download, MessageSquareText } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { getRequestBody, type RequestBodiesData } from '../../api'
import {
  downloadJsonFile,
  parseRequestBodyView,
} from '../../lib/request-body-view'
import { BodyConversationView, RawBodyView } from '../body-record-view'

type BodyViewMode = 'conversation' | 'raw'

function useRequestBodyRecord(requestId: string, open: boolean) {
  return useQuery({
    queryKey: ['request-body', requestId],
    enabled: open && requestId !== '',
    queryFn: async (): Promise<RequestBodiesData | null> => {
      const res = await getRequestBody(requestId)
      return res.data?.success ? (res.data.data ?? null) : null
    },
    staleTime: 60_000,
  })
}

interface RequestBodyDialogProps {
  requestId: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

/**
 * Admin-only viewer for the recorded request/response bodies of one relay
 * exchange. Bodies are fetched lazily so list and detail payloads never
 * carry them (docs/adr/0003-opt-in-body-recording.md).
 */
export function RequestBodyDialog(props: RequestBodyDialogProps) {
  const { t } = useTranslation()
  const [viewMode, setViewMode] = useState<BodyViewMode>('conversation')
  const { data, isFetching } = useRequestBodyRecord(props.requestId, props.open)
  const view = useMemo(
    () =>
      parseRequestBodyView(data?.request_body ?? '', data?.response_body ?? ''),
    [data]
  )
  const activeView: BodyViewMode = view.isConversation ? viewMode : 'raw'

  const shareLink = `${window.location.origin}/usage-logs/common?requestId=${encodeURIComponent(props.requestId)}&viewBody=1`

  const handleExport = () => {
    if (data == null) return
    downloadJsonFile(`request-bodies-${props.requestId}.json`, {
      request_id: props.requestId,
      request_body: data.request_body,
      response_body: data.response_body,
      request_skipped: data.request_skipped,
      response_skipped: data.response_skipped,
    })
  }

  const hasContent =
    data != null && (data.request_body !== '' || data.response_body !== '')

  let bodyContent = null
  if (isFetching && data == null) {
    bodyContent = <LoadingState size='sm' />
  } else if (data == null || !hasContent) {
    bodyContent = (
      <EmptyState
        icon={MessageSquareText}
        title={t('No recorded bodies for this request')}
        description={t(
          'Bodies are recorded only when body recording is enabled and the request used a token with the per-token switch on.'
        )}
        className='min-h-48'
      />
    )
  } else if (activeView === 'conversation') {
    bodyContent = (
      <BodyConversationView
        requestBody={data.request_body}
        responseBody={data.response_body}
        requestSkipped={data.request_skipped}
        responseSkipped={data.response_skipped}
      />
    )
  } else {
    bodyContent = (
      <RawBodyView
        rawRequest={view.rawRequest}
        rawResponse={view.rawResponse}
        requestSkipped={data.request_skipped}
        responseSkipped={data.response_skipped}
      />
    )
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Request & Response Bodies')}
      description={props.requestId}
      contentClassName='sm:max-w-2xl'
      contentHeight='min(72dvh, 720px)'
    >
      <div className='flex min-w-0 flex-col gap-3'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <Tabs
            value={activeView}
            onValueChange={(value) => {
              if (value === 'conversation' || value === 'raw') {
                setViewMode(value)
              }
            }}
          >
            <TabsList>
              <TabsTrigger value='conversation' disabled={!view.isConversation}>
                {t('Conversation')}
              </TabsTrigger>
              <TabsTrigger value='raw'>{t('Raw JSON')}</TabsTrigger>
            </TabsList>
          </Tabs>
          <div className='flex items-center gap-1'>
            <CopyButton
              value={shareLink}
              tooltip={t('Copy share link')}
              className='size-7'
            />
            <Button
              variant='ghost'
              size='icon'
              className='size-7'
              onClick={handleExport}
              disabled={data == null}
              aria-label={t('Export')}
              title={t('Export')}
            >
              <Download className='size-3.5' />
            </Button>
          </div>
        </div>
        {bodyContent}
      </div>
    </Dialog>
  )
}

/**
 * The inline trigger used in log rows: opens the viewer for one exchange.
 */
export function RequestBodyButton(props: { requestId: string }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)

  return (
    <>
      <button
        type='button'
        className='text-muted-foreground text-xs group-hover:underline'
        onClick={() => setOpen(true)}
        title={t('View request & response bodies')}
      >
        {t('Body')}
      </button>
      <RequestBodyDialog
        requestId={props.requestId}
        open={open}
        onOpenChange={setOpen}
      />
    </>
  )
}
