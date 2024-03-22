package siwe

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"slices"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
	"github.com/spruceid/siwe-go"

	"pkg.world.dev/world-engine/relay/nakama/utils"
)

const (
	// TODO: This static domain and URI should be configurable via an environment variable.
	DefaultDomain = "example.com"
	DefaultURI    = "https://example.com/v2/account/authenticate/custom"

	// DefaultTimeout is the length of time between the issue time and the expiration time. If the current time
	// is not between the issue time and expiration time of a message, the signature will be rejected.
	DefaultTimeout   = 5 * time.Minute
	DefaultStatement = "Log in to Nakama using SIWE."

	siweMessageCollection = "siwe_message_collection"
)

var (
	ErrMissingSignerAddress = errors.New("missing signer address")
	ErrMissingMessage       = errors.New("message field is required")
	ErrMissingSignature     = errors.New("signature field is required")

	maxNonce = big.NewInt(0).SetUint64(math.MaxUint64)
)

// GenerateResult will be serialized to JSON and sent back to a client. The client must sign the message in the
// SIWEMessage field, and resubmit their authentication request.
type GenerateResult struct {
	SIWEMessage string `json:"siwe_message"`
}

// nonceStorageObj is the struct that is serizlied to JSON and saved to the Nakama storage layer. Each signer address
// maps to a different nonceStorageObj. A signature is valid if the nonce contained in the signed message does NOT
// already have an entry in this object.
type nonceStorageObj struct {
	Nonces  []*nonceWindow `json:"nonces"`
	Version string         `json:"-"`
}

// nonceWindow represents a single nonce that was used as well as the window of time that it was valid for.
type nonceWindow struct {
	Nonce     string    `json:"nonce"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiredAt time.Time `json:"expired_at"`
}

// ValidateSignature ensures the given message is valid and the given signature is actually a signature for the message.
func ValidateSignature(ctx context.Context, nk runtime.NakamaModule, signerAddress, message, signature string) error {
	msg, err := siwe.ParseMessage(message)
	if err != nil {
		return eris.Wrap(err, "failed to parse message")
	}

	if msg.GetDomain() != DefaultDomain {
		return eris.New("domain is incorrect")
	}
	if uri := msg.GetURI(); uri.String() != DefaultURI {
		return eris.New("uri is incorrect")
	}

	nonce, err := getNonceWindow(msg)
	if err != nil {
		return eris.Wrap(err, "nonce does not have a valid time window")
	}

	// Make sure the signature is valid.
	pubKey, err := msg.VerifyEIP191(signature)
	if err != nil {
		return eris.Wrap(err, "failed to verify signature")
	}

	// Make sure the signer of the message and the signer in the message match
	actualSignerAddress := crypto.PubkeyToAddress(*pubKey).Hex()
	if actualSignerAddress != signerAddress {
		return eris.Wrap(err, "signer address in message and signature are different ")
	}

	// Make sure the nonce is valid and hasn't yet been used.
	nonceUsed, err := useNonce(ctx, nk, actualSignerAddress, nonce)
	if err != nil {
		return eris.Wrap(err, "failed to use nonce")
	} else if !nonceUsed {
		return eris.New("nonce has already been used")
	}
	return nil
}

func getNonceWindow(msg *siwe.Message) (*nonceWindow, error) {
	// Make sure the expiration time is in the future
	expString := msg.GetExpirationTime()
	if expString == nil {
		return nil, eris.New("missing expiration time")
	}
	expTime, err := time.Parse(time.RFC3339, *expString)
	if err != nil {
		return nil, eris.New("unable to parse expiration time")
	}
	if time.Now().After(expTime) {
		return nil, eris.New("message has expired")
	}

	// Make sure the issued time is in the past
	issuedAt, err := time.Parse(time.RFC3339, msg.GetIssuedAt())
	if err != nil {
		return nil, eris.New("failed to parse issued at time")
	}
	if time.Now().Before(issuedAt) {
		return nil, eris.New("message must be issued in the past")
	}

	delta := expTime.Sub(issuedAt)
	if delta > DefaultTimeout {
		return nil, eris.New("delta between expiration time and issued at time is too large")
	}
	return &nonceWindow{
		Nonce:     msg.GetNonce(),
		ExpiredAt: expTime,
		IssuedAt:  issuedAt,
	}, nil
}

// useNonce attempts to use the given nonce for the given signer address. If an identical nonce has been used, and
// that nonce has not yet expired, useNonce will return false. A return value of (true, nil) means the nonce was
// successfully used.
func useNonce(
	ctx context.Context,
	nk runtime.NakamaModule,
	signerAddress string,
	toUse *nonceWindow,
) (
	ok bool, err error,
) {
	obj, err := getNonceStorageObject(ctx, nk, signerAddress)
	if err != nil {
		return false, eris.Wrap(err, "failed to get nonces from storage")
	}

	// obj.Nonces contains the set of nonces that have already been used. If the nonce we're considering is NOT
	// in obj.Nonces, then it's a valid nonce. Add it to the set of nonces and save it back to stoage.
	foundNonce := false
	for _, n := range obj.Nonces {
		if n.Nonce == toUse.Nonce {
			foundNonce = true
			break
		}
	}
	if foundNonce {
		return false, nil
	}
	// This nonce is valid. Add it to the set of nonces we've previously seen.
	obj.Nonces = append(obj.Nonces, toUse)

	// Clean up any expired nonces.
	now := time.Now()
	obj.Nonces = slices.DeleteFunc(obj.Nonces, func(details *nonceWindow) bool {
		return now.After(details.ExpiredAt)
	})

	if err = setNoncesStorageObject(ctx, nk, signerAddress, obj); err != nil {
		return false, eris.New("failed to write new nonces back to DB")
	}
	return true, nil
}

// GenerateNewSIWEMessage generates an SIWE Message that can be signed and used to authenticate a user.
func GenerateNewSIWEMessage(signerAddress string) (*GenerateResult, error) {
	options := makeOptions()
	nonce, err := makeNonce()
	if err != nil {
		return nil, err
	}
	msg, err := siwe.InitMessage(DefaultDomain, signerAddress, DefaultURI, nonce, options)
	if err != nil {
		return nil, eris.Wrap(err, "failed to init siwe message")
	}

	return &GenerateResult{
		SIWEMessage: msg.String(),
	}, nil
}

// setNonceStorageObject saves the given nonce storage object to Nakama's storage layer. If the version in the storage
// object is out of date, an error will be returned.
func setNoncesStorageObject(
	ctx context.Context,
	nk runtime.NakamaModule,
	signerAddress string,
	obj *nonceStorageObj,
) error {
	bz, err := json.Marshal(obj)
	if err != nil {
		return eris.Wrap(err, "failed to marshal nonces")
	}

	writes := []*runtime.StorageWrite{
		{
			Collection: siweMessageCollection,
			Key:        signerAddress,
			UserID:     utils.AdminAccountID,
			Value:      string(bz),
			Version:    obj.Version,
		},
	}
	if _, err = nk.StorageWrite(ctx, writes); err != nil {
		return eris.Wrap(err, "failed to save nonces to storage")
	}
	return nil
}

// getNonceStorageObject loads the nonces associated with the given signer address.
func getNonceStorageObject(
	ctx context.Context,
	nk runtime.NakamaModule,
	signerAddress string,
) (
	*nonceStorageObj, error,
) {
	read := []*runtime.StorageRead{
		{
			Collection: siweMessageCollection,
			Key:        signerAddress,
			UserID:     utils.AdminAccountID,
		},
	}
	objs, err := nk.StorageRead(ctx, read)
	if err != nil {
		return nil, eris.Wrap(err, "failed to read valid nonces from DB")
	} else if len(objs) == 0 {
		// No existing storage object was found, so return a storage object with no nonces and no version
		return &nonceStorageObj{
			// When this is later saved back to the DB, it will only be successful it the stoage object
			// doesn't already exist in the DB.
			Version: "*",
		}, nil
	}

	var nonceObj nonceStorageObj
	if err := json.Unmarshal([]byte(objs[0].GetValue()), &nonceObj); err != nil {
		return nil, eris.Wrap(err, "failed to unmarshal nonces")
	}
	nonceObj.Version = objs[0].GetVersion()
	return &nonceObj, nil
}

func makeOptions() map[string]any {
	now := time.Now().UTC()
	return map[string]any{
		"issuedAt":       now.Format(time.RFC3339),
		"expirationTime": now.Add(DefaultTimeout).Format(time.RFC3339),
		"statement":      DefaultStatement,
	}
}

func makeNonce() (string, error) {
	n, err := rand.Int(rand.Reader, maxNonce)
	if err != nil {
		return "", eris.Wrap(err, "failed to generate nonce")
	}
	return n.String(), nil
}
