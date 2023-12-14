// Package statsd is a helper package that wraps some common statsd methods.
// It hides the datadog dependency so if we decide to migrate away from datadog in the future, we only need to
// edit this single file. For example, the https://pkg.go.dev/github.com/cactus/go-statsd-client/statsd package roughly
// implements datadog's ClientInterface interface.
package statsd

import (
	"time"

	ddstatsd "github.com/DataDog/datadog-go/v5/statsd"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog/log"
)

var client ddstatsd.ClientInterface = &ddstatsd.NoOpClient{}

func Client() ddstatsd.ClientInterface {
	return client
}

func EmitTickStat(start time.Time, stage string) {
	duration := time.Since(start)
	err := Client().Timing("tick", duration, []string{stage}, 1)
	if err != nil {
		log.Logger.Warn().Msgf("failed to emit tick stat: %v", err)
	}
}

func Init(address string, tags []string) error {
	if address == "" {
		return eris.New("address must not be empty")
	}
	opts := []ddstatsd.Option{
		// The statsd namespace is the prefix of all metrics
		ddstatsd.WithNamespace("cardinal"),
	}
	if len(tags) > 0 {
		opts = append(opts, ddstatsd.WithTags(tags))
	}

	newClient, err := ddstatsd.New(address, opts...)
	if err != nil {
		return err
	}
	// Success! replace the global client
	client = newClient
	return nil
}
