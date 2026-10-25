import { screen, waitFor, within } from '@testing-library/react'
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
import { CardForm } from './CardForm'

function data() {
  const todo = lane('Todo', 0)
  const task = cardType('task')
  const bug = cardType('bug')
  const web = project('web')
  const parent = card('Parent', todo.id, task.id, 0)
  const child = card('Child', todo.id, task.id, 1, {
    parent_card_id: parent.id,
    project_id: web.id,
  })
  return { todo, task, bug, web, parent, child }
}

function setup(props: Parameters<typeof CardForm>[0], d = data()) {
  const backend = createFakeBackend({
    lanes: [d.todo],
    cardTypes: [d.task, d.bug],
    projects: [d.web],
    cards: [d.parent, d.child],
  })
  return { ...renderWithApi(<CardForm {...props} />, backend), backend, d }
}

/** The dialog, once the card types are loaded and the form shows. */
async function dialog(name: string) {
  const form = await screen.findByRole('dialog', { name })
  await within(form).findByLabelText('Título')
  return form
}

describe('CardForm', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('asks for a card type before creating cards', async () => {
    const onClose = vi.fn()
    renderWithApi(
      <CardForm laneId="L" onClose={onClose} />,
      createFakeBackend(),
    )
    const link = await screen.findByRole('link', { name: 'tipo de card' })
    expect(link).toHaveAttribute('href', '#/card-types')
    await userEvent.click(link)
    expect(onClose).toHaveBeenCalled()
  })

  it('shows card type load failures', async () => {
    const backend = createFakeBackend()
    backend.failNext('GET /card-types', 500)
    renderWithApi(<CardForm laneId="L" onClose={vi.fn()} />, backend)
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Erro no servidor.',
    )
  })

  it('offers no projects when they cannot be loaded', async () => {
    const backend = createFakeBackend({ cardTypes: [cardType('task')] })
    backend.failNext('GET /projects', 500)
    renderWithApi(<CardForm laneId="L" onClose={vi.fn()} />, backend)
    const form = await dialog('Novo card')
    await waitFor(() => {
      expect(backend.requests).toContain('GET /projects')
    })
    expect(
      within(within(form).getByLabelText('Projeto')).getAllByRole('option'),
    ).toHaveLength(1)
  })

  it('creates a card with defaults', async () => {
    const d = data()
    const onClose = vi.fn()
    const { backend } = setup({ laneId: d.todo.id, onClose }, d)
    const form = await dialog('Novo card')
    await userEvent.type(within(form).getByLabelText('Título'), 'Minimal')
    await userEvent.click(within(form).getByRole('button', { name: 'Salvar' }))

    await waitFor(() => {
      expect(onClose).toHaveBeenCalled()
    })
    expect(backend.state.cards.at(-1)).toMatchObject({
      title: 'Minimal',
      card_type_id: d.task.id,
      project_id: null,
      parent_card_id: null,
      lane_id: d.todo.id,
    })
  })

  it('creates a card with every field', async () => {
    const d = data()
    const onClose = vi.fn()
    const { backend } = setup({ laneId: d.todo.id, onClose }, d)
    const form = await dialog('Novo card')
    await userEvent.type(within(form).getByLabelText('Título'), 'Full')
    await userEvent.type(within(form).getByLabelText('Descrição'), 'Details')
    await userEvent.selectOptions(within(form).getByLabelText('Tipo'), 'bug')
    await userEvent.selectOptions(within(form).getByLabelText('Projeto'), 'web')
    await userEvent.selectOptions(
      within(form).getByLabelText('Card pai'),
      'Parent',
    )
    await userEvent.click(within(form).getByRole('button', { name: 'Salvar' }))

    await waitFor(() => {
      expect(onClose).toHaveBeenCalled()
    })
    expect(backend.state.cards.at(-1)).toMatchObject({
      title: 'Full',
      description: 'Details',
      card_type_id: d.bug.id,
      project_id: d.web.id,
      parent_card_id: d.parent.id,
    })
  })

  it('shows validation errors', async () => {
    const d = data()
    const onClose = vi.fn()
    setup({ laneId: d.todo.id, onClose }, d)
    const form = await dialog('Novo card')
    await userEvent.click(within(form).getByRole('button', { name: 'Salvar' }))
    expect(await within(form).findByRole('alert')).toHaveTextContent(
      'title: is required',
    )
    expect(onClose).not.toHaveBeenCalled()
  })

  it('edits a card and shows its family', async () => {
    const d = data()
    const onClose = vi.fn()
    const { backend } = setup({ card: d.child, onClose }, d)
    const form = await dialog('Editar card')
    expect(await within(form).findByText('Parent')).toBeInTheDocument()
    expect(within(form).queryByLabelText('Card pai')).not.toBeInTheDocument()
    expect(within(form).getByLabelText('Projeto')).toHaveValue(d.web.id)

    const title = within(form).getByLabelText('Título')
    await userEvent.clear(title)
    await userEvent.type(title, 'Renamed')
    await userEvent.selectOptions(
      within(form).getByLabelText('Projeto'),
      'Sem projeto',
    )
    await userEvent.click(within(form).getByRole('button', { name: 'Salvar' }))
    await waitFor(() => {
      expect(onClose).toHaveBeenCalled()
    })
    expect(backend.state.cards[1]).toMatchObject({
      title: 'Renamed',
      project_id: null,
    })
  })

  it('lists the children of a parent card', async () => {
    const d = data()
    setup({ card: d.parent, onClose: vi.fn() }, d)
    const form = await dialog('Editar card')
    expect(await within(form).findByText('Cards filhos:')).toBeInTheDocument()
    expect(within(form).getByRole('listitem')).toHaveTextContent('Child')
  })

  it('deletes a card after confirmation', async () => {
    const d = data()
    const onClose = vi.fn()
    const { backend } = setup({ card: d.parent, onClose }, d)
    const form = await dialog('Editar card')
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    await userEvent.click(within(form).getByRole('button', { name: 'Excluir' }))
    expect(backend.state.cards).toHaveLength(2)

    confirm.mockReturnValue(true)
    backend.failNext('DELETE /cards', 409, {
      code: 'in_use',
      message: 'in use',
    })
    await userEvent.click(within(form).getByRole('button', { name: 'Excluir' }))
    expect(await within(form).findByRole('alert')).toHaveTextContent(
      'ainda está em uso',
    )

    await userEvent.click(within(form).getByRole('button', { name: 'Excluir' }))
    await waitFor(() => {
      expect(onClose).toHaveBeenCalled()
    })
    expect(backend.state.cards.map((c) => c.title)).toEqual(['Child'])
  })
})
