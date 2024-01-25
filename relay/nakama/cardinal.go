package main

// cardinal.go wraps the http requests to some cardinal endpoints.

import (
	"encoding/json"
	"io"
	"net/http"
	"pkg.world.dev/world-engine/relay/nakama/utils"

	"github.com/rotisserie/eris"
)

type endpoints struct {
	TxEndpoints    []string `json:"txEndpoints"`
	QueryEndpoints []string `json:"queryEndpoints"`
}

func getCardinalEndpoints() (txEndpoints []string, queryEndpoints []string, err error) {
	var resp *http.Response
	url := utils.MakeHTTPURL(ListEndpoints, globalCardinalAddress)
	//nolint:gosec,noctx // its ok. maybe revisit.
	resp, err = http.Get(url)
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
