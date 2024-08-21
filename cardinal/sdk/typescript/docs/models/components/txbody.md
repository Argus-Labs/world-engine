# TxBody

## Example Usage

```typescript
import { TxBody } from "cardinal/models/components";

let value: TxBody = {};
```

## Fields

| Field                 | Type                  | Required              | Description           |
| --------------------- | --------------------- | --------------------- | --------------------- |
| `body`                | Record<string, *any*> | :heavy_minus_sign:    | json string           |
| `hash`                | *string*              | :heavy_minus_sign:    | N/A                   |
| `namespace`           | *string*              | :heavy_minus_sign:    | N/A                   |
| `nonce`               | *number*              | :heavy_minus_sign:    | N/A                   |
| `personaTag`          | *string*              | :heavy_minus_sign:    | N/A                   |
| `signature`           | *string*              | :heavy_minus_sign:    | hex encoded string    |