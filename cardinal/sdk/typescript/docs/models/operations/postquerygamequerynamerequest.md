# PostQueryGameQueryNameRequest

## Example Usage

```typescript
import { PostQueryGameQueryNameRequest } from "cardinal/models/operations";

let value: PostQueryGameQueryNameRequest = {
    queryName: "<value>",
    requestBody: {
        key: "<value>",
    },
};
```

## Fields

| Field                      | Type                       | Required                   | Description                |
| -------------------------- | -------------------------- | -------------------------- | -------------------------- |
| `queryName`                | *string*                   | :heavy_check_mark:         | Name of a registered query |
| `requestBody`              | Record<string, *any*>      | :heavy_check_mark:         | Query to be executed       |