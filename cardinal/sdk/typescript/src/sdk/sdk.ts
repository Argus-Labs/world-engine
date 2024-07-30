/*
 * Code generated by Speakeasy (https://speakeasy.com). DO NOT EDIT.
 */

import { SDKHooks } from "../hooks/hooks.js";
import { SDKOptions, serverURLFromOptions } from "../lib/config.js";
import { encodeJSON as encodeJSON$, encodeSimple as encodeSimple$ } from "../lib/encodings.js";
import { HTTPClient } from "../lib/http.js";
import * as schemas$ from "../lib/schemas.js";
import { ClientSDK, RequestOptions } from "../lib/sdks.js";
import * as components from "../models/components/index.js";
import * as operations from "../models/operations/index.js";
import * as z from "zod";

export class Cardinal extends ClientSDK {
    private readonly options$: SDKOptions & { hooks?: SDKHooks };

    constructor(options: SDKOptions = {}) {
        const opt = options as unknown;
        let hooks: SDKHooks;
        if (
            typeof opt === "object" &&
            opt != null &&
            "hooks" in opt &&
            opt.hooks instanceof SDKHooks
        ) {
            hooks = opt.hooks;
        } else {
            hooks = new SDKHooks();
        }

        super({
            client: options.httpClient || new HTTPClient(),
            baseURL: serverURLFromOptions(options),
            hooks,
        });

        this.options$ = { ...options, hooks };
        void this.options$;
    }

    /**
     * Executes a CQL (Cardinal Query Language) query
     *
     * @remarks
     * Executes a CQL (Cardinal Query Language) query
     */
    async postCql(
        request: components.CardinalServerHandlerCQLQueryRequest,
        options?: RequestOptions
    ): Promise<components.CardinalServerHandlerCQLQueryResponse> {
        const input$ = request;

        const payload$ = schemas$.parse(
            input$,
            (value$) =>
                components.CardinalServerHandlerCQLQueryRequest$outboundSchema.parse(value$),
            "Input validation failed"
        );
        const body$ = encodeJSON$("body", payload$, { explode: true });

        const path$ = this.templateURLComponent("/cql")();

        const query$ = "";

        const headers$ = new Headers({
            "Content-Type": "application/json",
            Accept: "application/json",
        });

        const context = { operationID: "post_/cql", oAuth2Scopes: [], securitySource: null };

        const request$ = this.createRequest$(
            context,
            {
                method: "POST",
                path: path$,
                headers: headers$,
                query: query$,
                body: body$,
                timeoutMs: options?.timeoutMs || this.options$.timeoutMs || -1,
            },
            options
        );

        const response = await this.do$(request$, {
            context,
            errorCodes: ["400", "4XX", "5XX"],
            retryConfig: options?.retries || this.options$.retryConfig,
            retryCodes: options?.retryCodes || ["429", "500", "502", "503", "504"],
        });

        const [result$] = await this.matcher<components.CardinalServerHandlerCQLQueryResponse>()
            .json(200, components.CardinalServerHandlerCQLQueryResponse$inboundSchema)
            .fail([400, "4XX", "5XX"])
            .match(response);

        return result$;
    }

    /**
     * Retrieves a list of all entities in the game state
     *
     * @remarks
     * Retrieves a list of all entities in the game state
     */
    async postDebugState(
        options?: RequestOptions
    ): Promise<Array<components.PkgWorldDevWorldEngineCardinalTypesDebugStateElement>> {
        const path$ = this.templateURLComponent("/debug/state")();

        const query$ = "";

        const headers$ = new Headers({
            Accept: "application/json",
        });

        const context = {
            operationID: "post_/debug/state",
            oAuth2Scopes: [],
            securitySource: null,
        };

        const request$ = this.createRequest$(
            context,
            {
                method: "POST",
                path: path$,
                headers: headers$,
                query: query$,
                timeoutMs: options?.timeoutMs || this.options$.timeoutMs || -1,
            },
            options
        );

        const response = await this.do$(request$, {
            context,
            errorCodes: ["4XX", "5XX"],
            retryConfig: options?.retries || this.options$.retryConfig,
            retryCodes: options?.retryCodes || ["429", "500", "502", "503", "504"],
        });

        const [result$] = await this.matcher<
            Array<components.PkgWorldDevWorldEngineCardinalTypesDebugStateElement>
        >()
            .json(
                200,
                z.array(
                    components.PkgWorldDevWorldEngineCardinalTypesDebugStateElement$inboundSchema
                )
            )
            .fail(["4XX", "5XX"])
            .match(response);

        return result$;
    }

    /**
     * Establishes a new websocket connection to retrieve system events
     *
     * @remarks
     * Establishes a new websocket connection to retrieve system events
     */
    async getEvents(options?: RequestOptions): Promise<string | undefined> {
        const path$ = this.templateURLComponent("/events")();

        const query$ = "";

        const headers$ = new Headers({
            Accept: "application/json",
        });

        const context = { operationID: "get_/events", oAuth2Scopes: [], securitySource: null };

        const request$ = this.createRequest$(
            context,
            {
                method: "GET",
                path: path$,
                headers: headers$,
                query: query$,
                timeoutMs: options?.timeoutMs || this.options$.timeoutMs || -1,
            },
            options
        );

        const response = await this.do$(request$, {
            context,
            errorCodes: ["4XX", "5XX"],
            retryConfig: options?.retries || this.options$.retryConfig,
            retryCodes: options?.retryCodes || ["429", "500", "502", "503", "504"],
        });

        const [result$] = await this.matcher<string | undefined>()
            .json(101, z.string().optional())
            .void("2XX", z.string().optional())
            .fail(["4XX", "5XX"])
            .match(response);

        return result$;
    }

    /**
     * Retrieves the status of the server and game loop
     *
     * @remarks
     * Retrieves the status of the server and game loop
     */
    async getHealth(
        options?: RequestOptions
    ): Promise<components.CardinalServerHandlerGetHealthResponse> {
        const path$ = this.templateURLComponent("/health")();

        const query$ = "";

        const headers$ = new Headers({
            Accept: "application/json",
        });

        const context = { operationID: "get_/health", oAuth2Scopes: [], securitySource: null };

        const request$ = this.createRequest$(
            context,
            {
                method: "GET",
                path: path$,
                headers: headers$,
                query: query$,
                timeoutMs: options?.timeoutMs || this.options$.timeoutMs || -1,
            },
            options
        );

        const response = await this.do$(request$, {
            context,
            errorCodes: ["4XX", "5XX"],
            retryConfig: options?.retries || this.options$.retryConfig,
            retryCodes: options?.retryCodes || ["429", "500", "502", "503", "504"],
        });

        const [result$] = await this.matcher<components.CardinalServerHandlerGetHealthResponse>()
            .json(200, components.CardinalServerHandlerGetHealthResponse$inboundSchema)
            .fail(["4XX", "5XX"])
            .match(response);

        return result$;
    }

    /**
     * Retrieves all transaction receipts
     *
     * @remarks
     * Retrieves all transaction receipts
     */
    async postQueryReceiptsList(
        request: components.CardinalServerHandlerListTxReceiptsRequest,
        options?: RequestOptions
    ): Promise<components.CardinalServerHandlerListTxReceiptsResponse> {
        const input$ = request;

        const payload$ = schemas$.parse(
            input$,
            (value$) =>
                components.CardinalServerHandlerListTxReceiptsRequest$outboundSchema.parse(value$),
            "Input validation failed"
        );
        const body$ = encodeJSON$("body", payload$, { explode: true });

        const path$ = this.templateURLComponent("/query/receipts/list")();

        const query$ = "";

        const headers$ = new Headers({
            "Content-Type": "application/json",
            Accept: "application/json",
        });

        const context = {
            operationID: "post_/query/receipts/list",
            oAuth2Scopes: [],
            securitySource: null,
        };

        const request$ = this.createRequest$(
            context,
            {
                method: "POST",
                path: path$,
                headers: headers$,
                query: query$,
                body: body$,
                timeoutMs: options?.timeoutMs || this.options$.timeoutMs || -1,
            },
            options
        );

        const response = await this.do$(request$, {
            context,
            errorCodes: ["400", "4XX", "5XX"],
            retryConfig: options?.retries || this.options$.retryConfig,
            retryCodes: options?.retryCodes || ["429", "500", "502", "503", "504"],
        });

        const [result$] =
            await this.matcher<components.CardinalServerHandlerListTxReceiptsResponse>()
                .json(200, components.CardinalServerHandlerListTxReceiptsResponse$inboundSchema)
                .fail([400, "4XX", "5XX"])
                .match(response);

        return result$;
    }

    /**
     * Executes a query
     *
     * @remarks
     * Executes a query
     */
    async postQueryQueryGroupQueryName(
        request: operations.PostQueryQueryGroupQueryNameRequest,
        options?: RequestOptions
    ): Promise<operations.PostQueryQueryGroupQueryNameResponseBody> {
        const input$ = request;

        const payload$ = schemas$.parse(
            input$,
            (value$) => operations.PostQueryQueryGroupQueryNameRequest$outboundSchema.parse(value$),
            "Input validation failed"
        );
        const body$ = encodeJSON$("body", payload$.RequestBody, { explode: true });

        const pathParams$ = {
            queryGroup: encodeSimple$("queryGroup", payload$.queryGroup, {
                explode: false,
                charEncoding: "percent",
            }),
            queryName: encodeSimple$("queryName", payload$.queryName, {
                explode: false,
                charEncoding: "percent",
            }),
        };
        const path$ = this.templateURLComponent("/query/{queryGroup}/{queryName}")(pathParams$);

        const query$ = "";

        const headers$ = new Headers({
            "Content-Type": "application/json",
            Accept: "application/json",
        });

        const context = {
            operationID: "post_/query/{queryGroup}/{queryName}",
            oAuth2Scopes: [],
            securitySource: null,
        };

        const request$ = this.createRequest$(
            context,
            {
                method: "POST",
                path: path$,
                headers: headers$,
                query: query$,
                body: body$,
                timeoutMs: options?.timeoutMs || this.options$.timeoutMs || -1,
            },
            options
        );

        const response = await this.do$(request$, {
            context,
            errorCodes: ["400", "4XX", "5XX"],
            retryConfig: options?.retries || this.options$.retryConfig,
            retryCodes: options?.retryCodes || ["429", "500", "502", "503", "504"],
        });

        const [result$] = await this.matcher<operations.PostQueryQueryGroupQueryNameResponseBody>()
            .json(200, operations.PostQueryQueryGroupQueryNameResponseBody$inboundSchema)
            .fail([400, "4XX", "5XX"])
            .match(response);

        return result$;
    }

    /**
     * Submits a transaction
     *
     * @remarks
     * Submits a transaction
     */
    async postTxGameTxName(
        request: operations.PostTxGameTxNameRequest,
        options?: RequestOptions
    ): Promise<components.CardinalServerHandlerPostTransactionResponse> {
        const input$ = request;

        const payload$ = schemas$.parse(
            input$,
            (value$) => operations.PostTxGameTxNameRequest$outboundSchema.parse(value$),
            "Input validation failed"
        );
        const body$ = encodeJSON$("body", payload$["cardinal_server_handler.Transaction"], {
            explode: true,
        });

        const pathParams$ = {
            txName: encodeSimple$("txName", payload$.txName, {
                explode: false,
                charEncoding: "percent",
            }),
        };
        const path$ = this.templateURLComponent("/tx/game/{txName}")(pathParams$);

        const query$ = "";

        const headers$ = new Headers({
            "Content-Type": "application/json",
            Accept: "application/json",
        });

        const context = {
            operationID: "post_/tx/game/{txName}",
            oAuth2Scopes: [],
            securitySource: null,
        };

        const request$ = this.createRequest$(
            context,
            {
                method: "POST",
                path: path$,
                headers: headers$,
                query: query$,
                body: body$,
                timeoutMs: options?.timeoutMs || this.options$.timeoutMs || -1,
            },
            options
        );

        const response = await this.do$(request$, {
            context,
            errorCodes: ["400", "4XX", "5XX"],
            retryConfig: options?.retries || this.options$.retryConfig,
            retryCodes: options?.retryCodes || ["429", "500", "502", "503", "504"],
        });

        const [result$] =
            await this.matcher<components.CardinalServerHandlerPostTransactionResponse>()
                .json(200, components.CardinalServerHandlerPostTransactionResponse$inboundSchema)
                .fail([400, "4XX", "5XX"])
                .match(response);

        return result$;
    }

    /**
     * Creates a persona
     *
     * @remarks
     * Creates a persona
     */
    async postTxPersonaCreatePersona(
        request: components.CardinalServerHandlerTransaction,
        options?: RequestOptions
    ): Promise<components.CardinalServerHandlerPostTransactionResponse> {
        const input$ = request;

        const payload$ = schemas$.parse(
            input$,
            (value$) => components.CardinalServerHandlerTransaction$outboundSchema.parse(value$),
            "Input validation failed"
        );
        const body$ = encodeJSON$("body", payload$, { explode: true });

        const path$ = this.templateURLComponent("/tx/persona/create-persona")();

        const query$ = "";

        const headers$ = new Headers({
            "Content-Type": "application/json",
            Accept: "application/json",
        });

        const context = {
            operationID: "post_/tx/persona/create-persona",
            oAuth2Scopes: [],
            securitySource: null,
        };

        const request$ = this.createRequest$(
            context,
            {
                method: "POST",
                path: path$,
                headers: headers$,
                query: query$,
                body: body$,
                timeoutMs: options?.timeoutMs || this.options$.timeoutMs || -1,
            },
            options
        );

        const response = await this.do$(request$, {
            context,
            errorCodes: ["400", "4XX", "5XX"],
            retryConfig: options?.retries || this.options$.retryConfig,
            retryCodes: options?.retryCodes || ["429", "500", "502", "503", "504"],
        });

        const [result$] =
            await this.matcher<components.CardinalServerHandlerPostTransactionResponse>()
                .json(200, components.CardinalServerHandlerPostTransactionResponse$inboundSchema)
                .fail([400, "4XX", "5XX"])
                .match(response);

        return result$;
    }

    /**
     * Submits a transaction
     *
     * @remarks
     * Submits a transaction
     */
    async postTxTxGroupTxName(
        request: operations.PostTxTxGroupTxNameRequest,
        options?: RequestOptions
    ): Promise<components.CardinalServerHandlerPostTransactionResponse> {
        const input$ = request;

        const payload$ = schemas$.parse(
            input$,
            (value$) => operations.PostTxTxGroupTxNameRequest$outboundSchema.parse(value$),
            "Input validation failed"
        );
        const body$ = encodeJSON$("body", payload$["cardinal_server_handler.Transaction"], {
            explode: true,
        });

        const pathParams$ = {
            txGroup: encodeSimple$("txGroup", payload$.txGroup, {
                explode: false,
                charEncoding: "percent",
            }),
            txName: encodeSimple$("txName", payload$.txName, {
                explode: false,
                charEncoding: "percent",
            }),
        };
        const path$ = this.templateURLComponent("/tx/{txGroup}/{txName}")(pathParams$);

        const query$ = "";

        const headers$ = new Headers({
            "Content-Type": "application/json",
            Accept: "application/json",
        });

        const context = {
            operationID: "post_/tx/{txGroup}/{txName}",
            oAuth2Scopes: [],
            securitySource: null,
        };

        const request$ = this.createRequest$(
            context,
            {
                method: "POST",
                path: path$,
                headers: headers$,
                query: query$,
                body: body$,
                timeoutMs: options?.timeoutMs || this.options$.timeoutMs || -1,
            },
            options
        );

        const response = await this.do$(request$, {
            context,
            errorCodes: ["400", "4XX", "5XX"],
            retryConfig: options?.retries || this.options$.retryConfig,
            retryCodes: options?.retryCodes || ["429", "500", "502", "503", "504"],
        });

        const [result$] =
            await this.matcher<components.CardinalServerHandlerPostTransactionResponse>()
                .json(200, components.CardinalServerHandlerPostTransactionResponse$inboundSchema)
                .fail([400, "4XX", "5XX"])
                .match(response);

        return result$;
    }

    /**
     * Retrieves details of the game world
     *
     * @remarks
     * Contains the registered components, messages, queries, and namespace
     */
    async getWorld(
        options?: RequestOptions
    ): Promise<components.CardinalServerHandlerGetWorldResponse> {
        const path$ = this.templateURLComponent("/world")();

        const query$ = "";

        const headers$ = new Headers({
            Accept: "application/json",
        });

        const context = { operationID: "get_/world", oAuth2Scopes: [], securitySource: null };

        const request$ = this.createRequest$(
            context,
            {
                method: "GET",
                path: path$,
                headers: headers$,
                query: query$,
                timeoutMs: options?.timeoutMs || this.options$.timeoutMs || -1,
            },
            options
        );

        const response = await this.do$(request$, {
            context,
            errorCodes: ["400", "4XX", "5XX"],
            retryConfig: options?.retries || this.options$.retryConfig,
            retryCodes: options?.retryCodes || ["429", "500", "502", "503", "504"],
        });

        const [result$] = await this.matcher<components.CardinalServerHandlerGetWorldResponse>()
            .json(200, components.CardinalServerHandlerGetWorldResponse$inboundSchema)
            .fail([400, "4XX", "5XX"])
            .match(response);

        return result$;
    }
}