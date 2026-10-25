import { describe, expect, it, vi } from 'vitest'

import { createFakeBackend } from '../test/fakeBackend'
import { renderWithApi } from '../test/render'
import { useBoardEvents } from './events'

function Listener() {
  useBoardEvents()
  return null
}

describe('useBoardEvents', () => {
  it('refreshes the queries of each changed entity', () => {
    const { events, queryClient } = renderWithApi(
      <Listener />,
      createFakeBackend(),
    )
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')

    events.emit('card.moved')
    expect(invalidate).toHaveBeenLastCalledWith({ queryKey: ['cards'] })
    events.emit('lane.deleted')
    expect(invalidate).toHaveBeenLastCalledWith({ queryKey: ['lanes'] })
    events.emit('project.updated')
    expect(invalidate).toHaveBeenLastCalledWith({ queryKey: ['projects'] })
    events.emit('card_type.created')
    expect(invalidate).toHaveBeenLastCalledWith({ queryKey: ['cardTypes'] })
    events.emit('flow.published')
    expect(invalidate).toHaveBeenLastCalledWith({ queryKey: ['flows'] })
    events.emit('agent_profile.updated')
    expect(invalidate).toHaveBeenLastCalledWith({ queryKey: ['agentProfiles'] })
  })

  it('refreshes everything when the stream (re)connects', () => {
    const { events, queryClient } = renderWithApi(
      <Listener />,
      createFakeBackend(),
    )
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
    events.emit('open')
    expect(invalidate).toHaveBeenCalledWith()
  })

  it('closes the stream on unmount', () => {
    const { events, unmount } = renderWithApi(<Listener />, createFakeBackend())
    unmount()
    expect(events.closed).toBe(true)
  })
})
