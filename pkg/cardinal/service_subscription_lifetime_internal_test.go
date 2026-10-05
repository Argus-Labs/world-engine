package cardinal

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/argus-labs/world-engine/pkg/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/stretchr/testify/require"
)

func TestService_SubscriptionsRequireOpenStream(t *testing.T) {
	t.Parallel()
	svc := newServiceFixture(t, testutils.NewRand(t), false).svc
	user := &User{ID: "user"}
	subscriptions := []*cardinalv1.EventSubscription{{Address: svc.world.address, Events: []string{"currency"}}}

	// A stream can close after request validation. Mutation must check under its own lock.
	_, err := svc.addSubscriber(context.Background(), user, nil)
	require.NoError(t, err)
	validated, err := svc.subscriptionRequest(serviceTestContext(user.ID), subscriptions)
	require.NoError(t, err)
	svc.removeSubscriber(user)
	require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(svc.subscribeEvents(validated, subscriptions)))
	require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(svc.unsubscribeEvents(validated, subscriptions)))

	// The RPC callers propagate that outcome instead of acknowledging an unapplied change.
	_, err = svc.SubscribeEvents(serviceTestContext(user.ID), connect.NewRequest(&cardinalv1.SubscribeEventsRequest{
		Subscriptions: subscriptions,
	}))
	require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	_, err = svc.UnsubscribeEvents(serviceTestContext(user.ID), connect.NewRequest(&cardinalv1.UnsubscribeEventsRequest{
		Subscriptions: subscriptions,
	}))
	require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}
