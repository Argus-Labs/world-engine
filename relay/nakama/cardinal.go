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

// world is the response from the cardinal world endpoint.
type world struct {
	Namespace  string        `json:"namespace"`
	Components []fieldDetail `json:"components"` // list of component names
	Messages   []fieldDetail `json:"messages"`
	Queries    []fieldDetail `json:"queries"`
}

// fieldDetail is the response from the cardinal world endpoint.
type fieldDetail struct {
	Name   string         `json:"name"`   // name of the message or query
	Fields map[string]any `json:"fields"` // variable name and type
	URL    string         `json:"url,omitempty"`
}

func getCardinalEndpoints(cardinalAddress string) (txEndpoints []string, queryEndpoints []string, err error) {
	var resp *http.Response
	url := utils.MakeHTTPURL(WorldEndpoint, cardinalAddress)
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
	var w world
	if err = dec.Decode(&w); err != nil {
		return txEndpoints, queryEndpoints, eris.Wrap(err, "")
	}
	txEndpoints = make([]string, 0)
	for _, msg := range w.Messages {
		txEndpoints = append(txEndpoints, msg.URL)
	}
	queryEndpoints = make([]string, 0)
	for _, qry := range w.Queries {
		queryEndpoints = append(queryEndpoints, qry.URL)
	}
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
