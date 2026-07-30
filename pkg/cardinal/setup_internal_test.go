package cardinal

import (
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func NewTestClient(t *testing.T, natsURL string) *micro.Client {
	t.Helper()

	c, err := micro.NewClient(
		micro.WithNATSConfig(micro.NATSConfig{Name: "test-client", URL: natsURL}),
		micro.WithLogger(zerolog.Nop()),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		c.Close()
	})

	return c
}

func RandServiceAddress(prng *rand.Rand) *micro.ServiceAddress {
	return micro.GetAddress(
		"r-"+strconv.FormatInt(prng.Int64(), 10),
		micro.RealmInternal,
		"o-"+strconv.FormatInt(prng.Int64(), 10),
		"p-"+strconv.FormatInt(prng.Int64(), 10),
		"s-"+strconv.FormatInt(prng.Int64(), 10),
	)
}
