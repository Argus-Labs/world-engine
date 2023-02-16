package sidecar

import (
	jlconfig "github.com/JeremyLoy/config"
)

type Config struct {
	SidecarPort int `config:"SIDECAR_PORT"`
}

func LoadConfig() Config {
	var cfg Config
	err := jlconfig.FromEnv().To(&cfg)
	if err != nil {
		panic(err)
	}
	return cfg
}
