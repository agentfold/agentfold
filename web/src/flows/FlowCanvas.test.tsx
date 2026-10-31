import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { FlowCanvas } from './FlowCanvas'
import { fromApi, type StepNode } from './graph'

const { nodes, edges } = fromApi({
  nodes: [
    {
      id: 't',
      type: 'trigger',
      position: { x: 0, y: 0 },
      config: { event: 'card_created' },
    },
    {
      id: 'r',
      type: 'agent',
      position: { x: 300, y: 0 },
      config: { prompt: 'Revise', gateway: true, max_rounds: 2 },
    },
    { id: 'e', type: 'end', position: { x: 600, y: 0 }, config: {} },
    {
      id: 'pick',
      type: 'switch',
      position: { x: 900, y: 0 },
      config: { cases: [{ name: 'alto', expression: 'true' }] },
    },
  ],
  edges: [{ id: 'x', source: 't', source_handle: 'next', target: 'r' }],
})

function show(steps: StepNode[]) {
  return render(
    <div style={{ width: 800, height: 600 }}>
      <FlowCanvas
        nodes={steps}
        edges={edges}
        onNodesChange={vi.fn()}
        onEdgesChange={vi.fn()}
        onConnect={vi.fn()}
        isValidConnection={() => true}
      />
    </div>,
  )
}

describe('FlowCanvas', () => {
  it('draws each step with its outputs', () => {
    show(nodes)
    expect(screen.getByText('Gatilho')).toBeInTheDocument()
    expect(screen.getByText('Quando o card é criado')).toBeInTheDocument()
    expect(
      screen.getByText('Gateway, até 2 rodadas: Revise'),
    ).toBeInTheDocument()
    expect(screen.getByText('rejeitado')).toBeInTheDocument()
    expect(screen.getByText('Encerra o fluxo')).toBeInTheDocument()
    // A case of a Condition is named as written.
    expect(screen.getByText('alto')).toBeInTheDocument()
    expect(screen.getByText('senão')).toBeInTheDocument()
  })

  it('flags steps with problems', () => {
    const flagged = nodes.map((n, i) => ({
      ...n,
      data: {
        ...n.data,
        problems: i === 0 ? ['a'] : i === 1 ? ['a', 'b'] : undefined,
      },
    }))
    show(flagged)
    expect(screen.getByText('1 problema')).toBeInTheDocument()
    expect(screen.getByText('2 problemas')).toHaveAttribute('title', 'a\nb')
  })
})
