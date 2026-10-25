import { useDroppable } from '@dnd-kit/core'
import { SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable'

import type { Card, CardType, Lane, Project } from '../api/client'
import { useDeleteLane, useMoveLane } from '../api/queries'
import { Button } from '../components/Button'
import { ErrorMessage } from '../components/ErrorMessage'
import { CardTile } from './CardTile'
import { laneDropId } from './drop'

interface Props {
  lane: Lane
  isFirst: boolean
  isLast: boolean
  cards: Card[]
  cardsById: Map<string, Card>
  cardTypes: Map<string, CardType>
  projects: Map<string, Project>
  onEdit: () => void
  onAddCard: () => void
  onOpenCard: (card: Card) => void
}

export function LaneColumn({
  lane,
  isFirst,
  isLast,
  cards,
  cardsById,
  cardTypes,
  projects,
  onEdit,
  onAddCard,
  onOpenCard,
}: Props) {
  const { setNodeRef, isOver } = useDroppable({ id: laneDropId(lane.id) })
  const move = useMoveLane()
  const remove = useDeleteLane()

  return (
    <section
      aria-label={lane.name}
      className="group flex w-72 shrink-0 flex-col rounded-xl bg-slate-200/60 data-[over=true]:ring-2 data-[over=true]:ring-indigo-400"
      data-over={isOver}
    >
      <header
        className="flex items-center gap-1 rounded-t-xl border-t-4 px-3 pt-2 pb-1"
        style={{ borderTopColor: lane.color || 'transparent' }}
      >
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
              cardType={cardTypes.get(card.card_type_id)}
              project={
                card.project_id ? projects.get(card.project_id) : undefined
              }
              parent={
                card.parent_card_id
                  ? cardsById.get(card.parent_card_id)
                  : undefined
              }
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
