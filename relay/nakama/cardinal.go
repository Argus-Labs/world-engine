package main

// cardinal.go wraps the http requests to some cardinal endpoints.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/relay/nakama/events"
	"pkg.world.dev/world-engine/relay/nakama/persona"
	"pkg.world.dev/world-engine/relay/nakama/utils"
)

type endpoints struct {
	TxEndpoints    []string `json:"txEndpoints"`
	QueryEndpoints []string `json:"queryEndpoints"`
}

func getCardinalEndpoints(cardinalAddress string) (txEndpoints []string, queryEndpoints []string, err error) {
	var resp *http.Response
	url := utils.MakeHTTPURL(ListEndpoints, cardinalAddress)
	//nolint:gosec,noctx // its ok. maybe revisit.
	resp, err = http.Get(url)
	if err != nil {
		return txEndpoints, queryEndpoints, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		buf, _ := io.ReadAll(resp.Body)
		err = eris.Errorf("list endpoints (at %q) failed with status code %s: %v",
			url, resp.Status, string(buf))
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

func makeRequestAndReadResp(
	ctx context.Context,
	notifier *events.Notifier,
	endpoint string,
	payload io.Reader,
	cardinalAddress string,
) (res string, err error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		utils.MakeHTTPURL(endpoint, cardinalAddress),
		payload,
	)
	if err != nil {
		return res, eris.Wrapf(err, "request setup failed for endpoint %q", endpoint)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return res, eris.Wrapf(err, "request failed for endpoint %q", endpoint)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return res, eris.Wrapf(err, "failed to read response body, bad status: %s: %s", resp.Status, body)
		}
		return res, eris.Errorf("bad status code: %s: %s", resp.Status, body)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return res, eris.Wrapf(err, "failed to read response body, bad status: %s: %s", resp.Status, body)
	}
	if strings.HasPrefix(endpoint, TransactionEndpointPrefix) {
		var asTx persona.TxResponse

		if err = json.Unmarshal(body, &asTx); err != nil {
			return res, eris.Wrap(err, "failed to decode body")
		}
		userID, err := utils.GetUserID(ctx)
		if err != nil {
			return res, eris.Wrap(err, "unable to get user id")
		}
		notifier.AddTxHashToPendingNotifications(asTx.TxHash, userID)
	}
	return string(body), nil
}
