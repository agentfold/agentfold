import { describe, expect, it } from 'vitest'

import type { FlowGraph } from '../api/client'
import {
  canConnect,
  defaultConfig,
  edgeFor,
  fromApi,
  idProblem,
  renameStep,
  isEdit,
  newStep,
  nextPosition,
  nodeOutputs,
  summary,
  toApi,
  type StepData,
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
    ['http_request', { method: 'POST', url: '' }],
    ['transform', { output: { resultado: '= input' } }],
    ['switch', { cases: [{ name: 'sim', expression: '' }] }],
    ['split', { branches: ['a', 'b'] }],
    ['join', {}],
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
    [
      { kind: 'create_cards', config: {} },
      'Cria um card filho por item da saída anterior',
    ],
    [{ kind: 'wait_children', config: {} }, 'Espera todos os filhos com ?=?'],
    [
      {
        kind: 'wait_children',
        config: {
          mode: 'all_finished',
          property_key: 'dev',
          property_value: 'ok',
        },
      },
      'Espera todos os filhos terminarem (dev=ok)',
    ],
    [{ kind: 'human_review', config: {} }, 'Revisão dos artefatos do run'],
    [
      { kind: 'human_review', config: { subject: 'output' } },
      'Aprovação da saída do nó anterior',
    ],
    [{ kind: 'set_property', config: {} }, 'Define ?=? no card'],
    [
      { kind: 'set_property', config: { key: 'dev', value: 'ok' } },
      'Define dev=ok no card',
    ],
    [{ kind: 'check_property', config: {} }, 'Confere ?=? nas dependências'],
    [
      {
        kind: 'check_property',
        config: { key: 'dev', value: 'ok', wait: true },
      },
      'Espera dev=ok nas dependências',
    ],
    [{ kind: 'http_request', config: {} }, '? sem URL'],
    [
      { kind: 'http_request', config: { method: 'GET', url: 'https://x' } },
      'GET https://x',
    ],
    [
      {
        kind: 'http_request',
        config: { method: 'POST', url: 'https://x', mode: 'fire_and_forget' },
      },
      'Dispara POST https://x',
    ],
    [{ kind: 'transform', config: {} }, 'Sem saída'],
    [{ kind: 'transform', config: { output: null } }, 'Sem saída'],
    [{ kind: 'transform', config: { output: { a: 1, b: 2 } } }, 'Monta a, b'],
    [{ kind: 'transform', config: { output: {} } }, 'Monta um objeto vazio'],
    [{ kind: 'transform', config: { output: [1] } }, 'Monta um valor'],
    [{ kind: 'transform', config: { output: '=1' } }, 'Monta um valor'],
    [{ kind: 'switch', config: {} }, 'Sem casos'],
    [
      {
        kind: 'switch',
        config: {
          cases: [
            { name: 'alto', expression: 'true' },
            { name: 'erro', expression: 'false' },
          ],
        },
      },
      'Casos: alto, erro',
    ],
    [{ kind: 'split', config: {} }, 'Sem ramos'],
    [
      { kind: 'split', config: { branches: ['a', 'b'] } },
      'Ramos em paralelo: a, b',
    ],
    [{ kind: 'join', config: {} }, 'Espera todos os ramos'],
    [
      { kind: 'join', config: { mode: 'all', fail_fast: true } },
      'Espera todos os ramos; para na primeira falha',
    ],
    [
      { kind: 'join', config: { mode: 'first' } },
      'Segue com o primeiro ramo que chegar',
    ],
    [{ kind: 'end', config: {} }, 'Encerra o fluxo'],
  ] as const)('describes %j', (data, want) => {
    expect(summary(data as StepData)).toBe(want)
  })

  it('names the artifacts a review covers', () => {
    expect(
      summary({
        kind: 'human_review',
        config: { artifacts: ['Plano', 'Riscos'] },
      }),
    ).toBe('Revisão de Plano, Riscos')
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
    [{ kind: 'check_property', config: {} }, ['satisfied', 'pending']],
    [{ kind: 'http_request', config: {} }, ['success', 'failed']],
    [{ kind: 'transform', config: {} }, ['next', 'failed']],
    [{ kind: 'switch', config: {} }, ['else', 'failed']],
    [{ kind: 'split', config: {} }, []],
    [{ kind: 'split', config: { branches: ['a', 'b'] } }, ['a', 'b']],
    [{ kind: 'join', config: {} }, ['all_done', 'some_failed']],
    [
      { kind: 'switch', config: { cases: [{ name: 'a', expression: '' }] } },
      ['a', 'else', 'failed'],
    ],
    [{ kind: 'http_request', config: { mode: 'fire_and_forget' } }, ['next']],
    [{ kind: 'end', config: {} }, []],
  ] as const)('lists the outputs of %j', (data, want) => {
    expect(nodeOutputs(data as StepData)).toEqual(want)
  })

  it('validates and renames node ids', () => {
    const nodes = [newStep('agent', []), newStep('end', [])]
    expect(idProblem('agent-1', 'agent-1', nodes)).toBeNull()
    expect(idProblem('end-1', 'agent-1', nodes)).toBe(
      'Outro nó já usa esse id.',
    )
    expect(idProblem('a.b', 'agent-1', nodes)).toBe(
      'Use só letras, números, _ e -.',
    )
    const edges = [
      {
        id: 'agent-1.done',
        source: 'agent-1',
        sourceHandle: 'done',
        target: 'end-1',
      },
      { id: 'x', source: 'end-1', sourceHandle: null, target: 'agent-1' },
      { id: 'y', source: 'z', target: 'end-1' },
    ]
    const renamed = renameStep(nodes, edges, 'agent-1', 'dev')
    expect(renamed.nodes.map((n) => n.id)).toEqual(['dev', 'end-1'])
    expect(renamed.edges).toEqual([
      { id: 'dev.done', source: 'dev', sourceHandle: 'done', target: 'end-1' },
      { id: 'x', source: 'end-1', sourceHandle: null, target: 'dev' },
      edges[2],
    ])
    expect(
      renameStep(
        nodes,
        [{ id: 'q', source: 'agent-1', target: 'end-1' }],
        'agent-1',
        'a',
      ).edges[0].id,
    ).toBe('a.')
  })
})
