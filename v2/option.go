package cardinal

import (
	"time"

	"pkg.world.dev/world-engine/cardinal/v2/world"
)

type CardinalOption struct {
	worldOption    world.Option
	cardinalOption Option
}

type Option func(*Cardinal)

// WithDisableSignatureVerification disables signature verification for the HTTP server. This should only be
// used for local development.
func WithDisableSignatureVerification() CardinalOption {
	return CardinalOption{
		worldOption: world.WithVerifySignature(false),
	}
}

// WithTickChannel sets the channel that will be used to decide when world.doTick is executed. If unset, a loop interval
// of 1 second will be set. To set some other time, use: WithTickChannel(time.Tick(<some-duration>)). Tests can pass
// in a channel controlled by the test for fine-grained control over when ticks are executed.
func WithTickChannel(ch <-chan time.Time) CardinalOption {
	return CardinalOption{
		cardinalOption: func(cardinal *Cardinal) {
			cardinal.tickChannel = ch
		},
	}
}

func WithStartHook(hook func() error) CardinalOption {
	return CardinalOption{
		cardinalOption: func(c *Cardinal) {
			c.startHook = hook
		},
	}
}

// separateOptions separates the given options into ecs options, server options, and cardinal (this package) options.
// The different options are all grouped together to simplify the end user's experience, but under the hood different
// options are meant for different sub-systems.
func separateOptions(opts []CardinalOption) ([]Option, []world.Option) {
	cardinalOpts := make([]Option, 0)
	worldOpts := make([]world.Option, 0)

	for _, opt := range opts {
		if opt.cardinalOption != nil {
			cardinalOpts = append(cardinalOpts, opt.cardinalOption)
		}
		if opt.worldOption != nil {
			worldOpts = append(worldOpts, opt.worldOption)
		}
	}

	return cardinalOpts, worldOpts
}
