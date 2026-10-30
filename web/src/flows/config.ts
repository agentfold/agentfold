import type { AgentConfig } from '../api/client'

/** Comma-separated tool names ("Read, Edit") to a list. */
export const parseTools = (text: string) =>
  text
    .split(',')
    .map((t) => t.trim())
    .filter(Boolean)

/** A tool list as typed in its field. */
export const formatTools = (tools?: string[]) => (tools ?? []).join(', ')

/** A count typed in a number field, or undefined when empty. */
export const parseCount = (text: string) =>
  text === '' ? undefined : Number(text)

type Schema = AgentConfig['output_schema']

/** The schema in `text`, undefined when empty, false when not a JSON object. */
export function parseSchema(text: string): Schema | false {
  if (text.trim() === '') {
    return undefined
  }
  try {
    const value: unknown = JSON.parse(text)
    return typeof value === 'object' && value !== null && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : false
  } catch {
    return false
  }
}

/**
 * The output schema of an agent before a "Criar cards" node: the cards to
 * create, with the project by name (as in domain.ChildCardsSchema).
 */
export const childCardsSchema = {
  type: 'object',
  required: ['cards'],
  properties: {
    cards: {
      type: 'array',
      minItems: 1,
      items: {
        type: 'object',
        required: ['title'],
        properties: {
          ref: { type: 'string' },
          title: { type: 'string' },
          description: { type: 'string' },
          project: { type: 'string' },
          depends_on: { type: 'array', items: { type: 'string' } },
        },
      },
    },
  },
}
