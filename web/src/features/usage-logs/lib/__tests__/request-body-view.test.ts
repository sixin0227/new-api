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
import { describe, expect, test } from 'vitest'

import {
  parseRequestBodyView,
  prettyJson,
  messageContentText,
} from '../request-body-view'

describe('parseRequestBodyView', () => {
  test('extracts model and messages from an OpenAI chat request', () => {
    const view = parseRequestBodyView(
      JSON.stringify({
        model: 'gpt-4o',
        messages: [
          { role: 'system', content: 'be nice' },
          { role: 'user', content: 'hello' },
        ],
      }),
      ''
    )
    expect(view.isConversation).toBe(true)
    expect(view.model).toBe('gpt-4o')
    expect(view.messages).toEqual([
      { role: 'system', content: 'be nice' },
      { role: 'user', content: 'hello' },
    ])
  })

  test('extracts the assistant text from an OpenAI completion response', () => {
    const view = parseRequestBodyView(
      '{"messages":[{"role":"user","content":"hi"}]}',
      JSON.stringify({
        object: 'chat.completion',
        choices: [{ index: 0, message: { role: 'assistant', content: 'hey' } }],
      })
    )
    expect(view.responseText).toBe('hey')
  })

  test('extracts the first text block from a Claude messages response', () => {
    const view = parseRequestBodyView(
      '{"messages":[{"role":"user","content":"hi"}]}',
      JSON.stringify({
        type: 'message',
        content: [{ type: 'text', text: 'claude says hi' }],
      })
    )
    expect(view.responseText).toBe('claude says hi')
  })

  test('marks non-text blocks and keeps text blocks of multimodal content', () => {
    const view = parseRequestBodyView(
      JSON.stringify({
        messages: [
          {
            role: 'user',
            content: [
              { type: 'image_url', image_url: { url: 'data:' } },
              { type: 'text', text: 'describe this' },
            ],
          },
        ],
      }),
      ''
    )
    expect(view.messages).toEqual([
      { role: 'user', content: '[image_url]\ndescribe this' },
    ])
  })

  test('keeps unparseable payloads visible through the raw view only', () => {
    const view = parseRequestBodyView('not-json', 'also-not-json')
    expect(view.isConversation).toBe(false)
    expect(view.messages).toEqual([])
    expect(view.rawRequest).toBe('not-json')
    expect(view.rawResponse).toBe('also-not-json')
  })

  test('an error response without assistant text yields no response text', () => {
    const view = parseRequestBodyView(
      '{"messages":[{"role":"user","content":"hi"}]}',
      '{"error":{"message":"upstream overloaded"}}'
    )
    expect(view.responseText).toBe('')
  })
})

describe('prettyJson', () => {
  test('pretty-prints parseable JSON', () => {
    expect(prettyJson('{"a":1}')).toBe('{\n  "a": 1\n}')
  })

  test('returns the payload unchanged when it does not parse', () => {
    expect(prettyJson('data: something')).toBe('data: something')
    expect(prettyJson('')).toBe('')
  })
})

describe('messageContentText', () => {
  test('renders a string content as-is', () => {
    expect(messageContentText('plain')).toBe('plain')
  })

  test('renders null content as empty text', () => {
    expect(messageContentText(null)).toBe('')
  })
})
