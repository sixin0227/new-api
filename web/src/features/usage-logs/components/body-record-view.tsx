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
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

import {
  parseRequestBodyView,
  type BodyMessage,
} from '../lib/request-body-view'

const roleClassNames: Record<string, string> = {
  system: 'bg-muted/60',
  user: 'bg-blue-500/10 dark:bg-blue-400/10',
  assistant: 'bg-emerald-500/10 dark:bg-emerald-400/10',
  tool: 'bg-amber-500/10 dark:bg-amber-400/10',
}

function MessageBubble(props: { message: BodyMessage }) {
  const { t } = useTranslation()
  return (
    <div
      className={cn(
        'rounded-md border px-2.5 py-1.5',
        roleClassNames[props.message.role] ?? 'bg-muted/60'
      )}
    >
      <div className='text-muted-foreground mb-0.5 text-[11px] font-medium'>
        {props.message.role}
      </div>
      <div className='text-xs break-words whitespace-pre-wrap'>
        {props.message.content || t('(no text content)')}
      </div>
    </div>
  )
}

function ResponsePreview(props: {
  responseText: string
  rawResponse: string
  responseSkipped: boolean
}) {
  const { t } = useTranslation()
  if (props.responseText) {
    return (
      <div className='rounded-md border bg-emerald-500/5 px-2.5 py-1.5 text-xs break-words whitespace-pre-wrap'>
        {props.responseText}
      </div>
    )
  }
  if (props.responseSkipped) {
    return (
      <div className='text-muted-foreground text-xs'>
        {t('Response body not recorded (exceeds the size limit)')}
      </div>
    )
  }
  if (props.rawResponse) {
    return (
      <pre className='bg-muted/40 text-foreground max-h-64 overflow-auto rounded-md border p-2 font-mono text-xs break-all whitespace-pre-wrap'>
        {props.rawResponse}
      </pre>
    )
  }
  return (
    <div className='text-muted-foreground text-xs'>
      {t('No recorded bodies for this request')}
    </div>
  )
}

/**
 * Renders one recorded exchange the way a reader wants it: chat payloads as
 * role-colored bubbles plus the response text, anything else as pretty JSON.
 * Display-only — the raw bodies stay untouched in the record.
 */
export function BodyConversationView(props: {
  requestBody: string
  responseBody: string
  requestSkipped: boolean
  responseSkipped: boolean
  /** Hide the model chip when the surrounding UI already shows it. */
  showModel?: boolean
}) {
  const { t } = useTranslation()
  const showModel = props.showModel ?? true
  const view = useMemo(
    () => parseRequestBodyView(props.requestBody, props.responseBody),
    [props.requestBody, props.responseBody]
  )
  const messages = useMemo(
    () =>
      view.messages.map((message, position) => ({
        key: `${position}-${message.role}`,
        message,
      })),
    [view.messages]
  )

  let requestContent = null
  if (view.isConversation) {
    requestContent = (
      <>
        <div className='flex flex-wrap items-center gap-1.5'>
          {showModel && view.model && (
            <Badge variant='secondary' className='h-5 px-1.5 text-[10px]'>
              {view.model}
            </Badge>
          )}
          <span className='text-muted-foreground text-[11px]'>
            {t('{{count}} message(s)', { count: view.messages.length })}
          </span>
        </div>
        <div className='flex flex-col gap-1.5'>
          {messages.map((entry) => (
            <MessageBubble key={entry.key} message={entry.message} />
          ))}
        </div>
      </>
    )
  } else if (props.requestSkipped) {
    requestContent = (
      <div className='text-muted-foreground text-xs'>
        {t('Request body not recorded (exceeds the size limit)')}
      </div>
    )
  } else if (view.rawRequest) {
    requestContent = (
      <pre className='bg-muted/40 text-foreground max-h-64 overflow-auto rounded-md border p-2 font-mono text-xs break-all whitespace-pre-wrap'>
        {view.rawRequest}
      </pre>
    )
  } else {
    requestContent = (
      <div className='text-muted-foreground text-xs'>
        {t('No recorded bodies for this request')}
      </div>
    )
  }

  return (
    <div className='flex flex-col gap-3'>
      {requestContent}
      <section className='flex min-w-0 flex-col gap-1'>
        <span className='text-muted-foreground text-xs font-medium'>
          {t('Response Body')}
        </span>
        <ResponsePreview
          responseText={view.responseText}
          rawResponse={view.rawResponse}
          responseSkipped={props.responseSkipped}
        />
      </section>
    </div>
  )
}

/** One labeled raw payload with a copy button, used by the raw JSON view. */
export function RawBodySection(props: { label: string; body: string }) {
  const { t } = useTranslation()
  return (
    <section className='flex min-w-0 flex-col gap-1'>
      <div className='flex items-center justify-between gap-2'>
        <span className='text-muted-foreground text-xs font-medium'>
          {props.label}
        </span>
        <CopyButton
          value={props.body}
          size='icon'
          className='size-6'
          tooltip={t('Copy to clipboard')}
        />
      </div>
      <pre className='bg-muted/40 text-foreground max-h-64 min-h-24 overflow-auto rounded-md border p-2 font-mono text-xs break-all whitespace-pre-wrap'>
        {props.body}
      </pre>
    </section>
  )
}

/**
 * The raw JSON pair of one recorded exchange — request body above response
 * body — shared by the row's raw mode and the viewer dialog. Each section
 * keeps its own copy button, so copying yields that side's JSON alone.
 */
export function RawBodyView(props: {
  rawRequest: string
  rawResponse: string
  requestSkipped: boolean
  responseSkipped: boolean
}) {
  const { t } = useTranslation()
  return (
    <div className='flex flex-col gap-3'>
      {props.requestSkipped ? (
        <div className='text-muted-foreground text-xs'>
          {t('Request body not recorded (exceeds the size limit)')}
        </div>
      ) : (
        <RawBodySection label={t('Request Body')} body={props.rawRequest} />
      )}
      {props.responseSkipped ? (
        <div className='text-muted-foreground text-xs'>
          {t('Response body not recorded (exceeds the size limit)')}
        </div>
      ) : (
        <RawBodySection label={t('Response Body')} body={props.rawResponse} />
      )}
    </div>
  )
}
