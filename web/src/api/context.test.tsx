import { renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { useApi } from './context'

describe('useApi', () => {
  it('requires the ApiContext', () => {
    expect(() => renderHook(() => useApi())).toThrow(
      'useApi must be used inside ApiContext',
    )
  })
})
