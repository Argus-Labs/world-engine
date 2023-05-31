package config

type WorldEngineConfig struct {
	// DisplayDenom is the display denomination of the staking coin. (i.e. ATOM).
	DisplayDenom string `yaml:"displayDenom" json:"displayDenom"`
	// BaseDenom is the base denomination of the staking coin. (i.e. uATOM).
	BaseDenom string `yaml:"baseDenom" json:"baseDenom"`

	// Bech32Prefix is the prefix that all accounts on the chain will have. (i.e. cosmos, akash, evmos...).
	Bech32Prefix string `json:"bech32Prefix" yaml:"bech32Prefix"`

	// RouterAuthority is the address that will be able to update the shard namespace mapping.
	// If left blank, the governance module address will be used, and router namespaces will be updated via
	// chain governance.
	RouterAuthority string `json:"routerAuthority" yaml:"routerAuthority"`
}
