import { useSyncExternalStore } from 'react'

export type Page =
  'board' | 'cards' | 'review' | 'projects' | 'card-types' | 'flows' | 'agents'

const pages: Page[] = [
  'board',
  'cards',
  'review',
  'projects',
  'card-types',
  'flows',
  'agents',
]

export interface Route {
  page: Page
  /** Second path segment, e.g. the flow id in #/flows/<id>. */
  id?: string
}

export function pageHref(page: Page, id?: string): string {
  return id ? `#/${page}/${id}` : `#/${page}`
}

function readHash(): string {
  return window.location.hash
}

function subscribe(onChange: () => void) {
  window.addEventListener('hashchange', onChange)
  return () => {
    window.removeEventListener('hashchange', onChange)
  }
}

/** Parses a hash like #/flows/<id>; unknown pages fall back to the board. */
export function parseRoute(hash: string): Route {
  const [name, id] = hash.replace(/^#\//, '').split('/')
  const page = pages.find((p) => p === name)
  if (!page) {
    return { page: 'board' }
  }
  return id ? { page, id } : { page }
}

/** The page named by the URL hash (#/projects, #/flows/<id>…). */
export function useRoute(): Route {
  return parseRoute(useSyncExternalStore(subscribe, readHash))
}

/** Navigates to a page. */
export function goTo(page: Page, id?: string) {
  window.location.hash = pageHref(page, id)
}
