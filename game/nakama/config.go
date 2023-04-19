package main

import "github.com/JeremyLoy/config"

type Config struct {
	SidecarTarget  string `config:"SIDECAR_TARGET"`
	CardinalTarget string `config:"CARDINAl_TARGET"`
}

func LoadConfig() Config {
	var cfg Config
	err := config.FromEnv().To(&cfg)
	if err != nil {
		panic(err)
	}
	return cfg
}
