package main

// cardinal.go wraps the http requests to some cardinal endpoints.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	nakamaerrors "pkg.world.dev/world-engine/relay/nakama/errors"
	"pkg.world.dev/world-engine/relay/nakama/utils"

	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/sign"
)

var (
	listEndpoints               = "query/http/endpoints"
	createPersonaEndpoint       = "tx/persona/create-persona"
	readPersonaSignerEndpoint   = "query/persona/signer"
	transactionReceiptsEndpoint = "query/receipts/list"
	eventEndpoint               = "events"

	readPersonaSignerStatusUnknown   = "unknown"
	readPersonaSignerStatusAvailable = "available"
)

type txResponse struct {
	TxHash string `json:"txHash"`
	Tick   uint64 `json:"tick"`
}

type endpoints struct {
	TxEndpoints    []string `json:"txEndpoints"`
	QueryEndpoints []string `json:"queryEndpoints"`
}

func getCardinalEndpoints() (txEndpoints []string, queryEndpoints []string, err error) {
	var resp *http.Response
	url := utils.MakeHTTPURL(listEndpoints, globalCardinalAddress)
	//nolint:gosec,noctx // its ok. maybe revisit.
	resp, err = http.Post(url, "", nil)
	if err != nil {
		return txEndpoints, queryEndpoints, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		buf, _ := io.ReadAll(resp.Body)
		err = eris.Errorf("list endpoints (at %q) failed with status code %d: %v",
			url, resp.StatusCode, string(buf))
		return txEndpoints, queryEndpoints, err
	}
	dec := json.NewDecoder(resp.Body)
	var ep endpoints
	if err = dec.Decode(&ep); err != nil {
		return txEndpoints, queryEndpoints, eris.Wrap(err, "")
	}
	txEndpoints = ep.TxEndpoints
	queryEndpoints = ep.QueryEndpoints
	return txEndpoints, queryEndpoints, err
}

func cardinalCreatePersona(ctx context.Context, nk runtime.NakamaModule, personaTag string) (
	txHash string,
	tick uint64,
	err error,
) {
	defer func() {
		if r := recover(); r != nil {
			txHash = ""
			tick = 0
			err = eris.Errorf("a panic occurred in nakama in the function, cardinalCreatePersona:, %s", r)
		}
	}()

	signerAddress := getSignerAddress()
	createPersonaTx := struct {
		PersonaTag    string `json:"personaTag"`
		SignerAddress string `json:"signerAddress"`
	}{
		PersonaTag:    personaTag,
		SignerAddress: signerAddress,
	}

	key, nonce, err := getPrivateKeyAndANonce(ctx, nk)
	if err != nil {
		return "", 0, eris.Wrapf(err, "unable to get the private key or a nonce")
	}

	transaction, err := sign.NewSystemTransaction(key, globalNamespace, nonce, createPersonaTx)

	if err != nil {
		return "", 0, eris.Wrapf(err, "unable to create signed payload")
	}

	buf, err := transaction.Marshal()
	if err != nil {
		return "", 0, eris.Wrapf(err, "unable to marshal signed payload")
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		utils.MakeHTTPURL(createPersonaEndpoint, globalCardinalAddress),
		bytes.NewReader(buf),
	)
	if err != nil {
		return "", 0, eris.Wrapf(err, "unable to make request to %q", createPersonaEndpoint)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := utils.DoRequest(req)
	if err != nil {
		return "", 0, err
	}

	defer resp.Body.Close()

	if code := resp.StatusCode; code != http.StatusOK {
		buf, err = io.ReadAll(resp.Body)
		return "", 0, eris.Wrapf(err, "create persona response is not 200. code %v, body: %v", code, string(buf))
	}

	var createPersonaResponse txResponse

	if err = json.NewDecoder(resp.Body).Decode(&createPersonaResponse); err != nil {
		return "", 0, eris.Wrap(err, "unable to decode response")
	}
	if createPersonaResponse.TxHash == "" {
		return "", 0, eris.Errorf("tx response does not have a tx hash")
	}
	return createPersonaResponse.TxHash, createPersonaResponse.Tick, nil
}

func cardinalQueryPersonaSigner(ctx context.Context, personaTag string, tick uint64) (signerAddress string, err error) {
	readPersonaRequest := struct {
		PersonaTag string `json:"personaTag"`
		Tick       uint64 `json:"tick"`
	}{
		PersonaTag: personaTag,
		Tick:       tick,
	}

	buf, err := json.Marshal(readPersonaRequest)
	if err != nil {
		return "", eris.Wrap(err, "")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, utils.MakeHTTPURL(readPersonaSignerEndpoint, globalCardinalAddress),
		bytes.NewReader(buf))
	if err != nil {
		return "", eris.Wrap(err, "")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpResp, err := utils.DoRequest(httpReq)
	if err != nil {
		return "", err
	}
	defer httpResp.Body.Close()

	var resp struct {
		Status        string `json:"status"`
		SignerAddress string `json:"signerAddress"`
	}
	if err = json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return "", eris.Wrap(err, "")
	}
	if resp.Status == readPersonaSignerStatusUnknown {
		return "", eris.Wrap(nakamaerrors.ErrPersonaSignerUnknown, "")
	} else if resp.Status == readPersonaSignerStatusAvailable {
		return "", eris.Wrap(nakamaerrors.ErrPersonaSignerAvailable, "")
	}
	return resp.SignerAddress, nil
}
