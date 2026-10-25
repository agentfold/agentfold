import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render } from '@testing-library/react'
import type { ReactElement } from 'react'

import { createApiClient } from '../api/client'
import { ApiContext, type EventSourceLike } from '../api/context'
import type { FakeBackend } from './fakeBackend'

/** A controllable stand-in for EventSource. */
export class FakeEventSource implements EventSourceLike {
  listeners = new Map<string, (() => void)[]>()
  closed = false

  addEventListener(type: string, listener: () => void) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener])
  }

  close() {
    this.closed = true
  }

  emit(type: string) {
    for (const listener of this.listeners.get(type) ?? []) {
      listener()
    }
  }
}

export function newQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
}

/** Renders `ui` with the API pointing to `backend`. */
export function renderWithApi(ui: ReactElement, backend: FakeBackend) {
  const events = new FakeEventSource()
  const queryClient = newQueryClient()
  const api = {
    client: createApiClient('http://localhost/api', backend.fetch),
    openEvents: () => events,
  }
  const result = render(
    <QueryClientProvider client={queryClient}>
      <ApiContext value={api}>{ui}</ApiContext>
    </QueryClientProvider>,
  )
  return { ...result, events, queryClient }
}
