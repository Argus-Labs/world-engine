package rollup

import g1 "buf.build/gen/go/argus-labs/argus/grpc/go/v1/sidecarv1grpc"

type EVMNakamaHook struct {
	eventSignature string
	action         func(client g1.NakamaClient) error
}
