package redis

import (
	"context"
	"errors"
	"strconv"
	"sync"

	"github.com/redis/go-redis/v9"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog/log"
)

const (
	// NonceSlidingWindowSize is the maximum distance a new nonce can be from the max nonce before it is rejected
	// outright.
	NonceSlidingWindowSize = 1000

	// numOfNoncesToTriggerCleanup is the number of nonces in redis required for a cleanup pass to be initiated.
	// A cleanup consists of removing all nonces that are beyond the NonceSlidingWindowSize from the maximum seen nonce.
	// Each cleanup operation costs O(log(N)+M) where N is the number of items in the set and M is the number of items
	// to be removed. If this number is close to NonceSlidingWindowSize, we will spend more time removing old nonces
	// from redis, but the total number of nonces saved will be smaller. The larger this number gets, the less time we
	// will spend removing old nonces, but the total number of nonces saved will be larger.
	numOfNoncesToTriggerCleanup = NonceSlidingWindowSize * 1.5

	// maxValidNonce is the largest nonce that is guaranteed to have a unique ZSet score from all smaller nonces.
	// A ZSet in redis is used to track unique nonces. Each item in a ZSet has a score, which is stored as a float64.
	// Due to the precision loss when converting large integers to floating point numbers, at some point 2 distinct
	// nonces will map to the same score in the Redis ZSet.
	maxValidNonce       = (1 << (float64MantissaSize + 1)) - 1
	float64MantissaSize = 52
)

var ErrNonceHasAlreadyBeenUsed = errors.New("nonce has already been used")

type NonceStorage struct {
	Client *redis.Client
	// mutex locks the UseNonce function to make it safe for concurrent access. This is a single lock for all signer
	// addresses. An improvement on NonceStorage would have a different lock for each signer addresses.
	mutex *sync.Mutex
	// maxNonce tracks the highest nonce seen for a particular signer address
	maxNonce map[string]uint64
	// countNonce tracks the number of nonces stored in redis for each signer address. This count will increase as
	// nonces are used and decrease as out-of-window nonces are removed from redis.
	countNonce map[string]int
}

func NewNonceStorage(client *redis.Client) NonceStorage {
	return NonceStorage{
		Client:     client,
		mutex:      &sync.Mutex{},
		maxNonce:   map[string]uint64{},
		countNonce: map[string]int{},
	}
}

// UseNonce atomically marks the given nonce as used. The nonce is valid if nil is returned. A non-nil error means
// there was an error verifying the nonce, or the nonce was already used.
func (r *NonceStorage) UseNonce(signerAddress string, nonce uint64) error {
	if nonce > maxValidNonce {
		return eris.New("nonce is too large")
	}
	ctx := context.Background()
	signerAddressKey := r.nonceSetKey(signerAddress)

	// All redis and in-memory map operations happen inside a lock. This could be improved by creating a separate lock
	// for each signer address.
	r.mutex.Lock()
	defer r.mutex.Unlock()

	maxNonce, err := r.getMaxNonceForKey(ctx, signerAddressKey)
	if err != nil {
		return eris.Wrap(err, "failed to get max nonce for signer address")
	}

	// Nonces beyond the sliding window are invalid and can be rejected outright.
	if nonce < maxNonce && maxNonce-nonce >= NonceSlidingWindowSize {
		return eris.New("nonce is too old")
	}

	zItem := redis.Z{
		Score:  float64(nonce),
		Member: nonce,
	}

	added, err := r.Client.ZAdd(ctx, signerAddressKey, zItem).Result()
	if err != nil {
		return eris.Wrap(err, "failed to add nonce")
	}
	// A result of 0 from ZAdd means no new items were actually added to the Zset. This means the nonce was already
	// used.
	if added == 0 {
		return eris.Wrapf(ErrNonceHasAlreadyBeenUsed, "signer %q has already used nonce %d", signerAddress, nonce)
	}

	r.maxNonce[signerAddressKey] = max(r.maxNonce[signerAddressKey], nonce)
	r.countNonce[signerAddressKey]++

	if r.countNonce[signerAddressKey] > numOfNoncesToTriggerCleanup {
		r.cleanupOldNonces(ctx, signerAddressKey, r.maxNonce[signerAddressKey])
	}

	return nil
}

// cleanupOldNonces removes the record of all nonces that are older than NonceSlidingWindowSize. Nonces in that range
// can be rejected without checking storage. ZRemRangeByScore has a performance of O(log(N)+M) where N is the number
// of items in the set and M is the number of items to remove.
func (r *NonceStorage) cleanupOldNonces(ctx context.Context, signerAddressKey string, currMax uint64) {
	minScore := "-inf"
	maxScore := strconv.FormatUint(currMax-NonceSlidingWindowSize, 10)
	removed, err := r.Client.ZRemRangeByScore(ctx, signerAddressKey, minScore, maxScore).Result()
	if err != nil {
		log.Err(err).Msg("failed to remove old nonces")
		return
	}
	r.countNonce[signerAddressKey] -= int(removed)
}

// getMaxNonceForKey returns the highest used nonce for the given key.
func (r *NonceStorage) getMaxNonceForKey(ctx context.Context, signerAddressKey string) (uint64, error) {
	maxNonce, ok := r.maxNonce[signerAddressKey]
	if ok {
		return maxNonce, nil
	}
	// There isn't a max nonce in memory. Fetch it from redis.
	values, err := r.Client.ZRange(ctx, signerAddressKey, -1, 0).Result()
	if err != nil {
		return 0, eris.Wrap(err, "failed to get range of nonce values")
	}
	if len(values) == 0 {
		// No nonces have been used for this key
		maxNonce = 0
	} else {
		// At least 1 value was returned.
		maxNonce, err = strconv.ParseUint(values[0], 10, 64)
		if err != nil {
			return 0, eris.Wrapf(err, "failed to convert %q to uint64", values[0])
		}
	}
	r.maxNonce[signerAddressKey] = maxNonce
	return maxNonce, nil
}
