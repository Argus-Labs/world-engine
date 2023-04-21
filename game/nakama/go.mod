module github.com/argus-labs/argus/nakama

go 1.19

require (
	buf.build/gen/go/argus-labs/argus/grpc/go v1.2.0-20221220205637-74c1b18192c5.4
	buf.build/gen/go/argus-labs/argus/protocolbuffers/go v1.28.1-20221220205637-74c1b18192c5.4
	github.com/JeremyLoy/config v1.5.0
	github.com/heroiclabs/nakama-common v1.25.0
	google.golang.org/grpc v1.50.0
)

require (
	github.com/golang/protobuf v1.5.2 // indirect
	golang.org/x/net v0.0.0-20220624214902-1bab6f366d9e // indirect
	golang.org/x/sys v0.0.0-20220610221304-9f5ed59c137d // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/genproto v0.0.0-20220822174746-9e6da59bd2fc // indirect
	google.golang.org/protobuf v1.28.1 // indirect
)


replace (
 google.golang.org/protobuf => google.golang.org/protobuf v1.28.1
)
