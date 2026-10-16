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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'

import { getRequestBody, type RequestBodiesData } from '../../api'
import { RequestBodyDialog } from '../dialogs/request-body-dialog'

vi.mock('../../api', () => ({
  getRequestBody: vi.fn(),
}))

const mockedGetRequestBody = vi.mocked(getRequestBody)

const i18n = createInstance()

beforeEach(async () => {
  await i18n.init({
    lng: 'en',
    resources: { en },
    interpolation: { escapeValue: false },
  })
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

function record(overrides: Partial<RequestBodiesData> = {}): RequestBodiesData {
  return {
    request_body: JSON.stringify({
      model: 'gpt-4o',
      messages: [
        { role: 'system', content: 'be nice' },
        { role: 'user', content: 'hello there' },
      ],
    }),
    response_body: JSON.stringify({
      object: 'chat.completion',
      choices: [
        { index: 0, message: { role: 'assistant', content: 'general kenobi' } },
      ],
    }),
    request_skipped: false,
    response_skipped: false,
    ...overrides,
  }
}

function renderDialog() {
  mockedGetRequestBody.mockResolvedValue({
    data: { success: true, data: record() },
  } as Awaited<ReturnType<typeof getRequestBody>>)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <RequestBodyDialog requestId='req-1' open onOpenChange={() => {}} />
      </QueryClientProvider>
    </I18nextProvider>
  )
}

test('renders the conversation view with message bubbles and the response text', async () => {
  renderDialog()

  expect(await screen.findByText('hello there')).toBeVisible()
  expect(screen.getByText('be nice')).toBeVisible()
  expect(screen.getByText('general kenobi')).toBeVisible()
  expect(screen.getByText('gpt-4o')).toBeVisible()
  expect(mockedGetRequestBody).toHaveBeenCalledWith('req-1')
})

test('switching to Raw JSON shows the pretty-printed bodies', async () => {
  const user = userEvent.setup()
  renderDialog()

  await screen.findByText('hello there')
  await user.click(screen.getByRole('tab', { name: 'Raw JSON' }))

  const rawTexts = screen.getAllByText(/"model": "gpt-4o"/)
  expect(rawTexts.length).toBeGreaterThan(0)
  expect(screen.getByText(/"content": "general kenobi"/)).toBeVisible()
})

test('a skipped response states the size-limit reason instead of empty content', async () => {
  mockedGetRequestBody.mockResolvedValue({
    data: {
      success: true,
      data: record({ response_body: '', response_skipped: true }),
    },
  } as Awaited<ReturnType<typeof getRequestBody>>)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <RequestBodyDialog requestId='req-2' open onOpenChange={() => {}} />
      </QueryClientProvider>
    </I18nextProvider>
  )

  expect(
    await screen.findByText(
      'Response body not recorded (exceeds the size limit)'
    )
  ).toBeVisible()
  expect(screen.getByRole('tab', { name: 'Raw JSON' })).toBeEnabled()
})

test('no recorded bodies shows the enablement hint', async () => {
  mockedGetRequestBody.mockResolvedValue({
    data: {
      success: true,
      data: record({ request_body: '', response_body: '' }),
    },
  } as Awaited<ReturnType<typeof getRequestBody>>)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <RequestBodyDialog requestId='req-3' open onOpenChange={() => {}} />
      </QueryClientProvider>
    </I18nextProvider>
  )

  expect(
    await screen.findByText('No recorded bodies for this request')
  ).toBeVisible()
  // The conversation tab is not offered for an empty record.
  expect(screen.getByRole('tab', { name: 'Conversation' })).toHaveAttribute(
    'aria-disabled',
    'true'
  )
})
