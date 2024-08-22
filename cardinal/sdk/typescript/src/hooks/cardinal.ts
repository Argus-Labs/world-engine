import { BeforeRequestContext, BeforeRequestHook } from './types';
import { privateKeyToAccount } from 'viem/accounts'
import { createMsgToSign, customSign } from './sign'

function modifyRequest(request: Request, body: {[k: string]: any}) {
  const url = new URL(request.url)
  return new Request(url.origin + url.pathname, {
    method: request.method,
    headers: request.headers,
    body: JSON.stringify(body),
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

export class CardinalHook implements BeforeRequestHook {
  private namespace?: string;

  async beforeRequest(_hookCtx: BeforeRequestContext, request: Request): Promise<Request> {
    const url = new URL(request.url)

    if (!this.namespace) await this.setNamespace(url)

    if (url.pathname === '/tx/persona/create-persona') {
      const body = await request.json()
      const privateKey = url.searchParams.get('_privateKey') as `0x{string}`
      const account = privateKeyToAccount(privateKey)
      const txBody = {
        personaTag: body!.personaTag,
        signerAddress: account.address
      }
      const msg = createMsgToSign(body!.personaTag, this.namespace!, txBody)
      const signature = customSign(msg, privateKey)
      return modifyRequest(request, {
        ...body,
        signature,
        body: txBody
      })
    }

    if (url.pathname.startsWith('/tx/game/')) {
      const body = await request.json()
      const msg = createMsgToSign(body!.personaTag, this.namespace!, body!.body)
      const privateKey = url.searchParams.get('_privateKey') as `0x{string}`
      const signature = customSign(msg, privateKey)
      return modifyRequest(request, {
        ...body,
        signature,
      })
    }

    return request;
  }

  // this is called in beforeRequest instead of sdkInit because it can't be called 
  // synchronously in sdkInit, which could result in a race condition where the beforeRequest
  // is called before the setNamespace in sdkInit finishes.
  private async setNamespace(url: URL) {
    const res = await fetch(`${url.origin}/world`)
    const data = await res.json()
    this.namespace = data.namespace
  }
}
