# QueryRequest

## Example Usage

```typescript
import { QueryRequest } from "@arguslabs/cardinal/models/operations";

let value: QueryRequest = {
    queryName: "player-health",
    requestBody: {
        key: "<value>",
    },
};
```

## Fields

| Field                      | Type                       | Required                   | Description                | Example                    |
| -------------------------- | -------------------------- | -------------------------- | -------------------------- | -------------------------- |
| `queryName`                | *string*                   | :heavy_check_mark:         | Name of a registered query | player-health              |
| `requestBody`              | Record<string, *any*>      | :heavy_check_mark:         | Query to be executed       |                            |