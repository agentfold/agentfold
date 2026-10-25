import { describe, expect, it } from 'vitest'

import { formatPairs, parseLines, parsePairs } from './pairs'

describe('pairs', () => {
  it('parses lines and KEY=VALUE pairs', () => {
    expect(parseLines(' a \n\n b ')).toEqual(['a', 'b'])
    expect(parsePairs('A=1\n B = x=y \nnope\n=skip')).toEqual({
      A: '1',
      B: 'x=y',
    })
    expect(formatPairs({ A: '1', B: '2' })).toBe('A=1\nB=2')
  })
})
