import { useState, type SyntheticEvent } from 'react'
import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

import type { Artifact } from '../api/client'
import {
  useApproveArtifact,
  useCardArtifacts,
  useRequestArtifactChanges,
} from '../api/queries'
import { Button } from '../components/Button'
import { ErrorMessage } from '../components/ErrorMessage'
import { SelectField, TextArea } from '../components/Field'
import { QueryStatus } from '../components/Loading'

/** The artifacts agents collected on a card: pick one to read and review. */
export function ArtifactsPanel({ cardId }: { cardId: string }) {
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
  const artifact =
    artifacts.data.find((a) => a.id === selected) ?? artifacts.data[0]
  return (
    <div className="flex flex-col gap-4">
      {artifacts.data.length > 1 && (
        <SelectField
          label="Artefato"
          value={artifact.id}
          onChange={setSelected}
          options={artifacts.data.map((a) => ({ value: a.id, label: a.name }))}
        />
      )}
      <ArtifactReview key={artifact.id} artifact={artifact} />
    </div>
  )
}

const reviewNotes: Partial<Record<Artifact['review'], string>> = {
  revising: 'O agente está revisando…',
  approved: 'Aprovado.',
  closed:
    'A revisão terminou sem aprovação: a execução foi cancelada ou falhou.',
}

/**
 * One artifact: a version rendered as Markdown next to its review chat.
 * While a review awaits the human, the latest version takes messages
 * (selecting a passage and clicking "Revisar" quotes it) and approval.
 */
function ArtifactReview({ artifact }: { artifact: Artifact }) {
  const request = useRequestArtifactChanges()
  const approve = useApproveArtifact()
  const latest = artifact.versions[artifact.versions.length - 1]
  const [picked, setPicked] = useState<number>()
  const [draft, setDraft] = useState('')
  const [passage, setPassage] = useState('')
  const shown = artifact.versions.find((v) => v.version === picked) ?? latest
  const reviewing = artifact.review === 'open' || artifact.review === 'revising'
  const open = artifact.review === 'open' && shown === latest
  const busy = request.isPending || approve.isPending
  const note = reviewNotes[artifact.review]

  const onSelect = (e: SyntheticEvent<HTMLDivElement>) => {
    const selection = window.getSelection()
    const inside =
      selection !== null && e.currentTarget.contains(selection.anchorNode)
    setPassage(inside ? selection.toString().trim() : '')
  }
  const quote = () => {
    const line = `Citação sobre "${passage}":\n`
    setDraft((current) => (current === '' ? line : `${current}\n\n${line}`))
    setPassage('')
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
    <div className="grid gap-4 lg:grid-cols-[3fr_2fr]">
      <section aria-label={artifact.name} className="flex flex-col gap-2">
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
        {open && passage !== '' && (
          <Button variant="secondary" className="self-start" onClick={quote}>
            Revisar
          </Button>
        )}
        <div
          data-testid="artifact-content"
          className="markdown rounded-md border border-slate-200 p-4"
          onMouseUp={onSelect}
          onKeyUp={onSelect}
        >
          <Markdown remarkPlugins={[remarkGfm]}>{shown.content}</Markdown>
        </div>
      </section>
      <section aria-label="Revisão" className="flex flex-col gap-3">
        <h3 className="text-sm font-semibold">Revisão</h3>
        {artifact.messages.length === 0 && (
          <p className="text-sm text-slate-500">
            {artifact.review === 'open'
              ? 'Selecione um trecho e clique em “Revisar”, ou escreva o que ajustar. Aprovar segue o fluxo.'
              : 'Nenhuma mensagem.'}
          </p>
        )}
        <ol className="flex flex-col gap-2">
          {artifact.messages.map((m) => (
            <li
              key={m.id}
              className={`flex flex-col gap-1 rounded-md p-2 text-sm ${
                m.role === 'human'
                  ? 'ml-6 bg-indigo-50 text-indigo-950'
                  : 'mr-6 bg-slate-100 text-slate-800'
              }`}
            >
              <span className="text-xs text-slate-500">
                {m.role === 'human' ? 'Você' : 'Agente'} · versão{' '}
                {String(m.version)}
              </span>
              <span className="whitespace-pre-wrap">{m.text}</span>
            </li>
          ))}
        </ol>
        {note && (
          <p
            className={`text-sm ${artifact.review === 'revising' ? 'text-indigo-700' : 'text-slate-500'}`}
          >
            {note}
          </p>
        )}
        {reviewing && (
          <>
            <TextArea
              label="Mensagem para o agente"
              rows={5}
              value={draft}
              onChange={setDraft}
              disabled={!open}
            />
            <ErrorMessage error={request.error ?? approve.error} />
            <div className="flex justify-end gap-2">
              <Button
                variant="secondary"
                disabled={!open || busy}
                onClick={() => {
                  approve.mutate(artifact.id)
                }}
              >
                Aprovar
              </Button>
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
    </div>
  )
}
