import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Connection, EdgeChange, NodeChange } from '@xyflow/react'
import { describe, expect, it, vi } from 'vitest'

import { createFakeBackend, flow, lane } from '../test/fakeBackend'
import { renderWithApi } from '../test/render'
import type { FlowCanvasProps } from './FlowCanvas'
import { FlowEditor } from './FlowEditor'
import type { StepNode } from './graph'

// The canvas needs real layout; the editor is tested through the props it
// gives the canvas instead, which this fake records.
const canvas: { props?: FlowCanvasProps } = {}
vi.mock('./FlowCanvas', () => ({
  FlowCanvas: (props: FlowCanvasProps) => {
    canvas.props = props
    return (
      <ul aria-label="canvas">
        {props.nodes.map((n) => (
          <li key={n.id}>
            {n.id}
            {n.data.problems?.length ? ' (com problemas)' : ''}
          </li>
        ))}
      </ul>
    )
  },
}))

function props(): FlowCanvasProps {
  if (!canvas.props) throw new Error('canvas not rendered')
  return canvas.props
}

const nodeIds = () =>
  within(screen.getByRole('list', { name: 'canvas' }))
    .getAllByRole('listitem')
    .map((li) => li.textContent)

function nodesChange(changes: NodeChange<StepNode>[]) {
  act(() => {
    props().onNodesChange(changes)
  })
}

function edgesChange(changes: EdgeChange[]) {
  act(() => {
    props().onEdgesChange(changes)
  })
}

function connect(connection: Connection) {
  act(() => {
    props().onConnect(connection)
  })
}

async function setup(extra: Parameters<typeof flow>[1] = {}) {
  const f = flow('Dev', extra)
  const backend = createFakeBackend({ flows: [f], lanes: [lane('Todo', 0)] })
  const view = renderWithApi(<FlowEditor flowId={f.id} />, backend)
  await screen.findByRole('heading', { name: 'Dev' })
  return { ...view, backend, flow: f }
}

describe('FlowEditor', () => {
  it('shows load failures', async () => {
    const backend = createFakeBackend()
    backend.failNext('GET /flows', 404, {
      code: 'not_found',
      message: 'not found',
    })
    renderWithApi(<FlowEditor flowId="nope" />, backend)
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Registro não encontrado',
    )
  })

  it('shows the draft and its problems', async () => {
    await setup()
    expect(nodeIds()).toEqual(['trigger (com problemas)'])
    expect(screen.getByText('nunca publicado')).toBeInTheDocument()
    expect(
      within(screen.getByRole('region', { name: 'Problemas' })).getByText(
        /o gatilho precisa/,
      ),
    ).toBeInTheDocument()
    expect(
      screen.getByText('Selecione um nó para configurá-lo.'),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Salvar rascunho' }),
    ).toBeDisabled()
  })

  it('builds, saves and publishes a flow', async () => {
    const { backend } = await setup({ latest_version: 1 })
    expect(screen.getByText('v1 publicada')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '+ Agente' }))
    expect(nodeIds()).toEqual(['trigger', 'agent-1'])
    expect(screen.getByText('Alterações não salvas')).toBeInTheDocument()
    expect(
      screen.getByText('Salve para verificar o rascunho.'),
    ).toBeInTheDocument()

    // The new node is selected and configured in the side panel.
    await userEvent.type(screen.getByLabelText('Prompt inicial'), 'Do it')
    expect(props().nodes[1].data.config).toEqual({ prompt: 'Do it' })

    expect(
      props().isValidConnection({
        source: 'trigger',
        sourceHandle: 'next',
        target: 'agent-1',
        targetHandle: null,
      }),
    ).toBe(true)
    connect({
      source: 'trigger',
      sourceHandle: 'next',
      target: 'agent-1',
      targetHandle: null,
    })
    expect(props().edges).toHaveLength(1)
    expect(
      props().isValidConnection({
        source: 'trigger',
        sourceHandle: 'next',
        target: 'agent-1',
        targetHandle: null,
      }),
    ).toBe(false)

    await userEvent.click(
      screen.getByRole('button', { name: 'Salvar rascunho' }),
    )
    expect(
      await screen.findByText('O rascunho pode ser publicado.'),
    ).toBeInTheDocument()
    expect(backend.state.flows[0].draft.nodes).toHaveLength(2)

    await userEvent.click(screen.getByRole('button', { name: 'Publicar' }))
    expect(await screen.findByRole('status')).toHaveTextContent(
      'Versão 2 publicada.',
    )
  })

  it('saves pending changes before publishing', async () => {
    const { backend } = await setup()
    await userEvent.click(screen.getByRole('button', { name: '+ Fim' }))
    connect({
      source: 'trigger',
      sourceHandle: 'next',
      target: 'end-1',
      targetHandle: null,
    })
    await userEvent.click(screen.getByRole('button', { name: 'Publicar' }))
    expect(await screen.findByRole('status')).toHaveTextContent(
      'Versão 1 publicada.',
    )
    expect(backend.requests.filter((r) => r.startsWith('PUT'))).toHaveLength(1)
  })

  it('shows why publishing failed', async () => {
    await setup()
    await userEvent.click(screen.getByRole('button', { name: 'Publicar' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'O rascunho tem problemas',
    )
  })

  it('shows save failures', async () => {
    const { backend } = await setup()
    await userEvent.click(screen.getByRole('button', { name: '+ Fim' }))
    backend.failNext('PUT /flows', 500)
    await userEvent.click(
      screen.getByRole('button', { name: 'Salvar rascunho' }),
    )
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Erro no servidor.',
    )
    expect(screen.getByText('Alterações não salvas')).toBeInTheDocument()
  })

  it('selects, edits and removes nodes', async () => {
    await setup()
    nodesChange([{ type: 'select', id: 'trigger', selected: true }])
    expect(screen.queryByText('Alterações não salvas')).not.toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Gatilho' })).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Remover nó' }),
    ).not.toBeInTheDocument()
    await userEvent.selectOptions(
      screen.getByLabelText('Evento'),
      'card_entered_lane',
    )
    await userEvent.selectOptions(screen.getByLabelText('Baia'), 'Todo')
    expect(props().nodes[0].data.config).toMatchObject({
      event: 'card_entered_lane',
    })

    await userEvent.click(screen.getByRole('button', { name: '+ Review' }))
    connect({
      source: 'trigger',
      sourceHandle: 'next',
      target: 'review-1',
      targetHandle: null,
    })
    await userEvent.click(screen.getByRole('button', { name: 'Remover nó' }))
    expect(nodeIds()).toEqual(['trigger'])
    expect(props().edges).toEqual([])

    nodesChange([{ type: 'position', id: 'trigger', position: { x: 5, y: 5 } }])
    expect(props().nodes[0].position).toEqual({ x: 5, y: 5 })
  })

  it('tracks edge changes', async () => {
    await setup()
    await userEvent.click(screen.getByRole('button', { name: '+ Mover card' }))
    connect({
      source: 'trigger',
      sourceHandle: 'next',
      target: 'move_card-1',
      targetHandle: null,
    })
    await userEvent.click(
      screen.getByRole('button', { name: 'Salvar rascunho' }),
    )
    await waitFor(() => {
      expect(
        screen.queryByText('Alterações não salvas'),
      ).not.toBeInTheDocument()
    })

    edgesChange([{ type: 'select', id: 'trigger.next', selected: true }])
    expect(screen.queryByText('Alterações não salvas')).not.toBeInTheDocument()
    edgesChange([{ type: 'remove', id: 'trigger.next' }])
    expect(props().edges).toEqual([])
    expect(screen.getByText('Alterações não salvas')).toBeInTheDocument()
  })

  it('configures nodes while lanes and card types are unavailable', async () => {
    const f = flow('Dev')
    const backend = createFakeBackend({ flows: [f] })
    backend.failNext('GET /lanes', 500)
    backend.failNext('GET /card-types', 500)
    renderWithApi(<FlowEditor flowId={f.id} />, backend)
    await screen.findByRole('heading', { name: 'Dev' })
    nodesChange([{ type: 'select', id: 'trigger', selected: true }])
    await userEvent.selectOptions(
      screen.getByLabelText('Evento'),
      'card_entered_lane',
    )
    expect(
      within(screen.getByLabelText('Baia')).getAllByRole('option'),
    ).toHaveLength(1)
  })

  it('offers a trigger when the draft has none', async () => {
    await setup({
      draft: { nodes: [], edges: [] },
      problems: [{ message: 'sem gatilho' }],
    })
    expect(screen.getByText('sem gatilho')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '+ Gatilho' }))
    expect(nodeIds()).toEqual(['trigger-1'])
    expect(
      screen.queryByRole('button', { name: '+ Gatilho' }),
    ).not.toBeInTheDocument()
  })
})
