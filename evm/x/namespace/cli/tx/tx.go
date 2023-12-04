package tx

import (
	"errors"
	"fmt"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/version"
	"github.com/spf13/cobra"
	namespacetypes "pkg.world.dev/world-engine/evm/x/namespace/types"
)

// NewTxCmd returns a root CLI command handler for all x/bank transaction commands.
func NewTxCmd() *cobra.Command {
	txCmd := &cobra.Command{
		Use:                        namespacetypes.ModuleName,
		Short:                      "Namespace transaction subcommands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2, //nolint:gomnd // not needed.
		RunE:                       client.ValidateCmd,
	}

	txCmd.AddCommand(
		NewRegisterNamespaceCmd(),
	)

	return txCmd
}

// NewRegisterNamespaceCmd returns a CLI command handler for registering a namespace + game shard address pair.
// The gRPC address is used for Router calls.
func NewRegisterNamespaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "register [namespace] [gRPC address]",
		Short:   "Register a game shard's gRPC address",
		Long:    `Register a game shard's gRPC address, allowing for cross-shard communication from the EVM.'`,
		Example: fmt.Sprintf("%s tx namespace register foobar api.cool.game:9601", version.AppName),
		Args:    cobra.ExactArgs(2), //nolint:gomnd // not needed
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace := args[0]
			grpcAddress := args[1]

			if namespace == "" {
				return errors.New("namespace is required")
			}
			if grpcAddress == "" {
				return errors.New("gRPC address is required")
			}

			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			msg := namespacetypes.UpdateNamespaceRequest{
				Authority: clientCtx.GetFromAddress().String(),
				Namespace: &namespacetypes.Namespace{
					ShardName:    namespace,
					ShardAddress: grpcAddress,
				},
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), &msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)

	return cmd
}
