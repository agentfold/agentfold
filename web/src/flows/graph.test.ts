import { describe, expect, it } from 'vitest'

import type { FlowGraph } from '../api/client'
import {
  canConnect,
  defaultConfig,
  edgeFor,
  fromApi,
  isEdit,
  newStep,
  nextPosition,
  nodeOutputs,
  summary,
  toApi,
} from './graph'

const graph: FlowGraph = {
  nodes: [
    {
      id: 't',
      type: 'trigger',
      position: { x: 0, y: 0 },
      config: { event: 'card_created' },
    },
    {
      id: 'a',
      type: 'agent',
      position: { x: 300, y: 50 },
      config: { prompt: 'p' },
    },
    { id: 'e', type: 'end', position: { x: 100, y: 0 }, config: {} },
  ],
  edges: [{ id: 'x', source: 't', source_handle: 'next', target: 'a' }],
}

describe('fromApi / toApi', () => {
  it('round-trips a graph', () => {
    const { nodes, edges } = fromApi(graph)
    expect(nodes[0]).toMatchObject({
      id: 't',
      type: 'step',
      deletable: false,
      data: { kind: 'trigger' },
    })
    expect(nodes[1].deletable).toBe(true)
    expect(edges[0]).toMatchObject({
      source: 't',
      sourceHandle: 'next',
      target: 'a',
    })
    expect(toApi(nodes, edges)).toEqual(graph)
  })

  it('sends edges without handle as an empty output', () => {
    expect(
      toApi([], [{ id: 'x', source: 'a', target: 'b' }]).edges[0].source_handle,
    ).toBe('')
  })
})

describe('newStep', () => {
  it('picks an unused id and a place to the right of the others', () => {
    const { nodes } = fromApi(graph)
    const first = newStep('agent', nodes)
    expect(first).toMatchObject({
      id: 'agent-1',
      position: { x: 600, y: 50 },
      data: { config: { prompt: '' } },
    })
    expect(newStep('agent', [...nodes, first]).id).toBe('agent-2')
  })

  it('starts at the origin on an empty canvas', () => {
    expect(nextPosition([])).toEqual({ x: 0, y: 0 })
  })

  it.each([
    ['trigger', { event: 'card_created' }],
    ['agent', { prompt: '' }],
    ['move_card', {}],
    ['end', {}],
  ] as const)('gives %s nodes a default config', (kind, config) => {
    expect(defaultConfig(kind)).toEqual(config)
  })
})

describe('canConnect', () => {
  const { nodes } = fromApi(graph)
  const edges = fromApi(graph).edges

  it.each([
    [
      'a free output to another node',
      { source: 'a', sourceHandle: 'done', target: 'e' },
      true,
    ],
    [
      'into the trigger',
      { source: 'a', sourceHandle: 'done', target: 't' },
      false,
    ],
    [
      'into the same node',
      { source: 'a', sourceHandle: 'done', target: 'a' },
      false,
    ],
    [
      'into a missing node',
      { source: 'a', sourceHandle: 'done', target: 'z' },
      false,
    ],
    [
      'an output already used',
      { source: 't', sourceHandle: 'next', target: 'e' },
      false,
    ],
  ])('%s', (_, connection, want) => {
    expect(canConnect(connection, nodes, edges)).toBe(want)
  })
})

describe('edgeFor', () => {
  it('names the edge after its output', () => {
    expect(
      edgeFor({
        source: 'a',
        sourceHandle: 'done',
        target: 'e',
        targetHandle: null,
      }),
    ).toEqual({
      id: 'a.done',
      source: 'a',
      sourceHandle: 'done',
      target: 'e',
    })
    expect(
      edgeFor({
        source: 'a',
        sourceHandle: null,
        target: 'e',
        targetHandle: null,
      }).id,
    ).toBe('a.')
  })
})

describe('isEdit', () => {
  it('ignores selection and measuring', () => {
    expect(isEdit({ type: 'select', id: 'a', selected: true })).toBe(false)
    expect(isEdit({ type: 'dimensions', id: 'a' })).toBe(false)
    expect(isEdit({ type: 'remove', id: 'a' })).toBe(true)
  })
})

describe('summary', () => {
  it.each([
    [
      { kind: 'trigger', config: { event: 'card_created' } },
      'Quando o card é criado',
    ],
    [
      { kind: 'trigger', config: { event: 'card_entered_lane' } },
      'Quando o card entra numa baia',
    ],
    [{ kind: 'agent', config: { prompt: 'Implemente' } }, 'Implemente'],
    [{ kind: 'agent', config: {} }, 'Sem prompt'],
    [
      { kind: 'agent', config: { prompt: 'Revise', gateway: true } },
      'Gateway, até ? rodadas: Revise',
    ],
    [{ kind: 'move_card', config: {} }, 'Move o card de baia'],
    [{ kind: 'end', config: {} }, 'Encerra o fluxo'],
  ] as const)('describes %j', (data, want) => {
    expect(summary(data)).toBe(want)
  })
})

describe('nodeOutputs', () => {
  it.each([
    [{ kind: 'trigger', config: {} }, ['next']],
    [{ kind: 'agent', config: {} }, ['done', 'failed']],
    [
      { kind: 'agent', config: { gateway: true } },
      ['approved', 'rejected', 'max_rounds_exceeded', 'failed'],
    ],
    [{ kind: 'end', config: {} }, []],
  ] as const)('lists the outputs of %j', (data, want) => {
    expect(nodeOutputs(data)).toEqual(want)
  })
})
