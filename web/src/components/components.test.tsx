import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'

import { ApiError } from '../api/client'
import { ErrorMessage } from './ErrorMessage'
import { ColorField } from './Field'
import { QueryStatus } from './Loading'
import { Modal } from './Modal'

describe('Modal', () => {
  it('closes on Escape and on clicks outside the dialog', async () => {
    const onClose = vi.fn()
    const { container } = render(
      <Modal title="Title" onClose={onClose}>
        <p>Body</p>
      </Modal>,
    )
    expect(screen.getByRole('dialog', { name: 'Title' })).toBeInTheDocument()

    await userEvent.click(screen.getByText('Body'))
    await userEvent.keyboard('{a}')
    expect(onClose).not.toHaveBeenCalled()

    await userEvent.keyboard('{Escape}')
    expect(onClose).toHaveBeenCalledTimes(1)

    const backdrop = container.firstElementChild
    if (!backdrop) throw new Error('missing backdrop')
    await userEvent.click(backdrop)
    expect(onClose).toHaveBeenCalledTimes(2)
  })
})

describe('ErrorMessage', () => {
  it('renders nothing without an error', () => {
    const { container } = render(<ErrorMessage error={null} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('describes the error', () => {
    render(
      <ErrorMessage
        error={new ApiError(409, { code: 'in_use', message: 'in use' })}
      />,
    )
    expect(screen.getByRole('alert')).toHaveTextContent('ainda está em uso')
  })
})

describe('QueryStatus', () => {
  it('shows loading, errors and nothing once loaded', () => {
    const { rerender, container } = render(
      <QueryStatus isPending error={null} />,
    )
    expect(screen.getByText('Carregando…')).toBeInTheDocument()
    rerender(<QueryStatus isPending={false} error={new Error('x')} />)
    expect(screen.getByRole('alert')).toBeInTheDocument()
    rerender(<QueryStatus isPending={false} error={null} />)
    expect(container).toBeEmptyDOMElement()
  })
})

describe('ColorField', () => {
  function Picker() {
    const [color, setColor] = useState('#3b82f6')
    return <ColorField value={color} onChange={setColor} />
  }

  it('selects a color or none', async () => {
    render(<Picker />)
    expect(screen.getByRole('button', { name: '#3b82f6' })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
    await userEvent.click(screen.getByRole('button', { name: 'Sem cor' }))
    expect(screen.getByRole('button', { name: 'Sem cor' })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
    expect(screen.getByRole('button', { name: '#3b82f6' })).toHaveAttribute(
      'aria-pressed',
      'false',
    )
  })
})
