import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'

import type { Card, CardType, Project } from '../api/client'
import { ColorDot } from '../cardTypes/ColorDot'

interface Props {
  card: Card
  cardType?: CardType
  project?: Project
  parent?: Card
  onOpen: () => void
}

export function CardTile({ card, cardType, project, parent, onOpen }: Props) {
  const {
    attributes,
    listeners,
    setNodeRef,
    setActivatorNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: card.id })

  // Pointer drags work anywhere on the card; keyboard drags only start from
  // the handle (the activator), so Enter/Space on the card still open it.
  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className="flex items-start gap-1 rounded-lg border border-slate-200 bg-white p-3 shadow-xs data-[dragging=true]:opacity-50"
      data-dragging={isDragging}
      {...listeners}
    >
      <button
        type="button"
        className="min-w-0 flex-1 cursor-pointer text-left"
        onClick={onOpen}
      >
        <span className="block text-sm font-medium text-slate-900">
          {card.title}
        </span>
        {parent && (
          <span className="mt-1 block truncate text-xs text-slate-500">
            ↳ {parent.title}
          </span>
        )}
        <span className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-slate-600">
          {cardType && (
            <span className="inline-flex items-center gap-1.5">
              <ColorDot color={cardType.color} />
              {cardType.name}
            </span>
          )}
          {project && (
            <span className="font-mono text-slate-500">{project.name}</span>
          )}
        </span>
      </button>
      <button
        type="button"
        className="cursor-grab rounded px-1 text-slate-400 hover:bg-slate-100 hover:text-slate-600"
        ref={setActivatorNodeRef}
        {...attributes}
        aria-label={`Arrastar ${card.title}`}
      >
        ⠿
      </button>
    </li>
  )
}
