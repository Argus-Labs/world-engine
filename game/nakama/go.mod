module github.com/argus-labs/argus/nakama

go 1.19

require (
	buf.build/gen/go/argus-labs/argus/grpc/go v1.2.0-20221220205637-74c1b18192c5.4
	buf.build/gen/go/argus-labs/argus/protocolbuffers/go v1.28.1-20221220205637-74c1b18192c5.4
	buf.build/gen/go/argus-labs/cardinal/grpc/go v1.3.0-20230419204405-6273c6504412.1
	buf.build/gen/go/argus-labs/cardinal/protocolbuffers/go v1.28.1-20230419204405-6273c6504412.4
	github.com/JeremyLoy/config v1.5.0
	github.com/golang/protobuf v1.5.2
	github.com/heroiclabs/nakama-common v1.25.0
	google.golang.org/grpc v1.53.0
)

require (
	golang.org/x/net v0.5.0 // indirect
	golang.org/x/sys v0.4.0 // indirect
	golang.org/x/text v0.6.0 // indirect
	google.golang.org/genproto v0.0.0-20230110181048-76db0878b65f // indirect
	google.golang.org/protobuf v1.30.0 // indirect
)

replace google.golang.org/grpc => google.golang.org/grpc v1.50.0
