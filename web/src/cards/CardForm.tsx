import { useState } from 'react'

import type { Card } from '../api/client'
import {
  useCards,
  useCardTypes,
  useCreateCard,
  useDeleteCard,
  useProjects,
  useUpdateCard,
} from '../api/queries'
import { Button } from '../components/Button'
import { ErrorMessage } from '../components/ErrorMessage'
import { QueryStatus } from '../components/Loading'
import { SelectField, TextArea, TextField } from '../components/Field'
import { Modal } from '../components/Modal'
import { pageHref } from '../route'
import { CardQuestions } from './CardQuestions'
import { CardTimeline } from './CardTimeline'

type Props = { onClose: () => void } & (
  { laneId: string; card?: undefined } | { card: Card; laneId?: undefined }
)

/** Creates a card in a lane, or edits (and deletes) an existing one. */
export function CardForm({ card, laneId, onClose }: Props) {
  const cardTypes = useCardTypes()
  const projects = useProjects()
  const cards = useCards()
  const create = useCreateCard()
  const update = useUpdateCard()
  const remove = useDeleteCard()

  const types = cardTypes.data ?? []
  const [title, setTitle] = useState(card?.title ?? '')
  const [description, setDescription] = useState(card?.description ?? '')
  const [typeId, setTypeId] = useState(card?.card_type_id ?? '')
  const [projectId, setProjectId] = useState(card?.project_id ?? '')
  const [parentId, setParentId] = useState('')
  const save = card ? update : create

  const allCards = cards.data ?? []
  const parent = card?.parent_card_id
    ? allCards.find((c) => c.id === card.parent_card_id)
    : undefined
  const children = card
    ? allCards.filter((c) => c.parent_card_id === card.id)
    : []

  const heading = card ? 'Editar card' : 'Novo card'
  if (cardTypes.isPending || cardTypes.error) {
    return (
      <Modal title={heading} onClose={onClose}>
        <QueryStatus isPending={cardTypes.isPending} error={cardTypes.error} />
      </Modal>
    )
  }
  if (types.length === 0) {
    return (
      <Modal title={heading} onClose={onClose}>
        <p className="text-sm text-slate-600">
          Cadastre um{' '}
          <a
            className="text-indigo-600 underline"
            href={pageHref('card-types')}
            onClick={onClose}
          >
            tipo de card
          </a>{' '}
          antes de criar cards.
        </p>
      </Modal>
    )
  }

  return (
    <Modal title={heading} onClose={onClose}>
      <form
        className="flex flex-col gap-3"
        onSubmit={(e) => {
          e.preventDefault()
          const fields = {
            title,
            description,
            card_type_id: typeId || types[0].id,
            project_id: projectId || null,
          }
          const done = { onSuccess: onClose }
          if (card) {
            update.mutate({ id: card.id, ...fields }, done)
          } else {
            create.mutate(
              { ...fields, lane_id: laneId, parent_card_id: parentId || null },
              done,
            )
          }
        }}
      >
        {card && <CardQuestions cardId={card.id} />}
        <TextField label="Título" value={title} onChange={setTitle} autoFocus />
        <TextArea
          label="Descrição"
          value={description}
          onChange={setDescription}
        />
        <div className="grid grid-cols-2 gap-3">
          <SelectField
            label="Tipo"
            value={typeId || types[0].id}
            onChange={setTypeId}
            options={types.map((t) => ({ value: t.id, label: t.name }))}
          />
          <SelectField
            label="Projeto"
            value={projectId}
            onChange={setProjectId}
            options={[
              { value: '', label: 'Sem projeto' },
              ...(projects.data ?? []).map((p) => ({
                value: p.id,
                label: p.name,
              })),
            ]}
          />
        </div>
        {!card && (
          <SelectField
            label="Card pai"
            value={parentId}
            onChange={setParentId}
            options={[
              { value: '', label: 'Nenhum' },
              ...allCards.map((c) => ({ value: c.id, label: c.title })),
            ]}
          />
        )}
        {parent && (
          <p className="text-sm text-slate-600">
            Card pai: <strong className="font-medium">{parent.title}</strong>
          </p>
        )}
        {children.length > 0 && (
          <div className="text-sm text-slate-600">
            <p>Cards filhos:</p>
            <ul className="list-inside list-disc">
              {children.map((c) => (
                <li key={c.id}>{c.title}</li>
              ))}
            </ul>
          </div>
        )}
        {card && <CardTimeline cardId={card.id} />}
        <ErrorMessage error={save.error ?? remove.error} />
        <div className="flex items-center gap-2">
          {card && (
            <Button
              variant="danger"
              onClick={() => {
                if (window.confirm(`Excluir o card ${card.title}?`)) {
                  remove.mutate(card.id, { onSuccess: onClose })
                }
              }}
            >
              Excluir
            </Button>
          )}
          <span className="flex-1" />
          <Button onClick={onClose}>Cancelar</Button>
          <Button type="submit" variant="primary" disabled={save.isPending}>
            Salvar
          </Button>
        </div>
      </form>
    </Modal>
  )
}
