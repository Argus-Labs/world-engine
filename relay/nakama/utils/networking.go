package utils

import (
	"fmt"
	"io"
	"net/http"

	"github.com/rotisserie/eris"
)

func DoRequest(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, eris.Wrapf(err, "request to %q failed", req.URL)
	} else if resp.StatusCode != http.StatusOK {
		statusCode := resp.StatusCode
		var buf []byte
		buf, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, eris.Wrapf(err, "failed reading body in resp, status code: %d", statusCode)
		}
		reqBuf, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, eris.Wrapf(err, "failed reading body in request, status code: %d", statusCode)
		}
		return nil,
			eris.Errorf(
				"error to url: %s, with request body: %s, got response of %d: %s",
				req.URL,
				string(reqBuf),
				statusCode,
				string(buf))
	}
	return resp, nil
}

func MakeHTTPURL(resource string, url string) string {
	return fmt.Sprintf("http://%s/%s", url, resource)
}

func MakeWebSocketURL(resource string, url string) string {
	return fmt.Sprintf("ws://%s/%s", url, resource)
}
