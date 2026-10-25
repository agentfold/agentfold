import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { createFakeBackend, flow } from '../test/fakeBackend'
import { renderWithApi } from '../test/render'
import { FlowsPage } from './FlowsPage'

describe('FlowsPage', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    window.location.hash = ''
  })

  it('shows the empty state and load failures', async () => {
    renderWithApi(<FlowsPage />, createFakeBackend())
    expect(
      await screen.findByText('Nenhum fluxo cadastrado.'),
    ).toBeInTheDocument()

    const failing = createFakeBackend()
    failing.failNext('GET /flows', 500)
    renderWithApi(<FlowsPage />, failing)
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Erro no servidor.',
    )
  })

  it('lists flows with their state', async () => {
    const backend = createFakeBackend({
      flows: [
        flow('draft only'),
        flow('live', { latest_version: 3, enabled: true }),
      ],
    })
    renderWithApi(<FlowsPage />, backend)
    expect(
      await screen.findByRole('link', { name: 'draft only' }),
    ).toHaveAttribute('href', `#/flows/${backend.state.flows[0].id}`)
    expect(screen.getByText('Nunca publicado')).toBeInTheDocument()
    expect(screen.getByText('Versão 3 publicada')).toBeInTheDocument()

    const [draftToggle, liveToggle] = screen.getAllByRole('checkbox', {
      name: 'Ativo',
    })
    expect(draftToggle).toBeDisabled()
    expect(liveToggle).toBeChecked()

    await userEvent.click(liveToggle)
    await waitFor(() => {
      expect(backend.state.flows[1].enabled).toBe(false)
    })
  })

  it('creates a flow and opens it', async () => {
    const backend = createFakeBackend()
    renderWithApi(<FlowsPage />, backend)
    await userEvent.click(screen.getByRole('button', { name: 'Novo fluxo' }))
    const dialog = screen.getByRole('dialog', { name: 'Novo fluxo' })

    await userEvent.click(within(dialog).getByRole('button', { name: 'Criar' }))
    expect(await within(dialog).findByRole('alert')).toBeInTheDocument()

    await userEvent.type(within(dialog).getByLabelText('Nome'), 'Dev')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Criar' }))
    await waitFor(() => {
      expect(window.location.hash).toBe(`#/flows/${backend.state.flows[0].id}`)
    })
  })

  it('cancels the creation', async () => {
    renderWithApi(<FlowsPage />, createFakeBackend())
    await userEvent.click(screen.getByRole('button', { name: 'Novo fluxo' }))
    await userEvent.click(screen.getByRole('button', { name: 'Cancelar' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('deletes a flow after confirmation', async () => {
    const backend = createFakeBackend({ flows: [flow('Dev')] })
    renderWithApi(<FlowsPage />, backend)
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    await userEvent.click(
      await screen.findByRole('button', { name: 'Excluir Dev' }),
    )
    expect(backend.state.flows).toHaveLength(1)

    confirm.mockReturnValue(true)
    await userEvent.click(screen.getByRole('button', { name: 'Excluir Dev' }))
    expect(
      await screen.findByText('Nenhum fluxo cadastrado.'),
    ).toBeInTheDocument()
  })
})
