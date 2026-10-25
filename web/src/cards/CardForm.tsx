import { useState, type ReactNode } from 'react'

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

type Props = {
  onClose: () => void
  /** Called after saving; onClose by default. */
  onSaved?: () => void
  /** Renders in the page (the card page) instead of a dialog. */
  inline?: boolean
} & ({ laneId: string; card?: undefined } | { card: Card; laneId?: undefined })

/**
 * Creates a card in a lane, or edits (and deletes) an existing one. onClose
 * runs on cancel and after deleting.
 */
export function CardForm({
  card,
  laneId,
  onClose,
  onSaved = onClose,
  inline = false,
}: Props) {
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
  const frame = (content: ReactNode) =>
    inline ? (
      <section aria-label={heading}>{content}</section>
    ) : (
      <Modal title={heading} onClose={onClose}>
        {content}
      </Modal>
    )
  if (cardTypes.isPending || cardTypes.error) {
    return frame(
      <QueryStatus isPending={cardTypes.isPending} error={cardTypes.error} />,
    )
  }
  if (types.length === 0) {
    return frame(
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
      </p>,
    )
  }

  return frame(
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
        const done = { onSuccess: onSaved }
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
      <TextField
        label="Título"
        value={title}
        onChange={setTitle}
        autoFocus={!inline}
      />
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
          Card pai:{' '}
          <a
            className="font-medium text-indigo-600 hover:underline"
            href={pageHref('cards', parent.id)}
          >
            {parent.title}
          </a>
        </p>
      )}
      {children.length > 0 && (
        <div className="text-sm text-slate-600">
          <p>Cards filhos:</p>
          <ul className="list-inside list-disc">
            {children.map((c) => (
              <li key={c.id}>
                <a
                  className="text-indigo-600 hover:underline"
                  href={pageHref('cards', c.id)}
                >
                  {c.title}
                </a>
              </li>
            ))}
          </ul>
        </div>
      )}
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
        {!inline && <Button onClick={onClose}>Cancelar</Button>}
        <Button type="submit" variant="primary" disabled={save.isPending}>
          Salvar
        </Button>
      </div>
    </form>,
  )
}
