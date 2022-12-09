package simparams

import (
	"strings"

	serverconfig "github.com/cosmos/cosmos-sdk/server/config"
)

var (
	// BypassMinFeeMsgTypesKey defines the configuration key for the
	// BypassMinFeeMsgTypes value.
	//nolint: gosec
	BypassMinFeeMsgTypesKey = "bypass-min-fee-msg-types"

	// customArgusConfigTemplate defines Argus's custom application configuration TOML template.
	customArgusConfigTemplate = `
###############################################################################
###                        Custom Argus Configuration                        ###
###############################################################################
# bypass-min-fee-msg-types defines custom message types the operator may set that
# will bypass minimum fee checks during CheckTx.
#
# Example:
# ["/ibc.core.channel.v1.MsgRecvPacket", "/ibc.core.channel.v1.MsgAcknowledgement", ...]
bypass-min-fee-msg-types = [{{ range .BypassMinFeeMsgTypes }}{{ printf "%q, " . }}{{end}}]
`
)

// CustomConfigTemplate defines Argus's custom application configuration TOML
// template. It extends the core SDK template.
func CustomConfigTemplate() string {
	config := serverconfig.DefaultConfigTemplate
	lines := strings.Split(config, "\n")
	// add the Argus config at the second line of the file
	lines[2] += customArgusConfigTemplate
	return strings.Join(lines, "\n")
}

// CustomAppConfig defines Argus's custom application configuration.
type CustomAppConfig struct {
	serverconfig.Config

	// BypassMinFeeMsgTypes defines custom message types the operator may set that
	// will bypass minimum fee checks during CheckTx.
	BypassMinFeeMsgTypes []string `mapstructure:"bypass-min-fee-msg-types"`
}
