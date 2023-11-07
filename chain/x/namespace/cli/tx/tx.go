package tx

import (
	"errors"
	"fmt"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/version"
	"github.com/spf13/cobra"
	"net"
	namespacetypes "pkg.world.dev/world-engine/chain/x/namespace/types"
	"strings"
)

// NewTxCmd returns a root CLI command handler for all x/bank transaction commands.
func NewTxCmd() *cobra.Command {
	txCmd := &cobra.Command{
		Use:                        namespacetypes.ModuleName,
		Short:                      "Namespace transaction subcommands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
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
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace := args[0]
			grpcAddress := args[1]

			if namespace == "" {
				return errors.New("namespace is required")
			}
			if grpcAddress == "" {
				return errors.New("gRPC address is required")
			}

			if !isValidGRPCAddress(grpcAddress) {
				return errors.New("invalid gRPC address. please ensure format is `host:port_number`")
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

func isValidGRPCAddress(address string) bool {
	// Split the address into host and port
	parts := strings.Split(address, ":")
	if len(parts) != 2 {
		return false
	}

	// Check if the host is a valid IP address or hostname
	host := parts[0]
	if net.ParseIP(host) == nil {
		// If it's not a valid IP, check if it's a valid hostname
		if _, err := net.LookupHost(host); err != nil {
			return false
		}
	}

	// Check if the port is a valid number
	port := parts[1]
	if _, err := net.LookupPort("tcp", port); err != nil {
		return false
	}

	return true
}
