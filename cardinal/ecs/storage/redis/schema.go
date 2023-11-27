package redis

type SchemaStorage interface {
	GetSchema(componentName string) ([]byte, error)
	SetSchema(componentName string, schemaData []byte) error
}
