import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryKey,
} from '@tanstack/react-query'

import {
  unwrap,
  type Card,
  type CardInput,
  type CardTypeInput,
  type FlowGraph,
  type FlowInput,
  type McpServerInput,
  type LaneInput,
  type NewCard,
  type ProjectInput,
} from './client'
import { useApi } from './context'
import { placeCard } from './placement'

export const keys = {
  lanes: ['lanes'],
  projects: ['projects'],
  cardTypes: ['cardTypes'],
  cards: ['cards'],
  flows: ['flows'],
  flow: (id: string) => ['flows', id],
  agentProfiles: ['agentProfiles'],
  mcpServers: ['mcpServers'],
  latestRuns: ['flowRuns', 'latest'],
  flowRuns: ['flowRuns'],
  cardEvents: ['cardEvents'],
  cardTimeline: (id: string) => ['cardEvents', id],
} as const

const byId = (id: string) => ({ params: { path: { id } } })

// Queries

export function useLanes() {
  const { client } = useApi()
  return useQuery({
    queryKey: keys.lanes,
    queryFn: () => unwrap(client.GET('/lanes')),
  })
}

export function useProjects() {
  const { client } = useApi()
  return useQuery({
    queryKey: keys.projects,
    queryFn: () => unwrap(client.GET('/projects')),
  })
}

export function useCardTypes() {
  const { client } = useApi()
  return useQuery({
    queryKey: keys.cardTypes,
    queryFn: () => unwrap(client.GET('/card-types')),
  })
}

export function useCards() {
  const { client } = useApi()
  return useQuery({
    queryKey: keys.cards,
    queryFn: () => unwrap(client.GET('/cards')),
  })
}

/** The latest flow run of each card (an active one if there is any). */
export function useLatestRuns() {
  const { client } = useApi()
  return useQuery({
    queryKey: keys.latestRuns,
    queryFn: () => unwrap(client.GET('/flow-runs/latest')),
  })
}

export function useCardEvents(id: string) {
  const { client } = useApi()
  return useQuery({
    queryKey: keys.cardTimeline(id),
    queryFn: () => unwrap(client.GET('/cards/{id}/events', byId(id))),
  })
}

export function useFlows() {
  const { client } = useApi()
  return useQuery({
    queryKey: keys.flows,
    queryFn: () => unwrap(client.GET('/flows')),
  })
}

export function useFlow(id: string) {
  const { client } = useApi()
  return useQuery({
    queryKey: keys.flow(id),
    queryFn: () => unwrap(client.GET('/flows/{id}', byId(id))),
  })
}

export function useAgentProfiles() {
  const { client } = useApi()
  return useQuery({
    queryKey: keys.agentProfiles,
    queryFn: () => unwrap(client.GET('/agent-profiles')),
  })
}

export function useMcpServers() {
  const { client } = useApi()
  return useQuery({
    queryKey: keys.mcpServers,
    queryFn: () => unwrap(client.GET('/mcp-servers')),
  })
}

// Mutations

/** A mutation that refreshes `key` once it settles. */
function useChange<V, R>(key: QueryKey, fn: (vars: V) => Promise<R>) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSettled: () => queryClient.invalidateQueries({ queryKey: key }),
  })
}

export function useCreateLane() {
  const { client } = useApi()
  return useChange(keys.lanes, (body: LaneInput) =>
    unwrap(client.POST('/lanes', { body })),
  )
}

export function useUpdateLane() {
  const { client } = useApi()
  return useChange(keys.lanes, ({ id, ...body }: LaneInput & { id: string }) =>
    unwrap(client.PUT('/lanes/{id}', { ...byId(id), body })),
  )
}

export function useMoveLane() {
  const { client } = useApi()
  return useChange(
    keys.lanes,
    ({ id, position }: { id: string; position: number }) =>
      unwrap(
        client.POST('/lanes/{id}/move', { ...byId(id), body: { position } }),
      ),
  )
}

export function useDeleteLane() {
  const { client } = useApi()
  return useChange(keys.lanes, (id: string) =>
    unwrap(client.DELETE('/lanes/{id}', byId(id))),
  )
}

export function useCreateProject() {
  const { client } = useApi()
  return useChange(keys.projects, (body: ProjectInput) =>
    unwrap(client.POST('/projects', { body })),
  )
}

export function useUpdateProject() {
  const { client } = useApi()
  return useChange(
    keys.projects,
    ({ id, ...body }: ProjectInput & { id: string }) =>
      unwrap(client.PUT('/projects/{id}', { ...byId(id), body })),
  )
}

export function useDeleteProject() {
  const { client } = useApi()
  return useChange(keys.projects, (id: string) =>
    unwrap(client.DELETE('/projects/{id}', byId(id))),
  )
}

export function useCreateCardType() {
  const { client } = useApi()
  return useChange(keys.cardTypes, (body: CardTypeInput) =>
    unwrap(client.POST('/card-types', { body })),
  )
}

export function useUpdateCardType() {
  const { client } = useApi()
  return useChange(
    keys.cardTypes,
    ({ id, ...body }: CardTypeInput & { id: string }) =>
      unwrap(client.PUT('/card-types/{id}', { ...byId(id), body })),
  )
}

export function useDeleteCardType() {
  const { client } = useApi()
  return useChange(keys.cardTypes, (id: string) =>
    unwrap(client.DELETE('/card-types/{id}', byId(id))),
  )
}

export function useCreateCard() {
  const { client } = useApi()
  return useChange(keys.cards, (body: NewCard) =>
    unwrap(client.POST('/cards', { body })),
  )
}

export function useUpdateCard() {
  const { client } = useApi()
  return useChange(keys.cards, ({ id, ...body }: CardInput & { id: string }) =>
    unwrap(client.PUT('/cards/{id}', { ...byId(id), body })),
  )
}

export function useDeleteCard() {
  const { client } = useApi()
  return useChange(keys.cards, (id: string) =>
    unwrap(client.DELETE('/cards/{id}', byId(id))),
  )
}

export function useCreateFlow() {
  const { client } = useApi()
  return useChange(keys.flows, (name: string) =>
    unwrap(client.POST('/flows', { body: { name } })),
  )
}

export function useUpdateFlow() {
  const { client } = useApi()
  return useChange(keys.flows, ({ id, ...body }: FlowInput & { id: string }) =>
    unwrap(client.PUT('/flows/{id}', { ...byId(id), body })),
  )
}

export function useDeleteFlow() {
  const { client } = useApi()
  return useChange(keys.flows, (id: string) =>
    unwrap(client.DELETE('/flows/{id}', byId(id))),
  )
}

export function useSaveFlowDraft(id: string) {
  const { client } = useApi()
  return useChange(keys.flows, (body: FlowGraph) =>
    unwrap(client.PUT('/flows/{id}/draft', { ...byId(id), body })),
  )
}

export function usePublishFlow(id: string) {
  const { client } = useApi()
  return useChange(keys.flows, () =>
    unwrap(client.POST('/flows/{id}/publish', byId(id))),
  )
}

export function useCreateAgentProfile() {
  const { client } = useApi()
  return useChange(keys.agentProfiles, (name: string) =>
    unwrap(client.POST('/agent-profiles', { body: { name } })),
  )
}

export function useDeleteAgentProfile() {
  const { client } = useApi()
  return useChange(keys.agentProfiles, (id: string) =>
    unwrap(client.DELETE('/agent-profiles/{id}', byId(id))),
  )
}

export function useCreateMcpServer() {
  const { client } = useApi()
  return useChange(keys.mcpServers, (body: McpServerInput) =>
    unwrap(client.POST('/mcp-servers', { body })),
  )
}

export function useUpdateMcpServer() {
  const { client } = useApi()
  return useChange(
    keys.mcpServers,
    ({ id, ...body }: McpServerInput & { id: string }) =>
      unwrap(client.PUT('/mcp-servers/{id}', { ...byId(id), body })),
  )
}

export function useDeleteMcpServer() {
  const { client } = useApi()
  return useChange(keys.mcpServers, (id: string) =>
    unwrap(client.DELETE('/mcp-servers/{id}', byId(id))),
  )
}

export interface CardMove {
  id: string
  laneId: string
  position: number
}

/** Moves a card, showing the new place right away and undoing it on failure. */
export function useMoveCard() {
  const { client } = useApi()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, laneId, position }: CardMove) =>
      unwrap(
        client.POST('/cards/{id}/move', {
          ...byId(id),
          body: { lane_id: laneId, position },
        }),
      ),
    onMutate: async (move: CardMove) => {
      await queryClient.cancelQueries({ queryKey: keys.cards })
      const previous = queryClient.getQueryData<Card[]>(keys.cards)
      if (previous) {
        queryClient.setQueryData(
          keys.cards,
          placeCard(previous, move.id, move.laneId, move.position),
        )
      }
      return { previous }
    },
    onError: (_error, _move, context) => {
      queryClient.setQueryData(keys.cards, context?.previous)
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: keys.cards }),
  })
}
