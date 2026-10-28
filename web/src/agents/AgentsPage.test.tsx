import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { agentProfile, createFakeBackend, mcpServer } from '../test/fakeBackend'
import { renderWithApi } from '../test/render'
import { AgentsPage } from './AgentsPage'

describe('AgentsPage', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows empty lists and load failures', async () => {
    renderWithApi(<AgentsPage />, createFakeBackend())
    expect(
      await screen.findByText('Nenhum perfil cadastrado.'),
    ).toBeInTheDocument()
    expect(screen.getByText('Nenhum servidor cadastrado.')).toBeInTheDocument()

    const failing = createFakeBackend()
    failing.failNext('GET /agent-profiles', 500)
    failing.failNext('GET /mcp-servers', 500)
    renderWithApi(<AgentsPage />, failing)
    await waitFor(() => {
      expect(screen.getAllByRole('alert')).toHaveLength(2)
    })
  })

  it('lists profiles with their credential state', async () => {
    const backend = createFakeBackend({
      agentProfiles: [
        agentProfile('dev'),
        agentProfile('ci', { credential_kind: 'api_key' }),
        agentProfile('new', { ready: false, credential_kind: undefined }),
        agentProfile('oc', {
          kind: 'opencode',
          credential_kind: 'opencode_auth',
        }),
      ],
    })
    renderWithApi(<AgentsPage />, backend)
    expect(await screen.findByText('Pronto (token)')).toBeInTheDocument()
    expect(screen.getByText('Pronto (API key)')).toBeInTheDocument()
    expect(
      screen.getByText('Pronto (credenciais do opencode)'),
    ).toBeInTheDocument()
    expect(screen.getByText('· opencode')).toBeInTheDocument()
    expect(screen.getByText('Sem credencial')).toBeInTheDocument()

    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    await userEvent.click(screen.getByRole('button', { name: 'Excluir new' }))
    expect(backend.state.agentProfiles).toHaveLength(4)
    confirm.mockReturnValue(true)
    await userEvent.click(screen.getByRole('button', { name: 'Excluir new' }))
    await waitFor(() => {
      expect(screen.queryByText('Sem credencial')).not.toBeInTheDocument()
    })
  })

  it('creates a profile', async () => {
    const backend = createFakeBackend()
    renderWithApi(<AgentsPage />, backend)
    await userEvent.click(screen.getByRole('button', { name: 'Novo perfil' }))
    const dialog = screen.getByRole('dialog', { name: 'Novo perfil' })
    await userEvent.click(within(dialog).getByRole('button', { name: 'Criar' }))
    expect(await within(dialog).findByRole('alert')).toBeInTheDocument()
    await userEvent.type(within(dialog).getByLabelText('Nome'), 'dev')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Criar' }))
    expect(await screen.findByText('Sem credencial')).toBeInTheDocument()
    expect(backend.state.agentProfiles[0].kind).toBe('claude')

    await userEvent.click(screen.getByRole('button', { name: 'Novo perfil' }))
    const second = screen.getByRole('dialog', { name: 'Novo perfil' })
    await userEvent.type(within(second).getByLabelText('Nome'), 'oc')
    await userEvent.selectOptions(
      within(second).getByLabelText('Agente'),
      'opencode',
    )
    await userEvent.selectOptions(
      within(second).getByLabelText('Agente'),
      'claude',
    )
    await userEvent.selectOptions(
      within(second).getByLabelText('Agente'),
      'opencode',
    )
    await userEvent.click(within(second).getByRole('button', { name: 'Criar' }))
    expect(await screen.findByText('· opencode')).toBeInTheDocument()
    expect(backend.state.agentProfiles[1].kind).toBe('opencode')

    await userEvent.click(screen.getByRole('button', { name: 'Novo perfil' }))
    await userEvent.click(screen.getByRole('button', { name: 'Cancelar' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('creates a stdio MCP server with secrets', async () => {
    const backend = createFakeBackend()
    renderWithApi(<AgentsPage />, backend)
    await userEvent.click(screen.getByRole('button', { name: 'Novo servidor' }))
    const dialog = screen.getByRole('dialog', { name: 'Novo servidor MCP' })
    await userEvent.type(within(dialog).getByLabelText('Nome'), 'fs')
    await userEvent.type(within(dialog).getByLabelText('Comando'), 'npx')
    await userEvent.type(
      within(dialog).getByLabelText('Argumentos (um por linha)'),
      'server{enter}--ro',
    )
    await userEvent.type(
      within(dialog).getByLabelText(/Variáveis de ambiente/),
      'A=1',
    )
    await userEvent.type(
      within(dialog).getByLabelText(/Segredos: variáveis/),
      'TOKEN=t',
    )
    await userEvent.click(
      within(dialog).getByRole('button', { name: 'Salvar' }),
    )

    expect(
      await screen.findByText('npx server --ro · segredos: TOKEN'),
    ).toBeInTheDocument()
    expect(backend.state.mcpServers[0]).toMatchObject({
      env: { A: '1' },
      secret_names: ['TOKEN'],
    })
  })

  it('creates an http MCP server', async () => {
    const backend = createFakeBackend()
    renderWithApi(<AgentsPage />, backend)
    await userEvent.click(screen.getByRole('button', { name: 'Novo servidor' }))
    const dialog = screen.getByRole('dialog', { name: 'Novo servidor MCP' })
    await userEvent.type(within(dialog).getByLabelText('Nome'), 'sg')
    await userEvent.selectOptions(
      within(dialog).getByLabelText('Transporte'),
      'http',
    )
    await userEvent.type(within(dialog).getByLabelText('URL'), 'https://sg/mcp')
    await userEvent.type(within(dialog).getByLabelText(/Headers/), 'X=1')
    await userEvent.type(
      within(dialog).getByLabelText(/Segredos: headers/),
      'Authorization=Bearer t',
    )
    await userEvent.click(
      within(dialog).getByRole('button', { name: 'Salvar' }),
    )
    expect(
      await screen.findByText('https://sg/mcp · segredos: Authorization'),
    ).toBeInTheDocument()
    expect(backend.state.mcpServers[0]).toMatchObject({
      transport: 'http',
      headers: { X: '1' },
    })
  })

  it('edits an MCP server keeping or replacing its secrets', async () => {
    const backend = createFakeBackend({
      mcpServers: [
        mcpServer('sg', {
          transport: 'http',
          url: 'https://old',
          headers: { X: '1' },
          secret_names: ['Authorization'],
        }),
      ],
    })
    renderWithApi(<AgentsPage />, backend)
    await userEvent.click(
      await screen.findByRole('button', { name: 'Editar sg' }),
    )
    let dialog = screen.getByRole('dialog', { name: 'Editar servidor MCP' })
    expect(within(dialog).getByLabelText(/Headers/)).toHaveValue('X=1')
    expect(
      within(dialog).getByText('(atuais: Authorization)'),
    ).toBeInTheDocument()
    expect(within(dialog).queryByLabelText(/Segredos/)).not.toBeInTheDocument()
    const url = within(dialog).getByLabelText('URL')
    await userEvent.clear(url)
    await userEvent.type(url, 'https://new')
    await userEvent.click(
      within(dialog).getByRole('button', { name: 'Salvar' }),
    )
    expect(
      await screen.findByText('https://new · segredos: Authorization'),
    ).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Editar sg' }))
    dialog = screen.getByRole('dialog', { name: 'Editar servidor MCP' })
    await userEvent.click(within(dialog).getByLabelText(/Substituir segredos/))
    await userEvent.click(
      within(dialog).getByRole('button', { name: 'Salvar' }),
    )
    expect(await screen.findByText('https://new')).toBeInTheDocument()
  })

  it('edits a stdio server without secrets and deletes servers', async () => {
    const backend = createFakeBackend({
      mcpServers: [mcpServer('fs', { env: { A: '1' } })],
    })
    renderWithApi(<AgentsPage />, backend)
    await userEvent.click(
      await screen.findByRole('button', { name: 'Editar fs' }),
    )
    const dialog = screen.getByRole('dialog', { name: 'Editar servidor MCP' })
    expect(within(dialog).getByLabelText(/Variáveis de ambiente/)).toHaveValue(
      'A=1',
    )
    expect(within(dialog).queryByText(/atuais/)).not.toBeInTheDocument()
    await userEvent.click(
      within(dialog).getByRole('button', { name: 'Cancelar' }),
    )

    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    await userEvent.click(screen.getByRole('button', { name: 'Excluir fs' }))
    expect(backend.state.mcpServers).toHaveLength(1)
    confirm.mockReturnValue(true)
    await userEvent.click(screen.getByRole('button', { name: 'Excluir fs' }))
    expect(
      await screen.findByText('Nenhum servidor cadastrado.'),
    ).toBeInTheDocument()
  })

  it('tests a new MCP server as typed', async () => {
    const backend = createFakeBackend()
    renderWithApi(<AgentsPage />, backend)
    await userEvent.click(screen.getByRole('button', { name: 'Novo servidor' }))
    const dialog = screen.getByRole('dialog', { name: 'Novo servidor MCP' })
    const testButton = within(dialog).getByRole('button', { name: 'Testar' })

    await userEvent.click(testButton)
    expect(await within(dialog).findByRole('alert')).toBeInTheDocument()
    expect(backend.state.mcpTests).toHaveLength(0)

    await userEvent.type(within(dialog).getByLabelText('Nome'), 'sg')
    await userEvent.selectOptions(
      within(dialog).getByLabelText('Transporte'),
      'http',
    )
    expect(
      within(dialog).queryByText(/fora do sandbox/),
    ).not.toBeInTheDocument()
    await userEvent.type(within(dialog).getByLabelText('URL'), 'https://sg/mcp')
    await userEvent.type(
      within(dialog).getByLabelText(/Segredos: headers/),
      'Authorization=token t',
    )
    await userEvent.type(
      within(dialog).getByLabelText(/Tool para testar/),
      ' list_repos ',
    )
    const args = within(dialog).getByLabelText(/Argumentos da tool/)
    await userEvent.type(args, '[[1]')
    expect(
      within(dialog).getByText('JSON inválido: precisa ser um objeto.'),
    ).toBeInTheDocument()
    expect(testButton).toBeDisabled()
    await userEvent.clear(args)
    await userEvent.type(args, '{{')
    expect(testButton).toBeDisabled()
    await userEvent.clear(args)
    await userEvent.type(args, '{{"first":1}')
    backend.state.mcpTest = {
      ok: true,
      tools: ['list_repos', 'search'],
      tool_result: 'chat-web',
    }
    await userEvent.click(testButton)

    const outcome = await within(dialog).findByRole('status')
    expect(outcome).toHaveTextContent('Funcionou.')
    expect(outcome).toHaveTextContent('Tools: list_repos, search')
    expect(outcome).toHaveTextContent('chat-web')
    expect(backend.state.mcpTests[0]).toEqual({
      server: {
        name: 'sg',
        transport: 'http',
        url: 'https://sg/mcp',
        headers: {},
        secrets: { Authorization: 'token t' },
      },
      tool: 'list_repos',
      arguments: { first: 1 },
    })
    expect(backend.state.mcpServers).toHaveLength(0)
  })

  it('shows why a test failed', async () => {
    const backend = createFakeBackend({
      mcpTest: { ok: false, tools: [], error: 'não conectou: 401' },
    })
    renderWithApi(<AgentsPage />, backend)
    await userEvent.click(screen.getByRole('button', { name: 'Novo servidor' }))
    const dialog = screen.getByRole('dialog', { name: 'Novo servidor MCP' })
    await userEvent.type(within(dialog).getByLabelText('Nome'), 'fs')
    expect(within(dialog).getByText(/fora do sandbox/)).toBeInTheDocument()
    const testButton = within(dialog).getByRole('button', { name: 'Testar' })

    await userEvent.click(testButton)
    const outcome = await within(dialog).findByRole('status')
    expect(outcome).toHaveTextContent('Falhou: não conectou: 401')
    expect(outcome).not.toHaveTextContent('Tools')

    backend.state.mcpTest = {
      ok: false,
      tools: ['search'],
      error: 'a tool search devolveu um erro',
    }
    await userEvent.click(testButton)
    await waitFor(() => {
      expect(outcome).toHaveTextContent('Tools: search')
    })

    backend.state.mcpTest = { ok: false, tools: [] }
    await userEvent.click(testButton)
    await waitFor(() => {
      expect(within(dialog).getByRole('status')).toHaveTextContent(/^Falhou:$/)
    })

    backend.state.mcpTest = { ok: true, tools: [] }
    await userEvent.click(testButton)
    expect(
      await within(dialog).findByText('Tools: nenhuma'),
    ).toBeInTheDocument()
  })

  it('tests an edited server with its stored secrets', async () => {
    const sg = mcpServer('sg', {
      transport: 'http',
      url: 'https://sg/mcp',
      secret_names: ['Authorization'],
    })
    const backend = createFakeBackend({ mcpServers: [sg] })
    renderWithApi(<AgentsPage />, backend)
    await userEvent.click(
      await screen.findByRole('button', { name: 'Editar sg' }),
    )
    const dialog = screen.getByRole('dialog', { name: 'Editar servidor MCP' })
    await userEvent.click(
      within(dialog).getByRole('button', { name: 'Testar' }),
    )
    await within(dialog).findByRole('status')
    expect(backend.state.mcpTests[0]).toEqual({
      server: {
        name: 'sg',
        transport: 'http',
        url: 'https://sg/mcp',
        headers: {},
      },
      server_id: sg.id,
    })
  })
})
