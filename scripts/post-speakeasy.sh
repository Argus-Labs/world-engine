#!/usr/bin/bash

sed -i 's/privateKey?: string | undefined;/privateKey: string;/' cardinal/sdk/typescript/src/lib/config.ts
sed -i 's/privateKey?: string | undefined;/privateKey: string;/' cardinal/sdk/typescript/lib/config.d.ts

sed -i 's/namespace?: string | undefined;/namespace: string;/' cardinal/sdk/typescript/src/lib/config.ts
sed -i 's/namespace?: string | undefined;/namespace: string;/' cardinal/sdk/typescript/lib/config.d.ts

sed -i 's/serverURL?: string;/serverURL: string;/' cardinal/sdk/typescript/src/lib/config.ts
sed -i 's/serverURL?: string;/serverURL: string;/' cardinal/sdk/typescript/lib/config.d.ts
