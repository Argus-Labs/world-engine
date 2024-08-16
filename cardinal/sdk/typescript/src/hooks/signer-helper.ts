import { secp256k1 } from '@noble/curves/secp256k1'
// import { generatePrivateKey, privateKeyToAccount } from 'viem/accounts'
import { keccak256, toHex } from 'viem/utils'

// -------------------------------------------------------------------------- //
// copied source from viem directly instead of importing from node_modules, as doing that
// causes typescript errors that can't be fixed.
// TODO: remove this once pkg.world.dev/sign package follows eip-712
type Hex = `0x${string}`
type Signature = {
  r: Hex
  s: Hex
  /** @deprecated use `yParity`. */
  v: bigint
  yParity?: number | undefined
}
type SignParameters = {
  hash: Hex
  privateKey: Hex
}
type SignReturnType = Signature
type HexToBigIntOpts = {
  /** Whether or not the number of a signed representation. */
  signed?: boolean | undefined
  /** Size (in bytes) of the hex value. */
  size?: number | undefined
}
type ByteArray = Uint8Array

function isHex(
  value: unknown,
  { strict = true }: { strict?: boolean | undefined } = {},
): value is Hex {
  if (!value) return false
  if (typeof value !== 'string') return false
  return strict ? /^0x[0-9a-fA-F]*$/.test(value) : value.startsWith('0x')
}

function size_(value: Hex | ByteArray) {
  if (isHex(value, { strict: false })) return Math.ceil((value.length - 2) / 2)
  return value.length
}

function assertSize(hexOrBytes: Hex | ByteArray, { size }: { size: number }): void {
  if (size_(hexOrBytes) > size)
    throw new Error(`Size cannot exceed ${size} bytes. Given size: ${size_(hexOrBytes)} bytes.`)
}

function hexToBigInt(hex: Hex, opts: HexToBigIntOpts = {}): bigint {
  const { signed } = opts

  if (opts.size) assertSize(hex, { size: opts.size })

  const value = BigInt(hex)
  if (!signed) return value

  const size = (hex.length - 2) / 2
  const max = (BigInt(1) << (BigInt(size) * BigInt(8) - BigInt(1))) - BigInt(1)
  if (value <= max) return value

  return value - BigInt(`0x${'f'.padStart(size * 2, 'f')}`) - BigInt(1)
}

function signatureToHex({ r, s, v, yParity }: Signature): Hex {
  const vHex = (() => {
    if (v === BigInt(27) || yParity === 0) return '1b'
    if (v === BigInt(28) || yParity === 1) return '1c'
    throw new Error('Invalid v value')
  })()
  return `0x${new secp256k1.Signature(hexToBigInt(r), hexToBigInt(s)).toCompactHex()}${vHex}`
}

function sign({ hash, privateKey }: SignParameters): SignReturnType {
  const { r, s, recovery } = secp256k1.sign(hash.slice(2), privateKey.slice(2))
  return {
    r: toHex(r),
    s: toHex(s),
    v: recovery ? BigInt(28) : BigInt(27),
    yParity: recovery,
  }
}
// -------------------------------------------------------------------------- //

// we need this because world-engine doesn't follow any signing formats, e.g. eip-191 or eip-712.
// the plan is to use eip-712, (viem's signTypedData), but until that's implemented, we'll be using this
export function customSign(msg: string, privateKey: `0x${string}`) {
  const signature = sign({ hash: keccak256(toHex(msg)), privateKey })
  return signatureToHex(signature).slice(2)
}
