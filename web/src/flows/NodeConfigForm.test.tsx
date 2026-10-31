import { render, screen, within } from '@testing-library/react'
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
const task = cardType('task', { initial_lane_id: todo.id })

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
      screen.getByRole('option', {
        name: 'fresh · Claude Code (sem credencial)',
      }),
    ).toBeInTheDocument()
    await userEvent.selectOptions(profile, dev.id)
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

  it('turns the gateway and read-only mode on and off', async () => {
    const result = setup('agent', { prompt: 'Check' })
    expect(screen.queryByLabelText('Máx. de rodadas')).not.toBeInTheDocument()
    await userEvent.click(screen.getByLabelText(/Somente leitura/))
    await userEvent.click(screen.getByLabelText(/Gateway/))
    expect(result.config).toMatchObject({
      read_only: true,
      gateway: true,
      max_rounds: 3,
    })
    const rounds = screen.getByLabelText('Máx. de rodadas')
    await userEvent.clear(rounds)
    expect(result.config).toMatchObject({ max_rounds: undefined })
    await userEvent.type(rounds, '5')
    expect(result.config).toMatchObject({ max_rounds: 5 })

    await userEvent.click(screen.getByLabelText(/Somente leitura/))
    await userEvent.click(screen.getByLabelText(/Gateway/))
    expect(result.config).toEqual({
      prompt: 'Check',
      read_only: undefined,
      gateway: undefined,
      max_rounds: undefined,
    })
  })

  it('keeps the rounds of a gateway turned on again', async () => {
    const result = setup('agent', { max_rounds: 4 })
    await userEvent.click(screen.getByLabelText(/Gateway/))
    expect(result.config).toEqual({ gateway: true, max_rounds: 4 })
  })

  it('lists the artifacts to collect', async () => {
    const result = setup('agent', {})
    await userEvent.click(
      screen.getByRole('button', { name: 'Adicionar artefato' }),
    )
    await userEvent.click(
      screen.getByRole('button', { name: 'Adicionar artefato' }),
    )
    const first = screen.getByRole('group', { name: 'Artefato 1' })
    const second = screen.getByRole('group', { name: 'Artefato 2' })
    await userEvent.type(within(first).getByLabelText('Nome'), 'Plano')
    await userEvent.type(within(first).getByLabelText('Arquivo'), 'plan.md')
    expect(result.config).toEqual({
      artifacts: [
        { name: 'Plano', path: 'plan.md' },
        { name: '', path: '' },
      ],
    })
    await userEvent.click(
      within(second).getByRole('button', { name: 'Remover' }),
    )
    expect(result.config).toEqual({
      artifacts: [{ name: 'Plano', path: 'plan.md' }],
    })
    await userEvent.click(screen.getByRole('button', { name: 'Remover' }))
    expect(result.config).toEqual({ artifacts: undefined })
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

  it('tells the create cards node is retired', () => {
    setup('create_cards', {})
    expect(screen.getByText(/Este nó foi aposentado/)).toBeInTheDocument()
  })

  it('configures the wait for the children', async () => {
    const result = setup('wait_children', {})
    expect(screen.getByLabelText('Modo')).toHaveValue('all_succeed')
    await userEvent.selectOptions(screen.getByLabelText('Modo'), 'all_finished')
    await userEvent.type(screen.getByLabelText('Chave'), 'dev')
    await userEvent.type(screen.getByLabelText('Valor'), 'ok')
    expect(result.config).toEqual({
      mode: 'all_finished',
      property_key: 'dev',
      property_value: 'ok',
    })
    await userEvent.clear(screen.getByLabelText('Chave'))
    await userEvent.clear(screen.getByLabelText('Valor'))
    expect(result.config).toEqual({ mode: 'all_finished' })
  })

  it('tells that an old wait still uses the done lane', async () => {
    const result = setup('wait_children', { done_lane_id: todo.id })
    expect(screen.getByText(/filho na baia Todo/)).toBeInTheDocument()
    await userEvent.type(screen.getByLabelText('Chave'), 'd')
    expect(result.config).toEqual({ property_key: 'd' })
    expect(screen.queryByText(/filho na baia/)).not.toBeInTheDocument()
  })

  it('tells when the done lane of an old wait is gone', () => {
    setup('wait_children', { done_lane_id: 'gone' })
    expect(screen.getByText(/filho na baia removida/)).toBeInTheDocument()
  })

  it('names the artifacts a human reviews', async () => {
    const result = setup('human_review', {})
    const names = screen.getByLabelText('Artefatos revisados')
    await userEvent.type(names, 'Plano, Riscos')
    expect(result.config).toEqual({ artifacts: ['Plano', 'Riscos'] })
    await userEvent.clear(names)
    expect(result.config).toEqual({})
  })

  it('grants the board tools', async () => {
    const result = setup('agent', { prompt: 'p', allowed_tools: ['Read'] })
    expect(
      screen.queryByText('Tipos dos cards criados'),
    ).not.toBeInTheDocument()
    await userEvent.click(screen.getByLabelText('Criar cards filhos'))
    await userEvent.click(screen.getByLabelText('Ver os cards filhos'))
    expect(result.config).toMatchObject({
      allowed_tools: [
        'Read',
        'mcp__orquestrator__create_cards',
        'mcp__orquestrator__list_cards',
      ],
    })
    // The text field lists the other tools and keeps the granted ones.
    const tools = screen.getByLabelText('Tools liberadas')
    expect(tools).toHaveValue('Read')
    await userEvent.type(tools, ', Edit')
    expect(result.config).toMatchObject({
      allowed_tools: [
        'Read',
        'Edit',
        'mcp__orquestrator__create_cards',
        'mcp__orquestrator__list_cards',
      ],
    })
    await userEvent.click(screen.getByLabelText('Criar cards filhos'))
    expect(result.config).toMatchObject({
      allowed_tools: ['Read', 'Edit', 'mcp__orquestrator__list_cards'],
    })
  })

  it('picks the card types the agent creates', async () => {
    const result = setup('agent', {
      prompt: 'p',
      allowed_tools: ['mcp__orquestrator__create_cards'],
    })
    const types = screen.getByRole('group', { name: 'Tipos dos cards criados' })
    expect(within(types).getByText('sem baia inicial')).toBeInTheDocument()
    await userEvent.click(within(types).getByLabelText(/bug/))
    await userEvent.click(within(types).getByLabelText('task'))
    expect(result.config).toMatchObject({ card_type_ids: [bug.id, task.id] })
    await userEvent.click(within(types).getByLabelText(/bug/))
    expect(result.config).toMatchObject({ card_type_ids: [task.id] })
  })

  it('sets a property', async () => {
    const result = setup('set_property', {})
    await userEvent.type(screen.getByLabelText('Chave'), 'dev')
    await userEvent.type(screen.getByLabelText('Valor'), 'ok')
    expect(result.config).toEqual({ key: 'dev', value: 'ok' })
    await userEvent.clear(screen.getByLabelText('Chave'))
    await userEvent.clear(screen.getByLabelText('Valor'))
    expect(result.config).toEqual({ key: undefined, value: undefined })
  })

  it('checks a property, optionally waiting', async () => {
    const result = setup('check_property', { key: 'dev', value: 'ok' })
    await userEvent.click(screen.getByLabelText(/Esperar/))
    expect(result.config).toEqual({ key: 'dev', value: 'ok', wait: true })
    await userEvent.click(screen.getByLabelText(/Esperar/))
    expect(result.config).toEqual({ key: 'dev', value: 'ok', wait: undefined })
  })

  it('has nothing to configure on the end', () => {
    setup('end', {})
    expect(screen.getByText(/Não tem configuração/)).toBeInTheDocument()
  })

  it('configures an HTTP request', async () => {
    const result = setup('http_request', { method: 'POST', url: '' })
    await userEvent.selectOptions(screen.getByLabelText('Método'), 'PUT')
    await userEvent.type(
      screen.getByLabelText('URL'),
      'https://x/{{{{card.id}}',
    )
    await userEvent.type(screen.getByLabelText('Body (JSON, opcional)'), 'b')
    await userEvent.type(screen.getByLabelText('Timeout (segundos)'), '10')
    await userEvent.type(
      screen.getByLabelText('Status de sucesso'),
      '200, x, 409',
    )
    await userEvent.type(screen.getByLabelText('Tentativas'), '3')
    await userEvent.type(screen.getByLabelText('Intervalo (s)'), '5')
    await userEvent.selectOptions(
      screen.getByLabelText('Backoff'),
      'exponential',
    )
    expect(result.config).toEqual({
      method: 'PUT',
      url: 'https://x/{{card.id}}',
      body: 'b',
      timeout_seconds: 10,
      success_statuses: [200, 409],
      max_attempts: 3,
      retry_delay_seconds: 5,
      backoff: 'exponential',
    })

    await userEvent.clear(screen.getByLabelText('Body (JSON, opcional)'))
    await userEvent.clear(screen.getByLabelText('Status de sucesso'))
    await userEvent.selectOptions(screen.getByLabelText('Backoff'), 'fixed')
    expect(result.config).toMatchObject({
      body: undefined,
      success_statuses: undefined,
      backoff: undefined,
    })

    await userEvent.selectOptions(
      screen.getByLabelText('Modo'),
      'fire_and_forget',
    )
    expect(screen.queryByLabelText('Tentativas')).not.toBeInTheDocument()
    expect(result.config).toMatchObject({
      mode: 'fire_and_forget',
      max_attempts: undefined,
      retry_delay_seconds: undefined,
    })
    await userEvent.selectOptions(screen.getByLabelText('Modo'), 'wait')
    expect(result.config).toMatchObject({ mode: undefined })
  })

  it('edits the output of a Transform as JSON', async () => {
    const result = setup('transform', {})
    const field = screen.getByLabelText('Saída (JSON)')
    expect(field).toHaveValue('{}')
    await userEvent.clear(field)
    await userEvent.type(field, '{{"total": "= 1 + 2"')
    expect(screen.getByText('JSON inválido.')).toBeInTheDocument()
    expect(result.config).toEqual({})
    await userEvent.type(field, '}')
    expect(screen.queryByText('JSON inválido.')).not.toBeInTheDocument()
    expect(result.config).toEqual({ output: { total: '= 1 + 2' } })
  })

  it('edits the cases of a Condition', async () => {
    const result = setup('switch', {})
    await userEvent.click(
      screen.getByRole('button', { name: 'Adicionar caso' }),
    )
    const first = within(screen.getByRole('group', { name: 'Caso 1' }))
    await userEvent.type(first.getByLabelText('Nome'), 'alto')
    await userEvent.type(first.getByLabelText('Condição'), 'input > 1')
    await userEvent.click(
      screen.getByRole('button', { name: 'Adicionar caso' }),
    )
    expect(result.config).toEqual({
      cases: [
        { name: 'alto', expression: 'input > 1' },
        { name: '', expression: '' },
      ],
    })
    const second = within(screen.getByRole('group', { name: 'Caso 2' }))
    await userEvent.type(second.getByLabelText('Nome'), 'b')
    await userEvent.click(first.getByRole('button', { name: 'Remover' }))
    expect(result.config).toEqual({ cases: [{ name: 'b', expression: '' }] })
  })

  it('edits the branches of a Split', async () => {
    const result = setup('split', {})
    await userEvent.click(
      screen.getByRole('button', { name: 'Adicionar ramo' }),
    )
    await userEvent.click(
      screen.getByRole('button', { name: 'Adicionar ramo' }),
    )
    await userEvent.type(
      within(screen.getByRole('group', { name: 'Ramo 1' })).getByLabelText(
        'Nome',
      ),
      'erp',
    )
    expect(result.config).toEqual({ branches: ['erp', ''] })
    await userEvent.click(
      within(screen.getByRole('group', { name: 'Ramo 2' })).getByRole(
        'button',
        { name: 'Remover' },
      ),
    )
    expect(result.config).toEqual({ branches: ['erp'] })
  })

  it('chooses how a Join closes the branches', async () => {
    const result = setup('join', {})
    const mode = screen.getByLabelText('Como juntar')
    expect(mode).toHaveValue('all')
    await userEvent.selectOptions(mode, 'fail_fast')
    expect(result.config).toEqual({ mode: 'all', fail_fast: true })
    await userEvent.selectOptions(mode, 'first')
    expect(result.config).toEqual({ mode: 'first', fail_fast: undefined })
    expect(mode).toHaveValue('first')
    await userEvent.selectOptions(mode, 'all')
    expect(result.config).toEqual({ mode: 'all', fail_fast: undefined })
  })

  it('shows an HTTP request without config', () => {
    setup('http_request', {})
    expect(screen.getByLabelText('URL')).toHaveValue('')
    expect(screen.getByLabelText('Modo')).toHaveValue('wait')
  })

  it('edits the headers of an HTTP request', async () => {
    const result = setup('http_request', {
      method: 'GET',
      url: 'https://x',
      headers: [{ name: 'Authorization', secret: true, sealed: 'AAEC' }],
    })
    const sealed = within(screen.getByRole('group', { name: 'Header 1' }))
    expect(sealed.getByLabelText('Valor')).toHaveAttribute(
      'placeholder',
      'guardado; digite para trocar',
    )
    await userEvent.type(sealed.getByLabelText('Valor'), 'k')
    expect(result.config).toMatchObject({
      headers: [
        { name: 'Authorization', secret: true, value: 'k', sealed: undefined },
      ],
    })
    expect(sealed.getByLabelText('Valor')).toHaveAttribute('placeholder', '')

    await userEvent.click(
      screen.getByRole('button', { name: 'Adicionar header' }),
    )
    const added = within(screen.getByRole('group', { name: 'Header 2' }))
    await userEvent.type(added.getByLabelText('Nome'), 'X')
    await userEvent.click(added.getByLabelText('Secreto'))
    expect(result.config).toMatchObject({
      headers: [{ name: 'Authorization' }, { name: 'X', secret: true }],
    })
    await userEvent.click(added.getByLabelText('Secreto'))
    expect(result.config).toMatchObject({
      headers: [{}, { name: 'X', secret: undefined, sealed: undefined }],
    })

    await userEvent.click(sealed.getByRole('button', { name: 'Remover' }))
    await userEvent.click(screen.getByRole('button', { name: 'Remover' }))
    expect(result.config).toMatchObject({ headers: undefined })
  })

  it('switches a human review to approving the output', async () => {
    const result = setup('human_review', { artifacts: ['Plano'] })
    expect(screen.getByLabelText('O que revisar')).toHaveValue('artifacts')
    await userEvent.selectOptions(
      screen.getByLabelText('O que revisar'),
      'output',
    )
    expect(result.config).toEqual({ subject: 'output' })
    expect(
      screen.queryByLabelText('Artefatos revisados'),
    ).not.toBeInTheDocument()
    expect(screen.getByText(/aprovar o que o nó anterior/)).toBeInTheDocument()
    await userEvent.selectOptions(
      screen.getByLabelText('O que revisar'),
      'artifacts',
    )
    expect(result.config).toEqual({})
  })
})
