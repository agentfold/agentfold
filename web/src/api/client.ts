import createClient from 'openapi-fetch'

import type { components, paths } from './schema.gen'

type Schemas = components['schemas']
export type Lane = Schemas['Lane']
export type LaneInput = Schemas['LaneInput']
export type Project = Schemas['Project']
export type ProjectInput = Schemas['ProjectInput']
export type CardType = Schemas['CardType']
export type CardTypeInput = Schemas['CardTypeInput']
export type Card = Schemas['Card']
export type CardInput = Schemas['CardInput']
export type NewCard = Schemas['NewCard']
export type CardEvent = Schemas['CardEvent']
export type FlowSummary = Schemas['FlowSummary']
export type Flow = Schemas['Flow']
export type FlowInput = Schemas['FlowInput']
export type FlowVersion = Schemas['FlowVersion']
export type FlowGraph = Schemas['FlowGraph']
export type FlowNode = Schemas['FlowNode']
export type FlowEdge = Schemas['FlowEdge']
export type GraphProblem = Schemas['GraphProblem']
export type TriggerConfig = Schemas['TriggerConfig']
export type AgentConfig = Schemas['AgentConfig']
export type ReviewConfig = Schemas['ReviewConfig']
export type MoveCardConfig = Schemas['MoveCardConfig']
export type AgentProfile = Schemas['AgentProfile']
export type McpServer = Schemas['McpServer']
export type McpServerInput = Schemas['McpServerInput']
type ErrorBody = Schemas['Error']

export type ApiClient = ReturnType<typeof createClient<paths>>

/** Creates the client for the API served under `baseUrl`. */
export function createApiClient(
  baseUrl: string,
  fetchImpl: typeof fetch = (input, init) => fetch(input, init),
): ApiClient {
  return createClient<paths>({ baseUrl, fetch: fetchImpl })
}

/** An error answered by the API (see `Error` in the contract). */
export class ApiError extends Error {
  readonly status: number
  readonly code: ErrorBody['code']
  readonly field: string | undefined

  constructor(status: number, body: unknown) {
    const known = isErrorBody(body)
    super(known ? body.message : `HTTP ${String(status)}`)
    this.name = 'ApiError'
    this.status = status
    this.code = known ? body.code : 'internal'
    this.field = known ? body.field : undefined
  }
}

function isErrorBody(body: unknown): body is ErrorBody {
  return (
    typeof body === 'object' &&
    body !== null &&
    'code' in body &&
    'message' in body
  )
}

interface FetchResult<T> {
  data?: T
  error?: unknown
  response: Response
}

/** Returns the data of a successful call or throws an ApiError. */
export async function unwrap<T>(call: Promise<FetchResult<T>>): Promise<T> {
  const { data, error, response } = await call
  if (!response.ok) {
    throw new ApiError(response.status, error)
  }
  return data as T
}

/** A message for the user describing a failed call. */
export function errorMessage(error: unknown): string {
  if (!(error instanceof ApiError)) {
    return 'Não foi possível falar com o servidor.'
  }
  switch (error.code) {
    case 'in_use':
      return 'Não é possível excluir: ainda está em uso.'
    case 'conflict':
      return 'Já existe um registro com esse nome.'
    case 'not_found':
      return 'Registro não encontrado. A tela pode estar desatualizada.'
    case 'invalid_graph':
      return 'O rascunho tem problemas: corrija-os antes de publicar.'
    case 'internal':
      return 'Erro no servidor.'
    default:
      return error.message
  }
}
