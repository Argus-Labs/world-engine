<!-- Start SDK Example Usage [usage] -->
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