package public

import "github.com/invopop/jsonschema"

type IRead interface {
	// Name returns the name of the read.
	Name() string
	// HandleRead handles reads with concrete types, rather than encoded bytes.
	HandleRead(IWorld, any) (any, error)
	// HandleReadRaw is given a reference to the world, json encoded bytes that represent a read request
	// and is expected to return a json encoded response struct.
	HandleReadRaw(IWorld, []byte) ([]byte, error)
	// Schema returns the json schema of the read request.
	Schema() (request, reply *jsonschema.Schema)
	// DecodeEVMRequest decodes bytes originating from the evm into the request type, which will be ABI encoded.
	DecodeEVMRequest([]byte) (any, error)
	// EncodeEVMReply encodes the reply as an abi encoded struct.
	EncodeEVMReply(any) ([]byte, error)
	// DecodeEVMReply decodes EVM reply bytes, into the concrete go reply type.
	DecodeEVMReply([]byte) (any, error)
	// EncodeAsABI encodes a go struct in abi format. This is mostly used for testing.
	EncodeAsABI(any) ([]byte, error)
}
