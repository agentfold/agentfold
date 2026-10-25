import { useState } from 'react'

import type { McpServer, McpServerInput } from '../api/client'
import { useCreateMcpServer, useUpdateMcpServer } from '../api/queries'
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
 * editing, they are kept unless replaced.
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
  const stdio = transport === 'stdio'

  return (
    <Modal
      title={server ? 'Editar servidor MCP' : 'Novo servidor MCP'}
      onClose={onClose}
    >
      <form
        className="flex flex-col gap-3"
        onSubmit={(e) => {
          e.preventDefault()
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
        <ErrorMessage error={save.error} />
        <div className="flex justify-end gap-2">
          <Button onClick={onClose}>Cancelar</Button>
          <Button type="submit" variant="primary" disabled={save.isPending}>
            Salvar
          </Button>
        </div>
      </form>
    </Modal>
  )
}
