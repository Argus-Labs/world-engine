//nolint:forbidigo // This is an interactive example client.
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"connectrpc.com/connect"
	"github.com/argus-labs/world-engine/pkg/micro"
	gamegen "github.com/argus-labs/world-engine/pkg/template/basic/shards/game/gen"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/goccy/go-json"
	"github.com/shamaton/msgpack/v3"
	"google.golang.org/protobuf/proto"
)

const cardinalURL = "http://localhost:8080/organization/project/game"

func main() {
	listenOnly := flag.Bool("listen", false, "only listen for and print events")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	identity := "producer"
	if *listenOnly {
		identity = "listener"
	}

	address := micro.GetAddress("us-west-2", micro.RealmWorld, "organization", "project", "game")
	client := cardinalv1connect.NewCardinalServiceClient(http.DefaultClient, cardinalURL)

	streamRequest := connect.NewRequest(&cardinalv1.StartEventStreamRequest{
		Subscriptions: []*cardinalv1.EventSubscription{{
			Address: address,
			Events:  []string{"*"}, // Listen to all events
		}},
	})
	streamRequest.Header().Set("X-Email", identity)
	stream, err := client.StartEventStream(ctx, streamRequest)
	if err != nil {
		fatal("start event stream", err)
	}

	fmt.Printf("Connected as %s\n", identity)
	eventsDone := make(chan error, 1)
	go func() {
		eventsDone <- printEvents(stream)
	}()

	if *listenOnly {
		fmt.Println("Listening for events. Press Ctrl+C to quit.")
		select {
		case <-ctx.Done():
			return
		case err := <-eventsDone:
			if err != nil {
				fatal("receive events", err)
			}
			return
		}
	}

	fmt.Println("Press Enter to kill default-0 through default-9 in order.")
	fmt.Println("Type one or more spaces, then press Enter, to create a randomly named player.")
	lines := make(chan string)
	inputDone := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
		inputDone <- scanner.Err()
		close(lines)
	}()

	nextTarget := 0
	for {
		var line string
		select {
		case <-ctx.Done():
			return
		case err := <-eventsDone:
			if err != nil {
				fatal("receive events", err)
			}
			return
		case value, ok := <-lines:
			if !ok {
				if err := <-inputDone; err != nil {
					fatal("read input", err)
				}
				return
			}
			line = value
		}

		switch {
		case line == "":
			if nextTarget >= 10 {
				fmt.Println("No default players left; no command sent.")
				continue
			}
			target := fmt.Sprintf("default-%d", nextTarget)
			if err := sendCommand(ctx, client, identity, address, "attack-player", &gamegen.AttackPlayerCommand{
				Target: target,
				Damage: math.MaxUint32,
			}); err != nil {
				fmt.Fprintf(os.Stderr, "attack %s: %v\n", target, err)
				continue
			}
			fmt.Printf("Attacked %s with %d damage.\n", target, uint32(math.MaxUint32))
			nextTarget++
		case strings.Trim(line, " ") == "":
			name := "player-" + randomSuffix()
			if err := sendCommand(ctx, client, identity, address, "create-player", &gamegen.CreatePlayerCommand{
				Nickname: name,
			}); err != nil {
				fmt.Fprintf(os.Stderr, "create %s: %v\n", name, err)
				continue
			}
			fmt.Printf("Created %s.\n", name)
		default:
			fmt.Println("Unknown input: press Enter to attack or enter spaces to create a player.")
		}
	}
}

func sendCommand(
	ctx context.Context,
	client cardinalv1connect.CardinalServiceClient,
	identity string,
	address *micro.ServiceAddress,
	name string,
	payload proto.Message,
) error {
	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	request := connect.NewRequest(&cardinalv1.SendCommandRequest{
		Command: &iscv1.Command{
			Name:    name,
			Address: address,
			Persona: &iscv1.Persona{Id: identity},
			Payload: payloadBytes,
		},
	})
	request.Header().Set("X-Email", identity)
	_, err = client.SendCommand(ctx, request)
	return err
}

func printEvents(stream *connect.ServerStreamForClient[cardinalv1.StartEventStreamResponse]) error {
	for stream.Receive() {
		message := stream.Msg()
		event := message.GetEvent()
		if event == nil {
			continue
		}

		var payload map[string]any
		if err := msgpack.Unmarshal(event.GetPayload(), &payload); err != nil {
			fmt.Printf("event %s payload=%x (decode error: %v)\n", event.GetName(), event.GetPayload(), err)
			continue
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			fmt.Printf("event %s payload=%#v\n", event.GetName(), payload)
			continue
		}
		fmt.Printf("event %s payload=%s\n", event.GetName(), encoded)
	}
	return stream.Err()
}

func randomSuffix() string {
	var data [4]byte
	if _, err := rand.Read(data[:]); err != nil {
		fatal("generate random name", err)
	}
	return hex.EncodeToString(data[:])
}

func fatal(action string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", action, err)
	os.Exit(1)
}
