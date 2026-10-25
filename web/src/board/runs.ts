import type { FlowRun } from '../api/client'

/** Whether the run holds its card: only the flow moves it meanwhile. */
export function isActive(run: FlowRun | undefined): boolean {
  return run?.status === 'running' || run?.status === 'waiting'
}
