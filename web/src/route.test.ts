import { afterEach, describe, expect, it } from 'vitest'

import { goTo, pageHref, parseRoute } from './route'

describe('route', () => {
  afterEach(() => {
    window.location.hash = ''
  })

  it.each([
    ['', { page: 'board' }],
    ['#/projects', { page: 'projects' }],
    ['#/flows/abc', { page: 'flows', id: 'abc' }],
    ['#/nope/abc', { page: 'board' }],
  ])('parses %j', (hash, want) => {
    expect(parseRoute(hash)).toEqual(want)
  })

  it('builds links and navigates', () => {
    expect(pageHref('flows')).toBe('#/flows')
    expect(pageHref('flows', 'abc')).toBe('#/flows/abc')
    goTo('flows', 'abc')
    expect(window.location.hash).toBe('#/flows/abc')
  })
})
