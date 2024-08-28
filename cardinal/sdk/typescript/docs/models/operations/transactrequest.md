# TransactRequest

## Example Usage

```typescript
import { TransactRequest } from "@arguslabs/cardinal/models/operations";

let value: TransactRequest = {
    txName: "attack-player",
    txBody: {
        personaTag: "CoolMage",
    },
};
```

## Fields

| Field                                                  | Type                                                   | Required                                               | Description                                            | Example                                                |
| ------------------------------------------------------ | ------------------------------------------------------ | ------------------------------------------------------ | ------------------------------------------------------ | ------------------------------------------------------ |
| `txName`                                               | *string*                                               | :heavy_check_mark:                                     | Name of a registered message                           | attack-player                                          |
| `txBody`                                               | [components.TxBody](../../models/components/txbody.md) | :heavy_check_mark:                                     | Transaction details & message to be submitted          | {<br/>"personaTag": "CoolMage"<br/>}                   |