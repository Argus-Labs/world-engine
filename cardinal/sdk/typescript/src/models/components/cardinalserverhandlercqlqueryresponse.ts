/*
 * Code generated by Speakeasy (https://speakeasy.com). DO NOT EDIT.
 */

import {
    PkgWorldDevWorldEngineCardinalTypesEntityStateElement,
    PkgWorldDevWorldEngineCardinalTypesEntityStateElement$inboundSchema,
    PkgWorldDevWorldEngineCardinalTypesEntityStateElement$Outbound,
    PkgWorldDevWorldEngineCardinalTypesEntityStateElement$outboundSchema,
} from "./pkgworlddevworldenginecardinaltypesentitystateelement.js";
import * as z from "zod";

export type CardinalServerHandlerCQLQueryResponse = {
    results?: Array<PkgWorldDevWorldEngineCardinalTypesEntityStateElement> | undefined;
};

/** @internal */
export const CardinalServerHandlerCQLQueryResponse$inboundSchema: z.ZodType<
    CardinalServerHandlerCQLQueryResponse,
    z.ZodTypeDef,
    unknown
> = z.object({
    results: z
        .array(PkgWorldDevWorldEngineCardinalTypesEntityStateElement$inboundSchema)
        .optional(),
});

/** @internal */
export type CardinalServerHandlerCQLQueryResponse$Outbound = {
    results?: Array<PkgWorldDevWorldEngineCardinalTypesEntityStateElement$Outbound> | undefined;
};

/** @internal */
export const CardinalServerHandlerCQLQueryResponse$outboundSchema: z.ZodType<
    CardinalServerHandlerCQLQueryResponse$Outbound,
    z.ZodTypeDef,
    CardinalServerHandlerCQLQueryResponse
> = z.object({
    results: z
        .array(PkgWorldDevWorldEngineCardinalTypesEntityStateElement$outboundSchema)
        .optional(),
});

/**
 * @internal
 * @deprecated This namespace will be removed in future versions. Use schemas and types that are exported directly from this module.
 */
export namespace CardinalServerHandlerCQLQueryResponse$ {
    /** @deprecated use `CardinalServerHandlerCQLQueryResponse$inboundSchema` instead. */
    export const inboundSchema = CardinalServerHandlerCQLQueryResponse$inboundSchema;
    /** @deprecated use `CardinalServerHandlerCQLQueryResponse$outboundSchema` instead. */
    export const outboundSchema = CardinalServerHandlerCQLQueryResponse$outboundSchema;
    /** @deprecated use `CardinalServerHandlerCQLQueryResponse$Outbound` instead. */
    export type Outbound = CardinalServerHandlerCQLQueryResponse$Outbound;
}
