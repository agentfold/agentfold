import { act, fireEvent, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import {
  artifact,
  card,
  cardType,
  createFakeBackend,
  lane,
} from '../test/fakeBackend'
import { renderWithApi } from '../test/render'
import { ReviewPage } from './ReviewPage'

function setup() {
  const review = lane('Revisar plano', 0)
  const task = cardType('task')
  const c = card('Chat', review.id, task.id, 0)
  const backend = createFakeBackend({
    lanes: [review],
    cardTypes: [task],
    cards: [c],
    artifacts: [artifact(c.id, 'Plano', ['# Plano'])],
  })
  renderWithApi(<ReviewPage cardId={c.id} />, backend)
  return { backend, card: c }
}

describe('ReviewPage', () => {
  afterEach(() => {
    window.location.hash = ''
  })

  it('shows the artifacts of the card next to their review', async () => {
    const { card: c } = setup()
    expect(
      await screen.findByRole('heading', { name: 'Revisão · Chat' }),
    ).toBeInTheDocument()
    expect(await screen.findByText('Revisar plano')).toBeInTheDocument()
    expect(
      await screen.findByRole('heading', { name: 'Plano' }),
    ).toBeInTheDocument()
    expect(screen.getByRole('region', { name: 'Revisão' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Abrir card' })).toHaveAttribute(
      'href',
      `#/cards/${c.id}`,
    )
  })

  it('goes back to the board on Escape outside a field', async () => {
    setup()
    const message = await screen.findByLabelText('Mensagem para o agente')
    for (const target of [
      message,
      document.createElement('input'),
      document.createElement('select'),
    ]) {
      document.body.append(target)
      fireEvent.keyDown(target, { key: 'Escape' })
      expect(window.location.hash).toBe('')
    }
    fireEvent.keyDown(document.body, { key: 'Enter' })
    expect(window.location.hash).toBe('')
    act(() => {
      fireEvent.keyDown(document.body, { key: 'Escape' })
    })
    expect(window.location.hash).toBe('#/board')
  })

  it('says when the card is gone', async () => {
    const backend = createFakeBackend()
    renderWithApi(<ReviewPage cardId="gone" />, backend)
    expect(await screen.findByText(/Card não encontrado/)).toBeInTheDocument()
  })

  it('shows load failures', async () => {
    const backend = createFakeBackend()
    backend.failNext('GET /cards', 500)
    renderWithApi(<ReviewPage cardId="c" />, backend)
    expect(await screen.findByRole('alert')).toBeInTheDocument()
  })
})
