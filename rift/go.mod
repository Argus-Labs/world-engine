module pkg.world.dev/world-engine/rift

go 1.21.0

require (
	google.golang.org/grpc v1.59.0
	google.golang.org/protobuf v1.31.0
)

require (
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	golang.org/x/net v0.17.0 // indirect
	golang.org/x/sys v0.14.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20231030173426-d783a09b4405 // indirect
)

replace (
	google.golang.org/grpc => google.golang.org/grpc v1.50.0
	google.golang.org/protobuf => google.golang.org/protobuf v1.28.1
)
