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

/**
 * Parsing for the recorded request/response body viewer. Everything here is
 * display-only: payloads that do not parse stay visible through their raw
 * JSON view, and no value is ever used outside rendering.
 */

export interface BodyMessage {
  role: string
  content: string
}

export interface RequestBodyView {
  isConversation: boolean
  model: string
  messages: BodyMessage[]
  responseText: string
  rawRequest: string
  rawResponse: string
}

/** Pretty-print JSON for display; unparseable payloads stay as they are. */
export function prettyJson(raw: string): string {
  if (raw === '') return ''
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

function parseJson(raw: string): Record<string, unknown> | null {
  if (raw === '') return null
  try {
    const parsed: unknown = JSON.parse(raw)
    if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return null
    }
    return parsed as Record<string, unknown>
  } catch {
    return null
  }
}

/** Render one message content value (string or multimodal block array). */
export function messageContentText(content: unknown): string {
  if (typeof content === 'string') return content
  if (!Array.isArray(content)) {
    if (content == null) return ''
    return JSON.stringify(content)
  }
  const parts: string[] = []
  for (const block of content) {
    if (block === null || typeof block !== 'object') continue
    const item = block as Record<string, unknown>
    if (typeof item.text === 'string' && item.text !== '') {
      parts.push(item.text)
      continue
    }
    if (typeof item.type === 'string') {
      parts.push(`[${item.type}]`)
    }
  }
  return parts.join('\n')
}

function extractMessages(payload: Record<string, unknown>): BodyMessage[] {
  const messages = payload.messages
  if (!Array.isArray(messages)) return []
  const result: BodyMessage[] = []
  for (const message of messages) {
    if (message === null || typeof message !== 'object') continue
    const item = message as Record<string, unknown>
    result.push({
      role: typeof item.role === 'string' ? item.role : 'unknown',
      content: messageContentText(item.content),
    })
  }
  return result
}

function extractResponseText(payload: Record<string, unknown> | null): string {
  if (!payload) return ''
  const choices = payload.choices
  if (Array.isArray(choices) && choices.length > 0) {
    const first = choices[0] as Record<string, unknown> | null
    const message = first?.message as Record<string, unknown> | undefined
    if (typeof message?.content === 'string' && message.content !== '') {
      return message.content
    }
  }
  const content = payload.content
  if (Array.isArray(content) && content.length > 0) {
    const first = content[0] as Record<string, unknown> | null
    if (typeof first?.text === 'string' && first.text !== '') {
      return first.text
    }
  }
  return ''
}

export function parseRequestBodyView(
  requestBody: string,
  responseBody: string
): RequestBodyView {
  const requestPayload = parseJson(requestBody)
  const responsePayload = parseJson(responseBody)
  const messages = requestPayload ? extractMessages(requestPayload) : []
  const model =
    requestPayload && typeof requestPayload.model === 'string'
      ? requestPayload.model
      : ''
  return {
    isConversation: messages.length > 0,
    model,
    messages,
    responseText: extractResponseText(responsePayload),
    rawRequest: prettyJson(requestBody),
    rawResponse: prettyJson(responseBody),
  }
}

/** Download payloads as a JSON file through a temporary object URL. */
export function downloadJsonFile(filename: string, payload: unknown): void {
  const blob = new Blob([JSON.stringify(payload, null, 2)], {
    type: 'application/json',
  })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  anchor.click()
  URL.revokeObjectURL(url)
}
