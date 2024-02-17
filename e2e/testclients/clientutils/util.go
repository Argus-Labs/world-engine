package clientutils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	envNakamaAddress = "NAKAMA_ADDRESS"
)

type NakamaClient struct {
	addr               string
	authHeader         string
	notificationCursor string
}

func NewNakamaClient(_ *testing.T) *NakamaClient {
	host := os.Getenv(envNakamaAddress)
	if host == "" {
		host = "http://127.0.0.1:7350"
	}
	h := &NakamaClient{
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

type Event struct {
	Message string `json:"message"`
}

type Receipt struct {
	TxHash string         `json:"txHash"`
	Result map[string]any `json:"result"`
	Errors []string       `json:"errors"`
}

type NotificationCollection struct {
	Notifications   []NotificationItem `json:"notifications"`
	CacheableCursor string             `json:"cacheableCursor"`
}

func (c *NakamaClient) ListNotifications(k int) ([]*Receipt, []*Event, error) {
	path := "v2/notification"
	options := fmt.Sprintf("limit=%d&cursor=%s", k, c.notificationCursor)
	url := fmt.Sprintf("%s/%s?%s", c.addr, path, options)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", c.authHeader)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	bodyData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	data := NotificationCollection{
		Notifications:   make([]NotificationItem, 0),
		CacheableCursor: "",
	}
	err = json.Unmarshal(bodyData, &data)
	if err != nil {
		return nil, nil, err
	}
	c.notificationCursor = data.CacheableCursor
	receipts := make([]*Receipt, 0)
	events := make([]*Event, 0)
	for _, item := range data.Notifications {
		if item.Subject == "receipt" {
			receipt := Receipt{}
			err := json.Unmarshal([]byte(item.Content), &receipt)
			if err != nil {
				return nil, nil, err
			}
			receipts = append(receipts, &receipt)
		}
		if item.Subject == "event" {
			event := Event{}
			err := json.Unmarshal([]byte(item.Content), &event)
			if err != nil {
				return nil, nil, err
			}
			events = append(events, &event)
		}
	}
	return receipts, events, nil
}

func (c *NakamaClient) RegisterDevice(username, deviceID string) error {
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
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Event-Type", "application/json")
	// defaultkey is the default server key. See https://heroiclabs.com/docs/nakama/concepts/authentication/ for more
	// details.
	req.SetBasicAuth("defaultkey", "")

	resp, err := http.DefaultClient.Do(req)
	//	resp, err := http.Post(url, "application/json", reader)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		buf, err := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed with status code %d. body is:\n%v\nerror:%w", resp.StatusCode, string(buf), err)
	}

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("failed to decode body: %w", err)
	}
	c.authHeader = fmt.Sprintf("Bearer %s", body["token"].(string))
	return nil
}

func (c *NakamaClient) RPC(path string, body any) (*http.Response, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/v2/rpc/%s?unwrap", c.addr, path)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Event-Type", "application/json")
	req.Header.Set("Authorization", c.authHeader)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	return resp, nil
}

func CopyBody(r *http.Response) string {
	buf, err := io.ReadAll(r.Body)
	msg := fmt.Sprintf("response body is:\n%v\nReadAll error is:%v", string(buf), err)
	r.Body = io.NopCloser(bytes.NewReader(buf))
	return msg
}

const chars = "abcdefghijklmnopqrstuvwxyz"

func Triple(s string) (string, string, string) {
	return s, s, s
}

func RandomString() string {
	b := &strings.Builder{}
	for i := 0; i < 10; i++ {
		n := rand.Intn(len(chars)) //nolint:gosec // it's fine just a test.
		b.WriteString(chars[n : n+1])
	}
	return b.String()
}
