package utils

import (
	jlconfig "github.com/JeremyLoy/config"
)

// LoadConfig will load any matching environment variables into the generic struct passed to the function.
// The config package can match struct fields in PascalCase to env variables of snake case.
// for example:
//
//	type Config struct {
//			FooBar int
//	}
//
// ENV:
//
//	FOO_BAR=15
//
// this will load 15 into the FooBar field above.
//
// additionally, you can use field tags to match the environment variable casing exactly.
//
//	type Config struct {
//			someRandomField string `config:"I_WANT_THIS_FIELD"
//	}
func LoadConfig[cfg any]() (cfg, error) {
	var c cfg
	err := jlconfig.FromEnv().To(&c)
	return c, err
}
