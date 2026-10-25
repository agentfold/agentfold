import { useState } from 'react'

import type {
  AgentConfig,
  AgentProfile,
  McpServer,
  CardType,
  Lane,
  MoveCardConfig,
  ReviewConfig,
  TriggerConfig,
} from '../api/client'
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
    case 'review':
      return (
        <ReviewForm
          config={data.config as ReviewConfig}
          profiles={profiles}
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
    </div>
  )
}

interface ReviewFormProps {
  config: ReviewConfig
  profiles: AgentProfile[]
  onChange: (config: ReviewConfig) => void
}

function ReviewForm({ config, profiles, onChange }: ReviewFormProps) {
  return (
    <div className="flex flex-col gap-3">
      <ProfileField config={config} profiles={profiles} onChange={onChange} />
      <TextArea
        label="Prompt de review"
        rows={6}
        value={config.prompt ?? ''}
        onChange={(prompt) => {
          onChange({ ...config, prompt })
        }}
      />
      <ToolFields config={config} onChange={onChange} />
      <TextField
        label="Máx. de rodadas"
        type="number"
        hint="Depois disso o review sai por “limite de rodadas”."
        value={config.max_rounds === undefined ? '' : String(config.max_rounds)}
        onChange={(text) => {
          onChange({ ...config, max_rounds: parseCount(text) })
        }}
      />
    </div>
  )
}

interface ProfileFieldProps<C extends AgentConfig | ReviewConfig> {
  config: C
  profiles: AgentProfile[]
  onChange: (config: C) => void
}

/** The agent profile a node runs as. */
function ProfileField<C extends AgentConfig | ReviewConfig>({
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

interface ToolFieldsProps<C extends AgentConfig | ReviewConfig> {
  config: C
  onChange: (config: C) => void
}

function ToolFields<C extends AgentConfig | ReviewConfig>({
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
