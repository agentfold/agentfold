import { act, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import App from './App'
import { createFakeBackend } from './test/fakeBackend'
import { renderWithApi } from './test/render'

function goTo(hash: string) {
  act(() => {
    window.location.hash = hash
    window.dispatchEvent(new HashChangeEvent('hashchange'))
  })
}

describe('App', () => {
  afterEach(() => {
    goTo('')
  })

  it('switches pages with the URL hash', async () => {
    renderWithApi(<App />, createFakeBackend())
    expect(
      screen.getByRole('heading', { level: 1, name: 'Orquestrator' }),
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Board' })).toHaveAttribute(
      'aria-current',
      'page',
    )
    expect(
      await screen.findByText('O board ainda não tem baias.'),
    ).toBeInTheDocument()

    goTo('#/projects')
    expect(
      screen.getByRole('heading', { name: 'Projetos' }),
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Projetos' })).toHaveAttribute(
      'aria-current',
      'page',
    )

    goTo('#/card-types')
    expect(
      screen.getByRole('heading', { name: 'Tipos de card' }),
    ).toBeInTheDocument()

    goTo('#/agents')
    expect(
      screen.getByRole('heading', { name: 'Perfis de agente' }),
    ).toBeInTheDocument()

    goTo('#/flows')
    expect(screen.getByRole('heading', { name: 'Fluxos' })).toBeInTheDocument()

    goTo('#/flows/some-id')
    expect(await screen.findByRole('alert')).toBeInTheDocument()

    goTo('#/cards/some-id')
    expect(await screen.findByText(/Card não encontrado/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Board' })).toHaveAttribute(
      'aria-current',
      'page',
    )

    goTo('#/review/some-id')
    expect(await screen.findByText(/Card não encontrado/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Board' })).toHaveAttribute(
      'aria-current',
      'page',
    )

    goTo('#/review')
    expect(
      await screen.findByText('O board ainda não tem baias.'),
    ).toBeInTheDocument()

    goTo('#/cards')
    expect(
      await screen.findByText('O board ainda não tem baias.'),
    ).toBeInTheDocument()

    goTo('#/unknown')
    expect(screen.getByRole('link', { name: 'Board' })).toHaveAttribute(
      'aria-current',
      'page',
    )
  })

  it('listens to board events', () => {
    const { events } = renderWithApi(<App />, createFakeBackend())
    expect(events.listeners.has('card.created')).toBe(true)
  })
})
