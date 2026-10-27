import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  card,
  cardEvent,
  cardType,
  createFakeBackend,
  flowRun,
  lane,
  nodeOutput,
  nodeRun,
  project,
  question,
  runDetail,
} from '../test/fakeBackend'
import { renderWithApi } from '../test/render'
import { CardPage } from './CardPage'

function setup(runs: (cardId: string) => ReturnType<typeof runDetail>[]) {
  const todo = lane('Todo', 0)
  const task = cardType('task')
  const web = project('web')
  const c = card('Login', todo.id, task.id, 0, { project_id: web.id })
  const backend = createFakeBackend({
    lanes: [todo],
    cardTypes: [task],
    projects: [web],
    cards: [c],
    cardRuns: runs(c.id),
    questions: [question(c.id, 'Which DB?')],
    events: [cardEvent(c.id, 'card.created', { lane_id: todo.id })],
  })
  return {
    ...renderWithApi(<CardPage cardId={c.id} />, backend),
    backend,
    card: c,
  }
}

const tab = (name: string) => screen.getByRole('tab', { name })

describe('CardPage', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    window.location.hash = ''
  })

  it('shows the card, its questions and its run', async () => {
    const dev = nodeRun('dev', 'agent', { summary: 'did it' })
    setup((id) => [runDetail(id, 'waiting', [dev])])
    expect(
      await screen.findByRole('heading', { name: 'Login' }),
    ).toBeInTheDocument()
    expect(screen.getByText('Todo · task · web')).toBeInTheDocument()
    expect(screen.getByText('Aguardando · dev')).toBeInTheDocument()
    expect(
      await screen.findByRole('region', { name: 'Perguntas do agente' }),
    ).toBeInTheDocument()
    expect(tab('Execução')).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('article', { name: 'dev 1' })).toHaveTextContent(
      'Resumo: did it',
    )
  })

  it('cancels the active run after confirmation', async () => {
    const { backend } = setup((id) => [runDetail(id, 'running', [])])
    const button = await screen.findByRole('button', { name: 'Cancelar fluxo' })
    vi.spyOn(window, 'confirm').mockReturnValueOnce(false).mockReturnValue(true)
    await userEvent.click(button)
    expect(backend.state.cardRuns[0].status).toBe('running')
    await userEvent.click(button)
    await waitFor(() => {
      expect(backend.state.cardRuns[0].status).toBe('canceled')
    })
    expect(await screen.findByText('Cancelado · dev')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Cancelar fluxo' }),
    ).not.toBeInTheDocument()
  })

  it('shows why a cancel failed', async () => {
    const { backend } = setup((id) => [runDetail(id, 'running', [])])
    const button = await screen.findByRole('button', { name: 'Cancelar fluxo' })
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    backend.failNext('POST /flow-runs', 500)
    await userEvent.click(button)
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Erro no servidor.',
    )
  })

  it('edits the card, shows the timeline and the usage', async () => {
    const { backend } = setup((id) => [
      runDetail(id, 'done', [
        nodeRun('dev', 'agent'),
        nodeRun('move', 'move_card'),
        nodeRun('dev', 'agent', { iteration: 2 }),
      ]),
    ])
    await userEvent.click(await screen.findByRole('tab', { name: 'Detalhes' }))
    const form = screen.getByRole('region', { name: 'Editar card' })
    expect(
      within(form).queryByRole('button', { name: 'Cancelar' }),
    ).not.toBeInTheDocument()
    const title = within(form).getByLabelText('Título')
    await userEvent.clear(title)
    await userEvent.type(title, 'Sign in')
    await userEvent.click(within(form).getByRole('button', { name: 'Salvar' }))
    await waitFor(() => {
      expect(backend.state.cards[0].title).toBe('Sign in')
    })
    expect(window.location.hash).toBe('')

    await userEvent.click(tab('Linha do tempo'))
    expect(await screen.findByText('Card criado em Todo')).toBeInTheDocument()

    await userEvent.click(tab('Consumo'))
    const table = screen.getByRole('table')
    expect(within(table).getAllByRole('row')).toHaveLength(4)
    expect(within(table).getByText('Agente dev (2ª vez)')).toBeInTheDocument()
    expect(within(table).getByText('Total do card')).toBeInTheDocument()

    await userEvent.click(tab('Detalhes'))
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    await userEvent.click(screen.getByRole('button', { name: 'Excluir' }))
    await waitFor(() => {
      expect(window.location.hash).toBe('#/board')
    })
  })

  it('shows the child cards and the parent', async () => {
    const todo = lane('Todo', 0)
    const task = cardType('task')
    const parent = card('Chat', todo.id, task.id, 0)
    const api = card('API', todo.id, task.id, 1, {
      parent_card_id: parent.id,
    })
    const web = card('Web', 'gone', task.id, 2, { parent_card_id: parent.id })
    const backend = createFakeBackend({
      lanes: [todo],
      cardTypes: [task],
      cards: [parent, api, web],
      runs: [flowRun(api.id, 'done', { flow_name: 'impl' })],
    })
    renderWithApi(<CardPage cardId={parent.id} />, backend)
    const family = await screen.findByRole('region', {
      name: 'Cards relacionados',
    })
    expect(
      await within(family).findByText(
        'Cards filhos · 1/2 com o fluxo concluído',
      ),
    ).toBeInTheDocument()
    expect(within(family).getByText('Concluído · impl')).toBeInTheDocument()
    expect(within(family).getByRole('link', { name: 'Web' })).toHaveAttribute(
      'href',
      `#/cards/${web.id}`,
    )
    expect(within(family).getByText('Todo')).toBeInTheDocument()
  })

  it('links a child card to its parent', async () => {
    const todo = lane('Todo', 0)
    const task = cardType('task')
    const parent = card('Chat', todo.id, task.id, 0)
    const api = card('API', todo.id, task.id, 1, {
      parent_card_id: parent.id,
    })
    const backend = createFakeBackend({
      lanes: [todo],
      cardTypes: [task],
      cards: [parent, api],
    })
    renderWithApi(<CardPage cardId={api.id} />, backend)
    expect(await screen.findByRole('link', { name: 'Chat' })).toHaveAttribute(
      'href',
      `#/cards/${parent.id}`,
    )
    expect(screen.queryByText(/Cards filhos/)).not.toBeInTheDocument()
  })

  it('shows a card without runs', async () => {
    setup(() => [])
    expect(
      await screen.findByText('Nenhum fluxo rodou para este card.'),
    ).toBeInTheDocument()
    await userEvent.click(tab('Consumo'))
    expect(
      screen.getByText('Nenhum agente rodou para este card.'),
    ).toBeInTheDocument()
    await userEvent.click(tab('Artefatos'))
    expect(
      await screen.findByText('Nenhum artefato coletado neste card.'),
    ).toBeInTheDocument()
  })

  it('shows unknown cards and load failures', async () => {
    const backend = createFakeBackend()
    renderWithApi(<CardPage cardId="missing" />, backend)
    expect(await screen.findByText(/Card não encontrado/)).toBeInTheDocument()

    const failing = createFakeBackend()
    failing.failNext('GET /cards/x/runs', 500)
    renderWithApi(<CardPage cardId="x" />, failing)
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Erro no servidor.',
    )
  })
})

describe('RunsPanel', () => {
  it('shows the nodes of the chosen run and what the agents did', async () => {
    const review = nodeRun('rev', 'agent', {
      iteration: 2,
      output_handle: 'rejected',
      decision: 'rejected',
      feedback: 'add tests',
    })
    const approved = nodeRun('gate', 'agent', {
      output_handle: 'approved',
      decision: 'approved',
    })
    const dev = nodeRun('dev', 'agent', {
      status: 'running',
      output_handle: '',
      output: { output: { plan: 'x' } },
    })
    const failed = nodeRun('gone', 'removed', {
      status: 'failed',
      error: 'boom',
      output_handle: '',
    })
    const { backend } = setup((id) => [
      runDetail(id, 'running', [dev, review, approved, failed], {
        flow_name: 'impl',
      }),
      runDetail(id, 'failed', [nodeRun('old', 'agent')], {
        flow_name: 'old',
        trigger: 'artifact_revision',
        error: 'no result',
      }),
    ])
    backend.state.outputs = [
      nodeOutput(dev.id, 'text', 'Looking'),
      nodeOutput(dev.id, 'tool_use', '{"file":"a.go"}', { tool: 'Read' }),
      nodeOutput(dev.id, 'tool_result', 'denied', { is_error: true }),
      nodeOutput(dev.id, 'tool_result', 'ok'),
      nodeOutput(dev.id, 'result', 'done'),
      nodeOutput(dev.id, 'result', 'failed', { is_error: true }),
    ]
    const devNode = await screen.findByRole('article', { name: 'dev 1' })
    const steps = await within(devNode).findByRole('list', {
      name: 'Saída do agente',
    })
    expect(
      within(steps)
        .getAllByRole('listitem')
        .map((li) => li.textContent),
    ).toEqual([
      'Looking',
      'Read {"file":"a.go"}',
      'denied',
      'ok',
      'Resultado: done',
      'Resultado: failed',
    ])
    expect(within(devNode).getByText(/"plan": "x"/)).toBeInTheDocument()
    await userEvent.click(
      within(devNode).getByRole('button', { name: 'Ocultar saída do agente' }),
    )
    expect(within(devNode).queryByRole('list')).not.toBeInTheDocument()

    const reviewNode = screen.getByRole('article', { name: 'rev 2' })
    expect(reviewNode).toHaveTextContent('2ª vez')
    expect(reviewNode).toHaveTextContent('saída: rejeitado')
    expect(reviewNode).toHaveTextContent('Rejeitou: add tests')
    expect(screen.getByRole('article', { name: 'gate 1' })).toHaveTextContent(
      'Aprovou',
    )
    await userEvent.click(
      within(reviewNode).getByRole('button', { name: 'Ver saída do agente' }),
    )
    expect(
      await within(reviewNode).findByText('Nenhuma saída ainda.'),
    ).toBeInTheDocument()

    const failedNode = screen.getByRole('article', { name: 'gone 1' })
    expect(failedNode).toHaveTextContent('removed gone')
    expect(failedNode).toHaveTextContent('boom')
    expect(within(failedNode).queryByRole('button')).not.toBeInTheDocument()

    await userEvent.selectOptions(
      screen.getByLabelText('Execução'),
      backend.state.cardRuns[1].id,
    )
    expect(screen.getByText(/: no result/)).toBeInTheDocument()
    expect(screen.getByText('old (revisão de artefato)')).toBeInTheDocument()
    expect(screen.getByRole('article', { name: 'old 1' })).toHaveTextContent(
      'saída: concluído',
    )
  })

  it('shows output load failures', async () => {
    const dev = nodeRun('dev', 'agent', { status: 'running' })
    const { backend } = setup((id) => [runDetail(id, 'running', [dev])])
    backend.failNext(`GET /node-runs/${dev.id}/output`, 500)
    expect(await screen.findByText('Erro no servidor.')).toBeInTheDocument()
  })
})
