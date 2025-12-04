package matchmaking

import (
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/rotisserie/eris"
)

// config holds environment-based configuration for the matchmaking shard.
type config struct {
	// MatchProfilesPath is the path to the JSON file containing match profiles.
	MatchProfilesPath string `env:"MATCHMAKING_PROFILES_PATH" envDefault:""`

	// TicketTTL is the default time-to-live for tickets.
	TicketTTLSeconds int `env:"MATCHMAKING_TICKET_TTL_SECONDS" envDefault:"300"`

	// BackfillTTL is the default time-to-live for backfill requests.
	BackfillTTLSeconds int `env:"MATCHMAKING_BACKFILL_TTL_SECONDS" envDefault:"300"`
}

// loadConfig loads configuration from environment variables.
func loadConfig() (config, error) {
	cfg, err := env.ParseAs[config]()
	if err != nil {
		return config{}, eris.Wrap(err, "failed to parse matchmaking config")
	}
	return cfg, nil
}

// TicketTTL returns the ticket TTL as a duration.
func (c config) TicketTTL() time.Duration {
	return time.Duration(c.TicketTTLSeconds) * time.Second
}

// BackfillTTL returns the backfill request TTL as a duration.
func (c config) BackfillTTL() time.Duration {
	return time.Duration(c.BackfillTTLSeconds) * time.Second
}
