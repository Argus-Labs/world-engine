package engine

type Query interface {
	// Name returns the name of the query.
	Name() string
	// Group returns the group of the query.
	Group() string
	// HandleQuery handles queries with concrete types, rather than encoded bytes.
	HandleQuery(Context, any) (any, error)
	// HandleQueryRaw is given a reference to the engine, json encoded bytes that represent a query request
	// and is expected to return a json encoded response struct.
	HandleQueryRaw(Context, []byte) ([]byte, error)
	// DecodeEVMRequest decodes bytes originating from the evm into the request type, which will be ABI encoded.
	DecodeEVMRequest([]byte) (any, error)
	// EncodeEVMReply encodes the reply as an abi encoded struct.
	EncodeEVMReply(any) ([]byte, error)
	// DecodeEVMReply decodes EVM reply bytes, into the concrete go reply type.
	DecodeEVMReply([]byte) (any, error)
	// EncodeAsABI encodes a go struct in abi format. This is mostly used for testing.
	EncodeAsABI(any) ([]byte, error)
	// IsEVMCompatible reports if the query is able to be sent from the EVM.
	IsEVMCompatible() bool
}
