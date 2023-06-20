package storage

import (
	ormv1alpha1 "cosmossdk.io/api/cosmos/orm/v1alpha1"

	routerv1 "github.com/argus-labs/world-engine/chain/api/router/v1"
)

var (
	ORMPrefix = []byte("router")
)

var ModuleSchema = ormv1alpha1.ModuleSchemaDescriptor{
	SchemaFile: []*ormv1alpha1.ModuleSchemaDescriptor_FileEntry{
		{Id: 1, ProtoFileName: routerv1.File_router_v1_state_proto.Path()},
	},
	Prefix: ORMPrefix,
}
