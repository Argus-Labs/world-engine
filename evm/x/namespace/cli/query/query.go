package query

import (
	"errors"
	"fmt"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/version"
	"github.com/spf13/cobra"
	"pkg.world.dev/world-engine/evm/x/namespace/types"
)

func NewQueryCmd() *cobra.Command {
	queryCmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Namespace query subcommands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2, //nolint:gomnd // not needed
		RunE:                       client.ValidateCmd,
	}

	queryCmd.AddCommand(
		NewQueryNamespacesCmd(),
		NewQueryAddressCmd(),
	)
	return queryCmd
}

func NewQueryNamespacesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "Return a list of all namespace:grpc address pairs",
		Example: fmt.Sprintf("%s query namespace list", version.AppName),
		Args:    cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			query := types.NamespacesRequest{}

			queryClient := types.NewQueryServiceClient(clientCtx)
			res, err := queryClient.Namespaces(cmd.Context(), &query)
			if err != nil {
				return err
			}
			return clientCtx.PrintProto(res)
		},
	}
	return cmd
}

func NewQueryAddressCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "address [namespace]",
		Short:   "Return the address associated with a given namespace",
		Example: fmt.Sprintf("%s query namespace address foobar", version.AppName),
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			ns := args[0]
			if ns == "" {
				return errors.New("namespace is required")
			}
			query := types.AddressRequest{Namespace: ns}

			queryClient := types.NewQueryServiceClient(clientCtx)
			res, err := queryClient.Address(cmd.Context(), &query)
			if err != nil {
				return err
			}
			return clientCtx.PrintProto(res)
		},
	}
	return cmd
}
