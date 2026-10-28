import { useState } from 'react'

import type {
  McpServer,
  McpServerInput,
  McpServerTestResult,
} from '../api/client'
import {
  useCreateMcpServer,
  useTestMcpServer,
  useUpdateMcpServer,
} from '../api/queries'
import { Button } from '../components/Button'
import { ErrorMessage } from '../components/ErrorMessage'
import { SelectField, TextArea, TextField } from '../components/Field'
import { Modal } from '../components/Modal'
import { formatPairs, parseLines, parsePairs } from './pairs'

interface Props {
  server?: McpServer
  onClose: () => void
}

/**
 * Creates or edits an MCP server. Secret values are never shown: when
 * editing, they are kept unless replaced. The server can be tried out as
 * typed before saving.
 */
export function McpServerForm({ server, onClose }: Props) {
  const create = useCreateMcpServer()
  const update = useUpdateMcpServer()
  const save = server ? update : create
  const [name, setName] = useState(server?.name ?? '')
  const [transport, setTransport] = useState<McpServer['transport']>(
    server?.transport ?? 'stdio',
  )
  const [command, setCommand] = useState(server?.command ?? '')
  const [args, setArgs] = useState((server?.args ?? []).join('\n'))
  const [url, setUrl] = useState(server?.url ?? '')
  const [plain, setPlain] = useState(
    formatPairs(
      server ? (transport === 'http' ? server.headers : server.env) : {},
    ),
  )
  const [replaceSecrets, setReplaceSecrets] = useState(!server)
  const [secrets, setSecrets] = useState('')
  const [tool, setTool] = useState('')
  const [toolArgs, setToolArgs] = useState('')
  const test = useTestMcpServer()
  const stdio = transport === 'stdio'
  const toolArguments = parseArguments(toolArgs)

  function input(): McpServerInput {
    const body: McpServerInput = stdio
      ? {
          name,
          transport,
          command,
          args: parseLines(args),
          env: parsePairs(plain),
        }
      : { name, transport, url, headers: parsePairs(plain) }
    if (replaceSecrets) {
      body.secrets = parsePairs(secrets)
    }
    return body
  }

  return (
    <Modal
      title={server ? 'Editar servidor MCP' : 'Novo servidor MCP'}
      onClose={onClose}
    >
      <form
        className="flex flex-col gap-3"
        onSubmit={(e) => {
          e.preventDefault()
          const body = input()
          const done = { onSuccess: onClose }
          if (server) {
            update.mutate({ id: server.id, ...body }, done)
          } else {
            create.mutate(body, done)
          }
        }}
      >
        <div className="grid grid-cols-2 gap-3">
          <TextField label="Nome" value={name} onChange={setName} autoFocus />
          <SelectField
            label="Transporte"
            value={transport}
            onChange={(t) => {
              setTransport(t as McpServer['transport'])
            }}
            options={[
              { value: 'stdio', label: 'stdio (comando local)' },
              { value: 'http', label: 'http' },
            ]}
          />
        </div>
        {stdio ? (
          <>
            <TextField
              label="Comando"
              value={command}
              onChange={setCommand}
              placeholder="npx"
            />
            <TextArea
              label="Argumentos (um por linha)"
              rows={3}
              mono
              value={args}
              onChange={setArgs}
            />
          </>
        ) : (
          <TextField
            label="URL"
            value={url}
            onChange={setUrl}
            placeholder="https://…/mcp"
          />
        )}
        <TextArea
          label={
            stdio
              ? 'Variáveis de ambiente (CHAVE=valor)'
              : 'Headers (Nome=valor)'
          }
          rows={3}
          mono
          value={plain}
          onChange={setPlain}
        />
        {server && (
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={replaceSecrets}
              onChange={(e) => {
                setReplaceSecrets(e.target.checked)
              }}
            />
            Substituir segredos
            {server.secret_names.length > 0 && (
              <span className="text-slate-500">
                (atuais: {server.secret_names.join(', ')})
              </span>
            )}
          </label>
        )}
        {replaceSecrets && (
          <TextArea
            label={
              stdio
                ? 'Segredos: variáveis (CHAVE=valor)'
                : 'Segredos: headers (Nome=valor)'
            }
            hint="Guardados criptografados e nunca exibidos de novo."
            rows={3}
            mono
            value={secrets}
            onChange={setSecrets}
          />
        )}
        <fieldset className="flex flex-col gap-3 rounded-md border border-slate-200 p-3">
          <legend className="px-1 text-xs font-medium text-slate-600">
            Teste
          </legend>
          <p className="text-xs text-slate-500">
            Conecta e lista as tools. Alguns servidores só conferem o token
            quando uma tool é chamada: informe uma de leitura, porque ela roda
            de verdade.
            {stdio &&
              ' O comando roda no servidor do board, fora do sandbox dos agentes.'}
          </p>
          <div className="grid grid-cols-2 gap-3">
            <TextField
              label="Tool para testar (opcional)"
              value={tool}
              onChange={setTool}
              placeholder="list_repos"
            />
            <TextArea
              label="Argumentos da tool (JSON)"
              rows={1}
              mono
              value={toolArgs}
              onChange={setToolArgs}
              hint={
                !toolArguments.valid
                  ? 'JSON inválido: precisa ser um objeto.'
                  : undefined
              }
            />
          </div>
          <McpTestOutcome result={test.data} />
          <ErrorMessage error={test.error} />
        </fieldset>
        <ErrorMessage error={save.error} />
        <div className="flex justify-end gap-2">
          <Button
            className="mr-auto"
            disabled={test.isPending || !toolArguments.valid}
            onClick={() => {
              test.mutate({
                server: input(),
                server_id: server?.id,
                tool: tool.trim() || undefined,
                arguments: toolArguments.value,
              })
            }}
          >
            Testar
          </Button>
          <Button onClick={onClose}>Cancelar</Button>
          <Button type="submit" variant="primary" disabled={save.isPending}>
            Salvar
          </Button>
        </div>
      </form>
    </Modal>
  )
}

/** The tool arguments typed: none when blank, an object when valid. */
function parseArguments(text: string): {
  valid: boolean
  value?: Record<string, unknown>
} {
  if (!text.trim()) {
    return { valid: true }
  }
  try {
    const value: unknown = JSON.parse(text)
    if (typeof value === 'object' && value !== null && !Array.isArray(value)) {
      return { valid: true, value: value as Record<string, unknown> }
    }
  } catch {
    // shown as invalid below
  }
  return { valid: false }
}

/** What the last test found. */
function McpTestOutcome({ result }: { result?: McpServerTestResult }) {
  if (!result) {
    return null
  }
  const tools = result.tools.length > 0 ? result.tools.join(', ') : 'nenhuma'
  return (
    <div
      role="status"
      className={`flex flex-col gap-1 rounded-md px-3 py-2 text-sm ${result.ok ? 'bg-green-50 text-green-800' : 'bg-red-50 text-red-700'}`}
    >
      <p className="whitespace-pre-wrap">
        {result.ok ? 'Funcionou.' : `Falhou: ${result.error ?? ''}`}
      </p>
      {(result.ok || result.tools.length > 0) && <p>Tools: {tools}</p>}
      {result.tool_result && (
        <pre className="max-h-40 overflow-auto whitespace-pre-wrap font-mono text-xs">
          {result.tool_result}
        </pre>
      )}
    </div>
  )
}
