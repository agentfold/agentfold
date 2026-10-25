import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { cardType, createFakeBackend } from '../test/fakeBackend'
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
