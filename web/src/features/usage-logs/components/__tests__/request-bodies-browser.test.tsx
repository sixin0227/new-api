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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  deleteAllRequestBodies,
  deleteRequestBodies,
  exportRequestBodies,
  getRequestBodiesList,
  type RequestBodyListItem,
} from '../../api'
import { RequestBodiesBrowser } from '../request-bodies-browser'
import { UsageLogsProvider } from '../usage-logs-provider'

vi.mock('../../api', () => ({
  getRequestBodiesList: vi.fn(),
  exportRequestBodies: vi.fn(),
  deleteRequestBodies: vi.fn(),
  deleteAllRequestBodies: vi.fn(),
  getRequestBody: vi.fn(),
}))

const mockedList = vi.mocked(getRequestBodiesList)
const mockedExport = vi.mocked(exportRequestBodies)
const mockedDelete = vi.mocked(deleteRequestBodies)
const mockedDeleteAll = vi.mocked(deleteAllRequestBodies)

const i18n = createInstance()

const items: RequestBodyListItem[] = [
  {
    id: 2,
    request_id: 'req-newer',
    user_id: 7,
    username: 'alice',
    created_at: 1_700_000_000,
    model: 'gpt-4o',
    has_log_metadata: true,
    channel_id: 12,
    channel_name: 'openai-main',
    token_name: 'prod-token',
    is_stream: true,
    prompt_tokens: 12,
    completion_tokens: 34,
    use_time: 2,
    request_preview: 'hello there',
    response_preview: 'general kenobi',
    request_body: JSON.stringify({
      model: 'gpt-4o',
      messages: [
        { role: 'system', content: 'be nice' },
        { role: 'user', content: 'hello there' },
      ],
    }),
    response_body: JSON.stringify({
      choices: [
        {
          index: 0,
          message: { role: 'assistant', content: 'general kenobi' },
        },
      ],
    }),
    request_skipped: false,
    response_skipped: false,
  },
  {
    id: 1,
    request_id: 'req-older',
    user_id: 9,
    username: '',
    created_at: 1_699_000_000,
    model: 'claude-3',
    has_log_metadata: false,
    channel_id: 0,
    channel_name: '',
    token_name: '',
    is_stream: false,
    prompt_tokens: 0,
    completion_tokens: 0,
    use_time: 0,
    request_preview: 'summarize this',
    response_preview: '',
    request_body: JSON.stringify({
      model: 'claude-3',
      messages: [{ role: 'user', content: 'summarize this' }],
    }),
    response_body: '',
    request_skipped: false,
    response_skipped: true,
  },
]

function mockListResponse(listItems: RequestBodyListItem[] = items) {
  mockedList.mockResolvedValue({
    data: {
      success: true,
      data: {
        items: listItems,
        total: listItems.length,
        page: 1,
        page_size: 20,
      },
    },
  } as Awaited<ReturnType<typeof getRequestBodiesList>>)
}

beforeEach(async () => {
  await i18n.init({
    lng: 'en',
    resources: { en },
    interpolation: { escapeValue: false },
  })
  useAuthStore.setState((state) => ({
    auth: {
      ...state.auth,
      user: { id: 7, username: 'admin', role: ROLE.ADMIN },
      accessToken: 'test-token',
      accessExpiresAt: Math.floor(Date.now() / 1000) + 3600,
    },
  }))
  mockListResponse()
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
  useAuthStore.getState().auth.reset()
})

function renderBrowser() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <UsageLogsProvider>
          <RequestBodiesBrowser />
        </UsageLogsProvider>
      </QueryClientProvider>
    </I18nextProvider>
  )
}

test('rows render the recorded content directly, without an extra click', async () => {
  renderBrowser()

  // Message bubbles from the full request body: the system message only
  // exists in the recorded payload, not in the summary preview.
  expect(await screen.findByText('be nice')).toBeVisible()
  expect(screen.getByText('hello there')).toBeVisible()
  expect(screen.getByText('general kenobi')).toBeVisible()
  expect(screen.getByText('gpt-4o')).toBeVisible()
  expect(screen.getByText('alice')).toBeVisible()
  // A missing username falls back to the user id.
  expect(screen.getByText('#9')).toBeVisible()
  // The second exchange has no response yet: its skipped side is flagged.
  expect(screen.getByText('summarize this')).toBeVisible()
  expect(
    screen.getAllByText('Response body not recorded (exceeds the size limit)')
      .length
  ).toBeGreaterThan(0)
  // No per-row navigation trigger is needed anymore.
  expect(screen.queryByRole('button', { name: 'View' })).toBeNull()
})

test('each row shows the usage-log metadata of its exchange', async () => {
  renderBrowser()
  await screen.findByText('hello there')

  const channelLabels = screen.getAllByTitle('Channel')
  expect(channelLabels[0]).toHaveTextContent('openai-main')
  // A record without a surviving log row shows placeholders, not invented values.
  expect(channelLabels[1]).toHaveTextContent('-')

  const tokenLabels = screen.getAllByTitle('Token')
  expect(tokenLabels[0]).toHaveTextContent('prod-token')
  expect(tokenLabels[1]).toHaveTextContent('-')

  expect(screen.getByText('12 / 34')).toBeVisible()
  expect(screen.getByText('17 t/s')).toBeVisible()
  expect(screen.getByText('2.0s')).toBeVisible()
  expect(screen.getByTitle('Duration')).toHaveTextContent('-')
})

test('the page-wide switch flips every row to the raw JSON view', async () => {
  const user = userEvent.setup()
  renderBrowser()
  await screen.findByText('hello there')

  const rawToggle = screen.getByRole('button', { name: 'Raw JSON' })
  await user.click(rawToggle)

  expect(rawToggle).toHaveAttribute('aria-pressed', 'true')
  // Both rows render the labeled raw sections instead of chat bubbles; the
  // second row keeps its skipped-side note.
  expect(screen.getAllByText('Request Body')).toHaveLength(2)
  expect(
    screen.getAllByText('Response body not recorded (exceeds the size limit)')
      .length
  ).toBeGreaterThan(0)
  expect(screen.queryByText('be nice')).toBeNull()
  expect(screen.queryByText('summarize this')).toBeNull()
  // The pretty request JSON of the first record is on screen.
  expect(screen.getByText(/"model": "gpt-4o"/)).toBeVisible()
})

test('a single-row switch only flips that row', async () => {
  const user = userEvent.setup()
  renderBrowser()
  await screen.findByText('hello there')

  await user.click(screen.getAllByLabelText('Switch to raw JSON')[0])

  // The flipped row shows its raw sections; the other row keeps its bubbles.
  expect(screen.queryByText('be nice')).toBeNull()
  expect(screen.getByText('summarize this')).toBeVisible()
  expect(screen.getAllByText('Request Body')).toHaveLength(1)
  expect(screen.getAllByLabelText('Switch to raw JSON')).toHaveLength(1)
  expect(screen.getAllByLabelText('Switch to conversation')).toHaveLength(1)
})

test('copying a row yields that row pretty request JSON', async () => {
  const user = userEvent.setup()
  renderBrowser()
  await screen.findByText('hello there')

  await user.click(screen.getAllByLabelText('Copy request JSON')[0])

  const clipboardText = await window.navigator.clipboard.readText()
  expect(clipboardText).toBe(
    JSON.stringify(JSON.parse(items[0].request_body), null, 2)
  )
})

test('non-admin users see the administrator-only notice instead of records', async () => {
  useAuthStore.setState((state) => ({
    auth: { ...state.auth, user: { id: 7, username: 'bob', role: ROLE.USER } },
  }))
  renderBrowser()

  expect(await screen.findByText('Administrator access required')).toBeVisible()
  expect(mockedList).not.toHaveBeenCalled()
})

test('searching sends the trimmed contains filter and resets the page', async () => {
  const user = userEvent.setup()
  renderBrowser()

  await screen.findByText('hello there')
  await user.type(screen.getByLabelText('Search in bodies'), '  hello  ')
  await user.click(screen.getByRole('button', { name: 'Search' }))

  await waitFor(() => {
    expect(mockedList).toHaveBeenLastCalledWith(
      expect.objectContaining({ contains: 'hello', p: 1 })
    )
  })
})

test('changing the filters clears the selection and per-row modes', async () => {
  const user = userEvent.setup()
  renderBrowser()
  await screen.findByText('hello there')

  await user.click(screen.getAllByLabelText('Select this record')[0])
  await user.click(screen.getAllByLabelText('Switch to raw JSON')[0])
  expect(screen.getByRole('button', { name: 'Export Selected' })).toBeVisible()
  expect(screen.queryByText('be nice')).toBeNull()

  await user.click(screen.getByRole('button', { name: 'Search' }))

  await waitFor(() => {
    expect(screen.queryByRole('button', { name: 'Export Selected' })).toBeNull()
  })
  // The row override is dropped, so the row renders its conversation again.
  expect(await screen.findByText('be nice')).toBeVisible()
})

test('exporting the selection requests only the selected request ids', async () => {
  const user = userEvent.setup()
  const createObjectURL = vi.fn(() => 'blob:bodies')
  Object.defineProperty(URL, 'createObjectURL', {
    configurable: true,
    value: createObjectURL,
  })
  Object.defineProperty(URL, 'revokeObjectURL', {
    configurable: true,
    value: vi.fn(),
  })
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  mockedExport.mockResolvedValue({
    data: { success: true, data: { items: items.slice(0, 1) } },
  } as Awaited<ReturnType<typeof exportRequestBodies>>)

  try {
    renderBrowser()
    await screen.findByText('hello there')

    await user.click(screen.getAllByLabelText('Select this record')[0])
    await user.click(screen.getByRole('button', { name: 'Export Selected' }))

    await waitFor(() => {
      expect(mockedExport).toHaveBeenCalledWith(['req-newer'])
    })
    expect(createObjectURL).toHaveBeenCalled()
  } finally {
    Reflect.deleteProperty(URL, 'createObjectURL')
    Reflect.deleteProperty(URL, 'revokeObjectURL')
  }
})

test('deleting the selection confirms first and then deletes the request ids', async () => {
  const user = userEvent.setup()
  renderBrowser()

  await screen.findByText('hello there')
  await user.click(screen.getAllByLabelText('Select this record')[0])
  await user.click(screen.getByRole('button', { name: 'Delete Selected' }))

  // The confirmation gate must not delete anything on its own.
  expect(await screen.findByRole('alertdialog')).toBeVisible()
  expect(mockedDelete).not.toHaveBeenCalled()

  mockedDelete.mockResolvedValue({
    data: { success: true, data: { deleted: 1 } },
  } as Awaited<ReturnType<typeof deleteRequestBodies>>)
  await user.click(screen.getByRole('button', { name: 'Continue' }))

  await waitFor(() => {
    expect(mockedDelete).toHaveBeenCalledWith(['req-newer'])
  })
})

test('deleting one row confirms first and deletes only that request id', async () => {
  const user = userEvent.setup()
  renderBrowser()
  await screen.findByText('hello there')

  await user.click(screen.getAllByLabelText('Delete this record')[0])

  expect(await screen.findByRole('alertdialog')).toBeVisible()
  expect(mockedDelete).not.toHaveBeenCalled()

  mockedDelete.mockResolvedValue({
    data: { success: true, data: { deleted: 1 } },
  } as Awaited<ReturnType<typeof deleteRequestBodies>>)
  await user.click(screen.getByRole('button', { name: 'Continue' }))

  await waitFor(() => {
    expect(mockedDelete).toHaveBeenCalledWith(['req-newer'])
  })
})

test('deleting all sweeps the current filters after showing the exact count', async () => {
  const user = userEvent.setup()
  renderBrowser()
  await screen.findByText('hello there')

  await user.click(screen.getByRole('button', { name: 'Delete All' }))

  expect(await screen.findByRole('alertdialog')).toHaveTextContent(
    'Deleting removes all 2 record(s) matching the current filters'
  )
  expect(mockedDeleteAll).not.toHaveBeenCalled()

  mockedDeleteAll.mockResolvedValue({
    data: { success: true, data: { deleted: 2 } },
  } as Awaited<ReturnType<typeof deleteAllRequestBodies>>)
  await user.click(screen.getByRole('button', { name: 'Continue' }))

  await waitFor(() => {
    expect(mockedDeleteAll).toHaveBeenCalledWith({
      contains: undefined,
      user_id: undefined,
    })
  })
})
