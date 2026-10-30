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
  WaitChildrenConfig,
  HumanReviewConfig,
  SetPropertyConfig,
  CheckPropertyConfig,
} from '../api/client'
import { agentKindLabels } from '../agents/labels'
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
          cardTypes={cardTypes}
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
    case 'create_cards':
      return (
        <p className="rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-800">
          Este nó foi aposentado: as versões já publicadas com ele continuam
          rodando, mas um rascunho com ele não publica. Para criar cards filhos,
          libere &quot;Criar cards filhos&quot; nas tools do board do agente que
          planeja e escolha os tipos.
        </p>
      )
    case 'wait_children': {
      const config = data.config as WaitChildrenConfig
      return (
        <div className="flex flex-col gap-3">
          <p className="text-sm text-slate-500">
            O fluxo espera os cards filhos que não foram cancelados. Um filho
            conta como concluído quando tem a propriedade (gravada por um
            Definir propriedade no fluxo dele) e como falho quando a última
            execução dele falha ou é cancelada antes disso.
          </p>
          <SelectField
            label="Modo"
            value={config.mode ?? 'all_succeed'}
            onChange={(mode) => {
              onChange({ ...config, mode: mode as WaitChildrenConfig['mode'] })
            }}
            options={[
              {
                value: 'all_succeed',
                label: 'Todos com sucesso (sai na primeira falha)',
              },
              {
                value: 'all_finished',
                label: 'Todos terminarem (com sucesso ou falha)',
              },
            ]}
          />
          <div className="grid grid-cols-2 gap-3">
            <TextField
              label="Chave"
              placeholder="desenvolvimento"
              value={config.property_key ?? ''}
              onChange={(key) => {
                onChange({
                  ...config,
                  property_key: key || undefined,
                  done_lane_id: undefined,
                })
              }}
            />
            <TextField
              label="Valor"
              placeholder="concluido"
              value={config.property_value ?? ''}
              onChange={(value) => {
                onChange({
                  ...config,
                  property_value: value || undefined,
                  done_lane_id: undefined,
                })
              }}
            />
          </div>
          {config.done_lane_id && (
            <p className="text-sm text-amber-700">
              Este nó ainda conta como concluído o filho na baia{' '}
              {lanes.find((l) => l.id === config.done_lane_id)?.name ??
                'removida'}
              , como nas versões antigas: informe a propriedade para publicar.
            </p>
          )}
          <p className="text-xs text-slate-500">
            A saída algum falhou traz, para cada filho que falhou, o erro e a
            última mensagem do agente: ligue-a ao agente que refaz o plano.
          </p>
        </div>
      )
    }
    case 'human_review':
      return (
        <div className="flex flex-col gap-3">
          <p className="text-sm text-slate-500">
            O fluxo espera um humano revisar os artefatos na aba Artefatos do
            card. Cada mensagem dele sai por pediu ajustes, que deve levar ao
            agente que refaz o trabalho; o fluxo volta a esta revisão quando o
            agente termina. Aprovar sai por aprovado.
          </p>
          <ListField
            label="Artefatos revisados"
            placeholder="Plano"
            hint="Nomes separados por vírgula. Vazio: os artefatos coletados neste run."
            items={(data.config as HumanReviewConfig).artifacts}
            onChange={(artifacts) => {
              onChange(artifacts.length > 0 ? { artifacts } : {})
            }}
          />
        </div>
      )
    case 'set_property':
      return (
        <div className="flex flex-col gap-3">
          <p className="text-sm text-slate-500">
            Grava a propriedade no card. Quem espera por ela (o card pai num
            Esperar filhos, os cards que dependem deste num Conferir
            propriedade) é reavaliado.
          </p>
          <PropertyFields
            config={data.config as SetPropertyConfig}
            onChange={onChange}
          />
        </div>
      )
    case 'check_property': {
      const config = data.config as CheckPropertyConfig
      return (
        <div className="flex flex-col gap-3">
          <p className="text-sm text-slate-500">
            Confere se todos os cards de que este depende têm a propriedade: sai
            por satisfeitas (também sem dependências) ou por pendentes.
          </p>
          <PropertyFields config={config} onChange={onChange} />
          <Toggle
            label="Esperar"
            hint="Em vez de sair por pendentes, o fluxo espera até as dependências terem a propriedade."
            checked={config.wait ?? false}
            onChange={(wait) => {
              onChange({ ...config, wait: wait || undefined })
            }}
          />
        </div>
      )
    }
    case 'end':
      return (
        <p className="text-sm text-slate-500">
          Encerra o fluxo. Não tem configuração.
        </p>
      )
  }
}

interface PropertyFieldsProps<C extends SetPropertyConfig> {
  config: C
  onChange: (config: C) => void
}

/** The key and value of a property node. */
function PropertyFields<C extends SetPropertyConfig>({
  config,
  onChange,
}: PropertyFieldsProps<C>) {
  return (
    <div className="grid grid-cols-2 gap-3">
      <TextField
        label="Chave"
        placeholder="desenvolvimento"
        value={config.key ?? ''}
        onChange={(key) => {
          onChange({ ...config, key: key || undefined })
        }}
      />
      <TextField
        label="Valor"
        placeholder="concluido"
        value={config.value ?? ''}
        onChange={(value) => {
          onChange({ ...config, value: value || undefined })
        }}
      />
    </div>
  )
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
  cardTypes: CardType[]
  profiles: AgentProfile[]
  mcpServers: McpServer[]
  onChange: (config: AgentConfig) => void
}

function AgentForm({
  config,
  cardTypes,
  profiles,
  mcpServers,
  onChange,
}: AgentFormProps) {
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
      <ToolFields config={config} cardTypes={cardTypes} onChange={onChange} />
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
          label: `${p.name} · ${agentKindLabels[p.kind]}${p.ready ? '' : ' (sem credencial)'}`,
        })),
      ]}
    />
  )
}

interface ToolFieldsProps {
  config: AgentConfig
  cardTypes: CardType[]
  onChange: (config: AgentConfig) => void
}

/** The board tools a node only has when its allowed tools list them. */
const boardTools = [
  { name: 'create_cards', label: 'Criar cards filhos' },
  { name: 'list_cards', label: 'Ver os cards filhos' },
  { name: 'retry_card', label: 'Rodar um card filho de novo' },
  { name: 'cancel_card', label: 'Cancelar um card filho' },
].map((t) => ({ ...t, tool: `mcp__orquestrator__${t.name}` }))

const createCardsTool = 'mcp__orquestrator__create_cards'

const isBoardTool = (tool: string) => boardTools.some((t) => t.tool === tool)

function ToolFields({ config, cardTypes, onChange }: ToolFieldsProps) {
  const allowed = config.allowed_tools ?? []
  const types = new Set(config.card_type_ids ?? [])
  const granted = allowed.filter(isBoardTool)
  const others = allowed.filter((tool) => !isBoardTool(tool))
  return (
    <>
      <fieldset className="flex flex-col gap-1">
        <legend className="text-xs font-medium text-slate-600">
          Tools do board
        </legend>
        <p className="text-xs text-slate-500">
          Com elas o agente divide o card em cards filhos, que seguem os fluxos
          do tipo deles.
        </p>
        {boardTools.map((t) => (
          <label key={t.name} className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={granted.includes(t.tool)}
              onChange={(e) => {
                const tools = e.target.checked
                  ? [...allowed, t.tool]
                  : allowed.filter((tool) => tool !== t.tool)
                onChange({ ...config, allowed_tools: tools })
              }}
            />
            {t.label}
          </label>
        ))}
      </fieldset>
      {granted.includes(createCardsTool) && (
        <fieldset className="flex flex-col gap-1">
          <legend className="text-xs font-medium text-slate-600">
            Tipos dos cards criados
          </legend>
          <p className="text-xs text-slate-500">
            O agente só cria cards destes tipos, cada um na baia inicial do
            tipo. Com um só, ele nem escolhe.
          </p>
          {cardTypes.map((t) => (
            <label key={t.id} className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={types.has(t.id)}
                onChange={(e) => {
                  const ids = e.target.checked
                    ? [...types, t.id]
                    : [...types].filter((id) => id !== t.id)
                  onChange({ ...config, card_type_ids: ids })
                }}
              />
              {t.name}
              {!t.initial_lane_id && (
                <span className="text-xs text-amber-700">sem baia inicial</span>
              )}
            </label>
          ))}
        </fieldset>
      )}
      <ListField
        label="Tools liberadas"
        placeholder="Read, Edit, Bash(go test:*)"
        hint="Separadas por vírgula. Vazio: o padrão do agente."
        items={others}
        onChange={(items) => {
          onChange({ ...config, allowed_tools: [...items, ...granted] })
        }}
      />
      <ListField
        label="Tools bloqueadas"
        placeholder="Bash(git commit:*), Bash(git push:*)"
        items={config.disallowed_tools}
        onChange={(disallowed_tools) => {
          onChange({ ...config, disallowed_tools })
        }}
      />
    </>
  )
}

interface ListFieldProps {
  label: string
  placeholder: string
  hint?: string
  items?: string[]
  onChange: (items: string[]) => void
}

/**
 * A comma-separated list (tools, names) typed as text; the text is kept as
 * typed while it is parsed.
 */
function ListField({
  label,
  placeholder,
  hint,
  items,
  onChange,
}: ListFieldProps) {
  const [text, setText] = useState(formatTools(items))
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
