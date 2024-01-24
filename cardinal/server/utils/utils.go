package utils

const (
	DefaultPort = "4040"
	TxPrefix    = "/tx/"
	QueryPrefix = "/query/"
)

func GetQueryURL(group string, name string) string {
	return QueryPrefix + group + "/" + name
}

func GetTxURL(group string, name string) string {
	return TxPrefix + group + "/" + name
}
