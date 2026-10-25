import {
  closestCorners,
  DndContext,
  DragOverlay,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from '@dnd-kit/core'
import { sortableKeyboardCoordinates } from '@dnd-kit/sortable'
import { useState } from 'react'

import type { Card, Lane } from '../api/client'
import { laneCards } from '../api/placement'
import {
  useCards,
  useCardTypes,
  useLanes,
  useMoveCard,
  useProjects,
} from '../api/queries'
import { CardForm } from '../cards/CardForm'
import { Button } from '../components/Button'
import { ErrorMessage } from '../components/ErrorMessage'
import { QueryStatus } from '../components/Loading'
import { CardPreview, type BoardLookup } from './CardContent'
import { dropCard } from './drop'
import { LaneColumn } from './LaneColumn'
import { LaneForm } from './LaneForm'

type Dialog =
  | { kind: 'lane'; lane?: Lane }
  | { kind: 'new-card'; laneId: string }
  | { kind: 'card'; card: Card }

const byId = <T extends { id: string }>(items: T[] = []) =>
  new Map(items.map((item) => [item.id, item]))

export function Board() {
  const lanes = useLanes()
  const cards = useCards()
  const cardTypes = useCardTypes()
  const projects = useProjects()
  const moveCard = useMoveCard()
  const [dialog, setDialog] = useState<Dialog | null>(null)
  const [draggedId, setDraggedId] = useState<string | null>(null)
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  )

  const queries = [lanes, cards, cardTypes, projects]
  const failed = queries.find((q) => q.error)
  if (failed || !lanes.data || !cards.data) {
    return <QueryStatus isPending error={failed?.error} />
  }

  const allCards = cards.data
  const lookup: BoardLookup = {
    cards: byId(allCards),
    cardTypes: byId(cardTypes.data),
    projects: byId(projects.data),
  }
  const dragged = draggedId === null ? undefined : lookup.cards.get(draggedId)
  const onDragStart = ({ active }: DragStartEvent) => {
    setDraggedId(String(active.id))
  }
  const onDragEnd = (event: DragEndEvent) => {
    setDraggedId(null)
    dropCard(allCards, event, moveCard.mutate)
  }
  const close = () => {
    setDialog(null)
  }

  return (
    <div className="flex h-full flex-col gap-3">
      <ErrorMessage error={moveCard.error} />
      <DndContext
        sensors={sensors}
        collisionDetection={closestCorners}
        onDragStart={onDragStart}
        onDragEnd={onDragEnd}
        onDragCancel={() => {
          setDraggedId(null)
        }}
      >
        <div className="flex min-h-0 flex-1 items-start gap-4 overflow-x-auto pb-2">
          {lanes.data.map((lane, i, all) => (
            <LaneColumn
              key={lane.id}
              lane={lane}
              isFirst={i === 0}
              isLast={i === all.length - 1}
              cards={laneCards(allCards, lane.id)}
              lookup={lookup}
              onEdit={() => {
                setDialog({ kind: 'lane', lane })
              }}
              onAddCard={() => {
                setDialog({ kind: 'new-card', laneId: lane.id })
              }}
              onOpenCard={(card) => {
                setDialog({ kind: 'card', card })
              }}
            />
          ))}
          <div className="flex w-72 shrink-0 flex-col gap-2">
            {lanes.data.length === 0 && (
              <p className="text-sm text-slate-500">
                O board ainda não tem baias.
              </p>
            )}
            <Button
              className="justify-start"
              onClick={() => {
                setDialog({ kind: 'lane' })
              }}
            >
              + Nova baia
            </Button>
          </div>
        </div>
        {/* Drawn above the lanes, so the card is not clipped by its lane. */}
        <DragOverlay>
          {dragged && <CardPreview card={dragged} lookup={lookup} />}
        </DragOverlay>
      </DndContext>

      {dialog?.kind === 'lane' && (
        <LaneForm lane={dialog.lane} onClose={close} />
      )}
      {dialog?.kind === 'new-card' && (
        <CardForm laneId={dialog.laneId} onClose={close} />
      )}
      {dialog?.kind === 'card' && (
        <CardForm card={dialog.card} onClose={close} />
      )}
    </div>
  )
}
