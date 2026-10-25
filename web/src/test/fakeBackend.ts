// An in-memory implementation of the REST API, used as `fetch` in tests.
import type {
  AgentProfile,
  Card,
  CardEvent,
  CardType,
  Flow,
  FlowGraph,
  GraphProblem,
  Lane,
  McpServer,
  Project,
} from '../api/client'
import { laneCards, placeCard } from '../api/placement'

interface State {
  lanes: Lane[]
  projects: Project[]
  cardTypes: CardType[]
  cards: Card[]
  events: CardEvent[]
  flows: Flow[]
  agentProfiles: AgentProfile[]
  mcpServers: McpServer[]
}

type Body = Record<string, unknown>

const str = (v: unknown) => (typeof v === 'string' ? v : '')
type Reply = [number, unknown?]

const stamp = '2026-10-25T12:00:00Z'
let seq = 0
export const newId = () =>
  `00000000-0000-7000-8000-${String(++seq).padStart(12, '0')}`

export function lane(
  name: string,
  position: number,
  extra: Partial<Lane> = {},
): Lane {
  return { id: newId(), name, color: '', position, created_at: stamp, ...extra }
}

export function project(name: string, extra: Partial<Project> = {}): Project {
  return {
    id: newId(),
    name,
    source_kind: 'local',
    local_path: `/src/${name}`,
    created_at: stamp,
    ...extra,
  }
}

export function cardType(
  name: string,
  extra: Partial<CardType> = {},
): CardType {
  return { id: newId(), name, color: '', created_at: stamp, ...extra }
}

export function card(
  title: string,
  laneId: string,
  typeId: string,
  position: number,
  extra: Partial<Card> = {},
): Card {
  return {
    id: newId(),
    title,
    description: '',
    card_type_id: typeId,
    project_id: null,
    parent_card_id: null,
    lane_id: laneId,
    position,
    created_at: stamp,
    updated_at: stamp,
    ...extra,
  }
}

const notFound: Reply = [404, { code: 'not_found', message: 'not found' }]

function invalid(field: string): Reply {
  return [422, { code: 'validation', message: `${field}: is required`, field }]
}

export function agentProfile(
  name: string,
  extra: Partial<AgentProfile> = {},
): AgentProfile {
  return {
    id: newId(),
    kind: 'claude',
    name,
    ready: true,
    credential_kind: 'oauth_token',
    created_at: stamp,
    updated_at: stamp,
    ...extra,
  }
}

export function mcpServer(
  name: string,
  extra: Partial<McpServer> = {},
): McpServer {
  return {
    id: newId(),
    name,
    transport: 'stdio',
    command: 'npx',
    args: ['server'],
    env: {},
    headers: {},
    secret_names: [],
    created_at: stamp,
    updated_at: stamp,
    ...extra,
  }
}

/** A flow whose draft is a lone trigger, as the server creates them. */
export function flow(name: string, extra: Partial<Flow> = {}): Flow {
  return {
    id: newId(),
    name,
    enabled: false,
    latest_version: 0,
    created_at: stamp,
    updated_at: stamp,
    draft: {
      nodes: [
        {
          id: 'trigger',
          type: 'trigger',
          position: { x: 0, y: 0 },
          config: { event: 'card_created' },
        },
      ],
      edges: [],
    },
    problems: [
      { node_id: 'trigger', message: 'o gatilho precisa estar ligado a um nó' },
    ],
    ...extra,
  }
}

/**
 * The fake's stand-in for graph validation: a draft can be published when
 * every node is connected; otherwise each unconnected node is a problem.
 */
function draftProblems(graph: FlowGraph): GraphProblem[] {
  const linked = new Set(graph.edges.flatMap((e) => [e.source, e.target]))
  return graph.nodes
    .filter((n) => !linked.has(n.id))
    .map((n) => ({ node_id: n.id, message: 'nó solto' }))
}

export interface FakeBackend {
  state: State
  fetch: typeof fetch
  /** Requests received, as "METHOD /path". */
  requests: string[]
  /** Makes the next request whose "METHOD /path" starts with `prefix` fail. */
  failNext(prefix: string, status: number, body?: unknown): void
}

export function createFakeBackend(initial: Partial<State> = {}): FakeBackend {
  const state: State = {
    lanes: [],
    projects: [],
    cardTypes: [],
    cards: [],
    events: [],
    flows: [],
    agentProfiles: [],
    mcpServers: [],
    ...initial,
  }
  const requests: string[] = []
  const failures: { prefix: string; reply: Reply }[] = []

  function route(method: string, path: string, body: Body): Reply {
    const [, collection, id, sub] = path.split('/') as [
      string,
      string,
      string?,
      string?,
    ]
    const key = `${method} /${collection}${id ? '/:id' : ''}${sub ? `/${sub}` : ''}`
    switch (key) {
      case 'GET /lanes':
        return [200, [...state.lanes].sort((a, b) => a.position - b.position)]
      case 'POST /lanes': {
        if (!body.name) return invalid('name')
        const l = lane(str(body.name), state.lanes.length, {
          color: str(body.color),
        })
        state.lanes.push(l)
        return [201, l]
      }
      case 'PUT /lanes/:id':
        return update(state.lanes, id, body)
      case 'POST /lanes/:id/move': {
        const l = state.lanes.find((x) => x.id === id)
        if (!l) return notFound
        const others = state.lanes
          .filter((x) => x.id !== id)
          .sort((a, b) => a.position - b.position)
        others.splice(Math.min(Number(body.position), others.length), 0, l)
        others.forEach((x, i) => (x.position = i))
        return [200, l]
      }
      case 'DELETE /lanes/:id':
        if (state.cards.some((c) => c.lane_id === id)) {
          return [409, { code: 'in_use', message: 'in use' }]
        }
        return remove(state.lanes, id)
      case 'GET /projects':
        return [200, state.projects]
      case 'POST /projects': {
        if (!body.name) return invalid('name')
        const p = project(str(body.name), { local_path: str(body.local_path) })
        state.projects.push(p)
        return [201, p]
      }
      case 'PUT /projects/:id':
        return update(state.projects, id, body)
      case 'DELETE /projects/:id':
        return remove(state.projects, id)
      case 'GET /card-types':
        return [200, state.cardTypes]
      case 'POST /card-types': {
        if (!body.name) return invalid('name')
        const t = cardType(str(body.name), { color: str(body.color) })
        state.cardTypes.push(t)
        return [201, t]
      }
      case 'PUT /card-types/:id':
        return update(state.cardTypes, id, body)
      case 'DELETE /card-types/:id':
        return remove(state.cardTypes, id)
      case 'GET /cards':
        return [200, state.cards]
      case 'POST /cards': {
        if (!body.title) return invalid('title')
        const laneId = str(body.lane_id)
        const c = card(
          str(body.title),
          laneId,
          str(body.card_type_id),
          laneCards(state.cards, laneId).length,
          {
            description: str(body.description),
            project_id: (body.project_id as string | null | undefined) ?? null,
            parent_card_id:
              (body.parent_card_id as string | null | undefined) ?? null,
          },
        )
        state.cards.push(c)
        return [201, c]
      }
      case 'GET /cards/:id':
        return find(state.cards, id)
      case 'PUT /cards/:id':
        return update(state.cards, id, { project_id: null, ...body })
      case 'POST /cards/:id/move': {
        if (!state.cards.some((c) => c.id === id)) return notFound
        state.cards = placeCard(
          state.cards,
          str(id),
          str(body.lane_id),
          Number(body.position),
        )
        return find(state.cards, id)
      }
      case 'DELETE /cards/:id':
        return remove(state.cards, id)
      case 'GET /cards/:id/children':
        return [200, state.cards.filter((c) => c.parent_card_id === id)]
      case 'GET /cards/:id/events':
        return [200, state.events.filter((e) => e.card_id === id)]
      case 'GET /agent-profiles':
        return [200, state.agentProfiles]
      case 'POST /agent-profiles': {
        if (!body.name) return invalid('name')
        const p = agentProfile(str(body.name), {
          ready: false,
          credential_kind: undefined,
        })
        state.agentProfiles.push(p)
        return [201, p]
      }
      case 'DELETE /agent-profiles/:id':
        return remove(state.agentProfiles, id)
      case 'GET /mcp-servers':
        return [200, state.mcpServers]
      case 'POST /mcp-servers':
      case 'PUT /mcp-servers/:id': {
        if (!body.name) return invalid('name')
        const { secrets, ...config } = body as Partial<McpServer> & {
          secrets?: Record<string, string>
        }
        const names = secrets ? Object.keys(secrets) : undefined
        if (method === 'POST') {
          const m = mcpServer(str(body.name), {
            ...config,
            secret_names: names ?? [],
          })
          state.mcpServers.push(m)
          return [201, m]
        }
        const current = state.mcpServers.find((x) => x.id === id)
        if (!current) return notFound
        Object.assign(current, config, names ? { secret_names: names } : {})
        return [200, current]
      }
      case 'DELETE /mcp-servers/:id':
        return remove(state.mcpServers, id)
      case 'GET /flows':
        return [
          200,
          state.flows.map((f) => ({
            id: f.id,
            name: f.name,
            enabled: f.enabled,
            latest_version: f.latest_version,
            created_at: f.created_at,
            updated_at: f.updated_at,
          })),
        ]
      case 'POST /flows': {
        if (!body.name) return invalid('name')
        const f = flow(str(body.name))
        state.flows.push(f)
        return [201, f]
      }
      case 'GET /flows/:id':
        return find(state.flows, id)
      case 'PUT /flows/:id':
        return update(state.flows, id, body)
      case 'DELETE /flows/:id':
        return remove(state.flows, id)
      case 'PUT /flows/:id/draft': {
        const f = state.flows.find((x) => x.id === id)
        if (!f) return notFound
        f.draft = body as unknown as FlowGraph
        f.problems = draftProblems(f.draft)
        return [200, f]
      }
      case 'POST /flows/:id/publish': {
        const f = state.flows.find((x) => x.id === id)
        if (!f) return notFound
        if (f.problems.length > 0) {
          return [
            422,
            {
              code: 'invalid_graph',
              message: 'the draft has problems',
              problems: f.problems,
            },
          ]
        }
        f.latest_version++
        return [
          201,
          {
            id: newId(),
            flow_id: f.id,
            version: f.latest_version,
            graph: f.draft,
            published_at: stamp,
          },
        ]
      }
    }
    throw new Error(`fake backend: no route for ${key}`)
  }

  function find(items: { id: string }[], id?: string): Reply {
    const item = items.find((x) => x.id === id)
    return item ? [200, item] : notFound
  }

  function update(
    items: { id: string }[],
    id: string | undefined,
    body: Body,
  ): Reply {
    const item = items.find((x) => x.id === id)
    if (!item) return notFound
    if ('name' in body && !body.name) return invalid('name')
    if ('title' in body && !body.title) return invalid('title')
    Object.assign(item, body)
    return [200, item]
  }

  function remove(items: { id: string }[], id?: string): Reply {
    const i = items.findIndex((x) => x.id === id)
    if (i < 0) return notFound
    items.splice(i, 1)
    return [204]
  }

  const fakeFetch = async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = new Request(input, init)
    const path = new URL(request.url).pathname.replace(/^\/api/, '')
    const text = await request.text()
    const body = (text ? JSON.parse(text) : {}) as Body
    const name = `${request.method} ${path}`
    requests.push(name)

    const failure = failures.findIndex((f) => name.startsWith(f.prefix))
    const [status, data] =
      failure >= 0
        ? failures.splice(failure, 1)[0].reply
        : route(request.method, path, body)
    return new Response(data === undefined ? null : JSON.stringify(data), {
      status,
      headers: { 'Content-Type': 'application/json' },
    })
  }

  return {
    state,
    requests,
    fetch: fakeFetch,
    failNext(prefix, status, body) {
      failures.push({ prefix, reply: [status, body] })
    },
  }
}
