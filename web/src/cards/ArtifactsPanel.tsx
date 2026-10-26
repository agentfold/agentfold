import { useState, type SyntheticEvent } from 'react'
import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

import type { Artifact } from '../api/client'
import { useCardArtifacts, useReviseArtifact } from '../api/queries'
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

/**
 * One artifact: a version rendered as Markdown next to its review chat.
 * Selecting a passage and clicking "Revisar" quotes it in the message.
 */
function ArtifactReview({ artifact }: { artifact: Artifact }) {
  const revise = useReviseArtifact()
  const latest = artifact.versions[artifact.versions.length - 1]
  const [picked, setPicked] = useState<number>()
  const [draft, setDraft] = useState('')
  const [passage, setPassage] = useState('')
  const shown = artifact.versions.find((v) => v.version === picked) ?? latest
  const open = !artifact.frozen

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
    revise.mutate(
      { id: artifact.id, text: draft, version: shown.version },
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
        {artifact.frozen && (
          <p className="text-sm text-slate-500">
            Congelado: o card saiu da baia, então o artefato não pode mais ser
            revisado.
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
            {open
              ? 'Selecione um trecho e clique em “Revisar”, ou escreva direto para o agente que gerou o artefato.'
              : 'Nenhuma revisão.'}
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
        {artifact.revising && (
          <p className="text-sm text-indigo-700">O agente está revisando…</p>
        )}
        {open && (
          <>
            <TextArea
              label="Mensagem para o agente"
              rows={5}
              value={draft}
              onChange={setDraft}
            />
            <ErrorMessage error={revise.error} />
            <Button
              variant="primary"
              className="self-end"
              disabled={
                revise.isPending || artifact.revising || draft.trim() === ''
              }
              onClick={send}
            >
              Enviar
            </Button>
          </>
        )}
      </section>
    </div>
  )
}
