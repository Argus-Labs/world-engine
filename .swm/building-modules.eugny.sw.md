---
id: eugny
title: Building Modules
file_version: 1.1.2
app_version: 1.8.5
---

# Structure of Cosmos Modules

Cosmos modules generally have 6 parts to them: protobuf definitions, codec, Message implmentations, Module definition, Keeper implementation, CLI functionality, and app wiring.

# Protobuf Definitions

**Please follow the** [Protobuf Style Guide](https://developers.google.com/protocol-buffers/docs/style) **when building new protobuf modules, or editing existing protobuf modules.**

### Generating Protobuf Files

If you’ve made edits or built new protobuf modules, you can generate the protobuf stubs using [Mage](https://magefile.org/). Run `mage proto:all` in the root directory to generate, format, and lint all protobuf files.

### Defining New Protobuf Modules

Protobuf definitions are the outer skeleton of what your module will look like. This is generally the first place cosmos devs look to see how your module will work. Modules will commonly define 3 protobuf files:

*   Msg service (transactions/state transitions) `tx.proto`

    *   this defines the RPC endpoints for the transactions, and their corresponding request and return types.

        Example:

    ```protobuf
    syntax = "proto3";

    package argus.adapter.v1;

    option go_package = "github.com/argus-labs/argus/x/adapter/types/v1";

    service Msg {
      // ClaimQuestReward claims a quest reward.
      rpc ClaimQuestReward(MsgClaimQuestReward) returns (MsgClaimQuestRewardResponse);
    }

    // MsgClaimQuestReward is the Msg/ClaimQuestReward request type.
    message MsgClaimQuestReward {
      // user_id is the game client user_id.
      string user_id = 1;

      // quest_id is the id of the quest that was completed.
      string quest_id = 2;
    }

    // MsgClaimQuestRewardResponse is the Msg/ClaimQuestReward response type.
    message MsgClaimQuestRewardResponse {
      // reward_id is the ID of the reward claimed.
      string reward_id = 1;
    }
    ```

    <br/>

*   Query service (state queries) `query.proto`

    *   this defines the query RPC endpoints that can read from the blockchain state.

    *   query message request types should be prefixed by `Query` and post-fixed by `Request`

    *   query message return types should be prefixed by `Query` post-fixed by `Response`

    Example:

    ```protobuf
    prosyntax = "proto3";
    package ethermint.evm.v1;
    option go_package = "github.com/evmos/ethermint/x/evm/types";

    import "gogoproto/gogo.proto";
    import "google/api/annotations.proto";

    // Query defines the gRPC querier service.
    service Query {
      // Account queries an Ethereum account.
      rpc Account(QueryAccountRequest) returns (QueryAccountResponse) {
        option (google.api.http).get = "/ethermint/evm/v1/account/{address}";
      }
    }

    // QueryAccountRequest is the request type for the Query/Account RPC method.
    message QueryAccountRequest {
      option (gogoproto.equal) = false;
      option (gogoproto.goproto_getters) = false;

      // address is the ethereum hex address to query the account for.
      string address = 1;
    }

    // QueryAccountResponse is the response type for the Query/Account RPC method.
    message QueryAccountResponse {
      // balance is the balance of the EVM denomination.
      string balance = 1;
      // code_hash is the hex-formatted code bytes from the EOA.
      string code_hash = 2;
      // nonce is the account's sequence number.
      uint64 nonce = 3;
    }
    ```

*   Genesis `genesis.proto`

    *   This file defines the state needed to boot the module up with an initial state. For example, in the EVM module, some accounts need already be instantiated (i.e. for airdrops). Here is an example of such a file:

    ```protobuf
    syntax = "proto3";
    package ethermint.evm.v1;

    import "ethermint/evm/v1/evm.proto";
    import "gogoproto/gogo.proto";

    option go_package = "github.com/evmos/ethermint/x/evm/types";

    // GenesisState defines the evm module's genesis state.
    message GenesisState {
      // accounts is an array containing the ethereum genesis accounts.
      repeated GenesisAccount accounts = 1 [(gogoproto.nullable) = false];
      // params defines all the parameters of the module.
      Params params = 2 [(gogoproto.nullable) = false];
    }

    // GenesisAccount defines an account to be initialized in the genesis state.
    // Its main difference between with Geth's GenesisAccount is that it uses a
    // custom storage type and that it doesn't contain the private key field.
    message GenesisAccount {
      // address defines an ethereum hex formated address of an account
      string address = 1;
      // code defines the hex bytes of the account code.
      string code = 2;
      // storage defines the set of state key values for the account.
      repeated State storage = 3 [(gogoproto.nullable) = false, (gogoproto.castrepeated) = "Storage"];
    }
    ```

    *   GenesisState is the main object that will contain all the initial state needed for the module. Repeated nested messages will be iterated over and injected into the module state.

    Note: Your `go_package` option in the proto file should always point to the `types/<version>/` directory of your module.

    ### Caveats

    When creating messages or editing existing ones, there is one caveat with regard to protobuf types. While most types are okay for use in Cosmos SDK apps, the `map` type should be strictly avoided at all costs. `Map` iteration in Go is **NOT** deterministic, a property we must adhere to in blockchains. if you need a multi object container type, please use `repeated`.

# Codec

Your generated types and services must be registered for use in the Cosmos SDK codec.

First, define the global module codec. This will be used in some of the module’s `AppModule` implementations.

```go
import (
    "github.com/cosmos/cosmos-sdk/codec"
)

var (
    amino = codec.NewLegacyAmino()

    // ModuleCdc references the global module codec. Note, the codec should
    // ONLY be used in certain instances of tests and for JSON encoding as Amino is
    // still used for that purpose.
    //
    // The actual codec used for serialization should be provided to this module and
    // defined at the application level.
    ModuleCdc = codec.NewAminoCodec(amino)
)
```

If you want your types to be usable with Amino (for Ledger signing) you will need a `RegisterLegacyAminoCodec` that looks like this:

```go
// RegisterLegacyAminoCodec registers all the necessary types and interfaces for the module.
func RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
    cdc.RegisterConcrete(&MsgSomeMessage{}, "my-app/MsgSomeMessage", nil)
    // .. the rest of your messages ..
}
```

Then, we register the Msgs that will be used in transactions. To do this, create a function that called `RegisterInterfaces`. Example:

```go
func RegisterInterfaces(registry types.InterfaceRegistry) {
    registry.RegisterImplementations((*sdk.Msg)(nil),
        &MsgSomeMessage{},
        &MsgSomeOtherMessage{},
    )

    // &_Msg_serviceDesc is defined in tx.pb.go
    msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
```

Finally, we define an init function that Registers the amino codec when this module is imported into the app.

```go

func init() {
    RegisterLegacyAminoCodec(amino)
}
```

# Messages

🧑🏻‍💻If your module does not have any transaction messages, you can skip this part.

For your transaction messages to work in Cosmos SDK, they must implement the Cosmos Msg interface.

```go
type Msg interface {
    proto.Message
    ValidateBasic() error
    GetSigners() []AccAddress
}
```

`ValidateBasic` does stateless validation of your message. This can be useful for ensuring the provided input is valid before actually running a state transition function. For example, if your message contains an address string, you could check that the address has the correct format in `ValidateBasic`.

`GetSigners` returns the address of the signer of your message.

If you want your messages to work with amino, you must also implement the `LegacyMsg` interface.

```go
type LegacyMsg interface {
    types.Msg
    GetSignBytes() []byte
    Route() string
    Type() string
}
```

`GetSignBytes` returns the the bytes to sign of the message. This MUST be done using the global module codec defined in the [codec section](https://www.notion.so/Building-Modules-791d5f00f0a3480ca8124115d24a7260) :

```go
func (m MyMsg) GetSignBytes() []byte {
        return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(&m))
}
```

Both `Route` and `Type` simply return `sdk.MsgTypeURL(&m)`.

# AppModule Implementations

Every Cosmos module requires the implementation of an AppModule interface. AppModules are managed by baseapp’s module manager in the Cosmos SDK. The module manager which will take care of routing messages, registering the module’s services (query, tx, etc), initializing and exporting genesis, running end/begin block functions (if applicable), and registering the modules command line functions. [https://pkg.go.dev/github.com/cosmos/cosmos-sdkv0.46.6/types/module](https://pkg.go.dev/github.com/cosmos/cosmos-sdk@v0.46.6/types/module)

Modules implementations are generally placed in a file called `module.go` in the root of your module. For example, if you had a module named `Foo`, you would have `x/foo.module.go`

Start by importing the module package that contains the necessary interfaces:

`"github.com/cosmos/cosmos-sdk/types/module"`

The following interfaces are **required** to be implemented to be plugged into Cosmos SDK:

*   AppModuleBasic

*   AppModule

In order to ensure you fully implement these interfaces, place interface guards at the top of your file:

```go
var _ module.AppModule = YourAppModuleStruct{}
var _ module.AppModuleBasic = YourBasicAppModuleStruct{}

// if your module has EndBlock and/or BeginBlock code, 
// you can replace the module.AppModule interface guard with the following:

var _ module.BeginBlockAppModule = YourAppModuleStruct{}
var _ module.EndBlockAppModule = YourAppModuleStruct{}

type YourAppModuleStruct struct {}
type YourBasicAppModule struct {}

// .... implement the corresponding interface functions on these types.
```

Some of the function implementations may be no-ops, or simply return nil, if the functionality provided by the module is not needed. For example, some Modules that don’t have any queries will likely not have CLI query commands, so `GetQueryCmd()` from AppModuleBasic will just return nil.

AppModule implementation files can be quite long, so if you need examples for how certain functions should be implemented, you can take a look at these repositories:

*   [https://github.com/osmosis-labs/osmosis/blob/7374795e0de22f3a291ca59c5faffa7851acf3bd/x/superfluid/module.go](https://github.com/osmosis-labs/osmosis/blob/7374795e0de22f3a291ca59c5faffa7851acf3bd/x/superfluid/module.go)

*   [https://github.com/regen-network/regen-ledger/blob/release/v5.0.x/x/ecocredit/module/module.go](https://github.com/regen-network/regen-ledger/blob/release/v5.0.x/x/ecocredit/module/module.go)

*   [https://github.com/cosmos/cosmos-sdk/blob/release/v0.46.x/x/bank/module.go](https://github.com/cosmos/cosmos-sdk/blob/release/v0.46.x/x/bank/module.go)

# Keeper Implementations

Keepers are objects that implement the Msg service defined in your proto files. They handle state transitions, event emission, queries, and other custom functionality.

## Fields

Keepers that read or write from state need to have a store key. Store keys will be talked about in a different section TODO so for now just place it as a field in your struct if needed. you can import store keys from `"github.com/cosmos/cosmos-sdk/store/types"`.

Other dependencies that your module may have can be placed as fields in the keeper struct. For example if your module needs to be able to mint or transfer coins, you would need to have the bank keeper as a field on your keeper struct.

### Implementing services

Keepers must implement the server interfaces generated by protobuf. This will generally be a `QueryServer` and a `MsgServer`. To ensure your keeper fully implements the server, add interface guards:

```go
package Keeper
import (
    "github.com/evmos/ethermint/types"
    storetypes "github.com/cosmos/cosmos-sdk/store/types"
)

var (
    _ types.MsgServer   = &Keeper{}
    _ types.QueryServer = &Keeper{}
)

// Keeper implements the msg and query services.
type Keeper struct {
    storeKey storetypes.StoreKey
}

// ... implement the functions on the interfaces ...
```

### Reading from and Writing to state

Modules will commonly have to read from and write to state. This can be done through the SDK context. Before messages get routed to your keeper functions, the application will inject an sdk context into the base context.Context. You can extract with with the following helper function:

```go
package Keeper

import (
    "context"

    sdk "github.com/cosmos/cosmos-sdk/types"

    "github.com/argus-labs/argus/x/myModule/types"
)

// ... keeper stuff
//

func (k Keeper) SomeMsgServerFunction(ctx context.Context, types.MsgSomeMsg) (types.MsgSomeMsgResponse, error) {
    sdkCtx := sdk.UnwrapSDKContext(ctx)
}
```

Stores can now be accessed using the sdkCtx:

```go
// ... snip ...
    sdkCtx := sdk.UnwrapSDKContext(ctx)
    store := sdkCtx.KVStore(k.storeKey) // k is the keeper receiver
    store.Set([]byte("key"), []byte("value"))
```

Often times, you have multiple different state objects you’d like to store. In this case, a prefix store is useful. This allows all your objects to be saved under a specific prefix. For example, if you had a pet shelter module, you might store all `Dog` objects under the prefix `[]byte("dogs")`and cats under the prefix `[]byte("cats")`.

💁🏻 For storage efficiency however, its often best to define your key as a constant with just a single byte. For example:

```go
const (
    DogPrefix = []byte{0x1}
    CatPrefix = []byte{0x2}
    // etc...
)
```

To access a prefix store you can use the prefix package in cosmos sdk:

```go
import (
    // ... snip ....
    "github.com/cosmos/cosmos-sdk/store/prefix"
)

// ... snip ...
    sdkCtx := sdk.UnwrapSDKContext(ctx)
    store := sdkCtx.KVStore(k.storeKey) // k is the keeper receiver
    dogStore := prefix.NewStore(store, DogPrefix)
```

Keeper implementations will vary based on use case, but most modules will be structured similarly. You can refer to the below repos to see examples:

*   [https://github.com/cosmos/cosmos-sdk/blob/release/v0.46.x/x/authz/keeper/keeper.go](https://github.com/cosmos/cosmos-sdk/blob/release/v0.46.x/x/authz/keeper/keeper.go)

*   [https://github.com/evmos/ethermint/blob/main/x/evm/keeper/msg\_server.go](https://github.com/evmos/ethermint/blob/main/x/evm/keeper/msg_server.go)

*   [https://github.com/osmosis-labs/osmosis/blob/main/x/gamm/keeper/msg\_server.go](https://github.com/osmosis-labs/osmosis/blob/main/x/gamm/keeper/msg_server.go)

# Command Line Functionality

Modules typically come with command line utilities to interact with the msg and query services. These are usually defined in a subpackage within your module with the path: `x/module/client/cli tx.go , query.go`.

Modules should have a root transaction command, and root query command. The individual msg and query functions will be added as subcommands to these root commands. CLI commands are pretty generally pretty straightforward so an example is mostly all thats needed to understand them.

```go
package client

import (
    "github.com/spf13/cobra"

    sdkclient "github.com/cosmos/cosmos-sdk/client"
)

// TxCmd returns a root CLI command handler for all x/ecocredit transaction commands.
func TxCmd(name string) *cobra.Command {
    cmd := &cobra.Command{
        SuggestionsMinimumDistance: 2,
        DisableFlagParsing:         true,

        Use:   name,
        Short: "Ecocredit module transactions",
        RunE:  sdkclient.ValidateCmd,
    }
    cmd.AddCommand(
        TxCreateClassCmd(),
    )
    return cmd
}

// TxCreateClassCmd returns a transaction command that creates a credit class.
func TxCreateClassCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "create-class [issuers] [credit-type-abbrev] [metadata] [flags]",
        Short: "Creates a new credit class with transaction author (--from) as admin",
        Example: `regen tx ecocredit create-class regen1elq7ys34gpkj3jyvqee0h6yk4h9wsfxmgqelsw C regen:13toVgf5UjYBz6J29x28pLQyjKz5FpcW3f4bT5uRKGxGREWGKjEdXYG.rdf --class-fee 20000000uregen`,
        Args: cobra.ExactArgs(3),
        RunE: func(cmd *cobra.Command, args []string) error {
            clientCtx, err := sdkclient.GetClientTxContext(cmd)
            if err != nil {
                return err
            }

            // Get the class admin from the --from flag
            admin := clientCtx.GetFromAddress()

            // parse the comma-separated list of issuers
            issuers := strings.Split(args[0], ",")
            for i := range issuers {
                issuers[i] = strings.TrimSpace(issuers[i])
            }

            msg := types.MsgCreateClass{
                Admin:            admin.String(),
                Issuers:          issuers,
                Metadata:         args[2],
                CreditTypeAbbrev: args[1],
            }

            // parse and normalize credit class fee
            feeString, err := cmd.Flags().GetString(FlagClassFee)
            if err != nil {
                return err
            }
            if feeString != "" {
                fee, err := sdk.ParseCoinNormalized(feeString)
                if err != nil {
                    return fmt.Errorf("failed to parse class-fee: %w", err)
                }

                msg.Fee = &fee
            }

            return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), &msg)
        },
    }

    cmd.Flags().String(FlagClassFee, "", "the fee that the class creator will pay to create the credit class (e.g. \"20regen\")")

    return txFlags(cmd)
}
```

Your AppModuleBasic’s can then return `TxCmd` from the `GetTxCmd()`function.

# App Wiring

TODO: update

<br/>

This file was generated by Swimm. [Click here to view it in the app](https://app.swimm.io/repos/Z2l0aHViJTNBJTNBd29ybGQtZW5naW5lJTNBJTNBQXJndXMtTGFicw==/docs/eugny).
