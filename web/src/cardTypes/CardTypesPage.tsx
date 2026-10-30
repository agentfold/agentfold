import { useState } from 'react'

import type { CardType, Lane } from '../api/client'
import {
  useCardTypes,
  useCreateCardType,
  useDeleteCardType,
  useLanes,
  useUpdateCardType,
} from '../api/queries'
import { Button } from '../components/Button'
import { ErrorMessage } from '../components/ErrorMessage'
import { ColorField, SelectField, TextField } from '../components/Field'
import { QueryStatus } from '../components/Loading'
import { Modal } from '../components/Modal'
import { ColorDot } from './ColorDot'

export function CardTypesPage() {
  const types = useCardTypes()
  const lanes = useLanes()
  const remove = useDeleteCardType()
  const [editing, setEditing] = useState<CardType | 'new' | null>(null)

  return (
    <section className="mx-auto flex max-w-3xl flex-col gap-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-base font-semibold">Tipos de card</h2>
          <p className="text-sm text-slate-500">
            Classificam os cards; os gatilhos dos fluxos podem filtrar por tipo.
            Agentes só criam tasks dos tipos com baia inicial.
          </p>
        </div>
        <Button
          variant="primary"
          onClick={() => {
            setEditing('new')
          }}
        >
          Novo tipo
        </Button>
      </div>

      <QueryStatus isPending={types.isPending} error={types.error} />
      <ErrorMessage error={remove.error} />

      {types.data?.length === 0 && (
        <p className="text-sm text-slate-500">Nenhum tipo cadastrado.</p>
      )}
      {types.data && types.data.length > 0 && (
        <ul className="divide-y divide-slate-200 rounded-lg border border-slate-200 bg-white">
          {types.data.map((t) => (
            <li key={t.id} className="flex items-center gap-3 px-4 py-3">
              <ColorDot color={t.color} />
              <p className="flex-1 text-sm font-medium">{t.name}</p>
              {t.initial_lane_id && (
                <span className="text-xs text-slate-500">
                  Tasks em{' '}
                  {lanes.data?.find((l) => l.id === t.initial_lane_id)?.name}
                </span>
              )}
              <Button
                variant="ghost"
                aria-label={`Editar ${t.name}`}
                onClick={() => {
                  setEditing(t)
                }}
              >
                Editar
              </Button>
              <Button
                variant="ghost"
                aria-label={`Excluir ${t.name}`}
                onClick={() => {
                  if (window.confirm(`Excluir o tipo ${t.name}?`)) {
                    remove.mutate(t.id)
                  }
                }}
              >
                Excluir
              </Button>
            </li>
          ))}
        </ul>
      )}

      {editing && (
        <CardTypeForm
          cardType={editing === 'new' ? undefined : editing}
          lanes={lanes.data ?? []}
          onClose={() => {
            setEditing(null)
          }}
        />
      )}
    </section>
  )
}

interface FormProps {
  cardType?: CardType
  lanes: Lane[]
  onClose: () => void
}

function CardTypeForm({ cardType, lanes, onClose }: FormProps) {
  const create = useCreateCardType()
  const update = useUpdateCardType()
  const [name, setName] = useState(cardType?.name ?? '')
  const [color, setColor] = useState(cardType?.color ?? '')
  const [lane, setLane] = useState(cardType?.initial_lane_id ?? '')
  const save = cardType ? update : create

  return (
    <Modal
      title={cardType ? 'Editar tipo de card' : 'Novo tipo de card'}
      onClose={onClose}
    >
      <form
        className="flex flex-col gap-3"
        onSubmit={(e) => {
          e.preventDefault()
          const done = { onSuccess: onClose }
          const body = { name, color, initial_lane_id: lane || null }
          if (cardType) {
            update.mutate({ id: cardType.id, ...body }, done)
          } else {
            create.mutate(body, done)
          }
        }}
      >
        <TextField label="Nome" value={name} onChange={setName} autoFocus />
        <ColorField value={color} onChange={setColor} />
        <SelectField
          label="Baia inicial das tasks"
          value={lane}
          onChange={setLane}
          options={[
            { value: '', label: 'Nenhuma' },
            ...lanes.map((l) => ({ value: l.id, label: l.name })),
          ]}
        />
        <p className="text-xs text-slate-500">
          Onde nascem as tasks que agentes criam deste tipo. Nenhuma: agentes
          não criam tasks dele.
        </p>
        <ErrorMessage error={save.error} />
        <div className="flex justify-end gap-2">
          <Button onClick={onClose}>Cancelar</Button>
          <Button type="submit" variant="primary" disabled={save.isPending}>
            Salvar
          </Button>
        </div>
      </form>
    </Modal>
  )
}
