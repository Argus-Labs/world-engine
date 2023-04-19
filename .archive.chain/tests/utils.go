package tests

import jlconfig "github.com/JeremyLoy/config"

// LoadConfig loads the config from env variables.
func LoadConfig[T any]() T {
	var cfg T
	err := jlconfig.FromEnv().To(&cfg)
	if err != nil {
		panic(err)
	}
	return cfg
}
