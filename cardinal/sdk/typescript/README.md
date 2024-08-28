# cardinal

<div align="left">
    <a href="https://www.speakeasy.com/?utm_source=<no value>&utm_campaign=typescript"><img src="https://custom-icon-badges.demolab.com/badge/-Built%20By%20Speakeasy-212015?style=for-the-badge&logoColor=FBE331&logo=speakeasy&labelColor=545454" /></a>
    <a href="https://opensource.org/licenses/MIT">
        <img src="https://img.shields.io/badge/License-MIT-blue.svg" style="width: 100px; height: 28px;" />
    </a>
</div>

<!-- Start SDK Installation [installation] -->
## SDK Installation

### NPM

```bash
npm add @arguslabs/cardinal
```

### PNPM

```bash
pnpm add @arguslabs/cardinal
```

### Bun

```bash
bun add @arguslabs/cardinal
```

### Yarn

```bash
yarn add @arguslabs/cardinal zod

# Note that Yarn does not install peer dependencies automatically. You will need
# to install zod as shown above.
```
<!-- End SDK Installation [installation] -->

<!-- Start Requirements [requirements] -->
## Requirements

For supported JavaScript runtimes, please consult [RUNTIMES.md](RUNTIMES.md).
<!-- End Requirements [requirements] -->

<!-- Start SDK Example Usage [usage] -->
## SDK Example Usage

### Create a persona

```typescript
import { Cardinal } from "@arguslabs/cardinal";

const cardinal = new Cardinal();

async function run() {
    const result = await cardinal.createPersona({
        personaTag: "CoolMage",
    });

    // Handle the result
    console.log(result);
}

run();

```

### Execute a query

```typescript
import { Cardinal } from "@arguslabs/cardinal";

const cardinal = new Cardinal();

async function run() {
    const result = await cardinal.query({
        queryName: "player-health",
        requestBody: {
            key: "<value>",
        },
    });

    // Handle the result
    console.log(result);
}

run();

```
<!-- End SDK Example Usage [usage] -->

<!-- Start Available Resources and Operations [operations] -->
## Available Resources and Operations

### [Cardinal SDK](docs/sdks/cardinal/README.md)

* [queryCQL](docs/sdks/cardinal/README.md#querycql) - Executes a CQL (Cardinal Query Language) query
* [getDebugState](docs/sdks/cardinal/README.md#getdebugstate) - Retrieves a list of all entities in the game state
* [getHealth](docs/sdks/cardinal/README.md#gethealth) - Retrieves the status of the server and game loop
* [query](docs/sdks/cardinal/README.md#query) - Executes a query
* [getReceipts](docs/sdks/cardinal/README.md#getreceipts) - Retrieves all transaction receipts
* [transact](docs/sdks/cardinal/README.md#transact) - Submits a transaction
* [createPersona](docs/sdks/cardinal/README.md#createpersona) - Creates a persona
* [getWorld](docs/sdks/cardinal/README.md#getworld) - Retrieves details of the game world
<!-- End Available Resources and Operations [operations] -->

<!-- Start Retries [retries] -->
## Retries

Some of the endpoints in this SDK support retries.  If you use the SDK without any configuration, it will fall back to the default retry strategy provided by the API.  However, the default retry strategy can be overridden on a per-operation basis, or across the entire SDK.

To change the default retry strategy for a single API call, simply provide a retryConfig object to the call:
```typescript
import { Cardinal } from "@arguslabs/cardinal";

const cardinal = new Cardinal();

async function run() {
    const result = await cardinal.queryCQL(
        {
            cql: "CONTAINS(Health)",
        },
        {
            retries: {
                strategy: "backoff",
                backoff: {
                    initialInterval: 1,
                    maxInterval: 50,
                    exponent: 1.1,
                    maxElapsedTime: 100,
                },
                retryConnectionErrors: false,
            },
        }
    );

    // Handle the result
    console.log(result);
}

run();

```

If you'd like to override the default retry strategy for all operations that support retries, you can provide a retryConfig at SDK initialization:
```typescript
import { Cardinal } from "@arguslabs/cardinal";

const cardinal = new Cardinal({
    retryConfig: {
        strategy: "backoff",
        backoff: {
            initialInterval: 1,
            maxInterval: 50,
            exponent: 1.1,
            maxElapsedTime: 100,
        },
        retryConnectionErrors: false,
    },
});

async function run() {
    const result = await cardinal.queryCQL({
        cql: "CONTAINS(Health)",
    });

    // Handle the result
    console.log(result);
}

run();

```
<!-- End Retries [retries] -->

<!-- Start Error Handling [errors] -->
## Error Handling

All SDK methods return a response object or throw an error. If Error objects are specified in your OpenAPI Spec, the SDK will throw the appropriate Error type.

| Error Object    | Status Code     | Content Type    |
| --------------- | --------------- | --------------- |
| errors.SDKError | 4xx-5xx         | */*             |

Validation errors can also occur when either method arguments or data returned from the server do not match the expected format. The `SDKValidationError` that is thrown as a result will capture the raw value that failed validation in an attribute called `rawValue`. Additionally, a `pretty()` method is available on this error that can be used to log a nicely formatted string since validation errors can list many issues and the plain error string may be difficult read when debugging. 


```typescript
import { Cardinal } from "@arguslabs/cardinal";
import { SDKValidationError } from "@arguslabs/cardinal/models/errors";

const cardinal = new Cardinal();

async function run() {
    let result;
    try {
        result = await cardinal.queryCQL({
            cql: "CONTAINS(Health)",
        });
    } catch (err) {
        switch (true) {
            case err instanceof SDKValidationError: {
                // Validation errors can be pretty-printed
                console.error(err.pretty());
                // Raw value may also be inspected
                console.error(err.rawValue);
                return;
            }
            default: {
                throw err;
            }
        }
    }

    // Handle the result
    console.log(result);
}

run();

```
<!-- End Error Handling [errors] -->

<!-- No Server Selection [server] -->

<!-- Start Custom HTTP Client [http-client] -->
## Custom HTTP Client

The TypeScript SDK makes API calls using an `HTTPClient` that wraps the native
[Fetch API](https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API). This
client is a thin wrapper around `fetch` and provides the ability to attach hooks
around the request lifecycle that can be used to modify the request or handle
errors and response.

The `HTTPClient` constructor takes an optional `fetcher` argument that can be
used to integrate a third-party HTTP client or when writing tests to mock out
the HTTP client and feed in fixtures.

The following example shows how to use the `"beforeRequest"` hook to to add a
custom header and a timeout to requests and how to use the `"requestError"` hook
to log errors:

```typescript
import { Cardinal } from "@arguslabs/cardinal";
import { HTTPClient } from "@arguslabs/cardinal/lib/http";

const httpClient = new HTTPClient({
  // fetcher takes a function that has the same signature as native `fetch`.
  fetcher: (request) => {
    return fetch(request);
  }
});

httpClient.addHook("beforeRequest", (request) => {
  const nextRequest = new Request(request, {
    signal: request.signal || AbortSignal.timeout(5000)
  });

  nextRequest.headers.set("x-custom-header", "custom value");

  return nextRequest;
});

httpClient.addHook("requestError", (error, request) => {
  console.group("Request Error");
  console.log("Reason:", `${error}`);
  console.log("Endpoint:", `${request.method} ${request.url}`);
  console.groupEnd();
});

const sdk = new Cardinal({ httpClient });
```
<!-- End Custom HTTP Client [http-client] -->

<!-- Start Debugging [debug] -->
## Debugging

You can setup your SDK to emit debug logs for SDK requests and responses.

You can pass a logger that matches `console`'s interface as an SDK option.

> [!WARNING]
> Beware that debug logging will reveal secrets, like API tokens in headers, in log messages printed to a console or files. It's recommended to use this feature only during local development and not in production.

```typescript
import { Cardinal } from "@arguslabs/cardinal";

const sdk = new Cardinal({ debugLogger: console });
```
<!-- End Debugging [debug] -->

<!-- Start Standalone functions [standalone-funcs] -->
## Standalone functions

All the methods listed above are available as standalone functions. These
functions are ideal for use in applications running in the browser, serverless
runtimes or other environments where application bundle size is a primary
concern. When using a bundler to build your application, all unused
functionality will be either excluded from the final bundle or tree-shaken away.

To read more about standalone functions, check [FUNCTIONS.md](./FUNCTIONS.md).

<details>

<summary>Available standalone functions</summary>

- [createPersona](docs/sdks/cardinal/README.md#createpersona)
- [getDebugState](docs/sdks/cardinal/README.md#getdebugstate)
- [getHealth](docs/sdks/cardinal/README.md#gethealth)
- [getReceipts](docs/sdks/cardinal/README.md#getreceipts)
- [getWorld](docs/sdks/cardinal/README.md#getworld)
- [queryCQL](docs/sdks/cardinal/README.md#querycql)
- [query](docs/sdks/cardinal/README.md#query)
- [transact](docs/sdks/cardinal/README.md#transact)


</details>
<!-- End Standalone functions [standalone-funcs] -->

<!-- No Global Parameters [global-parameters] -->

<!-- Placeholder for Future Speakeasy SDK Sections -->

# Development

## Maturity

This SDK is in beta, and there may be breaking changes between versions without a major version update. Therefore, we recommend pinning usage
to a specific package version. This way, you can install the same version each time without breaking changes unless you are intentionally
looking for the latest version.

## Contributions

While we value open-source contributions to this SDK, this library is generated programmatically. Any manual changes added to internal files will be overwritten on the next generation. 
We look forward to hearing your feedback. Feel free to open a PR or an issue with a proof of concept and we'll do our best to include it in a future release. 

### SDK Created by [Speakeasy](https://www.speakeasy.com/?utm_source=&utm_campaign=typescript)
