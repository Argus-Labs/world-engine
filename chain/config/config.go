package config

type WorldEngineConfig struct {
	BaseDenom    string `json:"baseDenom,omitempty" config:"BASE_DENOM"`
	BaseDecimals int64  `json:"baseDecimals" config:"BASE_DECIMALS"`

	DisplayDenom    string `json:"displayDenom" config:"DISPLAY_DENOM"`
	DisplayDecimals int64  `json:"displayDecimals" config:"DISPLAY_DECIMALS"`

	Bech32Prefix string `json:"bech32Prefix" config:"BECH32_PREFIX"`

	RouterAuthority string `json:"routerAuthority" config:"ROUTER_AUTHORITY"`
}
