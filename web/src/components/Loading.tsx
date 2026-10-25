import { ErrorMessage } from './ErrorMessage'

interface Props {
  isPending: boolean
  error: unknown
}

/** Placeholder while queries load or after they fail; null once they succeed. */
export function QueryStatus({ isPending, error }: Props) {
  if (error) {
    return <ErrorMessage error={error} />
  }
  if (isPending) {
    return <p className="text-sm text-slate-500">Carregando…</p>
  }
  return null
}
