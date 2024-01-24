package utils

func GetQueryURL(group string, name string) string {
	return "/tx/" + group + "/" + name
}

func GetTxURL(group string, name string) string {
	return "/query/" + group + "/" + name
}
