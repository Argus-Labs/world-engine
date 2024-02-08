package signer

import (
	"context"
	"encoding/json"
	"testing"

	kms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	"github.com/ethereum/go-ethereum/common"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/sign"
)

const (
	// These "precomputed" constants are values that were computed with actual network calls to Google's KMS service.
	// They are saved here so that unit tests can run without hitting the KMS service.

	// precomputedPublicKeyPEM was generated via google's KMS API with the algorithm:
	// Elliptic Curve secp256k1 - SHA256 Digest
	precomputedPublicKeyPEM = `-----BEGIN PUBLIC KEY-----
MFYwEAYHKoZIzj0CAQYFK4EEAAoDQgAESWUEDAsP/3WQRa5fxjdLlQM4mYeYAvhz
esyjsoEDFTFKevyeDa6u83cNzv0lXeeTza8GSafyemA+4LtnYXorQw==
-----END PUBLIC KEY-----`
	// precomputedSignerAddress is the signer address from the public key decoded from the above PEM.
	precomputedSignerAddress = "0xDE0699273dae85C20f430C5BeFfC429239948499"

	// precomputedSignatureHex is a hex encoded signature for this transaction:
	// sign.Transaction{
	// 	PersonaTag: "some-persona-tag",
	// 	Namespace: "some-namespace",
	//  Nonce: 99,
	//  Body: `{"A":1,"B":2,"C":3}`
	// }
	precomputedSignatureHex = `` +
		`3045022100dfafbc7fea20b2485eaed90009` +
		`9205af4ca238420104f951cfe1388b544de5` +
		`af02206647c72359772e678b56a976812af7` +
		`e075831f630064611ea76c8a6bb2768a76`

	precomputedPersonaTag = "some-persona-tag"
	precomputedNamespace  = "some-namespace"
	precomputedNonce      = 99
	precomputedBody       = `{"A":1,"B":2,"C":3}`
)

func newPrecomputedTx() *sign.Transaction {
	return &sign.Transaction{
		PersonaTag: precomputedPersonaTag,
		Namespace:  precomputedNamespace,
		Nonce:      precomputedNonce,
		Body:       json.RawMessage(precomputedBody),
	}
}

func TestCanConvertPEMKeyToSignerAddress(t *testing.T) {
	as := &fakeAsymmetricSigner{
		pemToReturn: precomputedPublicKeyPEM,
	}

	ks, err := NewKMSSigner(context.Background(), &fakeNonceManager{}, as, "some_key_name")
	assert.NilError(t, err)
	assert.Equal(t, ks.SignerAddress(), precomputedSignerAddress)
}

func TestCanSignTxWithPrecomputedSignature(t *testing.T) {
	ctx := context.Background()
	kmsClient := newFakeSigner()
	txSigner, err := NewKMSSigner(ctx, &fakeNonceManager{precomputedNonce}, kmsClient, "some_key_path")
	assert.NilError(t, err)
	data := struct{ A, B, C int }{1, 2, 3}

	tx, err := txSigner.SignTx(ctx, precomputedPersonaTag, precomputedNamespace, data)
	assert.NilError(t, err)

	wantTx := newPrecomputedTx()
	assert.Equal(t, tx.PersonaTag, wantTx.PersonaTag)
	assert.Equal(t, tx.Namespace, wantTx.Namespace)
	assert.Equal(t, tx.Nonce, wantTx.Nonce)
	assert.Equal(t, string(tx.Body), string(wantTx.Body))

	// Also make sure the resulting signature can be verified by the sign package.
	assert.NilError(t, tx.Verify(precomputedSignerAddress))
}

// TestQueryRealKMSService is a test that will query Google's actual KMS service to sign a transaction. It's left here
// as a reference in case we ever want to generate a new set of signatures/publickeys for tests.
// To actually run this test you must replace the keyPath with a valid KMS key path that uses:
//
//	Elliptic Curve secp256k1 - SHA256 Digest
//
// In addition, the credsFile must be replaced with a valid google credentials file that has access to the above key.
func TestQueryRealKMSService(t *testing.T) {
	t.Skip("Do not query the actual KMS service in unit tests")

	ctx := context.Background()
	const credsFile = "/path/to/some/gcp/credentials/file.json"
	const keyPath = "projects/<project>/locations/global/keyRings/<keyRing>/cryptoKeys/<name>/cryptoKeyVersions/<num>"
	client, err := kms.NewKeyManagementClient(ctx, option.WithCredentialsFile(credsFile))
	assert.NilError(t, err)
	txSigner, err := NewKMSSigner(ctx, &fakeNonceManager{uint64(99)}, client, keyPath)
	assert.NilError(t, err)
	personaTag := "some-persona-tag"
	namespace := "some-namespace"
	data := struct{ A, B, C int }{1, 2, 3}

	tx, err := txSigner.SignTx(ctx, personaTag, namespace, data)
	assert.NilError(t, err)

	assert.NilError(t, tx.Verify(txSigner.SignerAddress()))
}

type fakeNonceManager struct {
	nonce uint64
}

var _ NonceManager = &fakeNonceManager{}

func (f *fakeNonceManager) SetNonce(_ context.Context, nonce uint64) error {
	f.nonce = nonce
	return nil
}

func (f *fakeNonceManager) IncNonce(context.Context) (nonce uint64, err error) {
	curr := f.nonce
	f.nonce++
	return curr, nil
}

type fakeAsymmetricSigner struct {
	sigToReturn string
	pemToReturn string
}

var _ AsymmetricSigner = fakeAsymmetricSigner{}

func newFakeSigner() AsymmetricSigner {
	return fakeAsymmetricSigner{
		sigToReturn: precomputedSignatureHex,
		pemToReturn: precomputedPublicKeyPEM,
	}
}

func (f fakeAsymmetricSigner) AsymmetricSign(_ context.Context, req *kmspb.AsymmetricSignRequest, _ ...gax.CallOption) (
	*kmspb.AsymmetricSignResponse, error,
) {
	signature := common.Hex2Bytes(precomputedSignatureHex)
	return &kmspb.AsymmetricSignResponse{
		Signature:            signature,
		SignatureCrc32C:      wrapperspb.Int64(int64(crc32c(signature))),
		VerifiedDigestCrc32C: true,
		Name:                 req.Name,
	}, nil
}

func (f fakeAsymmetricSigner) GetPublicKey(_ context.Context, _ *kmspb.GetPublicKeyRequest, _ ...gax.CallOption) (
	*kmspb.PublicKey, error,
) {
	return &kmspb.PublicKey{
		Pem: f.pemToReturn,
	}, nil
}
