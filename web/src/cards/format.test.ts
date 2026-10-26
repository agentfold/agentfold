import { describe, expect, it } from 'vitest'

import { formatCost, formatDuration, formatInt, runLabel } from './format'

describe('format', () => {
  it('formats numbers, costs and durations', () => {
    expect(formatInt(12345)).toBe('12.345')
    expect(formatCost(2.5)).toBe('US$\u00a02,50')
    expect(formatDuration(4400)).toBe('4s')
    expect(formatDuration(125_000)).toBe('2min 05s')
  })

  it('names runs by their flow and why they ran', () => {
    expect(runLabel({ flow_name: 'dev', trigger: 'card_created' })).toBe('dev')
    expect(runLabel({ flow_name: 'plan', trigger: 'artifact_revision' })).toBe(
      'plan (revisão de artefato)',
    )
  })
})
