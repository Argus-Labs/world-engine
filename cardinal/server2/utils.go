package server

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/sign"
)

var (
	ErrSystemTransactionRequired  = errors.New("system transaction required")
	ErrSystemTransactionForbidden = errors.New("system transaction forbidden")
	ErrNonceVerificationFailed    = errors.New("nonce verification failed")
)

func decode[T any](buf []byte) (T, error) {
	dec := json.NewDecoder(bytes.NewBuffer(buf))
	dec.DisallowUnknownFields()
	var val T
	if err := dec.Decode(&val); err != nil {
		return val, eris.Wrap(err, "error decoding")
	}
	return val, nil
}

func getSignerAddressFromPayload(sp sign.Transaction) (string, error) {
	msg, err := decode[ecs.CreatePersona](sp.Body)
	if err != nil {
		return "", err
	}
	return msg.SignerAddress, nil
}

func (handler *Handler) verifySignature(sp *sign.Transaction, isSystemTransaction bool,
) (err error) {
	// TODO: Check why we do this, this is from before via @jer
	if handler.disableSigVerification {
		populatePlaceholderFields(sp)
	}

	if sp.PersonaTag == "" {
		return errors.New("PersonaTag must not be empty")
	}

	// Handle the case where signature is disabled
	if handler.disableSigVerification {
		return nil
	}

	// Check that the namespace is correct
	if sp.Namespace != handler.w.Namespace().String() {
		return eris.Wrapf(ErrInvalidSignature, "got namespace %q but it must be %q",
			sp.Namespace, handler.w.Namespace().String())
	}
	if isSystemTransaction && !sp.IsSystemTransaction() {
		return eris.Wrap(ErrSystemTransactionRequired, "")
	} else if !isSystemTransaction && sp.IsSystemTransaction() {
		return eris.Wrap(ErrSystemTransactionForbidden, "")
	}

	var signerAddress string
	if sp.IsSystemTransaction() {
		// For system transactions, just use the signed address that is include in the signature.
		signerAddress, err = getSignerAddressFromPayload(*sp)
	} else {
		// For non-system transaction, get the signer address from storage. If this PersonaTag doesn't exist,
		// an error will be returned and the signature verification will fail.
		signerAddress, err = handler.w.GetSignerForPersonaTag(sp.PersonaTag, 0)
	}
	if err != nil {
		return err
	}

	// Verify signature
	if err = sp.Verify(signerAddress); err != nil {
		return eris.Wrap(errors.Join(ErrInvalidSignature, err), "")
	}

	// The signature is valid. Verify and use the nonce in an atomic operation
	if err = handler.w.UseNonce(signerAddress, sp.Nonce); err != nil {
		return eris.Wrap(err, ErrNonceVerificationFailed.Error())
	}

	return nil
}

func populatePlaceholderFields(request *sign.Transaction) {
	if request.Namespace == "" {
		request.Namespace = "placeholder-namespace"
	}

	if request.Signature == "" {
		request.Signature = "placeholder-signature"
	}
}

//func (handler *Handler) verifySignatureOfMapRequest(request map[string]interface{}, isSystemTransaction bool,
//) (payload []byte, sig *sign.Transaction, err error) {
//	if handler.disableSigVerification {
//		populatePlaceholderFields(request)
//	}
//	sp, err := sign.MappedTransaction(request)
//	if err != nil {
//		return nil, nil, eris.Wrap(err, ErrInvalidSignature.Error())
//	}
//	sig, err = handler.verifySignature(sp, isSystemTransaction)
//	if err != nil {
//		return nil, nil, eris.Wrapf(err, ErrInvalidSignature.Error())
//	}
//	if len(sp.Body) == 0 {
//		buf, err := json.Marshal(request)
//		if err != nil {
//			return nil, nil, eris.Wrap(err, "error marshalling json")
//		}
//		return buf, sp, nil
//	}
//
//	return sig.Body, sig, nil
//}
