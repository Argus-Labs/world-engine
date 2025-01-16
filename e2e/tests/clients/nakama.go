package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rotisserie/eris"
	"nhooyr.io/websocket"

	"pkg.world.dev/world-engine/assert"
)

const (
	envNakamaAddress = "NAKAMA_ADDRESS"
	chBufferSize     = 100
	chars            = "abcdefghijklmnopqrstuvwxyz"
)

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
	Notifications struct {
		Notifications []NotificationItem `json:"notifications"`
	} `json:"notifications"`
}

type NakamaClient struct {
	t          *testing.T
	addr       string
	authHeader string
	ReceiptCh  chan Receipt
	EventCh    chan Event
}

func NewNakamaClient(t *testing.T) *NakamaClient {
	host := os.Getenv(envNakamaAddress)
	if host == "" {
		host = "http://127.0.0.1:7350"
	}
	h := &NakamaClient{
		t:    t,
		addr: host,
		// Receipts and events will be placed on these channels. When the channel is filled, new receipts and events
		// will be dropped.
		ReceiptCh: make(chan Receipt, chBufferSize),
		EventCh:   make(chan Event, chBufferSize),
	}
	return h
}

func (c *NakamaClient) listenForNotifications() error {
	url := fmt.Sprintf("%s/ws", c.addr)
	opts := websocket.DialOptions{
		HTTPHeader: http.Header{},
	}
	opts.HTTPHeader.Set("Authorization", c.authHeader)
	//nolint:bodyclose // Docs say "You never need to close resp.Body yourself"
	conn, _, err := websocket.Dial(context.Background(), url, &opts)
	if err != nil {
		return err
	}

	isTestOver := &atomic.Bool{}
	c.t.Cleanup(func() {
		isTestOver.Store(true)
		assert.Check(c.t, nil == conn.Close(websocket.StatusNormalClosure, "test over"))
	})

	go func() {
		for {
			_, buf, err := conn.Read(context.Background())
			if err != nil {
				if !isTestOver.Load() {
					assert.Check(c.t, err == nil, "failed to read from we socket:", err)
				}
				return
			}
			var data NotificationCollection
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
func (c *NakamaClient) AuthenticateSIWE(username, signerAddress string, signFn func(msg string) string) error {
	ctx := context.Background()
	body := struct {
		ID   string `json:"id"`
		Vars struct {
			Type      string `json:"type"`
			Signature string `json:"signature"`
			Message   string `json:"message"`
		} `json:"vars"`
	}{}
	body.Vars.Type = "siwe"
	body.ID = signerAddress

	// The first authentication post is expected to fail. The failure message will contain an SIWE Message that
	// be signed and sent back in a second request.
	path := "v2/account/authenticate/custom"
	resp, err := c.doAuthPost(ctx, username, path, body)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		return errors.New("expected an initial unauthorized to get the siwe message")
	}
	nakamaErrorResponse := struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}{}
	if err = json.NewDecoder(resp.Body).Decode(&nakamaErrorResponse); err != nil {
		return err
	}

	siwe := struct {
		SIWEMessage string `json:"siwe_message"`
	}{}

	if err = json.Unmarshal([]byte(nakamaErrorResponse.Message), &siwe); err != nil {
		return err
	}

	// Now sign the message and re-send the authenciate/custom request
	body.Vars.Signature = signFn(siwe.SIWEMessage)
	body.Vars.Message = siwe.SIWEMessage

	resp, err = c.doAuthPost(ctx, username, path, body)
	if err != nil {
		return err
	}
	if err = c.validateSuccessfulAuth(resp); err != nil {
		return err
	}
	return nil
}

func (c *NakamaClient) RegisterDevice(username, deviceID string) error {
	path := "v2/account/authenticate/device"
	body := map[string]any{
		"id": deviceID,
	}

	resp, err := c.doAuthPost(context.Background(), username, path, body)
	if err != nil {
		return err
	}

	if err = c.validateSuccessfulAuth(resp); err != nil {
		return err
	}

	return nil
}

func (c *NakamaClient) validateSuccessfulAuth(resp *http.Response) error {
	body := map[string]any{}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		buf, err := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed with status code %d. body is:\n%v\nerror:%w", resp.StatusCode, string(buf),
			err)
	}

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("failed to decode body: %w", err)
	}

	token, ok := body["token"]
	if !ok {
		return eris.New("unable to find token")
	}

	tokenStr, ok := token.(string)
	if !ok {
		return eris.New("token is not a string")
	}

	c.authHeader = fmt.Sprintf("Bearer %s", tokenStr)
	if err := c.listenForNotifications(); err != nil {
		return fmt.Errorf("failed to start streaming notifications: %w", err)
	}
	return nil
}

func (c *NakamaClient) doAuthPost(ctx context.Context, username string, path string, body any) (*http.Response, error) {
	options := fmt.Sprintf("create=true&username=%s", username)
	url := fmt.Sprintf("%s/%s?%s", c.addr, path, options)
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	reader := bytes.NewReader(buf)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	// defaultkey is the default server key. See https://heroiclabs.com/docs/nakama/concepts/authentication/ for more
	// details.
	req.SetBasicAuth("defaultkey", "")
	return http.DefaultClient.Do(req)
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
