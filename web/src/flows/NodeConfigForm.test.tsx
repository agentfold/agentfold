import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it } from 'vitest'

import { agentProfile, cardType, lane, mcpServer } from '../test/fakeBackend'
import type { NodeKind, StepConfig } from './graph'
import { NodeConfigForm } from './NodeConfigForm'

const todo = lane('Todo', 0)
const dev = agentProfile('dev')
const fresh = agentProfile('fresh', { ready: false })
const sg = mcpServer('sg')
const bug = cardType('bug')
const task = cardType('task')

/** Renders the form for a node and exposes the latest config. */
function setup(kind: NodeKind, initial: StepConfig, mcpServers = [sg]) {
  const result: { config: StepConfig } = { config: initial }
  function Harness() {
    const [config, setConfig] = useState(initial)
    result.config = config
    return (
      <NodeConfigForm
        data={{ kind, config }}
        lanes={[todo]}
        cardTypes={[bug, task]}
        profiles={[dev, fresh]}
        mcpServers={mcpServers}
        onChange={setConfig}
      />
    )
  }
  render(<Harness />)
  return result
}

describe('NodeConfigForm', () => {
  it('configures a trigger', async () => {
    const result = setup('trigger', {})
    expect(screen.getByLabelText('Evento')).toHaveValue('card_created')
    expect(screen.queryByLabelText('Baia')).not.toBeInTheDocument()

    await userEvent.selectOptions(
      screen.getByLabelText('Evento'),
      'card_entered_lane',
    )
    await userEvent.selectOptions(screen.getByLabelText('Baia'), 'Todo')
    expect(result.config).toEqual({
      event: 'card_entered_lane',
      lane_id: todo.id,
    })

    await userEvent.selectOptions(
      screen.getByLabelText('Baia'),
      'Escolha a baia',
    )
    expect(result.config).toEqual({
      event: 'card_entered_lane',
      lane_id: undefined,
    })

    await userEvent.click(screen.getByLabelText('bug'))
    await userEvent.click(screen.getByLabelText('task'))
    expect(result.config).toMatchObject({ card_type_ids: [bug.id, task.id] })
    await userEvent.click(screen.getByLabelText('bug'))
    expect(result.config).toMatchObject({ card_type_ids: [task.id] })

    await userEvent.selectOptions(
      screen.getByLabelText('Evento'),
      'card_created',
    )
    expect(result.config).toEqual({
      event: 'card_created',
      lane_id: undefined,
      card_type_ids: [task.id],
    })
  })

  it('configures an agent', async () => {
    const result = setup('agent', {
      prompt: '',
      output_schema: { type: 'object' },
    })
    expect(screen.getByLabelText(/Schema de saída/)).toHaveValue(
      '{\n  "type": "object"\n}',
    )

    await userEvent.type(screen.getByLabelText('Prompt inicial'), 'Do it')
    await userEvent.type(screen.getByLabelText('Tools liberadas'), 'Read, Edit')
    await userEvent.type(
      screen.getByLabelText('Tools bloqueadas'),
      'Bash(git push:*)',
    )
    await userEvent.type(screen.getByLabelText('Modelo'), 'opus')
    await userEvent.type(screen.getByLabelText('Máx. de turnos'), '12')
    expect(result.config).toMatchObject({
      prompt: 'Do it',
      allowed_tools: ['Read', 'Edit'],
      disallowed_tools: ['Bash(git push:*)'],
      model: 'opus',
      max_turns: 12,
    })

    await userEvent.clear(screen.getByLabelText('Modelo'))
    await userEvent.clear(screen.getByLabelText('Máx. de turnos'))
    expect(result.config).toMatchObject({
      model: undefined,
      max_turns: undefined,
    })
  })

  it('picks the profile and MCP servers of an agent', async () => {
    const result = setup('agent', {})
    const profile = screen.getByLabelText('Perfil do agente')
    expect(
      screen.getByRole('option', { name: 'fresh (sem credencial)' }),
    ).toBeInTheDocument()
    await userEvent.selectOptions(profile, 'dev')
    await userEvent.click(screen.getByLabelText('sg'))
    expect(result.config).toEqual({
      profile_id: dev.id,
      mcp_server_ids: [sg.id],
    })
    await userEvent.click(screen.getByLabelText('sg'))
    await userEvent.selectOptions(profile, 'Escolha o perfil')
    expect(result.config).toEqual({ profile_id: undefined, mcp_server_ids: [] })
  })

  it('says when there are no MCP servers', () => {
    setup('agent', {}, [])
    expect(screen.getByText(/Nenhum cadastrado/)).toBeInTheDocument()
  })

  it('keeps the last valid output schema', async () => {
    const result = setup('agent', {})
    const schema = screen.getByLabelText(/Schema de saída/)
    expect(schema).toHaveValue('')

    await userEvent.type(schema, '[[')
    expect(
      screen.getByText('JSON inválido: precisa ser um objeto.'),
    ).toBeInTheDocument()
    expect(result.config).toEqual({})

    await userEvent.clear(schema)
    await userEvent.type(schema, '{{"type":"object"}')
    expect(screen.queryByText(/JSON inválido/)).not.toBeInTheDocument()
    expect(result.config).toEqual({ output_schema: { type: 'object' } })

    await userEvent.clear(schema)
    expect(result.config).toEqual({ output_schema: undefined })
  })

  it('configures a review', async () => {
    const result = setup('review', {
      prompt: 'Check',
      max_rounds: 3,
      allowed_tools: ['Read'],
    })
    expect(screen.getByLabelText('Tools liberadas')).toHaveValue('Read')
    const rounds = screen.getByLabelText('Máx. de rodadas')
    await userEvent.clear(rounds)
    expect(result.config).toMatchObject({ max_rounds: undefined })
    await userEvent.type(rounds, '5')
    await userEvent.selectOptions(
      screen.getByLabelText('Perfil do agente'),
      'dev',
    )
    await userEvent.type(screen.getByLabelText('Prompt de review'), '!')
    expect(result.config).toMatchObject({
      prompt: 'Check!',
      max_rounds: 5,
      profile_id: dev.id,
    })
  })

  it('starts a review from an empty config', () => {
    setup('review', {})
    expect(screen.getByLabelText('Prompt de review')).toHaveValue('')
    expect(screen.getByLabelText('Máx. de rodadas')).toHaveValue(null)
  })

  it('configures a card move', async () => {
    const result = setup('move_card', {})
    await userEvent.selectOptions(
      screen.getByLabelText('Baia de destino'),
      'Todo',
    )
    expect(result.config).toEqual({ lane_id: todo.id })
    await userEvent.selectOptions(
      screen.getByLabelText('Baia de destino'),
      'Escolha a baia',
    )
    expect(result.config).toEqual({})
  })

  it('has nothing to configure on the end', () => {
    setup('end', {})
    expect(screen.getByText(/Não tem configuração/)).toBeInTheDocument()
  })
})
