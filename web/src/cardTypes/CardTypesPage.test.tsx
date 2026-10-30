import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { cardType, createFakeBackend, lane } from '../test/fakeBackend'
import { renderWithApi } from '../test/render'
import { CardTypesPage } from './CardTypesPage'

describe('CardTypesPage', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows the empty state', async () => {
    renderWithApi(<CardTypesPage />, createFakeBackend())
    expect(
      await screen.findByText('Nenhum tipo cadastrado.'),
    ).toBeInTheDocument()
  })

  it('creates a card type with a color', async () => {
    const backend = createFakeBackend()
    renderWithApi(<CardTypesPage />, backend)
    await userEvent.click(screen.getByRole('button', { name: 'Novo tipo' }))
    const dialog = screen.getByRole('dialog', { name: 'Novo tipo de card' })

    await userEvent.click(
      within(dialog).getByRole('button', { name: 'Salvar' }),
    )
    expect(await within(dialog).findByRole('alert')).toBeInTheDocument()

    await userEvent.type(within(dialog).getByLabelText('Nome'), 'bug')
    await userEvent.click(
      within(dialog).getByRole('button', { name: '#ef4444' }),
    )
    await userEvent.click(
      within(dialog).getByRole('button', { name: 'Salvar' }),
    )

    expect(await screen.findByText('bug')).toBeInTheDocument()
    expect(backend.state.cardTypes[0]).toMatchObject({
      name: 'bug',
      color: '#ef4444',
    })
  })

  it('sets the lane tasks of the type start in', async () => {
    const todo = lane('Todo', 0)
    const backend = createFakeBackend({
      lanes: [todo],
      cardTypes: [cardType('task', { initial_lane_id: todo.id })],
    })
    renderWithApi(<CardTypesPage />, backend)
    expect(await screen.findByText('Tasks em Todo')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Editar task' }))
    const dialog = screen.getByRole('dialog', { name: 'Editar tipo de card' })
    const select = within(dialog).getByLabelText('Baia inicial das tasks')
    expect(select).toHaveValue(todo.id)
    await userEvent.selectOptions(select, 'Nenhuma')
    await userEvent.click(
      within(dialog).getByRole('button', { name: 'Salvar' }),
    )
    await waitFor(() => {
      expect(backend.state.cardTypes[0].initial_lane_id).toBeNull()
    })

    await userEvent.click(screen.getByRole('button', { name: 'Novo tipo' }))
    const create = screen.getByRole('dialog', { name: 'Novo tipo de card' })
    await userEvent.type(within(create).getByLabelText('Nome'), 'bug')
    await userEvent.selectOptions(
      within(create).getByLabelText('Baia inicial das tasks'),
      'Todo',
    )
    await userEvent.click(
      within(create).getByRole('button', { name: 'Salvar' }),
    )
    await waitFor(() => {
      expect(backend.state.cardTypes[1]).toMatchObject({
        name: 'bug',
        initial_lane_id: todo.id,
      })
    })
  })

  it('offers no lane while the lanes fail to load', async () => {
    const backend = createFakeBackend({ cardTypes: [cardType('task')] })
    backend.failNext('GET /lanes', 500, { code: 'internal', message: 'boom' })
    renderWithApi(<CardTypesPage />, backend)
    await userEvent.click(
      await screen.findByRole('button', { name: 'Editar task' }),
    )
    const select = screen.getByLabelText('Baia inicial das tasks')
    expect(within(select).getAllByRole('option')).toHaveLength(1)
  })

  it('edits a card type', async () => {
    const backend = createFakeBackend({
      cardTypes: [cardType('bug', { color: '#ef4444' })],
    })
    renderWithApi(<CardTypesPage />, backend)
    await userEvent.click(
      await screen.findByRole('button', { name: 'Editar bug' }),
    )
    const dialog = screen.getByRole('dialog', { name: 'Editar tipo de card' })
    expect(
      within(dialog).getByRole('button', { name: '#ef4444' }),
    ).toHaveAttribute('aria-pressed', 'true')

    await userEvent.click(
      within(dialog).getByRole('button', { name: 'Sem cor' }),
    )
    await userEvent.click(
      within(dialog).getByRole('button', { name: 'Salvar' }),
    )
    await waitFor(() => {
      expect(backend.state.cardTypes[0].color).toBe('')
    })
    await userEvent.click(screen.getByRole('button', { name: 'Editar bug' }))
    await userEvent.click(screen.getByRole('button', { name: 'Cancelar' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('deletes a card type after confirmation', async () => {
    const backend = createFakeBackend({ cardTypes: [cardType('bug')] })
    renderWithApi(<CardTypesPage />, backend)
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    await userEvent.click(
      await screen.findByRole('button', { name: 'Excluir bug' }),
    )
    expect(backend.state.cardTypes).toHaveLength(1)

    confirm.mockReturnValue(true)
    await userEvent.click(screen.getByRole('button', { name: 'Excluir bug' }))
    expect(
      await screen.findByText('Nenhum tipo cadastrado.'),
    ).toBeInTheDocument()
  })
})
