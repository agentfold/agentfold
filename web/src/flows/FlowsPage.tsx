import { useState } from 'react'

import {
  useCreateFlow,
  useDeleteFlow,
  useFlows,
  useUpdateFlow,
} from '../api/queries'
import { Button } from '../components/Button'
import { ErrorMessage } from '../components/ErrorMessage'
import { TextField } from '../components/Field'
import { QueryStatus } from '../components/Loading'
import { Modal } from '../components/Modal'
import { goTo, pageHref } from '../route'

export function FlowsPage() {
  const flows = useFlows()
  const update = useUpdateFlow()
  const remove = useDeleteFlow()
  const [creating, setCreating] = useState(false)

  return (
    <section className="mx-auto flex max-w-3xl flex-col gap-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-base font-semibold">Fluxos</h2>
          <p className="text-sm text-slate-500">
            Quando um card é criado ou entra numa baia, os fluxos ativos rodam
            agentes nele.
          </p>
        </div>
        <Button
          variant="primary"
          onClick={() => {
            setCreating(true)
          }}
        >
          Novo fluxo
        </Button>
      </div>

      <QueryStatus isPending={flows.isPending} error={flows.error} />
      <ErrorMessage error={update.error ?? remove.error} />

      {flows.data?.length === 0 && (
        <p className="text-sm text-slate-500">Nenhum fluxo cadastrado.</p>
      )}
      {flows.data && flows.data.length > 0 && (
        <ul className="divide-y divide-slate-200 rounded-lg border border-slate-200 bg-white">
          {flows.data.map((f) => (
            <li key={f.id} className="flex items-center gap-3 px-4 py-3">
              <div className="min-w-0 flex-1">
                <a
                  className="text-sm font-medium hover:underline"
                  href={pageHref('flows', f.id)}
                >
                  {f.name}
                </a>
                <p className="text-xs text-slate-500">
                  {f.latest_version > 0
                    ? `Versão ${String(f.latest_version)} publicada`
                    : 'Nunca publicado'}
                </p>
              </div>
              <label
                className="flex items-center gap-2 text-sm text-slate-600"
                title={
                  f.latest_version > 0
                    ? undefined
                    : 'Publique uma versão antes de ativar'
                }
              >
                <input
                  type="checkbox"
                  checked={f.enabled}
                  disabled={f.latest_version === 0}
                  onChange={(e) => {
                    update.mutate({
                      id: f.id,
                      name: f.name,
                      enabled: e.target.checked,
                    })
                  }}
                />
                Ativo
              </label>
              <Button
                variant="ghost"
                aria-label={`Excluir ${f.name}`}
                onClick={() => {
                  if (
                    window.confirm(`Excluir o fluxo ${f.name} e suas versões?`)
                  ) {
                    remove.mutate(f.id)
                  }
                }}
              >
                Excluir
              </Button>
            </li>
          ))}
        </ul>
      )}

      {creating && (
        <NewFlowForm
          onClose={() => {
            setCreating(false)
          }}
        />
      )}
    </section>
  )
}

function NewFlowForm({ onClose }: { onClose: () => void }) {
  const create = useCreateFlow()
  const [name, setName] = useState('')
  return (
    <Modal title="Novo fluxo" onClose={onClose}>
      <form
        className="flex flex-col gap-3"
        onSubmit={(e) => {
          e.preventDefault()
          create.mutate(name, {
            onSuccess: (flow) => {
              goTo('flows', flow.id)
            },
          })
        }}
      >
        <TextField label="Nome" value={name} onChange={setName} autoFocus />
        <ErrorMessage error={create.error} />
        <div className="flex justify-end gap-2">
          <Button onClick={onClose}>Cancelar</Button>
          <Button type="submit" variant="primary" disabled={create.isPending}>
            Criar
          </Button>
        </div>
      </form>
    </Modal>
  )
}
