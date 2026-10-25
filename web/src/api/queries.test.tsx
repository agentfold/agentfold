import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { describe, expect, it } from 'vitest'

import { card, createFakeBackend, type FakeBackend } from '../test/fakeBackend'
import { newQueryClient } from '../test/render'
import { createApiClient, type Card } from './client'
import { ApiContext } from './context'
import { keys, useMoveCard } from './queries'

function setup(backend: FakeBackend, cached?: Card[]) {
  const queryClient = newQueryClient()
  if (cached) {
    queryClient.setQueryData(keys.cards, cached)
  }
  const api = {
    client: createApiClient('http://localhost/api', backend.fetch),
    openEvents: () => ({
      addEventListener: () => undefined,
      close: () => undefined,
    }),
  }
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <ApiContext value={api}>{children}</ApiContext>
    </QueryClientProvider>
  )
  const { result } = renderHook(() => useMoveCard(), { wrapper })
  return { result, queryClient }
}

describe('useMoveCard', () => {
  it('shows the move before the server answers', async () => {
    const cards = [card('a', 'L1', 't', 0), card('b', 'L1', 't', 1)]
    const backend = createFakeBackend({ cards: structuredClone(cards) })
    const { result, queryClient } = setup(backend, cards)

    act(() => {
      result.current.mutate({ id: cards[0].id, laneId: 'L2', position: 0 })
    })
    await waitFor(() => {
      expect(queryClient.getQueryData<Card[]>(keys.cards)?.[0].lane_id).toBe(
        'L2',
      )
    })
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })
  })

  it('restores the cards when the move fails', async () => {
    const cards = [card('a', 'L1', 't', 0)]
    const backend = createFakeBackend({ cards: structuredClone(cards) })
    backend.failNext('POST /cards', 500)
    const { result, queryClient } = setup(backend, cards)

    act(() => {
      result.current.mutate({ id: cards[0].id, laneId: 'L2', position: 0 })
    })
    await waitFor(() => {
      expect(result.current.isError).toBe(true)
    })
    expect(queryClient.getQueryData(keys.cards)).toEqual(cards)
  })

  it('works without cached cards', async () => {
    const cards = [card('a', 'L1', 't', 0)]
    const { result } = setup(createFakeBackend({ cards }))
    act(() => {
      result.current.mutate({ id: cards[0].id, laneId: 'L2', position: 0 })
    })
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })
  })
})
