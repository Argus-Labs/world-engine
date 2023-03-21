package main

import "github.com/JeremyLoy/config"

type Config struct {
	SidecarTarget string `config:"SIDECAR_TARGET"`
	UseReceiver   bool   `config:"USE_RECEIVER"`
	ReceiverPort  uint64 `config:"RECEIVER_PORT"`
}

func LoadConfig() Config {
	var cfg Config
	err := config.FromEnv().To(&cfg)
	if err != nil {
		panic(err)
	}
	return cfg
}
