import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  artifact,
  artifactMessage,
  createFakeBackend,
  nodeRun,
  runDetail,
} from '../test/fakeBackend'

const stamp = '2026-10-30T16:39:18Z'
import { renderWithApi } from '../test/render'
import { ArtifactsPanel } from './ArtifactsPanel'

function setup(artifacts: ReturnType<typeof artifact>[]) {
  const backend = createFakeBackend({ artifacts })
  renderWithApi(<ArtifactsPanel cardId="c" />, backend)
  return backend
}

// jsdom lays nothing out: a selection sits at a fixed place.
beforeEach(() => {
  Range.prototype.getBoundingClientRect = () => new DOMRect(30, 20, 100, 16)
})

/** Selects the text of node, as a reader dragging over it would. */
function select(node: Node) {
  const range = document.createRange()
  range.selectNodeContents(node)
  const selection = window.getSelection()
  selection?.removeAllRanges()
  selection?.addRange(range)
  fireEvent.mouseUp(screen.getByTestId('artifact-content'))
}

describe('ArtifactsPanel', () => {
  it('says when there are no artifacts', async () => {
    setup([])
    expect(
      await screen.findByText('Nenhum artefato coletado neste card.'),
    ).toBeInTheDocument()
  })

  it('shows load failures', async () => {
    const backend = createFakeBackend()
    backend.failNext('GET /cards/c/artifacts', 500)
    renderWithApi(<ArtifactsPanel cardId="c" />, backend)
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Erro no servidor.',
    )
  })

  it('renders the versions as Markdown', async () => {
    const plan = artifact('c', 'Plano', [
      '# Plano v1',
      '# Plano v2\n\n| a | b |\n|---|---|\n| 1 | 2 |',
    ])
    setup([plan, artifact('c', 'Riscos', ['- um'])])
    expect(
      await screen.findByRole('heading', { name: 'Plano v2' }),
    ).toBeInTheDocument()
    expect(screen.getByRole('table')).toBeInTheDocument()
    expect(
      screen.getByRole('option', { name: 'Versão 2 (atual)' }),
    ).toBeInTheDocument()

    await userEvent.selectOptions(screen.getByLabelText('Versão'), '1')
    expect(
      screen.getByRole('heading', { name: 'Plano v1' }),
    ).toBeInTheDocument()

    await userEvent.click(screen.getByRole('tab', { name: 'Riscos' }))
    expect(screen.getByRole('tab', { name: 'Riscos' })).toHaveAttribute(
      'aria-selected',
      'true',
    )
    expect(screen.getByText('um')).toBeInTheDocument()
    expect(screen.queryByLabelText('Versão')).not.toBeInTheDocument()
  })

  it('opens on the artifact awaiting review', async () => {
    setup([
      artifact('c', 'Riscos', ['# Riscos'], { review: 'approved' }),
      artifact('c', 'Plano', ['# Plano']),
    ])
    expect(
      await screen.findByRole('heading', { name: 'Plano' }),
    ).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Plano' })).toHaveAttribute(
      'aria-selected',
      'true',
    )
    expect(screen.getByTitle('Aguardando revisão')).toBeInTheDocument()
  })

  it('quotes a selected passage and sends the message', async () => {
    const plan = artifact('c', 'Plano', ['# Plano\n\nUsar Postgres.'])
    const backend = setup([plan])
    const passage = await screen.findByText('Usar Postgres.')
    expect(
      screen.queryByRole('button', { name: 'Citar' }),
    ).not.toBeInTheDocument()

    act(() => {
      select(passage)
    })
    const cite = screen.getByRole('button', { name: 'Citar' })
    expect(cite).toHaveStyle({ top: '8px', left: '30px' })
    expect(fireEvent.mouseDown(cite)).toBe(false)
    await userEvent.click(cite)
    const message = screen.getByLabelText('Mensagem para o agente')
    expect(message).toHaveValue('> Usar Postgres.\n\n')
    expect(message).toHaveFocus()
    expect(window.getSelection()?.toString()).toBe('')
    await userEvent.type(message, 'troque por SQLite')

    act(() => {
      select(screen.getByRole('heading', { name: 'Plano' }))
    })
    await userEvent.click(screen.getByRole('button', { name: 'Citar' }))
    expect(message).toHaveValue(
      '> Usar Postgres.\n\ntroque por SQLite\n\n> Plano\n\n',
    )

    await userEvent.click(screen.getByRole('button', { name: 'Pedir ajustes' }))
    await waitFor(() => {
      expect(message).toHaveValue('')
    })
    expect(backend.state.artifacts[0].messages[0]).toMatchObject({
      role: 'human',
      version: 1,
    })
    expect(
      await screen.findByText('Agente · ajustando o artefato…'),
    ).toBeInTheDocument()
    expect(message).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Pedir ajustes' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Aprovar' })).toBeDisabled()
  })

  it('quotes passages of several lines and drops the offer on scroll', async () => {
    setup([artifact('c', 'Plano', ['```\nlinha 1\nlinha 2\n```'])])
    const code = await screen.findByText(/linha 1/)
    act(() => {
      select(code)
    })
    await userEvent.click(screen.getByRole('button', { name: 'Citar' }))
    expect(screen.getByLabelText('Mensagem para o agente')).toHaveValue(
      '> linha 1\n> linha 2\n\n',
    )

    act(() => {
      select(code)
    })
    fireEvent.scroll(screen.getByTestId('artifact-content'))
    expect(
      screen.queryByRole('button', { name: 'Citar' }),
    ).not.toBeInTheDocument()

    act(() => {
      select(code)
    })
    window.getSelection()?.removeAllRanges()
    fireEvent.mouseUp(screen.getByTestId('artifact-content'))
    expect(
      screen.queryByRole('button', { name: 'Citar' }),
    ).not.toBeInTheDocument()
  })

  it.each([
    [
      'running',
      [nodeRun('planner', 'agent', { status: 'running', started_at: stamp })],
      /O agente está trabalhando desde/,
    ],
    [
      'queued',
      [nodeRun('planner', 'agent', { status: 'queued', started_at: null })],
      'Nenhuma saída ainda.',
    ],
  ] as const)('shows the %s agent while it revises', async (_, nodes, text) => {
    const plan = artifact('c', 'Plano', ['x'], { review: 'revising' })
    const backend = createFakeBackend({
      artifacts: [plan],
      cardRuns: [
        runDetail('c', 'running', [
          nodeRun('review', 'human_review'),
          ...nodes,
        ]),
      ],
    })
    renderWithApi(<ArtifactsPanel cardId="c" />, backend)
    expect(await screen.findByText(text)).toBeInTheDocument()
    expect(
      screen.getByText('Agente · ajustando o artefato…'),
    ).toBeInTheDocument()
    expect(screen.queryByText('Nenhuma mensagem.')).not.toBeInTheDocument()
  })

  it('approves the review', async () => {
    const backend = setup([artifact('c', 'Plano', ['texto'])])
    await userEvent.click(
      await screen.findByRole('button', { name: 'Aprovar' }),
    )
    expect(await screen.findByText('Aprovado.')).toBeInTheDocument()
    expect(backend.state.artifacts[0].review).toBe('approved')
    expect(
      screen.queryByLabelText('Mensagem para o agente'),
    ).not.toBeInTheDocument()
  })

  it('shows why an approval failed', async () => {
    const backend = setup([artifact('c', 'Plano', ['texto'])])
    backend.failNext('POST /artifacts', 409, {
      code: 'review_closed',
      message: 'closed',
    })
    await userEvent.click(
      await screen.findByRole('button', { name: 'Aprovar' }),
    )
    expect(await screen.findByRole('alert')).toBeInTheDocument()
  })

  it('takes comments only on the latest version', async () => {
    setup([artifact('c', 'Plano', ['# Velho', '# Novo'])])
    await screen.findByRole('heading', { name: 'Novo' })
    await userEvent.selectOptions(screen.getByLabelText('Versão'), '1')
    expect(
      screen.getByText(/Versão antiga: os comentários e a aprovação/),
    ).toHaveTextContent('sobre a versão 2.')
    act(() => {
      select(screen.getByRole('heading', { name: 'Velho' }))
    })
    expect(
      screen.queryByRole('button', { name: 'Citar' }),
    ).not.toBeInTheDocument()
    expect(screen.getByLabelText('Mensagem para o agente')).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Aprovar' })).toBeDisabled()
  })

  it('ignores selections outside the artifact', async () => {
    setup([artifact('c', 'Plano', ['texto'])])
    const passage = await screen.findByText('texto')
    act(() => {
      select(screen.getByText(/Selecione um trecho/))
    })
    expect(
      screen.queryByRole('button', { name: 'Citar' }),
    ).not.toBeInTheDocument()
    vi.spyOn(window, 'getSelection').mockReturnValue(null)
    fireEvent.mouseUp(passage)
    expect(
      screen.queryByRole('button', { name: 'Citar' }),
    ).not.toBeInTheDocument()
    vi.restoreAllMocks()
  })

  it('shows the chat and why a message failed', async () => {
    const plan = artifact('c', 'Plano', ['a', 'b'])
    plan.messages = [
      artifactMessage(plan.id, 'human', 'mais detalhe'),
      artifactMessage(plan.id, 'agent', 'detalhei', { version: 2 }),
    ]
    const backend = setup([plan])
    const chat = await screen.findByRole('region', { name: 'Revisão' })
    expect(
      within(chat)
        .getAllByRole('listitem')
        .map((li) => li.textContent),
    ).toEqual(['Você · versão 1mais detalhe', 'Agente · versão 2detalhei'])
    backend.failNext('POST /artifacts', 409, {
      code: 'review_closed',
      message: 'closed',
    })
    await userEvent.type(screen.getByLabelText('Mensagem para o agente'), 'x')
    await userEvent.click(screen.getByRole('button', { name: 'Pedir ajustes' }))
    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByLabelText('Mensagem para o agente')).toHaveValue('x')
  })

  it.each([
    ['approved', 'Aprovado.'],
    ['closed', 'A revisão terminou sem aprovação'],
    ['none', undefined],
  ] as const)('keeps %s reviews read-only', async (review, note) => {
    setup([artifact('c', 'Plano', ['Usar Postgres.'], { review })])
    const passage = await screen.findByText('Usar Postgres.')
    expect(screen.getByText('Nenhuma mensagem.')).toBeInTheDocument()
    if (note) {
      expect(screen.getByText(new RegExp(note))).toBeInTheDocument()
    }
    act(() => {
      select(passage)
    })
    expect(
      screen.queryByRole('button', { name: 'Citar' }),
    ).not.toBeInTheDocument()
    expect(
      screen.queryByLabelText('Mensagem para o agente'),
    ).not.toBeInTheDocument()
  })
})
