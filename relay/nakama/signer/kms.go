package signer

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"hash/crc32"
	"math/big"

	"cloud.google.com/go/kms/apiv1/kmspb"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/googleapis/gax-go/v2"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"pkg.world.dev/world-engine/sign"
)

type kmsSigner struct {
	aSigner       AsymmetricSigner
	nonceManager  NonceManager
	keyName       string
	signerAddress string
}

// AsymmetricSigner is implemented by the kms.KeyManagementClient and it mainly exists so that testing can be easily
// done via a fake version of the kms service.
type AsymmetricSigner interface {
	AsymmetricSign(context.Context, *kmspb.AsymmetricSignRequest, ...gax.CallOption) (
		*kmspb.AsymmetricSignResponse, error)
	GetPublicKey(context.Context, *kmspb.GetPublicKeyRequest, ...gax.CallOption) (*kmspb.PublicKey, error)
}

var _ Signer = &kmsSigner{}

func NewKMSSigner(ctx context.Context, nonceManager NonceManager, asymmetricSigner AsymmetricSigner, keyName string) (
	Signer, error,
) {
	ks := &kmsSigner{aSigner: asymmetricSigner,
		nonceManager: nonceManager,
		keyName:      keyName,
	}
	if err := ks.populateSignerAddress(ctx); err != nil {
		return nil, eris.Wrap(err, "failed to populate signer address")
	}
	return ks, nil
}

// SignTx creates a sign.Transaction object with a signature. This doc page was used as a reference:
// https://cloud.google.com/kms/docs/create-validate-signatures#validate_ec_signature
func (k *kmsSigner) SignTx(ctx context.Context, personaTag string, namespace string, data any) (
	*sign.Transaction, error,
) {
	nonce, err := k.nonceManager.IncNonce(ctx)
	if err != nil {
		return nil, eris.Wrap(err, "failed to get new nonce")
	}
	bz, err := json.Marshal(data)
	if err != nil {
		return nil, eris.Wrap(err, "failed to marshal tx data")
	}

	unsignedTx := &sign.Transaction{
		PersonaTag: personaTag,
		Namespace:  namespace,
		Nonce:      nonce,
		Body:       bz,
	}
	digest := calculateDigest(unsignedTx)

	// Set up the KMS request to sign the transaction
	req := &kmspb.AsymmetricSignRequest{
		Name: k.keyName,
		Digest: &kmspb.Digest{
			Digest: &kmspb.Digest_Sha256{
				Sha256: digest.Bytes(),
			},
		},
		DigestCrc32C: wrapperspb.Int64(int64(crc32c(digest[:]))),
	}

	result, err := k.aSigner.AsymmetricSign(ctx, req)
	if err != nil {
		return nil, eris.Wrap(err, "failed to sign tx via KMS")
	}

	if !result.VerifiedDigestCrc32C ||
		result.Name != req.Name ||
		int64(crc32c(result.Signature)) != result.SignatureCrc32C.Value {
		return nil, fmt.Errorf("AsymmetricSign: request corrupted in-transit")
	}

	//	unsignedTx.Signature = string(result.Signature)
	ethSig, err := k.kmsSigToEthereumSig(digest, result.Signature)
	if err != nil {
		return nil, eris.Wrap(err, "")
	}
	unsignedTx.Signature = common.Bytes2Hex(ethSig)
	return unsignedTx, nil
}

func crc32c(data []byte) uint32 {
	t := crc32.MakeTable(crc32.Castagnoli)
	return crc32.Checksum(data, t)
}

// TODO: The "populateHash" func from the sign package should be exposed.
func calculateDigest(tx *sign.Transaction) common.Hash {
	// TODO: Check for any empty fields here.
	return crypto.Keccak256Hash(
		[]byte(tx.PersonaTag),
		[]byte(tx.Namespace),
		[]byte(fmt.Sprintf("%d", tx.Nonce)),
		tx.Body,
	)
}

func (k *kmsSigner) SignSystemTx(ctx context.Context, namespace string, data any) (*sign.Transaction, error) {
	return k.SignTx(ctx, sign.SystemPersonaTag, namespace, data)
}

func (k *kmsSigner) SignerAddress() string {
	return k.signerAddress
}

func (k *kmsSigner) populateSignerAddress(ctx context.Context) error {
	resp, err := k.aSigner.GetPublicKey(ctx, &kmspb.GetPublicKeyRequest{
		Name: k.keyName,
	})
	if err != nil {
		return eris.Wrap(err, "failed to get public key")
	}
	signerAddress, err := convertPemToSignerAddress(resp.Pem)
	if err != nil {
		return eris.Wrap(err, "failed to parse signer address")
	}
	k.signerAddress = signerAddress
	return nil
}

// The documentation at https://cloud.google.com/kms/docs/retrieve-public-key#kms-get-public-key-go recommends using
// x509.ParsePKIXPublicKey to convert the raw bytes from Google's API to an ecdsa.PublicKey. Unfortunately, it doesn't
// seem like ParsePKIXPublicKey supports the secp256k1 (asn1 1.3.132.0.10) curve.
// See https://cs.opensource.google/go/go/+/refs/tags/go1.21.6:src/crypto/x509/x509.go;l=504-525 for the curves that
// are supported.
//
// I've adapted the public key parsing code of https://pkg.go.dev/github.com/openware/pkg/signer to convert the KMS
// bytes to an ecdsa.PublicKey
type publicKeyInfo struct {
	Raw       asn1.RawContent
	Algorithm pkix.AlgorithmIdentifier
	PublicKey asn1.BitString
}

var oidPublicKeyECDSA = asn1.ObjectIdentifier{1, 2, 840, 10045, 2, 1}

func convertPemToSignerAddress(pemStr string) (string, error) {
	block, rest := pem.Decode([]byte(pemStr))
	if len(rest) > 0 {
		return "", eris.New("too many pem blocks when parsing public key")
	}

	// Google's KMS documentation at https://cloud.google.com/kms/docs/retrieve-public-key#kms-get-public-key-go
	// recommends this pattern for parsing public keys:
	//
	//	publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	//	if err != nil {
	//		return "", eris.Wrap(err, "failed to parse public key")
	//	}
	//	pubKey, ok := publicKey.(*ecdsa.PublicKey)
	//	if !ok {
	// 		return "", eris.Wrap(err, "public key is not elliptic curve")
	//	}
	//
	// But the x509 package does not seem to support the 1.2.840.10045.2.1 OID standard for elliptic curves.

	var pubKeyInfo publicKeyInfo
	rest, err := asn1.Unmarshal(block.Bytes, &pubKeyInfo)
	if err != nil {
		return "", eris.Wrap(err, "failed to unmarshal public key info")
	}
	if len(rest) > 0 {
		return "", eris.New("encountered unmarshalled bytes when parsing public key info")
	}
	if !pubKeyInfo.Algorithm.Algorithm.Equal(oidPublicKeyECDSA) {
		return "", eris.New("incorrect curve for public key")
	}
	pubKey, err := crypto.UnmarshalPubkey(pubKeyInfo.PublicKey.Bytes)
	if err != nil {
		return "", eris.Wrap(err, "failed to unmarshal public key")
	}
	signerAddress := crypto.PubkeyToAddress(*pubKey).Hex()
	return signerAddress, nil
}

func byteLenOfBigInt(n *big.Int) int {
	const bitsInByte = 8
	if n == nil {
		return 0
	}
	return (n.BitLen() + (bitsInByte - 1)) / bitsInByte
}

func (k *kmsSigner) kmsSigToEthereumSig(digest common.Hash, sig []byte) ([]byte, error) {
	var parsedSig struct{ R, S *big.Int }
	_, err := asn1.Unmarshal(sig, &parsedSig)
	if err != nil {
		return nil, eris.New("failed to unmarshal signature")
	}
	rLen := byteLenOfBigInt(parsedSig.R)
	sLen := byteLenOfBigInt(parsedSig.S)
	if rLen == 0 || rLen > 32 || sLen == 0 || sLen > 32 {
		return nil, eris.New("R and S of google's KMS signature must be between (0,32] bytes long")
	}

	var ethSig [65]byte
	parsedSig.R.FillBytes(ethSig[32-rLen : 32])
	parsedSig.S.FillBytes(ethSig[64-sLen : 64])

	for recoveryID := byte(0); recoveryID < 2; recoveryID++ {
		ethSig[64] = recoveryID
		var gotPubKey *ecdsa.PublicKey
		gotPubKey, err := crypto.SigToPub(digest.Bytes(), ethSig[:])
		if err != nil {
			continue
		}
		gotSignerAddress := crypto.PubkeyToAddress(*gotPubKey)
		if gotSignerAddress.Hex() == k.signerAddress {
			return ethSig[:], nil
		}
	}
	return nil, eris.New("failed to find recovery id for KMS signature")
}
