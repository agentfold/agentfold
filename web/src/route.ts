import { useSyncExternalStore } from 'react'

export type Page = 'board' | 'projects' | 'card-types'

const pages: Page[] = ['board', 'projects', 'card-types']

export function pageHref(page: Page): string {
  return `#/${page}`
}

function currentPage(): Page {
  const name = window.location.hash.replace(/^#\//, '')
  return pages.find((p) => p === name) ?? 'board'
}

function subscribe(onChange: () => void) {
  window.addEventListener('hashchange', onChange)
  return () => {
    window.removeEventListener('hashchange', onChange)
  }
}

/** The page named by the URL hash (#/projects…), defaulting to the board. */
export function usePage(): Page {
  return useSyncExternalStore(subscribe, currentPage)
}
