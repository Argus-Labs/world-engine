package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
	"pkg.world.dev/world-engine/sign"
)

// Archetypes can just be stored in program memory. It just a structure that allows us to quickly
// decipher combinations of components. There is no point in storing such information in a backend.
// at the very least, we may want to store the list of entities that an archetype has.
//
// Archetype -> group of entities for specific set of components. makes it easy to find entities based on comps.
// example:
//
//
//
// Normal Planet Archetype(1): EnergyComponent, OwnableComponent
// Entities (1), (2), (3)....
//
// In Go memory -> []Archetypes {arch1 (maps to above)}
//
// We need to consider if this needs to be stored in a backend at all. We _should_ be able to build archetypes from
// system restarts as they don't really contain any information about the location of anything stored in a backend.
//
// Something to consider -> we should do something i.e. RegisterComponents, and have it deterministically assign TypeID's to components.
// We could end up with some issues (needs to be determined).

type RedisStorage struct {
	WorldID                string
	ComponentStoragePrefix component.TypeID
	Client                 *redis.Client
	Log                    zerolog.Logger
	ArchetypeCache         ArchetypeAccessor
}

type Options = redis.Options

func NewRedisStorage(options Options, worldID string) RedisStorage {
	return RedisStorage{
		WorldID: worldID,
		Client:  redis.NewClient(&options),
		Log:     zerolog.New(os.Stdout),
	}
}

// ---------------------------------------------------------------------------
// 							COMPONENT INDEX STORAGE
// ---------------------------------------------------------------------------

var _ ComponentIndexStorage = &RedisStorage{}

func (r *RedisStorage) GetComponentIndexStorage(cid component.TypeID) ComponentIndexStorage {
	r.ComponentStoragePrefix = cid
	return r
}

// ComponentIndex returns the current component index for this archetype.
// If this archetype is missing, 0, false, nil will be returned. If you plan on using this index
// call IncrementIndex instead and use the returned index.
func (r *RedisStorage) ComponentIndex(ai ArchetypeID) (ComponentIndex, bool, error) {
	ctx := context.Background()
	key := r.archetypeIndexKey(ai)
	res := r.Client.Get(ctx, key)
	if err := res.Err(); err == redis.Nil {
		return 0, false, nil
	} else if err != nil {
		return 0, false, err
	}
	result, err := res.Result()
	if err != nil {
		return 0, false, err
	}
	if len(result) == 0 {
		return 0, false, nil
	}
	ret, err := res.Int()
	if err != nil {
		return 0, false, err
	}
	return ComponentIndex(ret), true, nil
}

func (r *RedisStorage) SetIndex(archID ArchetypeID, compIndex ComponentIndex) error {
	ctx := context.Background()
	key := r.archetypeIndexKey(archID)
	res := r.Client.Set(ctx, key, int64(compIndex), 0)
	return res.Err()
}

// IncrementIndex adds 1 to this archetype and returns the NEW value of the index. If this archetype
// doesn't exist, this index is initialized and 0 is returned.
func (r *RedisStorage) IncrementIndex(archID ArchetypeID) (ComponentIndex, error) {
	ctx := context.Background()
	idx, ok, err := r.ComponentIndex(archID)
	if err != nil {
		return 0, err
	} else if !ok {
		idx = 0
	} else {
		idx++
	}
	key := r.archetypeIndexKey(archID)
	res := r.Client.Set(ctx, key, int64(idx), 0)
	return idx, res.Err()
}

// DecrementIndex decreases the component index for this archetype by 1.
func (r *RedisStorage) DecrementIndex(archID ArchetypeID) error {
	ctx := context.Background()
	idx, ok, err := r.ComponentIndex(archID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("component index not found at archetype index %d", archID)
	}
	key := r.archetypeIndexKey(archID)
	newIdx := idx - 1
	res := r.Client.Set(ctx, key, int64(newIdx), 0)
	return res.Err()
}

// ---------------------------------------------------------------------------
// 							COMPONENT STORAGE MANAGER
// ---------------------------------------------------------------------------

var _ ComponentStorageManager = &RedisStorage{}

func (r *RedisStorage) GetComponentStorage(cid component.TypeID) ComponentStorage {
	r.ComponentStoragePrefix = cid
	return r
}

// ---------------------------------------------------------------------------
// 							COMPONENT STORAGE
// ---------------------------------------------------------------------------

func (r *RedisStorage) PushComponent(component component.IComponentType, archID ArchetypeID) error {
	ctx := context.Background()
	key := r.componentDataKey(archID, r.ComponentStoragePrefix)
	componentBz, err := component.New()
	if err != nil {
		return err
	}
	res := r.Client.RPush(ctx, key, componentBz)
	return res.Err()
}

func (r *RedisStorage) Component(archetypeID ArchetypeID, componentIndex ComponentIndex) ([]byte, error) {
	ctx := context.Background()
	key := r.componentDataKey(archetypeID, r.ComponentStoragePrefix)
	res := r.Client.LIndex(ctx, key, int64(componentIndex))
	if err := res.Err(); err == redis.Nil {
		return nil, ErrorComponentNotOnEntity
	} else if err != nil {
		return nil, err
	}
	bz, err := res.Bytes()
	if err != nil {
		return nil, err
	}
	return bz, nil
}

func (r *RedisStorage) SetComponent(archetypeID ArchetypeID, componentIndex ComponentIndex, compBz []byte) error {
	ctx := context.Background()
	key := r.componentDataKey(archetypeID, r.ComponentStoragePrefix)
	res := r.Client.LSet(ctx, key, int64(componentIndex), compBz)
	return res.Err()
}

// MoveComponent moves the given component from the source archetype to the target archetype. SwapRemove
// is used to remove the component from the source archetype.
func (r *RedisStorage) MoveComponent(source ArchetypeID, index ComponentIndex, dst ArchetypeID) error {
	ctx := context.Background()
	dKey := r.componentDataKey(dst, r.ComponentStoragePrefix)
	data, err := r.SwapRemove(source, index)
	if err != nil {
		return err
	}
	if err := r.Client.RPush(ctx, dKey, data).Err(); err != nil {
		return err
	}
	return err
}

// SwapRemove removes the given componentIndex from the archetypeID, and swaps the last item
// in the archetypeID into the newly vacant position. The removed component data is returned.
// if the removed item happens to be the last item in the list, no swapping will take place.
func (r *RedisStorage) SwapRemove(archetypeID ArchetypeID, componentIndex ComponentIndex) ([]byte, error) {
	ctx := context.Background()
	key := r.componentDataKey(archetypeID, r.ComponentStoragePrefix)
	data, err := r.Client.RPop(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	count, err := r.Client.LLen(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	// The component marked for removal was at the end of the list, so there's no need to 'swap' it back in.
	if count == int64(componentIndex) {
		return data, nil
	}
	if err := r.Client.LSet(ctx, key, int64(componentIndex), data).Err(); err != nil {
		return nil, err
	}
	return data, nil
}

func (r *RedisStorage) Contains(archetypeID ArchetypeID, componentIndex ComponentIndex) (bool, error) {
	ctx := context.Background()
	key := r.componentDataKey(archetypeID, r.ComponentStoragePrefix)
	res := r.Client.LIndex(ctx, key, int64(componentIndex))
	if err := res.Err(); err != nil {
		return false, err
	}
	result, err := res.Result()
	if err != nil {
		return false, err
	}
	return len(result) > 0, nil
}

// ---------------------------------------------------------------------------
// 							ENTITY LOCATION STORAGE
// ---------------------------------------------------------------------------

var _ EntityLocationStorage = &RedisStorage{}

func (r *RedisStorage) ContainsEntity(id EntityID) (bool, error) {
	ctx := context.Background()
	key := r.entityLocationKey(id)
	res := r.Client.Get(ctx, key)
	if err := res.Err(); err != nil {
		return false, err
	}
	locBz, err := res.Bytes()
	if err != nil {
		return false, err
	}
	return locBz != nil, nil
}

func (r *RedisStorage) Remove(id EntityID) error {
	ctx := context.Background()
	key := r.entityLocationKey(id)
	res := r.Client.Del(ctx, key)
	return res.Err()
}

func (r *RedisStorage) Insert(id EntityID, archID ArchetypeID, componentIndex ComponentIndex) error {
	ctx := context.Background()
	key := r.entityLocationKey(id)
	loc := NewLocation(archID, componentIndex)
	bz, err := Encode(loc)
	if err != nil {
		return err
	}
	res := r.Client.Set(ctx, key, bz, 0)
	if err := res.Err(); err != nil {
		return err
	}
	key = r.entityLocationLenKey()
	incRes := r.Client.Incr(ctx, key)
	if err := incRes.Err(); err != nil {
		return err
	}
	return nil
}

func (r *RedisStorage) SetLocation(id EntityID, location Location) error {
	ctx := context.Background()
	key := r.entityLocationKey(id)
	bz, err := Encode(location)
	if err != nil {
		return err
	}
	res := r.Client.Set(ctx, key, bz, 0)
	if err := res.Err(); err != nil {
		return err
	}
	return nil
}

func (r *RedisStorage) GetLocation(id EntityID) (loc Location, err error) {
	ctx := context.Background()
	key := r.entityLocationKey(id)
	res := r.Client.Get(ctx, key)
	if err := res.Err(); err != nil {
		return loc, err
	}
	bz, err := res.Bytes()
	if err != nil {
		return loc, err
	}
	loc, err = Decode[Location](bz)
	if err != nil {
		return loc, err
	}
	return loc, nil
}

func (r *RedisStorage) ArchetypeID(id EntityID) (ArchetypeID, error) {
	loc, err := r.GetLocation(id)
	return loc.ArchID, err
}

func (r *RedisStorage) ComponentIndexForEntity(id EntityID) (ComponentIndex, error) {
	loc, err := r.GetLocation(id)
	return loc.CompIndex, err
}

func (r *RedisStorage) Len() (int, error) {
	ctx := context.Background()
	key := r.entityLocationLenKey()
	res := r.Client.Get(ctx, key)
	if err := res.Err(); err != nil {
		return 0, err
	}
	length, err := res.Int()
	if err != nil {
		return 0, err
	}
	return length, nil
}

// ---------------------------------------------------------------------------
// 							Entity Manager
// ---------------------------------------------------------------------------

var _ EntityManager = &RedisStorage{}

func (r *RedisStorage) Destroy(e EntityID) {
	// this is just a no-op, not really needed for redis
	// since we're a bit more space efficient here
}

func (r *RedisStorage) NewEntity() (EntityID, error) {
	ctx := context.Background()
	key := r.nextEntityIDKey()
	res := r.Client.Get(ctx, key)
	var nextID uint64
	if err := res.Err(); err != nil {
		if res.Err() == redis.Nil {
			nextID = 0
		} else {
			return 0, err
		}
	} else {
		nextID, err = res.Uint64()
		if err != nil {
			return 0, err
		}
	}

	ent := EntityID(nextID)
	incRes := r.Client.Incr(ctx, key)
	if err := incRes.Err(); err != nil {
		return 0, err
	}
	return ent, nil
}

const (
	// Reids values are limited to 512 mb (https://redis.io/docs/data-types/tutorial).
	// The size of the values in this arbitrary key/value store is limited. If we find
	// that we want to save more data than this limit, we'll have to distribute the
	// data across multiple keys OR save it in a different way
	maxRedisValueSize = 5 * 1024 * 1025
)

var ErrorBufferTooLargeForRedisValue = errors.New("buffer is too large for redis value")

func (r *RedisStorage) Save(key string, buf []byte) error {
	if len(buf) > maxRedisValueSize {
		return ErrorBufferTooLargeForRedisValue
	}

	ctx := context.Background()
	redisKey := r.stateStorageKey(key)
	cmd := r.Client.Set(ctx, redisKey, buf, 0)
	if err := cmd.Err(); err != nil {
		return err
	}
	return nil
}

func (r *RedisStorage) Load(key string) (data []byte, ok bool, err error) {
	ctx := context.Background()
	redisKey := r.stateStorageKey(key)
	cmd := r.Client.Get(ctx, redisKey)
	buf, err := cmd.Bytes()
	if err != nil {
		return nil, false, err
	}
	return buf, true, nil
}

// ---------------------------------------------------------------------------
// 							Tick Manager
// ---------------------------------------------------------------------------

// TickDetails contains a breakdown of what tick we're currently on. If all 3 numbers are equal, that is the current
// tick. If Start is one ahead of Transaction, it means the tick has started, but we haven't uploaded the list
// of transactions for this tick. If Transaction is one ahead of End, it means the transactions have been uploaded
// but at least one System failed to run to completion.
type tickDetails struct {
	Start uint64
	End   uint64
}

var _ TickStorage = &RedisStorage{}

func (r *RedisStorage) getTickDetails(ctx context.Context) (tickDetails, error) {
	key := r.tickKey()
	buf, err := r.Client.Get(ctx, key).Bytes()
	if err != nil && err == redis.Nil {
		zero := tickDetails{}
		return zero, r.setTickDetails(ctx, zero)
	} else if err != nil {
		return tickDetails{}, err
	}

	details, err := Decode[tickDetails](buf)
	if err != nil {
		return tickDetails{}, err
	}
	return details, nil
}

func (r *RedisStorage) setTickDetails(ctx context.Context, details tickDetails) error {
	buf, err := Encode(details)
	if err != nil {
		return err
	}
	key := r.tickKey()
	return r.Client.Set(ctx, key, buf, 0).Err()
}

func (r *RedisStorage) GetTickNumbers() (start, end uint64, err error) {
	ctx := context.Background()
	details, err := r.getTickDetails(ctx)
	if err != nil {
		return 0, 0, err
	}
	return details.Start, details.End, nil
}

type pendingTransaction struct {
	TypeID transaction.TypeID
	TxID   transaction.TxID
	Data   []byte
	Sig    *sign.SignedPayload
}

func (r *RedisStorage) storeTransactions(ctx context.Context, txs []transaction.ITransaction, queues transaction.TxMap) error {
	var pending []pendingTransaction
	for _, tx := range txs {
		currList := queues[tx.ID()]
		for _, txData := range currList {
			buf, err := tx.Encode(txData.Value)
			if err != nil {
				return err
			}
			currItem := pendingTransaction{
				TypeID: tx.ID(),
				TxID:   txData.ID,
				Sig:    txData.Sig,
				Data:   buf,
			}
			pending = append(pending, currItem)
		}
	}
	return r.storePendingTransactionsInRedis(ctx, pending)
}

func (r *RedisStorage) storePendingTransactionsInRedis(ctx context.Context, pending []pendingTransaction) error {
	buf, err := Encode(pending)
	if err != nil {
		return err
	}
	key := r.pendingTransactionsKey()
	return r.Client.Set(ctx, key, buf, 0).Err()
}

func (r *RedisStorage) getPendingTransactionsFromRedis(ctx context.Context) ([]pendingTransaction, error) {
	key := r.pendingTransactionsKey()
	buf, err := r.Client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	return Decode[[]pendingTransaction](buf)
}

func (r *RedisStorage) StartNextTick(txs []transaction.ITransaction, queues transaction.TxMap) error {
	ctx := context.Background()
	if err := r.storeTransactions(ctx, txs, queues); err != nil {
		return err
	}
	if err := r.makeSnapshot(ctx); err != nil {
		return err
	}
	details, err := r.getTickDetails(ctx)
	if err != nil {
		return err
	}
	details.Start++
	return r.setTickDetails(ctx, details)
}

func (r *RedisStorage) FinalizeTick() error {
	ctx := context.Background()
	details, err := r.getTickDetails(ctx)
	if err != nil {
		return err
	}
	details.End++
	return r.setTickDetails(ctx, details)
}

const snapshotPrefix = "SNAP:"

// partitionKeys splits all keys in the DB into a list that has the snapshotPrefix and a list that does not
// contain the snapshotPrefix. Nonce keys are excluded from both sets of keys.
func (r *RedisStorage) partitionKeys(ctx context.Context) (stateKeys, snapshotKeys []string, err error) {
	keys, err := r.Client.Keys(ctx, "*").Result()
	if err != nil {
		return nil, nil, err
	}
	for _, key := range keys {
		// Nonce values (used for signature verification) are updated outside of the game Tick. Since they are not managed
		// by a normal ECS System, exclude them from snapshots and snapshot recovery.
		// There's currently no mechanism to recover these nonce values when recovering from the DA layer.
		// TODO: https://linear.app/arguslabs/issue/CAR-97/recover-nonce-values-when-recovering-from-the-da-layer
		if key == r.nonceKey() {
			continue
		}
		if strings.HasPrefix(key, snapshotPrefix) {
			snapshotKeys = append(snapshotKeys, key)
		} else {
			stateKeys = append(stateKeys, key)
		}
	}
	return stateKeys, snapshotKeys, nil
}

// copyKey copies the contents of the src key to the dst key. Ideally, this could be replaced
// the redis command r.Client.Copy(...), however miniredis (used for unit testing) has a bug
// where this copy command for lists doesn't work as expected.
// See https://linear.app/arguslabs/issue/WORLD-210/update-miniredis-version for more details.
func (r *RedisStorage) copyKey(ctx context.Context, src, dst string) error {
	keyType, err := r.Client.Type(ctx, src).Result()
	if err != nil {
		return err
	}
	if keyType != "list" {
		return r.Client.Copy(ctx, src, dst, 0, true).Err()
	}
	if err := r.Client.Del(ctx, dst).Err(); err != nil {
		return err
	}
	vals, err := r.Client.LRange(ctx, src, 0, -1).Result()
	if err != nil {
		return err
	}
	ivals := make([]interface{}, len(vals))
	for i, v := range vals {
		ivals[i] = v
	}

	return r.Client.RPush(ctx, dst, ivals...).Err()
}

func (r *RedisStorage) makeSnapshot(ctx context.Context) error {
	toCopy, toDelete, err := r.partitionKeys(ctx)
	if err != nil {
		return err
	}
	if len(toDelete) > 0 {
		// Unlink is like Del, but the actual reclamation of memory happens in an asynchronous thread
		if err := r.Client.Unlink(ctx, toDelete...).Err(); err != nil {
			return err
		}
	}

	for _, sourceKey := range toCopy {
		destKey := snapshotPrefix + sourceKey
		if err := r.copyKey(ctx, sourceKey, destKey); err != nil {
			return err
		}
	}
	return nil
}

// recoverSnapshot copies all the keys in the redis DB that are prefixed with snapshotPrefix to a new key with the
// prefix removed. The keys that are prefixed with snapshotPrefix represent the last good state of the DB.
func (r *RedisStorage) recoverSnapshot(ctx context.Context) error {
	stateKeys, snapshotKeys, err := r.partitionKeys(ctx)
	if err != nil {
		return err
	}
	if len(stateKeys) > 0 {
		// We are recovering from a snapshot, so the existing state keys are obsolete
		if _, err := r.Client.Unlink(ctx, stateKeys...).Result(); err != nil {
			return err
		}
	}
	for _, sourceKey := range snapshotKeys {
		destKey := strings.TrimPrefix(sourceKey, snapshotPrefix)
		if err := r.copyKey(ctx, sourceKey, destKey); err != nil {
			return err
		}
	}
	return nil
}

// Recover recovers the game state from the last game tick and any pending transactions that have been saved to the DB,
// but not yet applied to a game tick.
func (r *RedisStorage) Recover(txs []transaction.ITransaction) (transaction.TxMap, error) {
	ctx := context.Background()
	if err := r.recoverSnapshot(ctx); err != nil {
		return nil, err
	}
	pending, err := r.getPendingTransactionsFromRedis(ctx)
	if err != nil {
		return nil, err
	}
	idToTx := map[transaction.TypeID]transaction.ITransaction{}
	for _, tx := range txs {
		idToTx[tx.ID()] = tx
	}

	allQueues := transaction.TxMap{}
	for _, p := range pending {
		tx := idToTx[p.TypeID]
		txData, err := tx.Decode(p.Data)
		if err != nil {
			return nil, err
		}
		allQueues[tx.ID()] = append(allQueues[tx.ID()], transaction.TxAny{
			ID:    p.TxID,
			Sig:   p.Sig,
			Value: txData,
		})
	}
	return allQueues, nil
}

// ---------------------------------------------------------------------------
//							Nonce Storage
// ---------------------------------------------------------------------------

var _ NonceStorage = &RedisStorage{}

// GetNonce returns the saved nonce for the given signer address. While signer address will generally be a
// go-ethereum/common.Address, no verification happens at the redis storage level. Any string can be used for the
// signerAddress.
func (r *RedisStorage) GetNonce(signerAddress string) (uint64, error) {
	ctx := context.Background()
	n, err := r.Client.HGet(ctx, r.nonceKey(), signerAddress).Uint64()
	if err == redis.Nil {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	return n, nil

}

// SetNonce saves the given nonce value with the given signer address. Any string can be used for the signer address,
// and no nonce verification takes place.
func (r *RedisStorage) SetNonce(signerAddress string, nonce uint64) error {
	ctx := context.Background()
	return r.Client.HSet(ctx, r.nonceKey(), signerAddress, nonce).Err()
}
