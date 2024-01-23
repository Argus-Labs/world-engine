package persona

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
	"io"
	"net/http"
	"pkg.world.dev/world-engine/relay/nakama/constants"
	nakamaerrors "pkg.world.dev/world-engine/relay/nakama/errors"
	"pkg.world.dev/world-engine/relay/nakama/pk"
	"pkg.world.dev/world-engine/relay/nakama/utils"
	"pkg.world.dev/world-engine/sign"
)

var (
	createPersonaEndpoint            = "tx/persona/create-persona"
	readPersonaSignerEndpoint        = "query/persona/signer"
	readPersonaSignerStatusUnknown   = "unknown"
	readPersonaSignerStatusAvailable = "available"
)

type TxResponse struct {
	TxHash string `json:"txHash"`
	Tick   uint64 `json:"tick"`
}

func CardinalCreatePersona(ctx context.Context, nk runtime.NakamaModule, personaTag string) (
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

	signerAddress := pk.GetSignerAddress()
	createPersonaTx := struct {
		PersonaTag    string `json:"personaTag"`
		SignerAddress string `json:"signerAddress"`
	}{
		PersonaTag:    personaTag,
		SignerAddress: signerAddress,
	}

	key, nonce, err := pk.GetPrivateKeyAndANonce(ctx, nk)
	if err != nil {
		return "", 0, eris.Wrapf(err, "unable to get the private key or a nonce")
	}

	transaction, err := sign.NewSystemTransaction(key, constants.GlobalNamespace, nonce, createPersonaTx)

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
		utils.MakeHTTPURL(createPersonaEndpoint, constants.GlobalCardinalAddress),
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

	var createPersonaResponse TxResponse

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
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		utils.MakeHTTPURL(readPersonaSignerEndpoint, constants.GlobalCardinalAddress),
		bytes.NewReader(buf),
	)
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
