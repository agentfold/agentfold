import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'

import { createFakeBackend, question } from '../test/fakeBackend'
import { renderWithApi } from '../test/render'
import { CardQuestions } from './CardQuestions'

describe('CardQuestions', () => {
  it('answers the open questions of the card', async () => {
    const open = question('c1', 'Which database?')
    const answered = question('c1', 'Which port?', { answer: '5432' })
    const backend = createFakeBackend({
      questions: [
        open,
        answered,
        question('c1', 'Old?', { answer: 'x', delivered: true }),
        question('c2', 'Other card?'),
      ],
    })
    renderWithApi(<CardQuestions cardId="c1" />, backend)
    const section = await screen.findByRole('region', {
      name: 'Perguntas do agente',
    })
    expect(within(section).queryByText('Old?')).not.toBeInTheDocument()
    expect(within(section).queryByText('Other card?')).not.toBeInTheDocument()
    expect(within(section).getByLabelText('Which port?')).toHaveValue('5432')
    expect(
      within(section).getByText(/o agente retoma quando todas/),
    ).toBeInTheDocument()

    const reply = within(section).getAllByRole('button', { name: 'Responder' })
    expect(reply).toHaveLength(1)
    expect(reply[0]).toBeDisabled()
    await userEvent.type(
      within(section).getByLabelText('Which database?'),
      'Postgres',
    )
    await userEvent.click(reply[0])
    await waitFor(() => {
      expect(backend.state.questions[0].answer).toBe('Postgres')
    })
    expect(
      await within(section).findAllByRole('button', {
        name: 'Atualizar resposta',
      }),
    ).toHaveLength(2)
  })

  it('shows answers that can no longer change', async () => {
    const backend = createFakeBackend({ questions: [question('c1', 'DB?')] })
    renderWithApi(<CardQuestions cardId="c1" />, backend)
    const field = await screen.findByLabelText('DB?')
    await userEvent.type(field, 'x')
    backend.failNext('POST /questions', 409, {
      code: 'question_closed',
      message: 'closed',
    })
    await userEvent.click(screen.getByRole('button', { name: 'Responder' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Essa resposta já foi entregue ao agente e não pode mudar.',
    )
  })

  it('shows nothing without open questions, and load failures', async () => {
    const backend = createFakeBackend()
    const { container } = renderWithApi(<CardQuestions cardId="c1" />, backend)
    await waitFor(() => {
      expect(backend.requests).toContain('GET /cards/c1/questions')
    })
    expect(container).toBeEmptyDOMElement()

    const failing = createFakeBackend()
    failing.failNext('GET /cards/c2/questions', 500)
    renderWithApi(<CardQuestions cardId="c2" />, failing)
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Erro no servidor.',
    )
  })
})
