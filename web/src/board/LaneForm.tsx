import { useState } from 'react'

import type { Lane } from '../api/client'
import { useCreateLane, useUpdateLane } from '../api/queries'
import { Button } from '../components/Button'
import { ErrorMessage } from '../components/ErrorMessage'
import { ColorField, TextField } from '../components/Field'
import { Modal } from '../components/Modal'

interface Props {
  lane?: Lane
  onClose: () => void
}

export function LaneForm({ lane, onClose }: Props) {
  const create = useCreateLane()
  const update = useUpdateLane()
  const [name, setName] = useState(lane?.name ?? '')
  const [color, setColor] = useState(lane?.color ?? '')
  const save = lane ? update : create

  return (
    <Modal title={lane ? 'Editar baia' : 'Nova baia'} onClose={onClose}>
      <form
        className="flex flex-col gap-3"
        onSubmit={(e) => {
          e.preventDefault()
          const done = { onSuccess: onClose }
          if (lane) {
            update.mutate({ id: lane.id, name, color }, done)
          } else {
            create.mutate({ name, color }, done)
          }
        }}
      >
        <TextField label="Nome" value={name} onChange={setName} autoFocus />
        <ColorField value={color} onChange={setColor} />
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
