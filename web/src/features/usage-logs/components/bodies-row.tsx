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
import { Braces, Download, MessagesSquare, Trash2 } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { formatTimestampToDate } from '@/lib/format'

import type { RequestBodyListItem } from '../api'
import {
  downloadJsonFile,
  parseRequestBodyView,
} from '../lib/request-body-view'
import { BodyConversationView, RawBodyView } from './body-record-view'
import { StreamTpsCell, TimingMetricsCell } from './timing-metrics-cell'

export type BodyViewMode = 'conversation' | 'raw'

export interface BodiesRowProps {
  item: RequestBodyListItem
  /** Requested display mode; non-chat payloads always render as raw JSON. */
  mode: BodyViewMode
  selected: boolean
  onToggleSelected: (requestId: string, selected: boolean) => void
  onToggleMode: (requestId: string) => void
  onDelete: (item: RequestBodyListItem) => void
}

/**
 * One recorded exchange: its metadata line (time, model, channel, token,
 * user, stream, tokens, duration), then the recorded content itself, rendered
 * as chat bubbles or as raw JSON depending on the requested mode.
 */
export function BodiesRow(props: BodiesRowProps) {
  const { t } = useTranslation()
  const item = props.item
  const actor = item.username || `#${item.user_id}`
  const view = useMemo(
    () => parseRequestBodyView(item.request_body, item.response_body),
    [item.request_body, item.response_body]
  )
  const effectiveMode: BodyViewMode = view.isConversation ? props.mode : 'raw'
  const switchLabel =
    effectiveMode === 'conversation'
      ? t('Switch to raw JSON')
      : t('Switch to conversation')
  const hasTokens =
    item.has_log_metadata &&
    (item.prompt_tokens > 0 || item.completion_tokens > 0)
  const tokensPerSecond =
    item.has_log_metadata && item.use_time > 0 && item.completion_tokens > 0
      ? item.completion_tokens / item.use_time
      : null

  let channelLabel = '-'
  if (item.has_log_metadata) {
    channelLabel =
      item.channel_name || (item.channel_id ? `#${item.channel_id}` : '-')
  }
  const tokenLabel =
    item.has_log_metadata && item.token_name ? item.token_name : '-'

  const handleExport = () => {
    downloadJsonFile(`request-bodies-${item.request_id}.json`, {
      request_id: item.request_id,
      request_body: item.request_body,
      response_body: item.response_body,
      request_skipped: item.request_skipped,
      response_skipped: item.response_skipped,
    })
  }

  return (
    <div className='flex min-w-0 items-start gap-2 px-3 py-2.5'>
      <Checkbox
        className='mt-0.5'
        checked={props.selected}
        onCheckedChange={(value) =>
          props.onToggleSelected(item.request_id, Boolean(value))
        }
        aria-label={t('Select this record')}
      />
      <div className='min-w-0 flex-1'>
        <div className='text-muted-foreground flex flex-wrap items-center gap-x-2 gap-y-1 text-xs'>
          <span>{formatTimestampToDate(item.created_at)}</span>
          {item.model && (
            <Badge variant='secondary' className='h-4 px-1.5 text-[10px]'>
              {item.model}
            </Badge>
          )}
          <span className='max-w-40 truncate' title={t('Channel')}>
            {channelLabel}
          </span>
          <span className='max-w-32 truncate' title={t('Token')}>
            {tokenLabel}
          </span>
          <span>{actor}</span>
          {item.has_log_metadata ? (
            <StreamTpsCell
              isStream={item.is_stream}
              tokensPerSecond={tokensPerSecond}
              compact
            />
          ) : (
            <span title={t('Stream')}>-</span>
          )}
          {hasTokens ? (
            <span className='font-mono tabular-nums' title={t('Tokens')}>
              {item.prompt_tokens.toLocaleString()} /{' '}
              {item.completion_tokens.toLocaleString()}
            </span>
          ) : (
            <span title={t('Tokens')}>-</span>
          )}
          {item.has_log_metadata ? (
            <TimingMetricsCell
              useTimeSec={item.use_time}
              completionTokens={item.completion_tokens}
              isStream={false}
              indicator='dot'
              compact
            />
          ) : (
            <span title={t('Duration')}>-</span>
          )}
          {item.request_skipped && (
            <Badge variant='outline' className='h-4 px-1.5 text-[10px]'>
              {t('Request body not recorded (exceeds the size limit)')}
            </Badge>
          )}
          {item.response_skipped && (
            <Badge variant='outline' className='h-4 px-1.5 text-[10px]'>
              {t('Response body not recorded (exceeds the size limit)')}
            </Badge>
          )}
          <span className='ml-auto flex items-center gap-0.5'>
            <Button
              variant='ghost'
              size='icon'
              className='size-6'
              disabled={!view.isConversation}
              onClick={() => props.onToggleMode(item.request_id)}
              aria-label={view.isConversation ? switchLabel : undefined}
              title={
                view.isConversation
                  ? switchLabel
                  : t('Only chat payloads have a conversation view')
              }
            >
              {effectiveMode === 'conversation' ? (
                <Braces className='size-3' aria-hidden='true' />
              ) : (
                <MessagesSquare className='size-3' aria-hidden='true' />
              )}
            </Button>
            <CopyButton
              value={view.rawRequest}
              tooltip={t('Copy request JSON')}
              className='size-6'
            />
            <CopyButton
              value={view.rawResponse}
              tooltip={t('Copy response JSON')}
              className='size-6'
            />
            <Button
              variant='ghost'
              size='icon'
              className='size-6'
              onClick={handleExport}
              aria-label={t('Export')}
              title={t('Export')}
            >
              <Download className='size-3' />
            </Button>
            <Button
              variant='ghost'
              size='icon'
              className='text-destructive hover:text-destructive size-6'
              onClick={() => props.onDelete(item)}
              aria-label={t('Delete this record')}
              title={t('Delete this record')}
            >
              <Trash2 className='size-3' />
            </Button>
          </span>
        </div>
        <div className='bg-card/50 mt-2 max-h-96 overflow-y-auto rounded-md border p-2'>
          {effectiveMode === 'conversation' ? (
            <BodyConversationView
              requestBody={item.request_body}
              responseBody={item.response_body}
              requestSkipped={item.request_skipped}
              responseSkipped={item.response_skipped}
              showModel={false}
            />
          ) : (
            <RawBodyView
              rawRequest={view.rawRequest}
              rawResponse={view.rawResponse}
              requestSkipped={item.request_skipped}
              responseSkipped={item.response_skipped}
            />
          )}
        </div>
      </div>
    </div>
  )
}
