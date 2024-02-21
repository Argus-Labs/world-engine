/*
Package ecb allows for buffering of state changes to the ECS storage layer, and either committing those changes
in an atomic Redis transaction, or discarding the changes. In either case, the underlying Redis DB is never in an
intermediate state.

# Atomic options

There are two ways a batch of state changes can be grouped and applied/discarded.

EntityCommandBuffer.AtomicFn takes in a function that returns an error. The passed in function will be executed, and
any state made during that function call will be stored as pending state changes. During this time, reads using the
EntityCommandBuffer will report the pending values. Conversely, reading data directly from Redis the original value
(before AtomicFn was called).

If the passed in function returns an error, all pending state changes will be discarded.

If the passed in function returns no error, all pending state changes will be committed to Redis in an atomic
transaction.

Alternatively, EntityCommandBuffer can be used outside an AtomicFn context. State changes are stored as pending
operations. Read operations will report the pending state. Note, no changes to Redis are applied while pending
operations are accumulated.

Pending changes can be discarded with EntityCommandBuffer.DiscardPending. A subsequent read will return identical
data to the data stored in Redis.

Pending changes can be committed to redis with EntityCommandBuffer.FinalizeTick. All pending changes will
be packaged into a single redis [multi/exec pipeline](https://redis.io/docs/interact/transactions/) and applied
atomically. Reads to redis during this time will never return any pending state. For example, if a series of 100
commands increments some value from 0 to 100, and then FinalizeTick is called, reading this value from the DB will
only ever return 0 or 100 (depending on the exact timing of the call).

# Redis PrimitiveStorage Model

The Redis keys that store data in redis are defined in keys.go. All keys are prefixed with "ECB".

key:	"ECB:NEXT-ENTITY-ID"
value: 	An integer that represents the next available entity ID that can be assigned to some entity. It can be assumed
that entity IDs smaller than this value have already been assigned.

key:	fmt.Sprintf("ECB:COMPONENT-VALUE:TYPE-ID-%d:ENTITY-ID-%d", componentTypeID, entityID)
value: 	JSON serialized bytes that can be deserialized to the component with the matching componentTypeID. This
component data has been assigned to the entity matching the entityID.

key:	fmt.Sprintf("ECB:ARCHETYPE-ID:ENTITY-ID-%d", entityID)
value: 	An integer that represents the archetype ID that the matching entityID has been assigned to.

key: 	fmt.Sprintf("ECB:ACTIVE-ENTITY-IDS:ARCHETYPE-ID-%d", archetypeID)
value:	JSON serialized bytes that can be deserialized to a slice of integers. The integers represent the entity IDs
that currently belong to the matching archetypeID. Note, this is a reverse mapping of the previous key.

key:	"ECB:ARCHETYPE-ID-TO-COMPONENT-TYPES"
value:	JSON serialized bytes that can be deserialized to a map of archetype.ID to []component.ID. This field represents
what archetype IDs have already been assigned and what groups of components each archetype ID corresponds to. This field
must be loaded into memory before any entity creation or component addition/removals take place.

key: 	"ECB:START-TICK"
value:  An integer that represents the last tick that was started.

key: 	"ECB:END-TICK"
value: 	An integer that represents the last tick that was successfully completed.

key: 	"ECB:PENDING-TRANSACTIONS"
value:  JSON serialized bytes that can be deserialized to a list of transactions. These are the transactions that were
processed in the last started tick. This data is only relevant when the START-TICK number does not match the END-TICK
number.

# In-memory storage model

The in-memory data model roughly matches the model that is stored in redis, but there are some differences:

Components are stored as generic interfaces and not as serialized JSON.

# Potential Improvements

In redis, the ECB:ACTIVE-ENTITY-IDS and ECB:ARCHETYPE-ID:ENTITY-ID keys contains the same data, but are just reversed
mapping of one another. The amount of data in redis, and the data written can likely be reduced if we abandon one of
these keys and rebuild the other mapping in memory.

In memory, compValues are written to redis during a FinalizeTick cycle. Components that were not actually changed (e.g.
only read operations were performed) are still written to the DB.
*/
package gamestate
