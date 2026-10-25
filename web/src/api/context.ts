import { createContext, useContext } from 'react'

import type { ApiClient } from './client'

/** The subset of EventSource used by the app, so tests can provide a fake. */
export interface EventSourceLike {
  addEventListener(type: string, listener: () => void): void
  close(): void
}

export interface Api {
  client: ApiClient
  /** Opens the board event stream (GET /api/events). */
  openEvents: () => EventSourceLike
}

export const ApiContext = createContext<Api | null>(null)

export function useApi(): Api {
  const api = useContext(ApiContext)
  if (!api) {
    throw new Error('useApi must be used inside ApiContext')
  }
  return api
}
