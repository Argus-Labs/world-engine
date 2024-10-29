package utils

import (
	"strings"

	"pkg.world.dev/world-engine/cardinal/types/message"
)

func GetQueryURL(group string, name string) string {
	return "/query/" + group + "/" + name
}

func GetTxURL(name string) string {
	nameParts := strings.Split(name, ".")
	if len(nameParts) == 1 {
		return "/tx/" + message.DefaultGroup + "/" + nameParts[0]
	}
	return "/tx/" + nameParts[0] + "/" + nameParts[1]
}
