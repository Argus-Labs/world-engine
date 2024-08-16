# PostQueryGameQueryNameRequest

## Example Usage

```typescript
import { PostQueryGameQueryNameRequest } from "cardinal/models/operations";

let value: PostQueryGameQueryNameRequest = {
    queryName: "<value>",
    requestBody: {},
};
```

## Fields

| Field                                                                                                        | Type                                                                                                         | Required                                                                                                     | Description                                                                                                  |
| ------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------ |
| `queryName`                                                                                                  | *string*                                                                                                     | :heavy_check_mark:                                                                                           | Name of a registered query                                                                                   |
| `requestBody`                                                                                                | [operations.PostQueryGameQueryNameRequestBody](../../models/operations/postquerygamequerynamerequestbody.md) | :heavy_check_mark:                                                                                           | Query to be executed                                                                                         |