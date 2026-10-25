import { describe, expect, it } from 'vitest'

import { formatTools, parseCount, parseSchema, parseTools } from './config'

describe('tool lists', () => {
  it('parses and formats comma-separated names', () => {
    expect(parseTools(' Read, Bash(go test:*) ,, ')).toEqual([
      'Read',
      'Bash(go test:*)',
    ])
    expect(formatTools(['Read', 'Edit'])).toBe('Read, Edit')
    expect(formatTools(undefined)).toBe('')
  })
})

describe('parseCount', () => {
  it('reads numbers and empty fields', () => {
    expect(parseCount('5')).toBe(5)
    expect(parseCount('')).toBeUndefined()
  })
})

describe('parseSchema', () => {
  it.each([
    ['', undefined],
    ['  ', undefined],
    ['{"type":"object"}', { type: 'object' }],
    ['[1]', false],
    ['null', false],
    ['"x"', false],
    ['{', false],
  ])('parses %j', (text, want) => {
    expect(parseSchema(text)).toEqual(want)
  })
})
