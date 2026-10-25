import { useState } from 'react'

import type { Project } from '../api/client'
import {
  useCreateProject,
  useDeleteProject,
  useProjects,
  useUpdateProject,
} from '../api/queries'
import { Button } from '../components/Button'
import { ErrorMessage } from '../components/ErrorMessage'
import { TextField } from '../components/Field'
import { QueryStatus } from '../components/Loading'
import { Modal } from '../components/Modal'

export function ProjectsPage() {
  const projects = useProjects()
  const remove = useDeleteProject()
  const [editing, setEditing] = useState<Project | 'new' | null>(null)

  return (
    <section className="mx-auto flex max-w-3xl flex-col gap-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-base font-semibold">Projetos</h2>
          <p className="text-sm text-slate-500">
            Repositórios git locais em que os agentes trabalham.
          </p>
        </div>
        <Button
          variant="primary"
          onClick={() => {
            setEditing('new')
          }}
        >
          Novo projeto
        </Button>
      </div>

      <QueryStatus isPending={projects.isPending} error={projects.error} />
      <ErrorMessage error={remove.error} />

      {projects.data?.length === 0 && (
        <p className="text-sm text-slate-500">Nenhum projeto cadastrado.</p>
      )}
      {projects.data && projects.data.length > 0 && (
        <ul className="divide-y divide-slate-200 rounded-lg border border-slate-200 bg-white">
          {projects.data.map((p) => (
            <li key={p.id} className="flex items-center gap-3 px-4 py-3">
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium">{p.name}</p>
                <p className="truncate font-mono text-xs text-slate-500">
                  {p.local_path}
                </p>
              </div>
              <Button
                variant="ghost"
                aria-label={`Editar ${p.name}`}
                onClick={() => {
                  setEditing(p)
                }}
              >
                Editar
              </Button>
              <Button
                variant="ghost"
                aria-label={`Excluir ${p.name}`}
                onClick={() => {
                  if (window.confirm(`Excluir o projeto ${p.name}?`)) {
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

      {editing && (
        <ProjectForm
          project={editing === 'new' ? undefined : editing}
          onClose={() => {
            setEditing(null)
          }}
        />
      )}
    </section>
  )
}

interface FormProps {
  project?: Project
  onClose: () => void
}

function ProjectForm({ project, onClose }: FormProps) {
  const create = useCreateProject()
  const update = useUpdateProject()
  const [name, setName] = useState(project?.name ?? '')
  const [path, setPath] = useState(project?.local_path ?? '')
  const save = project ? update : create

  return (
    <Modal
      title={project ? 'Editar projeto' : 'Novo projeto'}
      onClose={onClose}
    >
      <form
        className="flex flex-col gap-3"
        onSubmit={(e) => {
          e.preventDefault()
          const body = { name, local_path: path }
          const done = { onSuccess: onClose }
          if (project) {
            update.mutate({ id: project.id, ...body }, done)
          } else {
            create.mutate(body, done)
          }
        }}
      >
        <TextField label="Nome" value={name} onChange={setName} autoFocus />
        <TextField
          label="Pasta local"
          value={path}
          onChange={setPath}
          placeholder="/home/voce/repos/projeto"
          hint="Caminho absoluto de um repositório git. É salvo como a raiz do repositório."
        />
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
