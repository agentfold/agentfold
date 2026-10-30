import {
  addEdge,
  applyEdgeChanges,
  applyNodeChanges,
  type Connection,
  type Edge,
  type EdgeChange,
  type IsValidConnection,
  type NodeChange,
} from '@xyflow/react'
import { useState } from 'react'

import type { Flow, FlowVersion } from '../api/client'
import {
  useAgentProfiles,
  useCardTypes,
  useFlow,
  useLanes,
  useMcpServers,
  usePublishFlow,
  useSaveFlowDraft,
} from '../api/queries'
import { Button } from '../components/Button'
import { ErrorMessage } from '../components/ErrorMessage'
import { QueryStatus } from '../components/Loading'
import { pageHref } from '../route'
import { FlowCanvas } from './FlowCanvas'
import {
  canConnect,
  edgeFor,
  fromApi,
  isEdit,
  kindLabels,
  newStep,
  nodeOutputs,
  toApi,
  type NodeKind,
  type StepConfig,
  type StepNode,
} from './graph'
import { NodeConfigForm } from './NodeConfigForm'

/** Loads a flow and opens its draft in the editor. */
export function FlowEditor({ flowId }: { flowId: string }) {
  const flow = useFlow(flowId)
  if (!flow.data) {
    return <QueryStatus isPending error={flow.error} />
  }
  return <DraftEditor flow={flow.data} />
}

const palette: NodeKind[] = [
  'agent',
  'move_card',
  'wait_children',
  'human_review',
  'set_property',
  'check_property',
  'end',
]

/**
 * Edits the draft locally; "Salvar" sends it to the server, which answers
 * with its problems. The flow query keeps refreshing (problems, version)
 * without resetting what is being edited.
 */
function DraftEditor({ flow }: { flow: Flow }) {
  const lanes = useLanes()
  const cardTypes = useCardTypes()
  const profiles = useAgentProfiles()
  const mcpServers = useMcpServers()
  const save = useSaveFlowDraft(flow.id)
  const publish = usePublishFlow(flow.id)
  const [initial] = useState(() => fromApi(flow.draft))
  const [nodes, setNodes] = useState<StepNode[]>(initial.nodes)
  const [edges, setEdges] = useState<Edge[]>(initial.edges)
  const [dirty, setDirty] = useState(false)
  const [published, setPublished] = useState<FlowVersion | null>(null)

  const edit = () => {
    setDirty(true)
    setPublished(null)
  }
  const onNodesChange = (changes: NodeChange<StepNode>[]) => {
    setNodes((current) => applyNodeChanges(changes, current))
    if (changes.some(isEdit)) {
      edit()
    }
  }
  const onEdgesChange = (changes: EdgeChange[]) => {
    setEdges((current) => applyEdgeChanges(changes, current))
    if (changes.some((c) => c.type !== 'select')) {
      edit()
    }
  }
  const onConnect = (connection: Connection) => {
    setEdges((current) => addEdge(edgeFor(connection), current))
    edit()
  }
  const isValidConnection: IsValidConnection = (connection) =>
    canConnect(connection, nodes, edges)

  const addStep = (kind: NodeKind) => {
    const step = newStep(kind, nodes)
    setNodes((current) => [
      ...current.map((n) => ({ ...n, selected: false })),
      { ...step, selected: true },
    ])
    edit()
  }
  const selected = nodes.find((n) => n.selected)
  // Turning the gateway on or off changes the outputs: edges leaving
  // from outputs the node no longer has go away.
  const setConfig = (node: StepNode, config: StepConfig) => {
    const outs = nodeOutputs({ ...node.data, config })
    setEdges((current) =>
      current.filter(
        (e) => e.source !== node.id || outs.includes(String(e.sourceHandle)),
      ),
    )
    setNodes((current) =>
      current.map((n) =>
        n.id === node.id ? { ...n, data: { ...n.data, config } } : n,
      ),
    )
    edit()
  }
  const removeStep = (id: string) => {
    setNodes((current) => current.filter((n) => n.id !== id))
    setEdges((current) =>
      current.filter((e) => e.source !== id && e.target !== id),
    )
    edit()
  }

  const saveDraft = () =>
    save.mutateAsync(toApi(nodes, edges)).then(() => {
      setDirty(false)
    })
  const onPublish = async () => {
    if (dirty) {
      await saveDraft()
    }
    setPublished(await publish.mutateAsync())
  }

  // Problems come from the last saved draft; show them on the nodes too.
  const problems = dirty ? [] : flow.problems
  const byNode = new Map<string, string[]>()
  for (const p of problems) {
    if (p.node_id) {
      byNode.set(p.node_id, [...(byNode.get(p.node_id) ?? []), p.message])
    }
  }
  const shown = nodes.map((n) => ({
    ...n,
    data: { ...n.data, problems: byNode.get(n.id) },
  }))
  const hasTrigger = nodes.some((n) => n.data.kind === 'trigger')

  return (
    <div className="flex h-full flex-col gap-3">
      <header className="flex flex-wrap items-center gap-3">
        <a
          href={pageHref('flows')}
          className="text-sm text-slate-500 hover:text-slate-900"
        >
          ← Fluxos
        </a>
        <h2 className="text-base font-semibold">{flow.name}</h2>
        <span className="rounded-full bg-slate-200 px-2 py-0.5 text-xs text-slate-600">
          {flow.latest_version > 0
            ? `v${String(flow.latest_version)} publicada`
            : 'nunca publicado'}
        </span>
        {dirty && (
          <span className="text-xs text-amber-700">Alterações não salvas</span>
        )}
        <span className="flex-1" />
        <Button
          disabled={!dirty || save.isPending}
          onClick={() => {
            void saveDraft().catch(() => undefined)
          }}
        >
          Salvar rascunho
        </Button>
        <Button
          variant="primary"
          disabled={save.isPending || publish.isPending}
          onClick={() => {
            void onPublish().catch(() => undefined)
          }}
        >
          Publicar
        </Button>
      </header>

      <ErrorMessage error={save.error ?? publish.error} />
      {published && (
        <p
          role="status"
          className="rounded-md bg-green-50 px-3 py-2 text-sm text-green-800"
        >
          Versão {published.version} publicada.
        </p>
      )}

      <div className="flex min-h-0 flex-1 gap-3">
        <aside className="flex w-40 shrink-0 flex-col gap-2">
          <p className="text-xs font-medium text-slate-500 uppercase">
            Adicionar
          </p>
          {(hasTrigger ? palette : (['trigger', ...palette] as NodeKind[])).map(
            (kind) => (
              <Button
                key={kind}
                className="justify-start"
                onClick={() => {
                  addStep(kind)
                }}
              >
                + {kindLabels[kind]}
              </Button>
            ),
          )}
          <p className="mt-2 text-xs text-slate-500">
            Ligue a saída de um nó (direita) à entrada de outro (esquerda).
            Delete remove o selecionado.
          </p>
        </aside>

        <div className="min-w-0 flex-1 rounded-xl border border-slate-200 bg-white">
          <FlowCanvas
            nodes={shown}
            edges={edges}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onConnect={onConnect}
            isValidConnection={isValidConnection}
          />
        </div>

        <aside className="flex w-80 shrink-0 flex-col gap-4 overflow-y-auto">
          {selected ? (
            <section className="flex flex-col gap-3 rounded-xl border border-slate-200 bg-white p-4">
              <div className="flex items-center justify-between">
                <h3 className="text-sm font-semibold">
                  {kindLabels[selected.data.kind]}
                </h3>
                {selected.data.kind !== 'trigger' && (
                  <Button
                    variant="ghost"
                    onClick={() => {
                      removeStep(selected.id)
                    }}
                  >
                    Remover nó
                  </Button>
                )}
              </div>
              <NodeConfigForm
                key={selected.id}
                data={selected.data}
                lanes={lanes.data ?? []}
                cardTypes={cardTypes.data ?? []}
                profiles={profiles.data ?? []}
                mcpServers={mcpServers.data ?? []}
                onChange={(config) => {
                  setConfig(selected, config)
                }}
              />
            </section>
          ) : (
            <p className="text-sm text-slate-500">
              Selecione um nó para configurá-lo.
            </p>
          )}

          <section aria-label="Problemas" className="flex flex-col gap-2">
            <h3 className="text-xs font-medium text-slate-500 uppercase">
              Para publicar
            </h3>
            {dirty && (
              <p className="text-sm text-slate-500">
                Salve para verificar o rascunho.
              </p>
            )}
            {!dirty && problems.length === 0 && (
              <p className="text-sm text-green-700">
                O rascunho pode ser publicado.
              </p>
            )}
            {problems.length > 0 && (
              <ul className="flex flex-col gap-1 text-sm text-red-700">
                {problems.map((p, i) => (
                  <li key={i}>
                    {p.node_id && (
                      <strong className="font-medium">{p.node_id}: </strong>
                    )}
                    {p.message}
                  </li>
                ))}
              </ul>
            )}
          </section>
        </aside>
      </div>
    </div>
  )
}
