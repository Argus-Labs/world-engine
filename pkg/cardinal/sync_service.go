package cardinal

import (
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	syncstate "github.com/argus-labs/world-engine/pkg/cardinal/sync"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

// prepareSync runs after all systems and before event dispatch. New readers receive the final
// baseline first and skip commits already represented in it. Subscription RPCs only set flags.
func (s *service) prepareSync() {
	world := s.world.sync
	if world == nil || world.admit == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for player, subscriber := range s.subscribers {
		subscriber.syncBaselineTick = false
		if _, subscribed := subscriber.events[syncstate.EventName]; !subscribed {
			subscriber.syncActive = false
			continue
		}
		if !world.admit(player) {
			// Revoke the sync segment without closing the shared transport: the same
			// tick can still contain spawn refusals or the session's cleanup events.
			if subscriber.syncActive {
				subscriber.syncEpoch = uuid.NewString()
				if err := s.enqueueSync(subscriber, syncstate.Frame{Sequence: world.sequence, Baseline: true}); err != nil {
					subscriber.cancel(err)
				}
			}
			subscriber.syncActive = false
			subscriber.syncNeedsBaseline = true
			continue
		}
		if subscriber.syncActive && !subscriber.syncNeedsBaseline {
			continue
		}
		subscriber.syncEpoch = uuid.NewString()
		records := make([]syncstate.Record, 0, len(world.records))
		for _, r := range world.records {
			records = append(records, r)
		}
		slices.SortFunc(records, func(a, b syncstate.Record) int {
			if a.ID.Entity < b.ID.Entity {
				return -1
			}
			if a.ID.Entity > b.ID.Entity {
				return 1
			}
			return 0
		})
		if err := s.enqueueSync(subscriber, syncstate.Frame{Sequence: world.sequence, Baseline: true, Records: records}); err != nil {
			subscriber.cancel(err)
			continue
		}
		subscriber.syncActive = true
		subscriber.syncNeedsBaseline = false
		subscriber.syncBaselineTick = true
		subscriber.syncSequence = world.sequence
	}
}

func (s *service) publishSyncCommit(commit syncCommit) error {
	world := s.world.sync
	if world == nil || world.admit == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for player, subscriber := range s.subscribers {
		if !subscriber.syncActive || commit.sequence <= subscriber.syncSequence {
			continue
		}
		if _, subscribed := subscriber.events[syncstate.EventName]; !subscribed {
			continue
		}
		if !world.admit(player) {
			continue
		}
		if err := s.enqueueSync(subscriber, syncstate.Frame{Sequence: commit.sequence, Records: commit.records, Removed: commit.removed}); err != nil {
			subscriber.cancel(err)
			continue
		}
		subscriber.syncSequence = commit.sequence
	}
	return nil
}

func (s *service) enqueueSync(subscriber *streamSubscriber, frame syncstate.Frame) error {
	frame.Version = syncstate.ProtocolVersion
	frame.Stream = subscriber.syncEpoch
	if s.world.sync.fingerprint == "" {
		fingerprint, err := s.world.SyncContract().Fingerprint()
		if err != nil {
			return err
		}
		s.world.sync.fingerprint = fingerprint
	}
	frame.Fingerprint = s.world.sync.fingerprint
	data, err := schema.Serialize(frame)
	if err != nil {
		return err
	}
	evt := &iscv1.Event{Name: syncstate.EventName, Payload: data}
	return subscriber.enqueue(&cardinalv1.StartEventStreamResponse{Address: s.world.address, Event: evt}, proto.Size(evt))
}
