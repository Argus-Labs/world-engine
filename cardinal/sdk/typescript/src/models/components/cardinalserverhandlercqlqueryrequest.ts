/*
 * Code generated by Speakeasy (https://speakeasy.com). DO NOT EDIT.
 */

import * as z from "zod";

export type CardinalServerHandlerCQLQueryRequest = {
    cql?: string | undefined;
};

/** @internal */
export const CardinalServerHandlerCQLQueryRequest$inboundSchema: z.ZodType<
    CardinalServerHandlerCQLQueryRequest,
    z.ZodTypeDef,
    unknown
> = z.object({
    cql: z.string().optional(),
});

/** @internal */
export type CardinalServerHandlerCQLQueryRequest$Outbound = {
    cql?: string | undefined;
};

/** @internal */
export const CardinalServerHandlerCQLQueryRequest$outboundSchema: z.ZodType<
    CardinalServerHandlerCQLQueryRequest$Outbound,
    z.ZodTypeDef,
    CardinalServerHandlerCQLQueryRequest
> = z.object({
    cql: z.string().optional(),
});

/**
 * @internal
 * @deprecated This namespace will be removed in future versions. Use schemas and types that are exported directly from this module.
 */
export namespace CardinalServerHandlerCQLQueryRequest$ {
    /** @deprecated use `CardinalServerHandlerCQLQueryRequest$inboundSchema` instead. */
    export const inboundSchema = CardinalServerHandlerCQLQueryRequest$inboundSchema;
    /** @deprecated use `CardinalServerHandlerCQLQueryRequest$outboundSchema` instead. */
    export const outboundSchema = CardinalServerHandlerCQLQueryRequest$outboundSchema;
    /** @deprecated use `CardinalServerHandlerCQLQueryRequest$Outbound` instead. */
    export type Outbound = CardinalServerHandlerCQLQueryRequest$Outbound;
}
