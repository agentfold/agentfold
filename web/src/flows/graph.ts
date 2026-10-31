import type { Connection, Edge, Node, NodeChange } from '@xyflow/react'

import type {
  AgentConfig,
  CreateCardsConfig,
  FlowGraph,
  FlowNode,
  MoveCardConfig,
  TriggerConfig,
  WaitChildrenConfig,
  HumanReviewConfig,
  SetPropertyConfig,
  CheckPropertyConfig,
  HTTPRequestConfig,
  TransformConfig,
  SwitchConfig,
} from '../api/client'

export type NodeKind = FlowNode['type']
export type StepConfig =
  | TriggerConfig
  | AgentConfig
  | MoveCardConfig
  | CreateCardsConfig
  | WaitChildrenConfig
  | HumanReviewConfig
  | SetPropertyConfig
  | CheckPropertyConfig
  | HTTPRequestConfig
  | TransformConfig
  | SwitchConfig
  | Record<string, never>

export interface StepData extends Record<string, unknown> {
  kind: NodeKind
  config: StepConfig
  /** Why the node prevents publishing, filled in for display only. */
  problems?: string[]
}

/** A flow node as the editor (React Flow) holds it. */
export type StepNode = Node<StepData, 'step'>

export const kindLabels: Record<NodeKind, string> = {
  trigger: 'Gatilho',
  agent: 'Agente',
  move_card: 'Mover card',
  create_cards: 'Criar cards (aposentado)',
  wait_children: 'Esperar filhos',
  human_review: 'Revisão humana',
  set_property: 'Definir propriedade',
  check_property: 'Conferir propriedade',
  http_request: 'HTTP Request',
  transform: 'Transformar',
  switch: 'Condição',
  end: 'Fim',
}

/** The outputs (source handles) of each node kind, as in the contract. */
const outputs: Record<NodeKind, string[]> = {
  trigger: ['next'],
  agent: ['done', 'failed'],
  move_card: ['next'],
  create_cards: ['next', 'failed'],
  wait_children: ['all_done', 'some_failed'],
  human_review: ['approved', 'changes_requested'],
  set_property: ['next'],
  check_property: ['satisfied', 'pending'],
  http_request: ['success', 'failed'],
  transform: ['next', 'failed'],
  switch: ['else', 'failed'],
  end: [],
}

/** The outputs of an agent node with the gateway on. */
const gatewayOutputs = ['approved', 'rejected', 'max_rounds_exceeded', 'failed']

/**
 * The outputs of a node: agents with the gateway on decide where to go, an
 * HTTP Request firing and forgetting only goes on, and a Condition has one
 * output per case before else and failed.
 */
export function nodeOutputs({ kind, config }: StepData): string[] {
  if (kind === 'switch') {
    const cases = (config as SwitchConfig).cases ?? []
    return [...cases.map((c) => c.name), ...outputs.switch]
  }
  if (kind === 'agent' && (config as AgentConfig).gateway) {
    return gatewayOutputs
  }
  if (
    kind === 'http_request' &&
    (config as HTTPRequestConfig).mode === 'fire_and_forget'
  ) {
    return ['next']
  }
  return outputs[kind]
}

export const outputLabels: Record<string, string> = {
  next: 'próximo',
  done: 'concluído',
  failed: 'falhou',
  approved: 'aprovado',
  rejected: 'rejeitado',
  max_rounds_exceeded: 'limite de rodadas',
  all_done: 'todos concluídos',
  some_failed: 'algum falhou',
  changes_requested: 'pediu ajustes',
  satisfied: 'satisfeitas',
  pending: 'pendentes',
  success: 'sucesso',
  else: 'senão',
}

export function defaultConfig(kind: NodeKind): StepConfig {
  switch (kind) {
    case 'trigger':
      return { event: 'card_created' }
    case 'agent':
      return { prompt: '' }
    case 'http_request':
      return { method: 'POST', url: '' }
    case 'transform':
      return { output: { resultado: '= input' } }
    case 'switch':
      return { cases: [{ name: 'sim', expression: '' }] }
    default:
      return {}
  }
}

function toStep(
  id: string,
  kind: NodeKind,
  position: { x: number; y: number },
  config: StepConfig,
): StepNode {
  // The trigger starts every run: it can be edited but not removed.
  return {
    id,
    type: 'step',
    position,
    data: { kind, config },
    deletable: kind !== 'trigger',
  }
}

export function fromApi(graph: FlowGraph): {
  nodes: StepNode[]
  edges: Edge[]
} {
  return {
    nodes: graph.nodes.map((n) => toStep(n.id, n.type, n.position, n.config)),
    edges: graph.edges.map((e) => ({
      id: e.id,
      source: e.source,
      sourceHandle: e.source_handle,
      target: e.target,
    })),
  }
}

export function toApi(nodes: StepNode[], edges: Edge[]): FlowGraph {
  return {
    nodes: nodes.map(
      (n) =>
        ({
          id: n.id,
          type: n.data.kind,
          position: n.position,
          config: n.data.config,
        }) as FlowNode,
    ),
    edges: edges.map((e) => ({
      id: e.id,
      source: e.source,
      source_handle: e.sourceHandle ?? '',
      target: e.target,
    })),
  }
}

/** Where a new node goes: to the right of the rightmost node. */
export function nextPosition(nodes: StepNode[]): { x: number; y: number } {
  if (nodes.length === 0) {
    return { x: 0, y: 0 }
  }
  const rightmost = nodes.reduce((a, b) =>
    b.position.x > a.position.x ? b : a,
  )
  return { x: rightmost.position.x + 300, y: rightmost.position.y }
}

/** A new node of `kind`, placed after the others, with an unused id. */
export function newStep(kind: NodeKind, nodes: StepNode[]): StepNode {
  const ids = new Set(nodes.map((n) => n.id))
  let i = 1
  while (ids.has(`${kind}-${String(i)}`)) {
    i++
  }
  return toStep(
    `${kind}-${String(i)}`,
    kind,
    nextPosition(nodes),
    defaultConfig(kind),
  )
}

/** What node ids are made of, so {{nodes.<id>}} placeholders name them. */
const nodeIdPattern = /^[A-Za-z0-9_-]+$/

/** Why `id` cannot be the new id of the node `from`; null when it can. */
export function idProblem(
  id: string,
  from: string,
  nodes: StepNode[],
): string | null {
  if (!nodeIdPattern.test(id)) {
    return 'Use só letras, números, _ e -.'
  }
  if (id !== from && nodes.some((n) => n.id === id)) {
    return 'Outro nó já usa esse id.'
  }
  return null
}

/**
 * Renames the node `from` to `to`; its edges follow it. Placeholders that
 * read it keep the old id: the draft problems point them out.
 */
export function renameStep(
  nodes: StepNode[],
  edges: Edge[],
  from: string,
  to: string,
): { nodes: StepNode[]; edges: Edge[] } {
  return {
    nodes: nodes.map((n) => (n.id === from ? { ...n, id: to } : n)),
    edges: edges.map((e) => {
      const source = e.source === from ? to : e.source
      const target = e.target === from ? to : e.target
      if (source === e.source && target === e.target) {
        return e
      }
      const id =
        source === e.source ? e.id : `${source}.${e.sourceHandle ?? ''}`
      return { ...e, id, source, target }
    }),
  }
}

/**
 * Whether a connection follows the flow rules: not into the trigger, not
 * into the same node, and each output used once.
 */
export function canConnect(
  connection: { source: string; target: string; sourceHandle?: string | null },
  nodes: StepNode[],
  edges: Edge[],
): boolean {
  const target = nodes.find((n) => n.id === connection.target)
  if (
    !target ||
    target.data.kind === 'trigger' ||
    connection.source === connection.target
  ) {
    return false
  }
  return !edges.some(
    (e) =>
      e.source === connection.source &&
      e.sourceHandle === connection.sourceHandle,
  )
}

/** The edge for a connection; its id names the output it leaves from. */
export function edgeFor(connection: Connection): Edge {
  return {
    id: `${connection.source}.${connection.sourceHandle ?? ''}`,
    source: connection.source,
    sourceHandle: connection.sourceHandle,
    target: connection.target,
  }
}

/** Whether a React Flow change edits the graph (and not only the view). */
export function isEdit(change: NodeChange<StepNode>): boolean {
  return change.type !== 'select' && change.type !== 'dimensions'
}

/** One line describing what the node is set to do. */
export function summary({ kind, config }: StepData): string {
  switch (kind) {
    case 'trigger':
      return (config as TriggerConfig).event === 'card_entered_lane'
        ? 'Quando o card entra numa baia'
        : 'Quando o card é criado'
    case 'agent': {
      const agent = config as AgentConfig
      const prompt = agent.prompt ?? ''
      const text = prompt === '' ? 'Sem prompt' : prompt
      return agent.gateway
        ? `Gateway, até ${String(agent.max_rounds ?? '?')} rodadas: ${text}`
        : text
    }
    case 'move_card':
      return 'Move o card de baia'
    case 'create_cards':
      return 'Cria um card filho por item da saída anterior'
    case 'wait_children': {
      const wait = config as WaitChildrenConfig
      const property = `${wait.property_key ?? '?'}=${wait.property_value ?? '?'}`
      return wait.mode === 'all_finished'
        ? `Espera todos os filhos terminarem (${property})`
        : `Espera todos os filhos com ${property}`
    }
    case 'human_review': {
      if ((config as HumanReviewConfig).subject === 'output') {
        return 'Aprovação da saída do nó anterior'
      }
      const names = (config as HumanReviewConfig).artifacts ?? []
      return names.length > 0
        ? `Revisão de ${names.join(', ')}`
        : 'Revisão dos artefatos do run'
    }
    case 'set_property': {
      const { key = '?', value = '?' } = config as SetPropertyConfig
      return `Define ${key}=${value} no card`
    }
    case 'check_property': {
      const { key = '?', value = '?', wait } = config as CheckPropertyConfig
      return `${wait ? 'Espera' : 'Confere'} ${key}=${value} nas dependências`
    }
    case 'http_request': {
      const { method = '?', url = '', mode } = config as HTTPRequestConfig
      const call = `${method} ${url === '' ? 'sem URL' : url}`
      return mode === 'fire_and_forget' ? `Dispara ${call}` : call
    }
    case 'transform': {
      const { output } = config as TransformConfig
      if (output === undefined || output === null) {
        return 'Sem saída'
      }
      return typeof output === 'object' && !Array.isArray(output)
        ? `Monta ${Object.keys(output).join(', ') || 'um objeto vazio'}`
        : 'Monta um valor'
    }
    case 'switch': {
      const names = ((config as SwitchConfig).cases ?? []).map((c) => c.name)
      return names.length > 0 ? `Casos: ${names.join(', ')}` : 'Sem casos'
    }
    case 'end':
      return 'Encerra o fluxo'
  }
}
