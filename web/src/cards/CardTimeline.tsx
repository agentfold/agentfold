import { useCardEvents, useLanes } from '../api/queries'
import { QueryStatus } from '../components/Loading'
import { formatTime } from './format'
import { describeEvent } from './timeline'

/** What happened to a card, oldest first. */
export function CardTimeline({ cardId }: { cardId: string }) {
  const events = useCardEvents(cardId)
  const lanes = useLanes()
  if (!events.data) {
    return <QueryStatus isPending error={events.error} />
  }
  const laneName = (id: string) =>
    lanes.data?.find((l) => l.id === id)?.name ?? 'baia removida'
  return (
    <section aria-label="Linha do tempo" className="flex flex-col gap-1">
      <h3 className="text-xs font-medium text-slate-600">Linha do tempo</h3>
      <ol className="flex flex-col gap-1 text-sm">
        {events.data.map((e) => (
          <li key={e.id} className="flex gap-2">
            <time
              dateTime={e.created_at}
              className="shrink-0 text-xs text-slate-500 tabular-nums"
            >
              {formatTime(e.created_at)}
            </time>
            <span className="text-slate-700">{describeEvent(e, laneName)}</span>
          </li>
        ))}
      </ol>
    </section>
  )
}
