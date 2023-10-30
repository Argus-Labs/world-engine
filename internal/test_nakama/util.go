package test_nakama

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

const (
	envNakamaAddress = "NAKAMA_ADDRESS"
)

type nakamaClient struct {
	addr               string
	authHeader         string
	notificationCursor string
}

func newClient(t *testing.T) *nakamaClient {
	host := os.Getenv(envNakamaAddress)
	if host == "" {
		host = "http://127.0.0.1:7350"
	}
	h := &nakamaClient{
		addr: host,
	}
	return h
}

type NotificationItem struct {
	ID         string    `json:"id"`
	Subject    string    `json:"subject"`
	Content    string    `json:"content"`
	Code       int       `json:"code"`
	CreateTime time.Time `json:"createTime"`
	Persistent bool      `json:"persistent"`
}

type Content struct {
	Message string `json:"message"`
}

type NotificationCollection struct {
	Notifications   []NotificationItem `json:"notifications"`
	CacheableCursor string             `json:"cacheableCursor"`
}

func (c *nakamaClient) listKNotifications(k int) ([]*Content, error) {
	path := "v2/notification"
	options := fmt.Sprintf("limit=%d&cursor=%s", k, c.notificationCursor)
	url := fmt.Sprintf("%s/%s?%s", c.addr, path, options)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.authHeader)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	bodyData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	data := NotificationCollection{
		Notifications:   make([]NotificationItem, 0),
		CacheableCursor: "",
	}
	err = json.Unmarshal(bodyData, &data)
	if err != nil {
		return nil, err
	}
	c.notificationCursor = data.CacheableCursor
	acc := make([]*Content, 0)
	for _, item := range data.Notifications {
		content := Content{}
		err := json.Unmarshal([]byte(item.Content), &content)
		if err != nil {
			return nil, err
		}
		if item.Subject == "event" {
			acc = append(acc, &content)
		}
	}
	return acc, nil
}

func (c *nakamaClient) registerDevice(username, deviceID string) error {
	path := "v2/account/authenticate/device"
	options := fmt.Sprintf("create=true&username=%s", username)
	url := fmt.Sprintf("%s/%s?%s", c.addr, path, options)
	body := map[string]any{
		"id": deviceID,
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	reader := bytes.NewReader(buf)
	req, err := http.NewRequest("POST", url, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	// defaultkey is the default server key. See https://heroiclabs.com/docs/nakama/concepts/authentication/ for more
	// details.
	req.SetBasicAuth("defaultkey", "")

	resp, err := http.DefaultClient.Do(req)
	//	resp, err := http.Post(url, "application/json", reader)
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
	url := fmt.Sprintf("%s/v2/rpc/%s?unwrap", c.addr, path)
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
	msg := fmt.Sprintf("response body is:\n%v\nReadAll error is:%v", string(buf), err)
	r.Body = io.NopCloser(bytes.NewReader(buf))
	return msg
}
