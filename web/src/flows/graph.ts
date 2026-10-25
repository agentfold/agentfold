import type { Connection, Edge, Node, NodeChange } from '@xyflow/react'

import type {
  AgentConfig,
  FlowGraph,
  FlowNode,
  MoveCardConfig,
  ReviewConfig,
  TriggerConfig,
} from '../api/client'

export type NodeKind = FlowNode['type']
export type StepConfig =
  | TriggerConfig
  | AgentConfig
  | ReviewConfig
  | MoveCardConfig
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
  review: 'Review',
  move_card: 'Mover card',
  end: 'Fim',
}

/** The outputs (source handles) of each node kind, as in the contract. */
export const outputs: Record<NodeKind, string[]> = {
  trigger: ['next'],
  agent: ['done', 'failed'],
  review: ['approved', 'changes_requested', 'max_rounds_exceeded'],
  move_card: ['next'],
  end: [],
}

export const outputLabels: Record<string, string> = {
  next: 'próximo',
  done: 'concluído',
  failed: 'falhou',
  approved: 'aprovado',
  changes_requested: 'pedir mudanças',
  max_rounds_exceeded: 'limite de rodadas',
}

export function defaultConfig(kind: NodeKind): StepConfig {
  switch (kind) {
    case 'trigger':
      return { event: 'card_created' }
    case 'agent':
      return { prompt: '' }
    case 'review':
      return { prompt: '', max_rounds: 3 }
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
      const prompt = (config as AgentConfig).prompt ?? ''
      return prompt === '' ? 'Sem prompt' : prompt
    }
    case 'review':
      return `Até ${String((config as ReviewConfig).max_rounds ?? '?')} rodadas`
    case 'move_card':
      return 'Move o card de baia'
    case 'end':
      return 'Encerra o fluxo'
  }
}
