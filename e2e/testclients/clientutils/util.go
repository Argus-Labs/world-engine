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

	"nhooyr.io/websocket"
	"pkg.world.dev/world-engine/assert"
)

const (
	envNakamaAddress = "NAKAMA_ADDRESS"
)

type NakamaClient struct {
	t                  *testing.T
	addr               string
	authHeader         string
	notificationCursor string
	ReceiptCh          chan Receipt
	EventCh            chan Event
}

func NewNakamaClient(t *testing.T) *NakamaClient {
	host := os.Getenv(envNakamaAddress)
	if host == "" {
		host = "http://127.0.0.1:7350"
	}
	h := &NakamaClient{
		t:         t,
		addr:      host,
		ReceiptCh: make(chan Receipt, 100),
		EventCh:   make(chan Event, 100),
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

type WSNotificationCollection struct {
	Notifications struct {
		Notifications []NotificationItem `json:"notifications"`
	} `json:"notifications"`
}

type NotificationCollection struct {
	Notifications   []NotificationItem `json:"notifications"`
	CacheableCursor string             `json:"cacheableCursor"`
}

func (c *NakamaClient) listenForNotifications() error {
	url := fmt.Sprintf("%s/ws", c.addr)
	opts := websocket.DialOptions{
		HTTPHeader: http.Header{},
	}
	opts.HTTPHeader.Set("Authorization", c.authHeader)
	conn, _, err := websocket.Dial(context.Background(), url, &opts)
	if err != nil {
		return err
	}

	c.t.Cleanup(func() {
		assert.Check(c.t, nil == conn.Close(websocket.StatusNormalClosure, "test over"))
	})

	go func() {
		for {
			_, buf, err := conn.Read(context.Background())
			if err != nil {
				if !strings.Contains(err.Error(), "StatusNormalClosure") {
					assert.Check(c.t, err == nil, "failed to read from we socket:", err)
				}
				return
			}
			var data WSNotificationCollection
			err = json.Unmarshal(buf, &data)
			assert.Check(c.t, err == nil, "failed to unmarshal notification")

			for _, n := range data.Notifications.Notifications {
				switch n.Subject {
				case "receipt":
					c.handleReceipt([]byte(n.Content))
				case "event":
					c.handleEvent([]byte(n.Content))
				default:
					assert.Check(c.t, false, "unknown notification subject: ", n.Subject)
				}
			}
		}
	}()
	return nil
}

func (c *NakamaClient) handleReceipt(bz []byte) {
	var receipt Receipt
	if err := json.Unmarshal(bz, &receipt); err != nil {
		assert.Check(c.t, false, "failed to unmarshal receipt", err)
	}
	select {
	case c.ReceiptCh <- receipt:
	default:
		c.t.Log("warning: receipt dropped")
	}
}

func (c *NakamaClient) handleEvent(bz []byte) {
	var event Event
	if err := json.Unmarshal(bz, &event); err != nil {
		assert.Check(c.t, false, "failed to unmarshal event", err)
	}
	select {
	case c.EventCh <- event:
	default:
		c.t.Log("warning: event dropped")
	}
}

// FetchNotifications fetches notifications and returns them as a generic slice.
// This is a helper function to avoid code duplication.
func (c *NakamaClient) FetchNotifications(k int) ([]NotificationItem, error) {
	path := "v2/notification"
	options := fmt.Sprintf("limit=%d&cursor=%s", k, c.notificationCursor)
	url := fmt.Sprintf("%s/%s?%s", c.addr, path, options)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.authHeader)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Handle non-OK responses here. For simplicity, we're just returning an error.
		return nil, fmt.Errorf("server returned non-OK status: %d", resp.StatusCode)
	}

	bodyData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data NotificationCollection
	err = json.Unmarshal(bodyData, &data)
	if err != nil {
		return nil, err
	}

	// Update the cursor for subsequent requests.
	c.notificationCursor = data.CacheableCursor

	return data.Notifications, nil
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
	req.Header.Set("Content-Type", "application/json")
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
	if err := c.listenForNotifications(); err != nil {
		return fmt.Errorf("failed to start streaming notifications: %w", err)
	}

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
	req.Header.Set("Content-Type", "application/json")
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
