package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"connectrpc.com/connect"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
	"github.com/rotisserie/eris"
	"github.com/shamaton/msgpack/v3"
	"golang.org/x/net/http2"
)

var (
	serverURL   = envOr("CARDINAL_CLIENT_URL", "http://localhost:5000")
	email       = envOr("CARDINAL_CLIENT_EMAIL", "alice@example.com")
	personaID   = envOr("CARDINAL_CLIENT_PERSONA", "dev-persona")
	bearerToken = os.Getenv("CARDINAL_CLIENT_BEARER_TOKEN")

	commandAddress = &microv1.ServiceAddress{
		Region:       envOr("CARDINAL_REGION", "us-west-2"),
		Realm:        microv1.ServiceAddress_REALM_WORLD,
		Organization: envOr("CARDINAL_ORG", "organization"),
		Project:      envOr("CARDINAL_PROJECT", "project"),
		ServiceId:    envOr("CARDINAL_SHARD_ID", "game"),
	}
)

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

type createPlayerCommand struct {
	Nickname string
}

type attackPlayerCommand struct {
	Target string
	Damage uint32
}

type callExternalCommand struct {
	Message string
}

type newPlayerEvent struct {
	Nickname string
}

type playerDeathEvent struct {
	Nickname string
}

type console struct {
	mu sync.Mutex
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := cardinalv1connect.NewCardinalServiceClient(h2cClient(), serverURL)
	streamReq := connect.NewRequest(&cardinalv1.StartEventStreamRequest{})
	setAuthHeaders(streamReq.Header())

	stream, err := client.StartEventStream(ctx, streamReq)
	if err != nil {
		return eris.Wrap(err, "failed to start event stream")
	}

	c := &console{}
	c.message("connected to %s", serverURL)
	c.message("commands: c <command-name> <inputs...> | s <event-name> | u <event-name> | q")
	c.prompt()

	receiveErr := make(chan error, 1)
	go func() {
		receiveErr <- receive(stream, c)
	}()

	input := make(chan string)
	go readInput(input)

	for {
		select {
		case err := <-receiveErr:
			return err
		case line, ok := <-input:
			if !ok {
				return nil
			}
			if err := handleInput(ctx, client, c, line); err != nil {
				c.message("error: %v", err)
			}
			c.prompt()
		}
	}
}

func readInput(out chan<- string) {
	defer close(out)

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		out <- scanner.Text()
	}
}

func handleInput(ctx context.Context, client cardinalv1connect.CardinalServiceClient, c *console, line string) error {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil
	}

	switch fields[0] {
	case "c":
		if len(fields) < 2 {
			return eris.New("usage: c <command-name> <inputs...>")
		}
		return sendCommand(ctx, client, fields[1], fields[2:])
	case "s":
		if len(fields) != 2 {
			return eris.New("usage: s <event-name>")
		}
		return subscribeEvent(ctx, client, fields[1])
	case "u":
		if len(fields) != 2 {
			return eris.New("usage: u <event-name>")
		}
		return unsubscribeEvent(ctx, client, fields[1])
	case "q":
		if len(fields) != 1 {
			return eris.New("usage: q")
		}
		return queryAll(ctx, client, c)
	default:
		return eris.Errorf("unknown command %q", fields[0])
	}
}

func sendCommand(ctx context.Context, client cardinalv1connect.CardinalServiceClient, name string, args []string) error {
	payload, err := commandPayload(name, args)
	if err != nil {
		return err
	}

	payloadBytes, err := msgpack.Marshal(payload)
	if err != nil {
		return eris.Wrapf(err, "failed to serialize command %q", name)
	}

	req := connect.NewRequest(&cardinalv1.SendCommandRequest{
		Command: &iscv1.Command{
			Name:    name,
			Address: commandAddress,
			Persona: &iscv1.Persona{Id: personaID},
			Payload: payloadBytes,
		},
	})
	setAuthHeaders(req.Header())

	_, err = client.SendCommand(ctx, req)
	return eris.Wrapf(err, "failed to send command %q", name)
}

func commandPayload(name string, args []string) (any, error) {
	switch name {
	case "create-player":
		if len(args) != 1 {
			return nil, eris.New("usage: c create-player <nickname>")
		}
		return createPlayerCommand{Nickname: args[0]}, nil
	case "attack-player":
		if len(args) != 2 {
			return nil, eris.New("usage: c attack-player <target> <damage>")
		}
		damage, err := strconv.ParseUint(args[1], 10, 32)
		if err != nil {
			return nil, eris.Wrap(err, "damage must be a uint32")
		}
		return attackPlayerCommand{Target: args[0], Damage: uint32(damage)}, nil
	case "call-external":
		if len(args) != 1 {
			return nil, eris.New("usage: c call-external <message>")
		}
		return callExternalCommand{Message: args[0]}, nil
	default:
		return map[string]any{"args": args}, nil
	}
}

func subscribeEvent(ctx context.Context, client cardinalv1connect.CardinalServiceClient, eventName string) error {
	req := connect.NewRequest(&cardinalv1.SubscribeEventsRequest{
		Subscriptions: []*cardinalv1.EventSubscription{
			{Address: commandAddress, Events: []string{eventName}},
		},
	})
	setAuthHeaders(req.Header())

	_, err := client.SubscribeEvents(ctx, req)
	return eris.Wrapf(err, "failed to subscribe to %q", eventName)
}

func unsubscribeEvent(ctx context.Context, client cardinalv1connect.CardinalServiceClient, eventName string) error {
	req := connect.NewRequest(&cardinalv1.UnsubscribeEventsRequest{
		Subscriptions: []*cardinalv1.EventSubscription{
			{Address: commandAddress, Events: []string{eventName}},
		},
	})
	setAuthHeaders(req.Header())

	_, err := client.UnsubscribeEvents(ctx, req)
	return eris.Wrapf(err, "failed to unsubscribe from %q", eventName)
}

func queryAll(ctx context.Context, client cardinalv1connect.CardinalServiceClient, c *console) error {
	req := connect.NewRequest(&cardinalv1.QueryRequest{
		Address: commandAddress,
		Query: &iscv1.Query{
			Match: iscv1.Query_MATCH_ALL,
		},
	})
	setAuthHeaders(req.Header())

	res, err := client.Query(ctx, req)
	if err != nil {
		return eris.Wrap(err, "failed to query all entities")
	}

	entities := res.Msg.GetResults().GetEntities()
	if len(entities) == 0 {
		c.message("query: no entities")
		return nil
	}

	c.message("query: %d entities", len(entities))
	for i, data := range entities {
		var entity map[string]any
		if err := msgpack.Unmarshal(data, &entity); err != nil {
			c.message("query[%d]: decode error: %v raw=%x", i, err, data)
			continue
		}
		c.message("query[%d]: %+v", i, entity)
	}
	return nil
}

func receive(stream *connect.ServerStreamForClient[cardinalv1.StartEventStreamResponse], c *console) error {
	for stream.Receive() {
		msg := stream.Msg()

		event := msg.GetEvent()
		if event == nil || event.GetName() == "" {
			continue
		}

		c.message("[%s] %s", event.GetName(), formatEvent(event))
	}
	if err := stream.Err(); err != nil {
		return eris.Wrap(err, "failed to receive stream response")
	}
	return nil
}

func formatEvent(event *iscv1.Event) string {
	switch event.GetName() {
	case "new-player":
		var payload newPlayerEvent
		if err := msgpack.Unmarshal(event.GetPayload(), &payload); err != nil {
			return fmt.Sprintf("decode error: %v raw=%x", err, event.GetPayload())
		}
		return fmt.Sprintf("%+v", payload)
	case "player-death":
		var payload playerDeathEvent
		if err := msgpack.Unmarshal(event.GetPayload(), &payload); err != nil {
			return fmt.Sprintf("decode error: %v raw=%x", err, event.GetPayload())
		}
		return fmt.Sprintf("%+v", payload)
	default:
		return fmt.Sprintf("raw=%x", event.GetPayload())
	}
}

func (c *console) message(format string, args ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	fmt.Printf("\r\033[2K%s\n", fmt.Sprintf(format, args...))
}

func (c *console) prompt() {
	c.mu.Lock()
	defer c.mu.Unlock()

	fmt.Print("> ")
}

func setAuthHeaders(header http.Header) {
	if bearerToken != "" {
		header.Set("Authorization", "Bearer "+bearerToken)
		return
	}
	header.Set("X-Email", email)
}

func h2cClient() *http.Client {
	return &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, network, addr)
			},
		},
	}
}
