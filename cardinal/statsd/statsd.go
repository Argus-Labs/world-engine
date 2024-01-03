// Package statsd is a helper package that wraps some common statsd methods.
// It hides the datadog dependency so if we decide to migrate away from datadog in the future, we only need to
// edit this single file. For example, the https://pkg.go.dev/github.com/cactus/go-statsd-client/statsd package roughly
// implements datadog's ClientInterface interface.
package statsd

import (
	"strings"
	"time"

	ddstatsd "github.com/DataDog/datadog-go/v5/statsd"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog/log"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

var client ddstatsd.ClientInterface = &ddstatsd.NoOpClient{}

func Client() ddstatsd.ClientInterface {
	return client
}

func EmitTickStat(start time.Time, stage string) {
	duration := time.Since(start)
	err := Client().Timing("tick", duration, []string{"stage:" + stage}, 1)
	if err != nil {
		log.Logger.Warn().Msgf("failed to emit tick stat: %v", err)
	}
}

func Init(statsdAddress, traceAddress string, tags []string) error {
	if statsdAddress == "" && traceAddress == "" {
		return eris.New("at least one of the statsd or trace address must be set")
	}
	if statsdAddress != "" {
		if err := initStatsd(statsdAddress, tags); err != nil {
			return err
		}
	}
	if traceAddress != "" {
		initRuntimeMetrics(traceAddress, statsdAddress, tags)
	}
	return nil
}

func initStatsd(address string, tags []string) error {
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

// initRuntimeMetrics starts the exporting of golang runtime metrics to the datadog agent.
func initRuntimeMetrics(traceAddress, statsdAddress string, tags []string) {
	opts := []tracer.StartOption{
		tracer.WithRuntimeMetrics(),
		tracer.WithAgentAddr(traceAddress),
	}
	if statsdAddress != "" {
		opts = append(opts, tracer.WithDogstatsdAddress(statsdAddress))
	}
	for _, tag := range tags {
		key, value := tagToTraceTag(tag)
		opts = append(opts, tracer.WithGlobalTag(key, value))
	}

	tracer.Start(opts...)
}

// tagToTraceTag converts metric tags (a string of the form "<some_key>:<some_value>") to a key and value pair
// suitable for trace tags. If there is no colon in the tag, the entire tag is treated as the key
func tagToTraceTag(tag string) (string, any) {
	colonLoc := strings.Index(tag, ":")
	if colonLoc == -1 {
		return tag, nil
	}
	key := tag[:colonLoc]
	value := tag[colonLoc+1:]
	if len(key) == 0 {
		return value, nil
	}
	if len(value) == 0 {
		return key, nil
	}
	return key, value
}
