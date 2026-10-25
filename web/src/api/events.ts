import { useQueryClient, type QueryKey } from '@tanstack/react-query'
import { useEffect } from 'react'

import { useApi } from './context'
import { keys } from './queries'

/** Which cached queries each entity of a board event affects. */
const affected: Record<string, QueryKey> = {
  lane: keys.lanes,
  project: keys.projects,
  card_type: keys.cardTypes,
  card: keys.cards,
  flow: keys.flows,
  agent_profile: keys.agentProfiles,
  mcp_server: keys.mcpServers,
}

const changes = ['created', 'updated', 'moved', 'deleted', 'published']

/**
 * Keeps the cached board in sync with the server: every board event refreshes
 * the queries of its entity, and (re)connecting refreshes everything, since
 * events may have been missed meanwhile.
 */
export function useBoardEvents() {
  const { openEvents } = useApi()
  const queryClient = useQueryClient()

  useEffect(() => {
    const source = openEvents()
    source.addEventListener('open', () => {
      void queryClient.invalidateQueries()
    })
    for (const [entity, queryKey] of Object.entries(affected)) {
      for (const change of changes) {
        source.addEventListener(`${entity}.${change}`, () => {
          void queryClient.invalidateQueries({ queryKey })
        })
      }
    }
    return () => {
      source.close()
    }
  }, [openEvents, queryClient])
}
