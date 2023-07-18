package testsuite

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"

	"gotest.tools/v3/assert"
)

const (
	envNakamaAddress = "NAKAMA_ADDRESS"
)

type nakamaClient struct {
	addr       string
	authHeader string
}

func newClient() *nakamaClient {
	host := os.Getenv("NAKAMA_ADDRESS")
	if host == "" {
		host = "localhost:7350"
	}

	h := &nakamaClient{
		addr: host,
	}
	return h
}

func (c *nakamaClient) registerDevice(username, deviceID string) error {
	addr := fmt.Sprintf("http://defaultkey:@%s", c.addr)
	path := "v2/account/authenticate/device"
	options := fmt.Sprintf("create=true&username=%s", username)
	url := fmt.Sprintf("%s/%s?%s", addr, path, options)
	body := map[string]any{
		"id": deviceID,
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	reader := bytes.NewReader(buf)

	resp, err := http.Post(url, "application/json", reader)
	if err != nil {
		return err
	}
	if 200 != resp.StatusCode {
		buf, err := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed with status code %d. body is:\n%v\nerror:%w", resp.StatusCode, string(buf), err)
	}

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("failed to decode body: %w", err)
	}
	c.authHeader = fmt.Sprintf("Bearer %s", body["token"].(string))
	return nil
}

func (c *nakamaClient) rpc(path string, body any) (*http.Response, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("http://%s/v2/rpc/%s?unwrap", c.addr, path)
	req, err := http.NewRequest("POST", url, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.authHeader)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	return resp, nil
}

func copyBody(r *http.Response) string {
	buf, err := io.ReadAll(r.Body)
	msg := fmt.Sprintf("response body is:\n%v\nReadAll error:%v", string(buf), err)
	r.Body = io.NopCloser(bytes.NewReader(buf))
	return msg
}

func bodyToMap(t *testing.T, r *http.Response) map[string]any {
	m := map[string]any{}
	err := json.NewDecoder(r.Body).Decode(&m)
	assert.NilError(t, err)
	return m
}
