import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

afterEach(() => {
  cleanup()
})

// React Flow measures the DOM with APIs jsdom lacks.
class ResizeObserverStub {
  observe = () => undefined
  unobserve = () => undefined
  disconnect = () => undefined
}
globalThis.ResizeObserver = ResizeObserverStub
globalThis.DOMMatrixReadOnly = class {
  m22 = 1
} as unknown as typeof DOMMatrixReadOnly
