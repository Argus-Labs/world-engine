package posthog

import (
	"context"
	"maps"
	"time"

	posthoggo "github.com/posthog/posthog-go"
	"github.com/rotisserie/eris"
	"go.opentelemetry.io/otel/trace"
)

// Options configures the PostHog client.
// If APIKey is empty, the client will be disabled and methods become no-ops.
type Options struct {
	// APIKey for your PostHog project.
	APIKey string
	// DistinctID for the posthog project.
	DistinctID string
	// BaseProperties to add to all events.
	BaseProperties map[string]any
}

// Client is a thin wrapper around posthog-go that supports no-op usage when disabled
// and adds optional OpenTelemetry trace correlation.
type Client struct {
	client         posthoggo.Client
	isEnabled      bool
	distinctID     string
	baseProperties map[string]any
}

// New creates a PostHog client. If APIKey is empty, a disabled client is returned.
func New(opt Options) (*Client, error) {
	if opt.APIKey == "" {
		// PostHog is disabled if APIKey is empty
		return &Client{isEnabled: false}, nil
	}
	if opt.DistinctID == "" {
		return &Client{isEnabled: false}, eris.New("posthog distinctID is required")
	}

	underlying := posthoggo.New(opt.APIKey)

	return &Client{
		client:         underlying,
		isEnabled:      true,
		distinctID:     opt.DistinctID,
		baseProperties: opt.BaseProperties,
	}, nil
}

// Capture sends a PostHog capture event. When disabled, it is a no-op.
// If ctx contains an active span, trace identifiers are added to properties.
func (c *Client) Capture(ctx context.Context, event string, properties map[string]any) error {
	if !c.isInitialized() {
		return nil
	}

	mergedProperties := make(map[string]any, len(properties)+len(c.baseProperties)+2)
	maps.Copy(mergedProperties, c.baseProperties)
	maps.Copy(mergedProperties, properties)

	if spanCtx := trace.SpanContextFromContext(ctx); spanCtx.IsValid() {
		mergedProperties["trace_id"] = spanCtx.TraceID().String()
		mergedProperties["span_id"] = spanCtx.SpanID().String()
	}

	return c.client.Enqueue(posthoggo.Capture{
		DistinctId: c.distinctID,
		Timestamp:  time.Now(),
		Event:      event,
		Properties: mergedProperties,
	})
}

// Shutdown closes the client and flushes queued events. When disabled, it is a no-op.
func (c *Client) Shutdown() error {
	if !c.isInitialized() {
		return nil
	}
	return c.client.Close()
}

func (c *Client) isInitialized() bool {
	return c.client != nil && c.isEnabled
}
