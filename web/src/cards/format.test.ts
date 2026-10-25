import { describe, expect, it } from 'vitest'

import { formatCost, formatDuration, formatInt } from './format'

describe('format', () => {
  it('formats numbers, costs and durations', () => {
    expect(formatInt(12345)).toBe('12.345')
    expect(formatCost(2.5)).toBe('US$\u00a02,50')
    expect(formatDuration(4400)).toBe('4s')
    expect(formatDuration(125_000)).toBe('2min 05s')
  })
})
