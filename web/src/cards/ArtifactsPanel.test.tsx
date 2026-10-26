import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import {
  artifact,
  artifactMessage,
  createFakeBackend,
} from '../test/fakeBackend'
import { renderWithApi } from '../test/render'
import { ArtifactsPanel } from './ArtifactsPanel'

function setup(artifacts: ReturnType<typeof artifact>[]) {
  const backend = createFakeBackend({ artifacts })
  renderWithApi(<ArtifactsPanel cardId="c" />, backend)
  return backend
}

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

    await userEvent.selectOptions(screen.getByLabelText('Artefato'), 'Riscos')
    expect(screen.getByRole('listitem')).toHaveTextContent('um')
    expect(screen.queryByLabelText('Versão')).not.toBeInTheDocument()
  })

  it('quotes a selected passage and sends the message', async () => {
    const plan = artifact('c', 'Plano', ['# Plano\n\nUsar Postgres.'])
    const backend = setup([plan])
    const passage = await screen.findByText('Usar Postgres.')
    expect(
      screen.queryByRole('button', { name: 'Revisar' }),
    ).not.toBeInTheDocument()

    act(() => {
      select(passage)
    })
    await userEvent.click(screen.getByRole('button', { name: 'Revisar' }))
    const message = screen.getByLabelText('Mensagem para o agente')
    expect(message).toHaveValue('Citação sobre "Usar Postgres.":\n')
    await userEvent.type(message, 'troque por SQLite')

    act(() => {
      select(screen.getByRole('heading', { name: 'Plano' }))
    })
    await userEvent.click(screen.getByRole('button', { name: 'Revisar' }))
    expect(message).toHaveValue(
      'Citação sobre "Usar Postgres.":\ntroque por SQLite\n\nCitação sobre "Plano":\n',
    )

    await userEvent.click(screen.getByRole('button', { name: 'Enviar' }))
    await waitFor(() => {
      expect(message).toHaveValue('')
    })
    expect(backend.state.artifacts[0].messages[0]).toMatchObject({
      role: 'human',
      version: 1,
    })
    expect(
      await screen.findByText('O agente está revisando…'),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Enviar' })).toBeDisabled()
  })

  it('ignores selections outside the artifact', async () => {
    setup([artifact('c', 'Plano', ['texto'])])
    const passage = await screen.findByText('texto')
    act(() => {
      select(screen.getByText(/Selecione um trecho/))
    })
    expect(
      screen.queryByRole('button', { name: 'Revisar' }),
    ).not.toBeInTheDocument()
    vi.spyOn(window, 'getSelection').mockReturnValue(null)
    fireEvent.mouseUp(passage)
    expect(
      screen.queryByRole('button', { name: 'Revisar' }),
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
      code: 'artifact_frozen',
      message: 'frozen',
    })
    await userEvent.type(screen.getByLabelText('Mensagem para o agente'), 'x')
    await userEvent.click(screen.getByRole('button', { name: 'Enviar' }))
    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByLabelText('Mensagem para o agente')).toHaveValue('x')
  })

  it('keeps frozen artifacts read-only', async () => {
    setup([artifact('c', 'Plano', ['Usar Postgres.'], { frozen: true })])
    const passage = await screen.findByText('Usar Postgres.')
    expect(screen.getByText(/Congelado/)).toBeInTheDocument()
    expect(screen.getByText('Nenhuma revisão.')).toBeInTheDocument()
    act(() => {
      select(passage)
    })
    expect(
      screen.queryByRole('button', { name: 'Revisar' }),
    ).not.toBeInTheDocument()
    expect(
      screen.queryByLabelText('Mensagem para o agente'),
    ).not.toBeInTheDocument()
  })
})
