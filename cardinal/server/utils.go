package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/sign"
)

var (
	ErrorSystemTransactionRequired  = errors.New("system transaction required")
	ErrorSystemTransactionForbidden = errors.New("system transaction forbidden")
)

// fixes a path to contain a leading slash.
// if the path already contains a leading slash, it is simply returned as is.
func conformPath(p string) string {
	if p[0] != '/' {
		p = "/" + p
	}
	return p
}

func writeUnauthorized(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = fmt.Fprintf(w, "unauthorized: %v", err)
	log.Info().Msgf("unauthorized: %v", err)
}

func writeError(w http.ResponseWriter, msg string, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = fmt.Fprintf(w, "%s: %v", msg, err)
	log.Info().Msgf("%s: %v", msg, err)
}

func writeBadRequest(w http.ResponseWriter, msg string, err error) {
	w.WriteHeader(http.StatusBadRequest)
	_, _ = fmt.Fprintf(w, "%s: %v", msg, err)
	log.Info().Msgf("%s: %v", msg, err)
}

// writeResult takes in a json body string and writes it to the response writer.
func writeResult(w http.ResponseWriter, body json.RawMessage) {
	// Allow cors header
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Json content header
	w.Header().Set("Content-Type", "application/json")

	err := json.NewEncoder(w).Encode(body)
	if err != nil {
		writeError(w, "unable to encode body", err)
	}
}

func decode[T any](buf []byte) (T, error) {
	dec := json.NewDecoder(bytes.NewBuffer(buf))
	dec.DisallowUnknownFields()
	var val T
	if err := dec.Decode(&val); err != nil {
		return val, err
	}
	return val, nil
}

func makeWriteHandler(payload interface{}) (http.HandlerFunc, error) {
	res, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal payload: %w", err)
	}
	return func(writer http.ResponseWriter, request *http.Request) {
		writeResult(writer, res)
	}, nil
}

func getSignerAddressFromPayload(sp sign.SignedPayload) (string, error) {
	createPersonaTx, err := decode[ecs.CreatePersonaTransaction](sp.Body)
	if err != nil {
		return "", err
	}
	return createPersonaTx.SignerAddress, nil
}

func (t *Handler) verifySignatureOfSignedPayload(sp *sign.SignedPayload, isSystemTransaction bool) (payload []byte, sig *sign.SignedPayload, err error) {
	if sp.PersonaTag == "" {
		return nil, nil, errors.New("PersonaTag must not be empty")
	}

	// Handle the case where signature is disabled
	if t.disableSigVerification {
		return sp.Body, sp, nil
	}
	///////////////////////////////////////////////

	// Check that the namespace is correct
	if sp.Namespace != t.w.Namespace() {
		return nil, nil, fmt.Errorf("%w: got namespace %q but it must be %q", ErrorInvalidSignature, sp.Namespace, t.w.Namespace())
	}
	if isSystemTransaction && !sp.IsSystemPayload() {
		return nil, nil, ErrorSystemTransactionRequired
	} else if !isSystemTransaction && sp.IsSystemPayload() {
		return nil, nil, ErrorSystemTransactionForbidden
	}

	var signerAddress string
	if sp.IsSystemPayload() {
		// For system transactions, just use the signed address that is include in the signature.
		signerAddress, err = getSignerAddressFromPayload(*sp)
	} else {
		// For non-system transaction, get the signer address from storage. If this PersonaTag doesn't exist,
		// an error will be returned and the signature verification will fail.
		signerAddress, err = t.w.GetSignerForPersonaTag(sp.PersonaTag, 0)
	}
	if err != nil {
		return nil, nil, err
	}

	// Check the nonce
	nonce, err := t.w.GetNonce(signerAddress)
	if err != nil {
		return nil, nil, err
	}
	if sp.Nonce <= nonce {
		return nil, nil, fmt.Errorf("%w: got nonce %d, but must be greater than %d",
			ErrorInvalidSignature, sp.Nonce, nonce)
	}

	// Verify signature
	if err := sp.Verify(signerAddress); err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrorInvalidSignature, err)
	}
	// Update nonce
	if err := t.w.SetNonce(signerAddress, sp.Nonce); err != nil {
		return nil, nil, err
	}
	return sp.Body, sp, nil
}

func (t *Handler) verifySignatureOfHttpRequest(request *http.Request, isSystemTransaction bool) (payload []byte, sig *sign.SignedPayload, err error) {
	buf, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, nil, errors.New("unable to read body")
	}
	sp, err := sign.UnmarshalSignedPayload(buf)
	if err != nil {
		return nil, nil, err
	}
	payload, sig, err = t.verifySignatureOfSignedPayload(sp, isSystemTransaction)
	if len(sp.Body) == 0 {
		return buf, sp, nil
	} else {
		return payload, sig, err
	}
}

func (t *Handler) verifySignatureOfMapRequest(request map[string]interface{}, isSystemTransaction bool) (payload []byte, sig *sign.SignedPayload, err error) {
	sp, err := sign.MappedSignedPayload(request)
	if err != nil {
		return nil, nil, err
	}
	payload, sig, err = t.verifySignatureOfSignedPayload(sp, isSystemTransaction)
	if len(sp.Body) == 0 {
		buf, err := json.Marshal(request)
		if err != nil {
			return nil, nil, err
		}
		return buf, sp, nil
	} else {
		return payload, sig, err
	}
}
