import { useState } from 'react'

import type { Question } from '../api/client'
import { useAnswerQuestion, useCardQuestions } from '../api/queries'
import { Button } from '../components/Button'
import { ErrorMessage } from '../components/ErrorMessage'
import { TextArea } from '../components/Field'

/**
 * The questions agents asked on a card and still wait on, each with a field
 * to answer it. Nothing shows when there are none.
 */
export function CardQuestions({ cardId }: { cardId: string }) {
  const questions = useCardQuestions(cardId)
  const open = (questions.data ?? []).filter((q) => !q.delivered)
  if (questions.error) {
    return <ErrorMessage error={questions.error} />
  }
  if (open.length === 0) {
    return null
  }
  return (
    <section
      aria-label="Perguntas do agente"
      className="flex flex-col gap-3 rounded-lg border border-amber-200 bg-amber-50 p-3"
    >
      <h3 className="text-sm font-semibold text-amber-900">
        O agente espera suas respostas
      </h3>
      {open.map((q) => (
        <QuestionItem key={q.id} question={q} />
      ))}
    </section>
  )
}

function QuestionItem({ question }: { question: Question }) {
  const answer = useAnswerQuestion()
  const [text, setText] = useState(question.answer ?? '')
  return (
    <div className="flex flex-col gap-2">
      <TextArea
        label={question.text}
        value={text}
        onChange={setText}
        rows={2}
        hint={
          question.answer === null
            ? undefined
            : 'Respondida; o agente retoma quando todas forem respondidas.'
        }
      />
      <ErrorMessage error={answer.error} />
      <Button
        variant="primary"
        className="self-end"
        disabled={answer.isPending || text.trim() === ''}
        onClick={() => {
          answer.mutate({ id: question.id, answer: text })
        }}
      >
        {question.answer === null ? 'Responder' : 'Atualizar resposta'}
      </Button>
    </div>
  )
}
