/*
 * Code generated by Speakeasy (https://speakeasy.com). DO NOT EDIT.
 */

import { remap as remap$ } from "../../lib/primitives.js";
import * as components from "../components/index.js";
import * as z from "zod";

export type PostTxGameTxNameGlobals = {
    privateKey?: string | undefined;
    namespace?: string | undefined;
};

export type PostTxGameTxNameRequest = {
    /**
     * Name of a registered message
     */
    txName: string;
    /**
     * Transaction details & message to be submitted
     */
    txBody: components.TxBody;
};

/** @internal */
export const PostTxGameTxNameGlobals$inboundSchema: z.ZodType<
    PostTxGameTxNameGlobals,
    z.ZodTypeDef,
    unknown
> = z
    .object({
        _privateKey: z.string().optional(),
        _namespace: z.string().optional(),
    })
    .transform((v) => {
        return remap$(v, {
            _privateKey: "privateKey",
            _namespace: "namespace",
        });
    });

/** @internal */
export type PostTxGameTxNameGlobals$Outbound = {
    _privateKey?: string | undefined;
    _namespace?: string | undefined;
};

/** @internal */
export const PostTxGameTxNameGlobals$outboundSchema: z.ZodType<
    PostTxGameTxNameGlobals$Outbound,
    z.ZodTypeDef,
    PostTxGameTxNameGlobals
> = z
    .object({
        privateKey: z.string().optional(),
        namespace: z.string().optional(),
    })
    .transform((v) => {
        return remap$(v, {
            privateKey: "_privateKey",
            namespace: "_namespace",
        });
    });

/**
 * @internal
 * @deprecated This namespace will be removed in future versions. Use schemas and types that are exported directly from this module.
 */
export namespace PostTxGameTxNameGlobals$ {
    /** @deprecated use `PostTxGameTxNameGlobals$inboundSchema` instead. */
    export const inboundSchema = PostTxGameTxNameGlobals$inboundSchema;
    /** @deprecated use `PostTxGameTxNameGlobals$outboundSchema` instead. */
    export const outboundSchema = PostTxGameTxNameGlobals$outboundSchema;
    /** @deprecated use `PostTxGameTxNameGlobals$Outbound` instead. */
    export type Outbound = PostTxGameTxNameGlobals$Outbound;
}

/** @internal */
export const PostTxGameTxNameRequest$inboundSchema: z.ZodType<
    PostTxGameTxNameRequest,
    z.ZodTypeDef,
    unknown
> = z
    .object({
        txName: z.string(),
        TxBody: components.TxBody$inboundSchema,
    })
    .transform((v) => {
        return remap$(v, {
            TxBody: "txBody",
        });
    });

/** @internal */
export type PostTxGameTxNameRequest$Outbound = {
    txName: string;
    TxBody: components.TxBody$Outbound;
};

/** @internal */
export const PostTxGameTxNameRequest$outboundSchema: z.ZodType<
    PostTxGameTxNameRequest$Outbound,
    z.ZodTypeDef,
    PostTxGameTxNameRequest
> = z
    .object({
        txName: z.string(),
        txBody: components.TxBody$outboundSchema,
    })
    .transform((v) => {
        return remap$(v, {
            txBody: "TxBody",
        });
    });

/**
 * @internal
 * @deprecated This namespace will be removed in future versions. Use schemas and types that are exported directly from this module.
 */
export namespace PostTxGameTxNameRequest$ {
    /** @deprecated use `PostTxGameTxNameRequest$inboundSchema` instead. */
    export const inboundSchema = PostTxGameTxNameRequest$inboundSchema;
    /** @deprecated use `PostTxGameTxNameRequest$outboundSchema` instead. */
    export const outboundSchema = PostTxGameTxNameRequest$outboundSchema;
    /** @deprecated use `PostTxGameTxNameRequest$Outbound` instead. */
    export type Outbound = PostTxGameTxNameRequest$Outbound;
}
