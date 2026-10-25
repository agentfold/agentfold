import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  card,
  cardType,
  createFakeBackend,
  lane,
  project,
} from '../test/fakeBackend'
import { renderWithApi } from '../test/render'
import { Board } from './Board'

function boardData() {
  const todo = lane('Todo', 0, { color: '#3b82f6' })
  const done = lane('Done', 1)
  const bug = cardType('bug', { color: '#ef4444' })
  const web = project('web')
  const parent = card('Parent', todo.id, bug.id, 0, { project_id: web.id })
  const child = card('Child', todo.id, bug.id, 1, { parent_card_id: parent.id })
  const orphan = card('Orphan', done.id, 'missing-type', 0, {
    parent_card_id: 'missing-card',
  })
  return { todo, done, bug, web, parent, child, orphan }
}

function setup() {
  const d = boardData()
  const backend = createFakeBackend({
    lanes: [d.todo, d.done],
    cardTypes: [d.bug],
    projects: [d.web],
    cards: [d.parent, d.child, d.orphan],
  })
  const view = renderWithApi(<Board />, backend)
  return { ...d, backend, ...view }
}

const laneSection = (name: string) => screen.findByRole('region', { name })

describe('Board', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows lanes with their cards', async () => {
    setup()
    const todo = await laneSection('Todo')
    const tiles = within(todo).getAllByRole('listitem')
    expect(tiles.map((t) => t.textContent)).toEqual([
      'Parentbugweb⠿',
      'Child↳ Parentbug⠿',
    ])
    expect(
      within(await laneSection('Done')).getByText('Orphan'),
    ).toBeInTheDocument()
  })

  it('shows load failures', async () => {
    const backend = createFakeBackend()
    backend.failNext('GET /cards', 500)
    renderWithApi(<Board />, backend)
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Erro no servidor.',
    )
  })

  it('creates and edits lanes', async () => {
    const { backend } = setup()
    await userEvent.click(
      await screen.findByRole('button', { name: '+ Nova baia' }),
    )
    let dialog = screen.getByRole('dialog', { name: 'Nova baia' })
    await userEvent.type(within(dialog).getByLabelText('Nome'), 'Review')
    await userEvent.click(
      within(dialog).getByRole('button', { name: 'Salvar' }),
    )
    expect(await laneSection('Review')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Editar Review' }))
    dialog = screen.getByRole('dialog', { name: 'Editar baia' })
    expect(within(dialog).getByLabelText('Nome')).toHaveValue('Review')
    await userEvent.click(
      within(dialog).getByRole('button', { name: '#22c55e' }),
    )
    await userEvent.click(
      within(dialog).getByRole('button', { name: 'Salvar' }),
    )
    await waitFor(() => {
      expect(backend.state.lanes[2].color).toBe('#22c55e')
    })

    await userEvent.click(screen.getByRole('button', { name: 'Editar Review' }))
    await userEvent.click(screen.getByRole('button', { name: 'Cancelar' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('reorders lanes', async () => {
    const { backend } = setup()
    expect(
      await screen.findByRole('button', { name: 'Mover Todo para a esquerda' }),
    ).toBeDisabled()
    expect(
      screen.getByRole('button', { name: 'Mover Done para a direita' }),
    ).toBeDisabled()

    await userEvent.click(
      screen.getByRole('button', { name: 'Mover Todo para a direita' }),
    )
    await waitFor(() => {
      expect(
        screen.getAllByRole('region').map((r) => r.getAttribute('aria-label')),
      ).toEqual(['Done', 'Todo'])
    })
    await userEvent.click(
      screen.getByRole('button', { name: 'Mover Todo para a esquerda' }),
    )
    await waitFor(() => {
      expect(backend.state.lanes.find((l) => l.name === 'Todo')?.position).toBe(
        0,
      )
    })
  })

  it('deletes lanes after confirmation', async () => {
    const { backend } = setup()
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    await userEvent.click(
      await screen.findByRole('button', { name: 'Excluir Todo' }),
    )
    expect(backend.requests).not.toContain(expect.stringMatching(/^DELETE/))

    confirm.mockReturnValue(true)
    await userEvent.click(screen.getByRole('button', { name: 'Excluir Todo' }))
    expect(
      await within(await laneSection('Todo')).findByRole('alert'),
    ).toHaveTextContent('ainda está em uso')

    await userEvent.click(screen.getByRole('button', { name: '+ Nova baia' }))
    await userEvent.type(screen.getByLabelText('Nome'), 'Empty')
    await userEvent.click(screen.getByRole('button', { name: 'Salvar' }))
    await userEvent.click(
      await screen.findByRole('button', { name: 'Excluir Empty' }),
    )
    await waitFor(() => {
      expect(
        screen.queryByRole('region', { name: 'Empty' }),
      ).not.toBeInTheDocument()
    })
  })

  it('adds a card to a lane', async () => {
    const { backend, done } = setup()
    const lane = await laneSection('Done')
    await userEvent.click(
      within(lane).getByRole('button', { name: '+ Adicionar card' }),
    )
    const dialog = screen.getByRole('dialog', { name: 'Novo card' })
    await userEvent.type(within(dialog).getByLabelText('Título'), 'New')
    await userEvent.click(
      within(dialog).getByRole('button', { name: 'Salvar' }),
    )
    expect(await within(lane).findByText('New')).toBeInTheDocument()
    expect(backend.state.cards.at(-1)).toMatchObject({
      title: 'New',
      lane_id: done.id,
    })
  })

  it('opens a card to edit it', async () => {
    setup()
    await userEvent.click(await screen.findByRole('button', { name: /^Child/ }))
    expect(
      screen.getByRole('dialog', { name: 'Editar card' }),
    ).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Cancelar' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('moves a card with the keyboard', async () => {
    const { backend } = setup()
    const handle = await screen.findByRole('button', {
      name: 'Arrastar Parent',
    })
    act(() => {
      handle.focus()
    })
    // The keyboard sensor starts listening one tick after the drag starts.
    const tick = () =>
      act(() => new Promise((resolve) => setTimeout(resolve, 0)))
    fireEvent.keyDown(handle, { code: 'Space' })
    await tick()
    expect(handle.closest('li')).toHaveAttribute('data-dragging', 'true')
    fireEvent.keyDown(document, { code: 'ArrowDown' })
    await tick()
    fireEvent.keyDown(document, { code: 'Space' })
    await tick()
    await waitFor(() => {
      expect(screen.getAllByRole('listitem')).toHaveLength(3)
    })
    expect(backend.requests.length).toBeGreaterThan(0)
  })
})
