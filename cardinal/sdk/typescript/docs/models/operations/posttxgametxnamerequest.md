# PostTxGameTxNameRequest

## Example Usage

```typescript
import { PostTxGameTxNameRequest } from "cardinal/models/operations";

let value: PostTxGameTxNameRequest = {
    txName: "<value>",
    cardinalServerHandlerTransaction: {},
};
```

## Fields

| Field                                                                                                      | Type                                                                                                       | Required                                                                                                   | Description                                                                                                |
| ---------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| `txName`                                                                                                   | *string*                                                                                                   | :heavy_check_mark:                                                                                         | Name of a registered message                                                                               |
| `cardinalServerHandlerTransaction`                                                                         | [components.CardinalServerHandlerTransaction](../../models/components/cardinalserverhandlertransaction.md) | :heavy_check_mark:                                                                                         | Transaction details & message to be submitted                                                              |