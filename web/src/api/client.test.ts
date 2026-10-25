import { afterEach, describe, expect, it, vi } from 'vitest'

import { ApiError, createApiClient, errorMessage, unwrap } from './client'

const response = (status: number) => new Response(null, { status })

describe('ApiError', () => {
  it('reads the error body of the contract', () => {
    const err = new ApiError(422, {
      code: 'validation',
      message: 'name: is required',
      field: 'name',
    })
    expect(err).toMatchObject({
      status: 422,
      code: 'validation',
      field: 'name',
      message: 'name: is required',
    })
  })

  it.each([null, 'Bad Gateway', { message: 'no code' }])(
    'falls back for unknown bodies (%j)',
    (body) => {
      const err = new ApiError(502, body)
      expect(err).toMatchObject({
        status: 502,
        code: 'internal',
        field: undefined,
        message: 'HTTP 502',
      })
    },
  )
})

describe('unwrap', () => {
  it('returns the data of successful calls', async () => {
    await expect(
      unwrap(Promise.resolve({ data: 1, response: response(200) })),
    ).resolves.toBe(1)
  })

  it('throws an ApiError for failed calls', async () => {
    const call = unwrap(
      Promise.resolve({
        error: { code: 'not_found', message: 'not found' },
        response: response(404),
      }),
    )
    await expect(call).rejects.toMatchObject({ status: 404, code: 'not_found' })
  })
})

describe('errorMessage', () => {
  it.each([
    ['in_use', 'Não é possível excluir: ainda está em uso.'],
    ['conflict', 'Já existe um registro com esse nome.'],
    ['not_found', 'Registro não encontrado. A tela pode estar desatualizada.'],
    ['internal', 'Erro no servidor.'],
    ['validation', 'detail'],
  ])('describes %s errors', (code, message) => {
    expect(errorMessage(new ApiError(400, { code, message: 'detail' }))).toBe(
      message,
    )
  })

  it('describes network failures', () => {
    expect(errorMessage(new TypeError('fetch failed'))).toBe(
      'Não foi possível falar com o servidor.',
    )
  })
})

describe('createApiClient', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('uses the global fetch by default', async () => {
    const fetchMock = vi.fn(() => Promise.resolve(Response.json([])))
    vi.stubGlobal('fetch', fetchMock)
    const client = createApiClient('http://localhost/api')
    await expect(unwrap(client.GET('/lanes'))).resolves.toEqual([])
    expect(fetchMock).toHaveBeenCalledOnce()
  })
})
