package utils

import (
	"github.com/rotisserie/eris"
	"os"
	"strings"
)

const (
	EnvCardinalAddr      = "CARDINAL_ADDR"
	EnvCardinalNamespace = "CARDINAL_NAMESPACE"
)

var (
	GlobalCardinalAddress string
	GlobalNamespace       string
)

func InitCardinalAddress() error {
	GlobalCardinalAddress = os.Getenv(EnvCardinalAddr)
	if GlobalCardinalAddress == "" {
		return eris.Errorf("must specify a cardinal server via %s", EnvCardinalAddr)
	}
	return nil
}

func InitNamespace() error {
	GlobalNamespace = os.Getenv(EnvCardinalNamespace)
	if GlobalNamespace == "" {
		return eris.Errorf("must specify a cardinal namespace via %s", EnvCardinalNamespace)
	}
	return nil
}

func GetDebugModeFromEnvironment() bool {
	devModeString := os.Getenv("ENABLE_DEBUG")
	return strings.ToLower(devModeString) == "true"
}
