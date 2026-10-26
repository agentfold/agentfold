import { useState } from 'react'

import type {
  AgentConfig,
  AgentProfile,
  ArtifactSpec,
  McpServer,
  CardType,
  Lane,
  MoveCardConfig,
  TriggerConfig,
} from '../api/client'
import { Button } from '../components/Button'
import { SelectField, TextArea, TextField } from '../components/Field'
import { formatTools, parseCount, parseSchema, parseTools } from './config'
import type { StepConfig, StepData } from './graph'

interface Props {
  data: StepData
  lanes: Lane[]
  cardTypes: CardType[]
  profiles: AgentProfile[]
  mcpServers: McpServer[]
  onChange: (config: StepConfig) => void
}

/** The settings of the selected node, by kind. */
export function NodeConfigForm({
  data,
  lanes,
  cardTypes,
  profiles,
  mcpServers,
  onChange,
}: Props) {
  switch (data.kind) {
    case 'trigger':
      return (
        <TriggerForm
          config={data.config as TriggerConfig}
          lanes={lanes}
          cardTypes={cardTypes}
          onChange={onChange}
        />
      )
    case 'agent':
      return (
        <AgentForm
          config={data.config as AgentConfig}
          profiles={profiles}
          mcpServers={mcpServers}
          onChange={onChange}
        />
      )
    case 'move_card':
      return (
        <SelectField
          label="Baia de destino"
          value={(data.config as MoveCardConfig).lane_id ?? ''}
          onChange={(laneId) => {
            onChange(laneId ? { lane_id: laneId } : {})
          }}
          options={laneOptions(lanes)}
        />
      )
    case 'end':
      return (
        <p className="text-sm text-slate-500">
          Encerra o fluxo. Não tem configuração.
        </p>
      )
  }
}

function laneOptions(lanes: Lane[]) {
  return [
    { value: '', label: 'Escolha a baia' },
    ...lanes.map((l) => ({ value: l.id, label: l.name })),
  ]
}

interface TriggerFormProps {
  config: TriggerConfig
  lanes: Lane[]
  cardTypes: CardType[]
  onChange: (config: TriggerConfig) => void
}

function TriggerForm({ config, lanes, cardTypes, onChange }: TriggerFormProps) {
  const selected = new Set(config.card_type_ids ?? [])
  return (
    <div className="flex flex-col gap-3">
      <SelectField
        label="Evento"
        value={config.event ?? 'card_created'}
        onChange={(event) => {
          const next = event as TriggerConfig['event']
          onChange(
            next === 'card_created'
              ? { ...config, event: next, lane_id: undefined }
              : { ...config, event: next },
          )
        }}
        options={[
          { value: 'card_created', label: 'Card criado' },
          { value: 'card_entered_lane', label: 'Card entrou numa baia' },
        ]}
      />
      {config.event === 'card_entered_lane' && (
        <SelectField
          label="Baia"
          value={config.lane_id ?? ''}
          onChange={(laneId) => {
            onChange({ ...config, lane_id: laneId || undefined })
          }}
          options={laneOptions(lanes)}
        />
      )}
      <fieldset className="flex flex-col gap-1">
        <legend className="text-xs font-medium text-slate-600">
          Tipos de card
        </legend>
        <p className="text-xs text-slate-500">
          Nenhum marcado: dispara para todos os tipos.
        </p>
        {cardTypes.map((t) => (
          <label key={t.id} className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={selected.has(t.id)}
              onChange={(e) => {
                const ids = e.target.checked
                  ? [...selected, t.id]
                  : [...selected].filter((id) => id !== t.id)
                onChange({ ...config, card_type_ids: ids })
              }}
            />
            {t.name}
          </label>
        ))}
      </fieldset>
    </div>
  )
}

interface AgentFormProps {
  config: AgentConfig
  profiles: AgentProfile[]
  mcpServers: McpServer[]
  onChange: (config: AgentConfig) => void
}

function AgentForm({ config, profiles, mcpServers, onChange }: AgentFormProps) {
  const selected = new Set(config.mcp_server_ids ?? [])
  return (
    <div className="flex flex-col gap-3">
      <ProfileField config={config} profiles={profiles} onChange={onChange} />
      <fieldset className="flex flex-col gap-1">
        <legend className="text-xs font-medium text-slate-600">
          Servidores MCP
        </legend>
        {mcpServers.length === 0 && (
          <p className="text-xs text-slate-500">
            Nenhum cadastrado (aba Agentes).
          </p>
        )}
        {mcpServers.map((m) => (
          <label key={m.id} className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={selected.has(m.id)}
              onChange={(e) => {
                const ids = e.target.checked
                  ? [...selected, m.id]
                  : [...selected].filter((id) => id !== m.id)
                onChange({ ...config, mcp_server_ids: ids })
              }}
            />
            {m.name}
          </label>
        ))}
      </fieldset>
      <TextArea
        label="Prompt inicial"
        rows={6}
        value={config.prompt ?? ''}
        onChange={(prompt) => {
          onChange({ ...config, prompt })
        }}
      />
      <ToolFields config={config} onChange={onChange} />
      <div className="grid grid-cols-2 gap-3">
        <TextField
          label="Modelo"
          value={config.model ?? ''}
          placeholder="padrão"
          onChange={(model) => {
            onChange({ ...config, model: model || undefined })
          }}
        />
        <TextField
          label="Máx. de turnos"
          type="number"
          value={config.max_turns === undefined ? '' : String(config.max_turns)}
          onChange={(text) => {
            onChange({ ...config, max_turns: parseCount(text) })
          }}
        />
      </div>
      <SchemaField
        schema={config.output_schema}
        onChange={(output_schema) => {
          onChange({ ...config, output_schema })
        }}
      />
      <Toggle
        label="Somente leitura"
        hint="Bloqueia a edição de arquivos, como num revisor."
        checked={config.read_only ?? false}
        onChange={(read_only) => {
          onChange({ ...config, read_only: read_only || undefined })
        }}
      />
      <Toggle
        label="Gateway"
        hint="O agente decide com approve ou reject; as saídas passam a ser aprovado, rejeitado, limite de rodadas e falhou."
        checked={config.gateway ?? false}
        onChange={(gateway) => {
          onChange(
            gateway
              ? { ...config, gateway, max_rounds: config.max_rounds ?? 3 }
              : { ...config, gateway: undefined, max_rounds: undefined },
          )
        }}
      />
      {config.gateway && (
        <TextField
          label="Máx. de rodadas"
          type="number"
          hint="Depois disso a rejeição sai por “limite de rodadas”."
          value={
            config.max_rounds === undefined ? '' : String(config.max_rounds)
          }
          onChange={(text) => {
            onChange({ ...config, max_rounds: parseCount(text) })
          }}
        />
      )}
      <ArtifactsField
        artifacts={config.artifacts ?? []}
        onChange={(artifacts) => {
          onChange({
            ...config,
            artifacts: artifacts.length > 0 ? artifacts : undefined,
          })
        }}
      />
    </div>
  )
}

interface ToggleProps {
  label: string
  hint: string
  checked: boolean
  onChange: (checked: boolean) => void
}

function Toggle({ label, hint, checked, onChange }: ToggleProps) {
  return (
    <label className="flex items-start gap-2 text-sm">
      <input
        type="checkbox"
        className="mt-1"
        checked={checked}
        onChange={(e) => {
          onChange(e.target.checked)
        }}
      />
      <span className="flex flex-col">
        <span className="font-medium">{label}</span>
        <span className="text-xs text-slate-500">{hint}</span>
      </span>
    </label>
  )
}

interface ArtifactsFieldProps {
  artifacts: ArtifactSpec[]
  onChange: (artifacts: ArtifactSpec[]) => void
}

/** The files the node collects into the card when it finishes. */
function ArtifactsField({ artifacts, onChange }: ArtifactsFieldProps) {
  const set = (i: number, change: Partial<ArtifactSpec>) => {
    onChange(artifacts.map((a, j) => (j === i ? { ...a, ...change } : a)))
  }
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="text-xs font-medium text-slate-600">Artefatos</legend>
      <p className="text-xs text-slate-500">
        Arquivos do workspace que viram artefatos do card quando o agente
        termina. Diga no prompt onde salvá-los.
      </p>
      {artifacts.map((a, i) => (
        <div
          key={i}
          role="group"
          aria-label={`Artefato ${String(i + 1)}`}
          className="grid grid-cols-[1fr_1fr_auto] items-end gap-2"
        >
          <TextField
            label="Nome"
            placeholder="Plano"
            value={a.name}
            onChange={(name) => {
              set(i, { name })
            }}
          />
          <TextField
            label="Arquivo"
            placeholder="plan.md"
            value={a.path}
            onChange={(path) => {
              set(i, { path })
            }}
          />
          <Button
            variant="ghost"
            onClick={() => {
              onChange(artifacts.filter((_, j) => j !== i))
            }}
          >
            Remover
          </Button>
        </div>
      ))}
      <Button
        variant="ghost"
        className="self-start"
        onClick={() => {
          onChange([...artifacts, { name: '', path: '' }])
        }}
      >
        Adicionar artefato
      </Button>
    </fieldset>
  )
}

interface ProfileFieldProps<C extends AgentConfig> {
  config: C
  profiles: AgentProfile[]
  onChange: (config: C) => void
}

/** The agent profile a node runs as. */
function ProfileField<C extends AgentConfig>({
  config,
  profiles,
  onChange,
}: ProfileFieldProps<C>) {
  return (
    <SelectField
      label="Perfil do agente"
      value={config.profile_id ?? ''}
      onChange={(id) => {
        onChange({ ...config, profile_id: id || undefined })
      }}
      options={[
        { value: '', label: 'Escolha o perfil' },
        ...profiles.map((p) => ({
          value: p.id,
          label: p.ready ? p.name : `${p.name} (sem credencial)`,
        })),
      ]}
    />
  )
}

interface ToolFieldsProps<C extends AgentConfig> {
  config: C
  onChange: (config: C) => void
}

function ToolFields<C extends AgentConfig>({
  config,
  onChange,
}: ToolFieldsProps<C>) {
  return (
    <>
      <ToolsField
        label="Tools liberadas"
        placeholder="Read, Edit, Bash(go test:*)"
        hint="Separadas por vírgula. Vazio: o padrão do agente."
        tools={config.allowed_tools}
        onChange={(allowed_tools) => {
          onChange({ ...config, allowed_tools })
        }}
      />
      <ToolsField
        label="Tools bloqueadas"
        placeholder="Bash(git commit:*), Bash(git push:*)"
        tools={config.disallowed_tools}
        onChange={(disallowed_tools) => {
          onChange({ ...config, disallowed_tools })
        }}
      />
    </>
  )
}

interface ToolsFieldProps {
  label: string
  placeholder: string
  hint?: string
  tools?: string[]
  onChange: (tools: string[]) => void
}

/** A tool list typed as text; the text is kept as typed while it is parsed. */
function ToolsField({
  label,
  placeholder,
  hint,
  tools,
  onChange,
}: ToolsFieldProps) {
  const [text, setText] = useState(formatTools(tools))
  return (
    <TextField
      label={label}
      placeholder={placeholder}
      hint={hint}
      value={text}
      onChange={(next) => {
        setText(next)
        onChange(parseTools(next))
      }}
    />
  )
}

type Schema = AgentConfig['output_schema']

interface SchemaFieldProps {
  schema: Schema
  onChange: (schema: Schema) => void
}

/** Edits the output JSON Schema as text; only valid JSON objects are kept. */
function SchemaField({ schema, onChange }: SchemaFieldProps) {
  const [text, setText] = useState(
    schema ? JSON.stringify(schema, null, 2) : '',
  )
  const [invalid, setInvalid] = useState(false)
  return (
    <div className="flex flex-col gap-1">
      <TextArea
        label="Schema de saída (JSON Schema, opcional)"
        rows={6}
        mono
        value={text}
        onChange={(next) => {
          setText(next)
          const parsed = parseSchema(next)
          setInvalid(parsed === false)
          if (parsed !== false) {
            onChange(parsed)
          }
        }}
      />
      {invalid && (
        <p className="text-xs text-red-600">
          JSON inválido: precisa ser um objeto.
        </p>
      )}
    </div>
  )
}
