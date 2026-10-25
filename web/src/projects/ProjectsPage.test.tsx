import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { createFakeBackend, project } from '../test/fakeBackend'
import { renderWithApi } from '../test/render'
import { ProjectsPage } from './ProjectsPage'

describe('ProjectsPage', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows the empty state', async () => {
    renderWithApi(<ProjectsPage />, createFakeBackend())
    expect(
      await screen.findByText('Nenhum projeto cadastrado.'),
    ).toBeInTheDocument()
  })

  it('shows load failures', async () => {
    const backend = createFakeBackend()
    backend.failNext('GET /projects', 500)
    renderWithApi(<ProjectsPage />, backend)
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Erro no servidor.',
    )
  })

  it('creates a project', async () => {
    const backend = createFakeBackend()
    renderWithApi(<ProjectsPage />, backend)
    await userEvent.click(screen.getByRole('button', { name: 'Novo projeto' }))
    const dialog = screen.getByRole('dialog', { name: 'Novo projeto' })

    await userEvent.click(
      within(dialog).getByRole('button', { name: 'Salvar' }),
    )
    expect(await within(dialog).findByRole('alert')).toHaveTextContent(
      'name: is required',
    )

    await userEvent.type(within(dialog).getByLabelText('Nome'), 'web')
    await userEvent.type(
      within(dialog).getByLabelText('Pasta local'),
      '/src/web',
    )
    await userEvent.click(
      within(dialog).getByRole('button', { name: 'Salvar' }),
    )

    expect(await screen.findByText('/src/web')).toBeInTheDocument()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(backend.state.projects[0]).toMatchObject({
      name: 'web',
      local_path: '/src/web',
    })
  })

  it('edits a project', async () => {
    const backend = createFakeBackend({ projects: [project('web')] })
    renderWithApi(<ProjectsPage />, backend)
    await userEvent.click(
      await screen.findByRole('button', { name: 'Editar web' }),
    )
    const dialog = screen.getByRole('dialog', { name: 'Editar projeto' })
    const name = within(dialog).getByLabelText('Nome')
    expect(within(dialog).getByLabelText('Pasta local')).toHaveValue('/src/web')

    await userEvent.clear(name)
    await userEvent.type(name, 'site')
    await userEvent.click(
      within(dialog).getByRole('button', { name: 'Salvar' }),
    )
    expect(await screen.findByText('site')).toBeInTheDocument()
  })

  it('cancels the form', async () => {
    renderWithApi(<ProjectsPage />, createFakeBackend())
    await userEvent.click(screen.getByRole('button', { name: 'Novo projeto' }))
    await userEvent.click(screen.getByRole('button', { name: 'Cancelar' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('deletes a project after confirmation', async () => {
    const backend = createFakeBackend({ projects: [project('web')] })
    renderWithApi(<ProjectsPage />, backend)
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)

    await userEvent.click(
      await screen.findByRole('button', { name: 'Excluir web' }),
    )
    expect(confirm).toHaveBeenCalledWith('Excluir o projeto web?')
    expect(backend.state.projects).toHaveLength(1)

    confirm.mockReturnValue(true)
    backend.failNext('DELETE /projects', 409, {
      code: 'in_use',
      message: 'in use',
    })
    await userEvent.click(screen.getByRole('button', { name: 'Excluir web' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'ainda está em uso',
    )

    await userEvent.click(screen.getByRole('button', { name: 'Excluir web' }))
    await waitFor(() => {
      expect(screen.getByText('Nenhum projeto cadastrado.')).toBeInTheDocument()
    })
  })
})
