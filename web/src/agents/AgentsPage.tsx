import { useState } from 'react'

import type { McpServer } from '../api/client'
import {
  useAgentProfiles,
  useCreateAgentProfile,
  useDeleteAgentProfile,
  useDeleteMcpServer,
  useMcpServers,
} from '../api/queries'
import { Button } from '../components/Button'
import { ErrorMessage } from '../components/ErrorMessage'
import { TextField } from '../components/Field'
import { QueryStatus } from '../components/Loading'
import { Modal } from '../components/Modal'
import { McpServerForm } from './McpServerForm'

/** Agent profiles and external MCP servers that flow nodes use. */
export function AgentsPage() {
  return (
    <div className="mx-auto flex max-w-3xl flex-col gap-10">
      <Profiles />
      <McpServers />
    </div>
  )
}

const credentialLabels = { oauth_token: 'token', api_key: 'API key' }

function Profiles() {
  const profiles = useAgentProfiles()
  const remove = useDeleteAgentProfile()
  const [creating, setCreating] = useState(false)

  return (
    <section className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-base font-semibold">Perfis de agente</h2>
          <p className="text-sm text-slate-500">
            Contas do Claude com que os nós rodam. A credencial é cadastrada
            pelo terminal:{' '}
            <code className="rounded bg-slate-200 px-1 text-xs">
              mise run agent:login -- --profile &lt;nome&gt;
            </code>
          </p>
        </div>
        <Button
          variant="primary"
          onClick={() => {
            setCreating(true)
          }}
        >
          Novo perfil
        </Button>
      </div>
      <QueryStatus isPending={profiles.isPending} error={profiles.error} />
      <ErrorMessage error={remove.error} />
      {profiles.data?.length === 0 && (
        <p className="text-sm text-slate-500">Nenhum perfil cadastrado.</p>
      )}
      {profiles.data && profiles.data.length > 0 && (
        <ul className="divide-y divide-slate-200 rounded-lg border border-slate-200 bg-white">
          {profiles.data.map((p) => (
            <li key={p.id} className="flex items-center gap-3 px-4 py-3">
              <p className="flex-1 text-sm font-medium">{p.name}</p>
              {p.credential_kind ? (
                <span className="rounded-full bg-green-100 px-2 py-0.5 text-xs text-green-800">
                  Pronto ({credentialLabels[p.credential_kind]})
                </span>
              ) : (
                <span className="rounded-full bg-amber-100 px-2 py-0.5 text-xs text-amber-800">
                  Sem credencial
                </span>
              )}
              <Button
                variant="ghost"
                aria-label={`Excluir ${p.name}`}
                onClick={() => {
                  if (window.confirm(`Excluir o perfil ${p.name}?`)) {
                    remove.mutate(p.id)
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
        <NewProfileForm
          onClose={() => {
            setCreating(false)
          }}
        />
      )}
    </section>
  )
}

function NewProfileForm({ onClose }: { onClose: () => void }) {
  const create = useCreateAgentProfile()
  const [name, setName] = useState('')
  return (
    <Modal title="Novo perfil" onClose={onClose}>
      <form
        className="flex flex-col gap-3"
        onSubmit={(e) => {
          e.preventDefault()
          create.mutate(name, { onSuccess: onClose })
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

function McpServers() {
  const servers = useMcpServers()
  const remove = useDeleteMcpServer()
  const [editing, setEditing] = useState<McpServer | 'new' | null>(null)

  return (
    <section className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-base font-semibold">Servidores MCP</h2>
          <p className="text-sm text-slate-500">
            MCPs externos (ex.: Sourcegraph) que os nós Agente podem usar.
          </p>
        </div>
        <Button
          variant="primary"
          onClick={() => {
            setEditing('new')
          }}
        >
          Novo servidor
        </Button>
      </div>
      <QueryStatus isPending={servers.isPending} error={servers.error} />
      <ErrorMessage error={remove.error} />
      {servers.data?.length === 0 && (
        <p className="text-sm text-slate-500">Nenhum servidor cadastrado.</p>
      )}
      {servers.data && servers.data.length > 0 && (
        <ul className="divide-y divide-slate-200 rounded-lg border border-slate-200 bg-white">
          {servers.data.map((m) => (
            <li key={m.id} className="flex items-center gap-3 px-4 py-3">
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium">{m.name}</p>
                <p className="truncate font-mono text-xs text-slate-500">
                  {m.transport === 'http'
                    ? m.url
                    : [m.command, ...m.args].join(' ')}
                  {m.secret_names.length > 0 &&
                    ` · segredos: ${m.secret_names.join(', ')}`}
                </p>
              </div>
              <Button
                variant="ghost"
                aria-label={`Editar ${m.name}`}
                onClick={() => {
                  setEditing(m)
                }}
              >
                Editar
              </Button>
              <Button
                variant="ghost"
                aria-label={`Excluir ${m.name}`}
                onClick={() => {
                  if (window.confirm(`Excluir o servidor ${m.name}?`)) {
                    remove.mutate(m.id)
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
        <McpServerForm
          server={editing === 'new' ? undefined : editing}
          onClose={() => {
            setEditing(null)
          }}
        />
      )}
    </section>
  )
}
