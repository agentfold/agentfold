import { useDndContext, useDroppable } from '@dnd-kit/core'
import { SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable'

import type { CSSProperties } from 'react'

import type { Card, Lane } from '../api/client'
import { useDeleteLane, useMoveLane } from '../api/queries'
import { Button } from '../components/Button'
import { ErrorMessage } from '../components/ErrorMessage'
import type { BoardLookup } from './CardContent'
import { CardTile } from './CardTile'
import { isLaneTarget, laneDropId } from './drop'

interface Props {
  lane: Lane
  isFirst: boolean
  isLast: boolean
  cards: Card[]
  lookup: BoardLookup
  onEdit: () => void
  onAddCard: () => void
  onOpenCard: (card: Card) => void
}

export function LaneColumn({
  lane,
  isFirst,
  isLast,
  cards,
  lookup,
  onEdit,
  onAddCard,
  onOpenCard,
}: Props) {
  const { setNodeRef } = useDroppable({ id: laneDropId(lane.id) })
  const { over } = useDndContext()
  const move = useMoveLane()
  const remove = useDeleteLane()

  return (
    <section
      aria-label={lane.name}
      // Highlighted (lane color on top, darker background) under the mouse
      // and while a card is dragged over it; faint gray otherwise.
      className="group flex w-72 shrink-0 flex-col rounded-xl bg-slate-200/60 transition-colors hover:bg-slate-200 data-[target=true]:bg-slate-200"
      data-target={isLaneTarget(over, lane.id, cards)}
      style={{ '--lane-color': lane.color || '#64748b' } as CSSProperties}
    >
      <header className="flex items-center gap-1 rounded-t-xl border-t-4 border-t-slate-300 px-3 pt-2 pb-1 transition-colors group-hover:border-t-(--lane-color) group-data-[target=true]:border-t-(--lane-color)">
        <h3 className="flex-1 truncate text-sm font-semibold">{lane.name}</h3>
        <span className="text-xs text-slate-500">{cards.length}</span>
        {/* Lane actions show on hover or keyboard focus, leaving room for the name. */}
        <div className="hidden items-center group-focus-within:flex group-hover:flex">
          <Button
            variant="ghost"
            className="px-1.5"
            aria-label={`Mover ${lane.name} para a esquerda`}
            disabled={isFirst}
            onClick={() => {
              move.mutate({ id: lane.id, position: lane.position - 1 })
            }}
          >
            ←
          </Button>
          <Button
            variant="ghost"
            className="px-1.5"
            aria-label={`Mover ${lane.name} para a direita`}
            disabled={isLast}
            onClick={() => {
              move.mutate({ id: lane.id, position: lane.position + 1 })
            }}
          >
            →
          </Button>
          <Button
            variant="ghost"
            className="px-1.5"
            aria-label={`Editar ${lane.name}`}
            onClick={onEdit}
          >
            ✎
          </Button>
          <Button
            variant="ghost"
            className="px-1.5"
            aria-label={`Excluir ${lane.name}`}
            onClick={() => {
              if (window.confirm(`Excluir a baia ${lane.name}?`)) {
                remove.mutate(lane.id)
              }
            }}
          >
            ✕
          </Button>
        </div>
      </header>
      <div className="px-2">
        <ErrorMessage error={move.error ?? remove.error} />
      </div>
      <SortableContext
        items={cards.map((c) => c.id)}
        strategy={verticalListSortingStrategy}
      >
        <ul
          ref={setNodeRef}
          className="flex min-h-12 flex-1 flex-col gap-2 overflow-y-auto p-2"
        >
          {cards.map((card) => (
            <CardTile
              key={card.id}
              card={card}
              lookup={lookup}
              onOpen={() => {
                onOpenCard(card)
              }}
            />
          ))}
        </ul>
      </SortableContext>
      <Button
        variant="ghost"
        className="m-2 mt-0 justify-start"
        onClick={onAddCard}
      >
        + Adicionar card
      </Button>
    </section>
  )
}
