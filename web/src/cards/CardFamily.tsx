import type { Card, FlowRun } from '../api/client'
import { childProgress } from '../board/children'
import { RunBadge } from '../board/RunBadge'
import { pageHref } from '../route'

interface Props {
  card: Card
  cards: Card[]
  laneName: (id: string) => string | undefined
  /** The latest flow run of each card, once loaded. */
  latestRuns: FlowRun[] | undefined
}

/** A card's properties as key=value chips. */
function Properties({ card }: { card: Card }) {
  return Object.entries(card.properties).map(([key, value]) => (
    <code
      key={key}
      className="rounded bg-slate-100 px-1.5 py-0.5 text-xs text-slate-700"
    >
      {key}={value}
    </code>
  ))
}

/**
 * The properties of a card, its parent, the cards it depends on and its
 * child cards, with where each one is.
 */
export function CardFamily({ card, cards, laneName, latestRuns }: Props) {
  const runs = new Map((latestRuns ?? []).map((r) => [r.card_id, r]))
  const parent = cards.find((c) => c.id === card.parent_card_id)
  const dependencies = cards.filter((c) => card.depends_on.includes(c.id))
  const hasProperties = Object.keys(card.properties).length > 0
  const { children, done } = childProgress(card.id, cards, runs)
  if (
    !parent &&
    children.length === 0 &&
    dependencies.length === 0 &&
    !hasProperties
  ) {
    return null
  }
  return (
    <section
      aria-label="Cards relacionados"
      className="flex flex-col gap-2 rounded-lg bg-white p-4 shadow-xs"
    >
      {parent && (
        <p className="text-sm text-slate-600">
          Card pai:{' '}
          <a
            className="text-indigo-600 hover:underline"
            href={pageHref('cards', parent.id)}
          >
            {parent.title}
          </a>
        </p>
      )}
      {hasProperties && (
        <p className="flex flex-wrap items-center gap-2 text-sm text-slate-600">
          Propriedades: <Properties card={card} />
        </p>
      )}
      {dependencies.length > 0 && (
        <>
          <h3 className="text-sm font-semibold">Depende de</h3>
          <ul className="divide-y divide-slate-100">
            {dependencies.map((c) => (
              <li
                key={c.id}
                className="flex flex-wrap items-center gap-x-3 gap-y-1 py-2"
              >
                <a
                  className="min-w-0 flex-1 truncate text-sm text-indigo-600 hover:underline"
                  href={pageHref('cards', c.id)}
                >
                  {c.title}
                </a>
                <Properties card={c} />
                <span className="text-xs text-slate-500">
                  {laneName(c.lane_id)}
                </span>
              </li>
            ))}
          </ul>
        </>
      )}
      {children.length > 0 && (
        <>
          <h3 className="text-sm font-semibold">
            Cards filhos · {done}/{children.length} com o fluxo concluído
          </h3>
          <ul className="divide-y divide-slate-100">
            {children.map((c) => {
              const run = runs.get(c.id)
              return (
                <li key={c.id} className="flex items-center gap-3 py-2">
                  <a
                    className="min-w-0 flex-1 truncate text-sm text-indigo-600 hover:underline"
                    href={pageHref('cards', c.id)}
                  >
                    {c.title}
                  </a>
                  {c.canceled_at && (
                    <span className="text-xs text-red-600">Cancelada</span>
                  )}
                  <span className="text-xs text-slate-500">
                    {laneName(c.lane_id)}
                  </span>
                  {run && <RunBadge run={run} />}
                </li>
              )
            })}
          </ul>
        </>
      )}
    </section>
  )
}
