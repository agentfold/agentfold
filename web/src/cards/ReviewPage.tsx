import { useEffect } from 'react'

import { useCardRuns, useCards, useLanes } from '../api/queries'
import { QueryStatus } from '../components/Loading'
import { goTo, pageHref } from '../route'
import { ApprovalPanel } from './ApprovalPanel'
import { ArtifactsPanel } from './ArtifactsPanel'

/**
 * The review of a card's artifacts on the whole screen: the artifact on the
 * left, its review chat on the right. When the latest run awaits the
 * approval of an output instead, that output and the answer to it. Escape
 * outside a field goes back to the board.
 */
export function ReviewPage({ cardId }: { cardId: string }) {
  const cards = useCards()
  const lanes = useLanes()
  const runs = useCardRuns(cardId)

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const typing =
        e.target instanceof HTMLTextAreaElement ||
        e.target instanceof HTMLInputElement ||
        e.target instanceof HTMLSelectElement
      if (e.key === 'Escape' && !typing) {
        goTo('board')
      }
    }
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('keydown', onKey)
    }
  }, [])

  if (!cards.data) {
    return <QueryStatus isPending error={cards.error} />
  }
  const card = cards.data.find((c) => c.id === cardId)
  if (!card) {
    return (
      <p className="text-sm text-slate-600">
        Card não encontrado.{' '}
        <a className="text-indigo-600 underline" href={pageHref('board')}>
          Voltar ao board
        </a>
      </p>
    )
  }
  const lane = lanes.data?.find((l) => l.id === card.lane_id)
  const approval = runs.data?.[0]?.nodes.find(
    (n) => n.approval && n.status === 'awaiting_review',
  )
  return (
    <div className="flex h-full min-h-[32rem] flex-col gap-3">
      <header className="flex flex-wrap items-baseline gap-x-4 gap-y-1">
        <a
          className="text-sm text-indigo-600 hover:underline"
          href={pageHref('board')}
        >
          ← Board
        </a>
        <h2 className="text-lg font-semibold">
          {approval ? 'Aprovação' : 'Revisão'} · {card.title}
        </h2>
        {lane && <span className="text-sm text-slate-500">{lane.name}</span>}
        <a
          className="ml-auto text-sm text-indigo-600 hover:underline"
          href={pageHref('cards', card.id)}
        >
          Abrir card
        </a>
      </header>
      {approval ? (
        <ApprovalPanel node={approval} className="flex-1" />
      ) : (
        <ArtifactsPanel cardId={card.id} className="flex-1" />
      )}
    </div>
  )
}
