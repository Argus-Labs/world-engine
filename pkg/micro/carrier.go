package micro

import (
	"strings"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel/propagation"
)

// headerCarrier adapts nats.Header for OpenTelemetry context propagation.
//
// propagation.HeaderCarrier is built for http.Header and canonicalizes keys on Get, but NATS keeps
// header keys as sent, and the wire can deliver "traceparent" in a different case than it was set.
// This carrier writes lowercase keys and reads case-insensitively, as the W3C trace-context spec
// requires.
type headerCarrier nats.Header

var (
	_ propagation.TextMapCarrier = headerCarrier{}
	_ propagation.ValuesGetter   = headerCarrier{}
)

func (c headerCarrier) Get(key string) string {
	if v := c.Values(key); len(v) > 0 {
		return v[0]
	}
	return ""
}

// Values returns every value stored under key, ignoring case. The Baggage propagator reads
// through this so repeated baggage headers are all kept. An exact-case match wins outright so
// that a header we wrote ourselves is not shadowed by a differently cased duplicate picked in
// map order.
func (c headerCarrier) Values(key string) []string {
	if v, ok := c[key]; ok {
		return v
	}
	var values []string
	for k, v := range c {
		if strings.EqualFold(k, key) {
			values = append(values, v...)
		}
	}
	return values
}

func (c headerCarrier) Set(key, value string) {
	c[strings.ToLower(key)] = []string{value}
}

func (c headerCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}
