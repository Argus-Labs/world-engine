import { Awaitable, BeforeRequestContext, BeforeRequestHook } from './types';
import { privateKeyToAccount } from 'viem/accounts'
import { customSign } from './signer-helper'

async function createPersonaRequest(request: Request) {
  const body = await request.json()
  const url = new URL(request.url)
  const privateKey = url.searchParams.get('_privateKey') as `0x{string}`
  const namespace = url.searchParams.get('_namespace')

  const account = privateKeyToAccount(privateKey)
  // nonce isn't checked anymore so just use any arbitrary value
  const msg = `${body!.personaTag}${namespace}0{"personaTag":"${body!.personaTag}","signerAddress":"${account.address}"}`
  const signature = customSign(msg, privateKey)

  return new Request(url.origin + url.pathname, {
    method: request.method,
    headers: request.headers,
    body: JSON.stringify({
      ...body!,
      signature,
      body: {
        personaTag: body!.personaTag,
        signerAddress: account.address
      }
    }),
    cache: request.cache,
    credentials: request.credentials,
    integrity: request.integrity,
    keepalive: request.keepalive,
    mode: request.mode,
    referrer: request.referrer,
    referrerPolicy: request.referrerPolicy,
    signal: request.signal,
  })
}

async function transactionRequest(request: Request) {
  const body = await request.json()
  const url = new URL(request.url)
  const privateKey = url.searchParams.get('_privateKey') as `0x{string}`
  const namespace = url.searchParams.get('_namespace')

  const msg = `${body!.personaTag}${namespace}0${JSON.stringify(body!.body)}`
  const signature = customSign(msg, privateKey)

  return new Request(url.origin + url.pathname, {
    method: request.method,
    headers: request.headers,
    body: JSON.stringify({
      ...body!,
      signature,
    }),
    cache: request.cache,
    credentials: request.credentials,
    integrity: request.integrity,
    keepalive: request.keepalive,
    mode: request.mode,
    referrer: request.referrer,
    referrerPolicy: request.referrerPolicy,
    signal: request.signal,
  })
}

export class SignerHook implements BeforeRequestHook {
  beforeRequest(_hookCtx: BeforeRequestContext, request: Request): Awaitable<Request> {
    const url = new URL(request.url)

    if (url.pathname === '/tx/persona/create-persona') {
      return createPersonaRequest(request)
    }

    if (url.pathname.startsWith('/tx/game/')) {
      return transactionRequest(request)
    }

    return request;
  }
}
