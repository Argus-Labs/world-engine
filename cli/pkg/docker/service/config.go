package service

import (
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

type Config struct {
	RootDir   string
	Namespace string
	NATSURL   string
	Debug     bool
	Timeout   int
	WorldToml worldtoml.Config
}
