import { useState } from 'react'

import {
  useCancelRun,
  useCardRuns,
  useCards,
  useCardTypes,
  useLanes,
  useProjects,
} from '../api/queries'
import { isActive } from '../board/runs'
import { RunBadge } from '../board/RunBadge'
import { Button } from '../components/Button'
import { ErrorMessage } from '../components/ErrorMessage'
import { QueryStatus } from '../components/Loading'
import { goTo, pageHref } from '../route'
import { ArtifactsPanel } from './ArtifactsPanel'
import { CardForm } from './CardForm'
import { CardQuestions } from './CardQuestions'
import { CardTimeline } from './CardTimeline'
import { RunsPanel } from './RunsPanel'
import { UsagePanel } from './UsagePanel'

const tabs = [
  { id: 'runs', label: 'Execução' },
  { id: 'details', label: 'Detalhes' },
  { id: 'artifacts', label: 'Artefatos' },
  { id: 'timeline', label: 'Linha do tempo' },
  { id: 'usage', label: 'Consumo' },
] as const

type Tab = (typeof tabs)[number]['id']

/** Everything about a card: its runs live, questions, timeline and usage. */
export function CardPage({ cardId }: { cardId: string }) {
  const cards = useCards()
  const runs = useCardRuns(cardId)
  const lanes = useLanes()
  const cardTypes = useCardTypes()
  const projects = useProjects()
  const cancel = useCancelRun()
  const [tab, setTab] = useState<Tab>('runs')

  const failed = cards.error ?? runs.error
  if (failed || !cards.data || !runs.data) {
    return <QueryStatus isPending error={failed} />
  }
  const card = cards.data.find((c) => c.id === cardId)
  if (!card) {
    return (
      <p className="text-sm text-slate-600">
        Card não encontrado.{' '}
        <a className="text-indigo-600 underline" href={pageHref('board')}>
          Voltar ao board
        </a>
      </p>
    )
  }
  const latest = runs.data[0] as (typeof runs.data)[number] | undefined
  const lane = lanes.data?.find((l) => l.id === card.lane_id)
  const cardType = cardTypes.data?.find((t) => t.id === card.card_type_id)
  const project = projects.data?.find((p) => p.id === card.project_id)

  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-4">
      <a
        className="text-sm text-indigo-600 hover:underline"
        href={pageHref('board')}
      >
        ← Board
      </a>
      <header className="flex flex-wrap items-start gap-3">
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <h2 className="text-xl font-semibold">{card.title}</h2>
          <p className="text-sm text-slate-600">
            {[lane?.name, cardType?.name, project?.name]
              .filter(Boolean)
              .join(' · ')}
          </p>
        </div>
        {latest && <RunBadge run={latest} />}
        {latest && isActive(latest) && (
          <Button
            variant="danger"
            disabled={cancel.isPending}
            onClick={() => {
              if (window.confirm(`Cancelar o fluxo ${latest.flow_name}?`)) {
                cancel.mutate(latest.id)
              }
            }}
          >
            Cancelar fluxo
          </Button>
        )}
      </header>
      <ErrorMessage error={cancel.error} />
      <CardQuestions cardId={card.id} />
      <nav role="tablist" className="flex gap-1 border-b border-slate-200">
        {tabs.map((t) => (
          <button
            key={t.id}
            type="button"
            role="tab"
            aria-selected={tab === t.id}
            className={`-mb-px border-b-2 px-3 py-2 text-sm font-medium ${
              tab === t.id
                ? 'border-indigo-600 text-indigo-700'
                : 'border-transparent text-slate-600 hover:text-slate-900'
            }`}
            onClick={() => {
              setTab(t.id)
            }}
          >
            {t.label}
          </button>
        ))}
      </nav>
      <div role="tabpanel" className="rounded-lg bg-white p-4 shadow-xs">
        {tab === 'runs' && <RunsPanel runs={runs.data} />}
        {tab === 'details' && (
          <CardForm
            key={card.updated_at}
            card={card}
            inline
            onSaved={() => undefined}
            onClose={() => {
              goTo('board')
            }}
          />
        )}
        {tab === 'artifacts' && <ArtifactsPanel cardId={card.id} />}
        {tab === 'timeline' && <CardTimeline cardId={card.id} />}
        {tab === 'usage' && <UsagePanel runs={runs.data} />}
      </div>
    </div>
  )
}
