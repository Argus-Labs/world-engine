package epoch

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"buf.build/go/protovalidate"
	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Log interface {
	Publish(context.Context, Epoch) error
	EpochCount() (uint64, error)
	Consume(context.Context, func(*iscv1.Epoch) error) error
	ConsumeBatch(context.Context, int, func(*iscv1.Epoch) error) (int, error)
}

var _ Log = (*JetStreamLog)(nil)

type JetStreamLog struct {
	js          jetstream.JetStream // JetStream client
	stream      jetstream.Stream    // The epoch stream
	consumer    jetstream.Consumer  // Reusable JetStream consumer
	subject     string              // Epoch subject
	epochHeight uint64
	tel         *telemetry.Telemetry
}

func NewJetStreamLog(opts JetStreamLogOptions) (*JetStreamLog, error) {
	if err := opts.Validate(); err != nil {
		return nil, eris.Wrap(err, "invalid options passed")
	}

	subject := micro.Endpoint(opts.Address, "epoch")
	streamName := formatStreamName(opts.Address)

	js, err := jetstream.New(opts.Client.Conn)
	if err != nil {
		return nil, eris.Wrap(err, "failed to create jetstream client")
	}

	streamConfig := jetstream.StreamConfig{
		Name:      streamName,
		Subjects:  []string{subject},
		Retention: jetstream.LimitsPolicy,
		Storage:   jetstream.FileStorage,
		Replicas:  1,
		// TODO: handle this option in this module instead of as a WorldOption as this isn't meant to
		// be configurable.
		MaxBytes: 0,
	}

	logger := opts.Telemetry.GetLogger("shard")

	// Try to get existing stream first, if it exists we'll update it
	stream, err := js.Stream(context.Background(), streamName) //nolint: staticcheck, wastedassign // this is ok
	if err != nil {
		// Stream doesn't exist, try to create it
		logger.Debug().Str("stream", streamConfig.Name).Msg("creating epoch stream")
		stream, err = js.CreateStream(context.Background(), streamConfig)
		if err != nil {
			return nil, eris.Wrapf(err, "failed to create epoch stream (name=%s, subjects=%v, maxBytes=%d)",
				streamConfig.Name, streamConfig.Subjects, streamConfig.MaxBytes)
		}
	} else {
		// TODO: figure out why this branch exists and if it can be removed safely.
		// Stream exists, update it with new config
		logger.Debug().Str("stream", streamConfig.Name).Msg("updating epoch stream")
		stream, err = js.UpdateStream(context.Background(), streamConfig)
		if err != nil {
			return nil, eris.Wrapf(err, "failed to update existing epoch stream (name=%s, subjects=%v, maxBytes=%d)",
				streamConfig.Name, streamConfig.Subjects, streamConfig.MaxBytes)
		}
	}

	consumer, err := stream.CreateConsumer(context.Background(), jetstream.ConsumerConfig{
		FilterSubject: subject,
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return nil, eris.Wrap(err, "failed to create stream consumer")
	}

	return &JetStreamLog{
		js:       js,
		stream:   stream,
		consumer: consumer,
		subject:  subject,
		tel:      opts.Telemetry,
	}, nil
}

func (j *JetStreamLog) Publish(ctx context.Context, epoch Epoch) error {
	logger := j.tel.GetLogger("log")
	logger.Debug().Uint64("epoch", epoch.EpochHeight).Msg("publishing epoch")

	epochPb := iscv1.Epoch{
		EpochHeight: epoch.EpochHeight,
		Hash:        epoch.Hash,
	}

	for _, tick := range epoch.Ticks {
		tickPb := &iscv1.Tick{
			Header: &iscv1.TickHeader{
				TickHeight: tick.Header.TickHeight,
				Timestamp:  timestamppb.New(tick.Header.Timestamp),
			},
			Data: &iscv1.TickData{},
		}

		for _, command := range tick.Data.Commands {
			commandPb := &iscv1.Command{
				Name:    command.Name,
				Address: command.Address,
				Persona: &iscv1.Persona{Id: command.Persona},
			}

			if command.Payload != nil {
				pbStruct, err := marshalToStruct(command.Payload)
				if err != nil {
					return eris.Wrap(err, "failed to marshal command payload")
				}
				commandPb.Payload = pbStruct
			}

			tickPb.Data.Commands = append(tickPb.Data.Commands, commandPb)
		}

		epochPb.Ticks = append(epochPb.Ticks, tickPb)
	}

	payload, err := proto.Marshal(&epochPb)
	if err != nil {
		return eris.Wrap(err, "failed to marshal epoch")
	}
	ack, err := j.js.Publish(ctx, j.subject, payload, epochPublishOptions(j.subject, epoch.EpochHeight)...)
	if err != nil {
		return eris.Wrap(err, "failed to publish epoch")
	}

	// Verify stream sequence matches expected epoch height (1:1 mapping).
	// Stream sequences start at 1, so we'll have to compare with epochHeight+1. We can't set the
	// stream sequence to start at 0, so we'll just have to deal with it here.
	expectedSeq := epoch.EpochHeight + 1
	if ack.Sequence != expectedSeq {
		return eris.Errorf("epoch sequence mismatch: expected %d, got %d", expectedSeq, ack.Sequence)
	}

	logger.Debug().Uint64("epoch", epoch.EpochHeight).Uint64("seq", ack.Sequence).Hex("hash", epoch.Hash).Msg("epoch published")
	return nil
}

func (j *JetStreamLog) EpochCount() (uint64, error) {
	// Get consumer info to determine how many messages are pending.
	info, err := j.consumer.Info(context.Background())
	if err != nil {
		return 0, eris.Wrap(err, "failed to fetch consumer info")
	}
	return info.NumPending, nil
}

func (j *JetStreamLog) Consume(ctx context.Context, callback func(epoch *iscv1.Epoch) error) error {
	msgs, err := j.consumer.Fetch(1, jetstream.FetchMaxWait(0)) // 0 = wait indefinitely
	if err != nil {
		return eris.Wrap(err, "failed to fetch message")
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case msg, ok := <-msgs.Messages():
		if !ok {
			if err := msgs.Error(); err != nil {
				return eris.Wrap(err, "error during message fetch")
			}
			return nil
		}

		epoch := iscv1.Epoch{}
		if err := proto.Unmarshal(msg.Data(), &epoch); err != nil {
			return eris.Wrap(err, "failed to unmarshal epoch")
		}

		if err := protovalidate.Validate(&epoch); err != nil {
			return eris.Wrap(err, "failed to validate epoch")
		}

		if err := callback(&epoch); err != nil {
			return eris.Wrap(err, "callback failed")
		}

		if err := msg.Ack(); err != nil {
			return eris.Wrap(err, "failed to acknowledge message")
		}
	}

	return nil
}

func (j *JetStreamLog) ConsumeBatch(ctx context.Context, batchSize int, callback func(epoch *iscv1.Epoch) error) (int, error) {
	batch, err := j.consumer.FetchNoWait(int(batchSize))
	if err != nil {
		return 0, eris.Wrap(err, "failed to fetch message batch")
	}

	processed := 0
	for message := range batch.Messages() {
		processed++

		epoch := iscv1.Epoch{}
		if err := proto.Unmarshal(message.Data(), &epoch); err != nil {
			return 0, eris.Wrap(err, "failed to unmarshal epoch")
		}

		if err := protovalidate.Validate(&epoch); err != nil {
			return 0, eris.Wrap(err, "failed to validate epoch")
		}

		if err := callback(&epoch); err != nil {
			return 0, eris.Wrap(err, "callback failed")
		}

		if err := message.Ack(); err != nil {
			return 0, eris.Wrap(err, "failed to acknowledge message")
		}
	}

	// Check for errors that occurred during message delivery.
	if err := batch.Error(); err != nil {
		return 0, eris.Wrap(err, "error occurred during message batch delivery")
	}

	return processed, nil
}

// -------------------------------------------------------------------------------------------------
// Options
// -------------------------------------------------------------------------------------------------

type JetStreamLogOptions struct {
	Client    *micro.Client         // NATS client for inter-service communication
	Address   *micro.ServiceAddress // This Cardinal shard's service address
	Telemetry *telemetry.Telemetry
}

// Validate checks that all required fields in ShardServiceOptions are not nil.
func (opts JetStreamLogOptions) Validate() error {
	if opts.Client == nil {
		return eris.New("client cannot be nil")
	}
	if opts.Address == nil {
		return eris.New("address cannot be nil")
	}
	if opts.Telemetry == nil {
		return eris.New("telemetry cannot be nil")
	}
	return nil
}

// -------------------------------------------------------------------------------------------------
// Utilities
// -------------------------------------------------------------------------------------------------

// formatStreamName creates a unique stream name based on the service address.
func formatStreamName(address *micro.ServiceAddress) string {
	return fmt.Sprintf("%s_%s_%s_epoch",
		address.GetOrganization(), address.GetProject(), address.GetServiceId())
}

// epochPublishOptions creates publish options for epoch messages with deduplication and ordering.
// It uses ExpectLastSequence for ordering which is persisted with stream state and survives NATS restarts,
// unlike ExpectLastMsgID which relies on an in-memory deduplication cache.
// epochHeight is used as the expected last sequence since epochs map 1:1 with stream sequences.
func epochPublishOptions(subject string, epochHeight uint64) []jetstream.PublishOpt {
	opts := []jetstream.PublishOpt{jetstream.WithMsgID(fmt.Sprintf("%s-%d", subject, epochHeight))}
	if epochHeight > 0 {
		opts = append(opts, jetstream.WithExpectLastSequence(epochHeight))
	}
	return opts
}

func marshalToStruct(payload any) (*structpb.Struct, error) {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return nil, eris.Wrap(err, "failed to marshal payload")
	}

	var m map[string]any
	if err := json.Unmarshal(bytes, &m); err != nil {
		return nil, eris.Wrap(err, "failed to unmarshal payload to map[string]any")
	}

	pbStruct, err := structpb.NewStruct(m)
	if err != nil {
		return nil, eris.Wrap(err, "failed to convert map to structpb.Struct")
	}
	return pbStruct, nil
}

// epochHeightFromMsgID extracts the epoch height from a JetStream message ID.
// Message IDs are in the format "<subject>-<epochHeight>".
func epochHeightFromMsgID(msgID, subject string) uint64 {
	epochStr := strings.TrimPrefix(msgID, subject+"-")
	epochHeight, err := strconv.ParseUint(epochStr, 10, 64)
	assert.That(err == nil, "epoch isn't published in the expected format")
	return epochHeight
}
