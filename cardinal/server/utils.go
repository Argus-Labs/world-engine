package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/sign"
	"github.com/rs/zerolog/log"
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

func (t *Handler) verifySignature(request *http.Request, getSignedAddressFromWorld bool) (payload []byte, sig *sign.SignedPayload, err error) {
	buf, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, nil, errors.New("unable to read body")
	}

	sp, err := sign.UnmarshalSignedPayload(buf)
	if err != nil {
		return nil, nil, err
	}

	if sp.PersonaTag == "" {
		return nil, nil, errors.New("PersonaTag must not be empty")
	}

	// Handle the case where signature is disabled
	if t.disableSigVerification {
		return sp.Body, sp, nil
	}
	///////////////////////////////////////////////

	// Check that the namespace is correct
	if sp.Namespace != t.w.GetNamespace() {
		return nil, nil, fmt.Errorf("%w: got namespace %q but it must be %q", ErrorInvalidSignature, sp.Namespace, t.w.GetNamespace())
	}

	var signerAddress string
	if getSignedAddressFromWorld {
		// Use 0 as the tick. We don't care about any pending CreatePersonaTxs, we just want to know the
		// current signer address for the given persona. Any error will fail this request.
		signerAddress, err = t.w.GetSignerForPersonaTag(sp.PersonaTag, 0)
	} else {
		signerAddress, err = getSignerAddressFromPayload(*sp)
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

	if len(sp.Body) == 0 {
		return buf, sp, nil
	}
	return sp.Body, sp, nil
}
