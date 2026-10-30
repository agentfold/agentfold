import { useEffect, useRef, useState, type SyntheticEvent } from 'react'
import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

import type { Artifact } from '../api/client'
import {
  useApproveArtifact,
  useCardArtifacts,
  useCardRuns,
  useRequestArtifactChanges,
} from '../api/queries'
import { Button } from '../components/Button'
import { ErrorMessage } from '../components/ErrorMessage'
import { SelectField, TextArea } from '../components/Field'
import { QueryStatus } from '../components/Loading'
import { NodeTranscript } from './NodeTranscript'

/**
 * The artifacts agents collected on a card, as tabs: each one side by side
 * with its review chat. It fills the height its className gives it.
 */
export function ArtifactsPanel({
  cardId,
  className = '',
}: {
  cardId: string
  className?: string
}) {
  const artifacts = useCardArtifacts(cardId)
  const [selected, setSelected] = useState('')
  if (!artifacts.data) {
    return <QueryStatus isPending error={artifacts.error} />
  }
  if (artifacts.data.length === 0) {
    return (
      <p className="text-sm text-slate-500">
        Nenhum artefato coletado neste card.
      </p>
    )
  }
  // The one awaiting the human comes first unless another was picked.
  const artifact =
    artifacts.data.find((a) => a.id === selected) ??
    artifacts.data.find((a) => a.review === 'open') ??
    artifacts.data[0]
  return (
    <div className={`flex min-h-0 flex-col gap-3 ${className}`}>
      {artifacts.data.length > 1 && (
        <nav
          role="tablist"
          aria-label="Artefatos"
          className="flex gap-1 border-b border-slate-200"
        >
          {artifacts.data.map((a) => (
            <button
              key={a.id}
              type="button"
              role="tab"
              aria-selected={a === artifact}
              className={`-mb-px inline-flex items-center gap-1.5 border-b-2 px-3 py-2 text-sm font-medium ${
                a === artifact
                  ? 'border-indigo-600 text-indigo-700'
                  : 'border-transparent text-slate-600 hover:text-slate-900'
              }`}
              onClick={() => {
                setSelected(a.id)
              }}
            >
              {a.name}
              {a.review === 'open' && (
                <span
                  title="Aguardando revisão"
                  className="size-1.5 rounded-full bg-amber-500"
                />
              )}
            </button>
          ))}
        </nav>
      )}
      <ArtifactReview key={artifact.id} cardId={cardId} artifact={artifact} />
    </div>
  )
}

const reviewNotes: Partial<Record<Artifact['review'], string>> = {
  approved: 'Aprovado.',
  closed:
    'A revisão terminou sem aprovação: a execução foi cancelada ou falhou.',
}

/** A passage selected in the artifact and where to offer quoting it. */
interface Passage {
  text: string
  top: number
  left: number
}

/** A passage as a Markdown quote, followed by a blank line to answer. */
const quoted = (text: string) =>
  `${text
    .split('\n')
    .map((line) => `> ${line}`)
    .join('\n')}\n\n`

/**
 * One artifact: a version rendered as Markdown (left) next to its review
 * chat (right). While a review awaits the human, the latest version takes
 * messages (selecting a passage offers quoting it) and approval.
 */
function ArtifactReview({
  cardId,
  artifact,
}: {
  cardId: string
  artifact: Artifact
}) {
  const request = useRequestArtifactChanges()
  const approve = useApproveArtifact()
  const latest = artifact.versions[artifact.versions.length - 1]
  const [picked, setPicked] = useState<number>()
  const [draft, setDraft] = useState('')
  const [passage, setPassage] = useState<Passage>()
  const [quotes, setQuotes] = useState(0)
  const message = useRef<HTMLTextAreaElement>(null)
  const chat = useRef<HTMLOListElement>(null)
  const shown = artifact.versions.find((v) => v.version === picked) ?? latest
  const reviewing = artifact.review === 'open' || artifact.review === 'revising'
  const open = artifact.review === 'open' && shown === latest
  const busy = request.isPending || approve.isPending
  const note = reviewNotes[artifact.review]

  // The chat keeps its latest message in sight.
  useEffect(() => {
    const list = chat.current
    if (artifact.messages.length > 0 && list) {
      list.scrollTop = list.scrollHeight
    }
  }, [artifact.messages.length, artifact.review])

  // After quoting, the cursor waits at the end of the message.
  useEffect(() => {
    const field = message.current
    if (quotes > 0 && field) {
      field.focus()
      field.setSelectionRange(field.value.length, field.value.length)
    }
  }, [quotes])

  const onSelect = (e: SyntheticEvent<HTMLDivElement>) => {
    const selection = window.getSelection()
    const text = selection?.toString().trim() ?? ''
    if (
      !open ||
      !selection ||
      text === '' ||
      !e.currentTarget.contains(selection.anchorNode)
    ) {
      setPassage(undefined)
      return
    }
    const rect = selection.getRangeAt(0).getBoundingClientRect()
    setPassage({ text, top: Math.max(rect.top - 40, 8), left: rect.left })
  }
  const quote = (text: string) => {
    setDraft((current) =>
      current.trim() === ''
        ? quoted(text)
        : `${current.trimEnd()}\n\n${quoted(text)}`,
    )
    setPassage(undefined)
    window.getSelection()?.removeAllRanges()
    setQuotes((n) => n + 1)
  }
  const send = () => {
    request.mutate(
      { id: artifact.id, text: draft },
      {
        onSuccess: () => {
          setDraft('')
        },
      },
    )
  }

  return (
    <div className="grid min-h-0 flex-1 gap-4 lg:grid-cols-2">
      <section
        aria-label={artifact.name}
        className="flex min-h-0 flex-col gap-2"
      >
        <div className="flex flex-wrap items-end gap-3">
          <h3 className="flex-1 text-base font-semibold">
            {artifact.name}{' '}
            <code className="text-xs font-normal text-slate-500">
              {shown.path}
            </code>
          </h3>
          {artifact.versions.length > 1 && (
            <SelectField
              label="Versão"
              value={String(shown.version)}
              onChange={(v) => {
                setPicked(Number(v))
                setPassage(undefined)
              }}
              options={artifact.versions.map((v) => ({
                value: String(v.version),
                label:
                  v === latest
                    ? `Versão ${String(v.version)} (atual)`
                    : `Versão ${String(v.version)}`,
              }))}
            />
          )}
        </div>
        {reviewing && shown !== latest && (
          <p className="text-sm text-slate-500">
            Versão antiga: os comentários e a aprovação são sobre a versão{' '}
            {String(latest.version)}.
          </p>
        )}
        <div
          data-testid="artifact-content"
          className="markdown min-h-0 flex-1 overflow-y-auto rounded-md border border-slate-200 bg-white p-4"
          onMouseUp={onSelect}
          onKeyUp={onSelect}
          onScroll={() => {
            setPassage(undefined)
          }}
        >
          <Markdown remarkPlugins={[remarkGfm]}>{shown.content}</Markdown>
        </div>
      </section>
      <section aria-label="Revisão" className="flex min-h-0 flex-col gap-3">
        <div className="flex items-center gap-3">
          <h3 className="flex-1 text-base font-semibold">Revisão</h3>
          {reviewing && (
            <Button
              variant="secondary"
              disabled={!open || busy}
              onClick={() => {
                approve.mutate(artifact.id)
              }}
            >
              Aprovar
            </Button>
          )}
        </div>
        <ol
          ref={chat}
          aria-label="Conversa da revisão"
          className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto rounded-md border border-slate-200 bg-slate-50 p-3"
        >
          {artifact.messages.length === 0 && artifact.review !== 'revising' && (
            <li className="text-sm text-slate-500">
              {artifact.review === 'open'
                ? 'Selecione um trecho do artefato para citá-lo, ou escreva o que ajustar. Aprovar segue o fluxo.'
                : 'Nenhuma mensagem.'}
            </li>
          )}
          {artifact.messages.map((m) => (
            <li
              key={m.id}
              className={`flex flex-col gap-1 rounded-lg p-3 ${
                m.role === 'human'
                  ? 'ml-8 bg-indigo-50'
                  : 'mr-8 border border-slate-200 bg-white'
              }`}
            >
              <span className="text-xs text-slate-500">
                {m.role === 'human' ? 'Você' : 'Agente'} · versão{' '}
                {String(m.version)}
              </span>
              <div className="markdown break-words">
                <Markdown remarkPlugins={[remarkGfm]}>{m.text}</Markdown>
              </div>
            </li>
          ))}
          {artifact.review === 'revising' && <AgentProgress cardId={cardId} />}
        </ol>
        {note && <p className="text-sm text-slate-500">{note}</p>}
        {reviewing && (
          <>
            <TextArea
              ref={message}
              label="Mensagem para o agente"
              rows={4}
              value={draft}
              onChange={setDraft}
              disabled={!open}
            />
            <ErrorMessage error={request.error ?? approve.error} />
            <div className="flex justify-end">
              <Button
                variant="primary"
                disabled={!open || busy || draft.trim() === ''}
                onClick={send}
              >
                Pedir ajustes
              </Button>
            </div>
          </>
        )}
      </section>
      {passage && (
        <Button
          variant="primary"
          className="fixed z-40 shadow-lg"
          style={{ top: passage.top, left: passage.left }}
          // Keeps the selection while the button is pressed.
          onMouseDown={(e) => {
            e.preventDefault()
          }}
          onClick={() => {
            quote(passage.text)
          }}
        >
          Citar
        </Button>
      )}
    </div>
  )
}

/** The agent at work on the changes, its steps live as they finish. */
function AgentProgress({ cardId }: { cardId: string }) {
  const runs = useCardRuns(cardId)
  const node = (runs.data ?? [])
    .flatMap((r) => r.nodes)
    .findLast(
      (n) =>
        n.node_type === 'agent' &&
        (n.status === 'queued' || n.status === 'running'),
    )
  return (
    <li className="mr-8 flex flex-col gap-2 rounded-lg border border-indigo-200 bg-white p-3">
      <span className="text-xs text-indigo-700">
        Agente · ajustando o artefato…
      </span>
      {node && (
        <NodeTranscript
          nodeRunId={node.id}
          startedAt={node.started_at ?? undefined}
        />
      )}
    </li>
  )
}
