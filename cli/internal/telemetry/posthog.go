package telemetry

import (
	"os"
	"path/filepath"
	"time"

	"github.com/denisbrodbeck/machineid"
	"github.com/posthog/posthog-go"
	"github.com/rs/zerolog/log"

	"github.com/argus-labs/world-engine/cli/internal/store"
)

const (
	PostInstallationEvent = "World CLI Installation"
	RunningEvent          = "World CLI Running"
	timestampFile         = "last_logged_time"
)

// posthogClientAPI is the subset of posthog.Client this package depends on.
// Narrowing to it lets tests inject a fake that records Enqueue/Close instead
// of the real in-memory + HTTP client.
type posthogClientAPI interface {
	Enqueue(posthog.Message) error
	Close() error
}

var (
	//nolint:gochecknoglobals // Global PostHog client for telemetry across the application
	posthogClient posthogClientAPI
	//nolint:gochecknoglobals // Global flag to track if PostHog has been initialized
	posthogInitialized bool
	//nolint:gochecknoglobals // Global timestamp for rate limiting telemetry events
	lastLoggedTime time.Time
)

// PosthogInit Posthog initialization.
func PosthogInit(posthogAPIKey string) {
	if posthogAPIKey == "" {
		return
	}
	posthogInitWithClient(posthog.New(posthogAPIKey))
}

// posthogInitWithClient binds the PostHog client and loads the daily dedupe
// timestamp. It deliberately does NOT stamp the dedupe file: the stamp is
// written only after a Running event is successfully enqueued (see
// PosthogCaptureEvent), so a path that never captures the event -- or that
// drops the flush before it is delivered -- does not consume the day's quota.
// Previously PosthogInit stamped "today" unconditionally, so any invocation
// that exited before flushing (kong's --help/--version/parse-error [os.Exit], a
// crash, or the post-installation path that never captures Running) silently
// suppressed the Running event for the rest of the day.
func posthogInitWithClient(c posthogClientAPI) {
	posthogClient = c
	posthogInitialized = true

	// get last logged time
	lastTime, err := getLastLoggedTime()
	if err != nil {
		log.Err(err).Msg("Cannot get last logged time")
	}

	lastLoggedTime = lastTime
}

// getLastLoggedTime reads the last visited timestamp from the file.
func getLastLoggedTime() (time.Time, error) {
	filePath, err := getTimestampFilePath()
	if err != nil {
		return time.Time{}, err
	}

	if _, err = os.Stat(filePath); os.IsNotExist(err) {
		// Return a zero time if the file does not exist
		return time.Time{}, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return time.Time{}, err
	}

	timestamp, err := time.Parse(time.DateOnly, string(data))
	if err != nil {
		return time.Time{}, err
	}

	return timestamp, nil
}

// getTimestampFilePath returns the path to the timestamp file.
func getTimestampFilePath() (string, error) {
	storeDir, err := store.SetupStoreDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(storeDir, timestampFile), nil
}

// updateLastLoggedTime updates the last visited timestamp in the file.
func updateLastLoggedTime(timestamp time.Time) error {
	filePath, err := getTimestampFilePath()
	if err != nil {
		return err
	}

	data := []byte(timestamp.Format(time.DateOnly))

	return os.WriteFile(filePath, data, 0644) //nolint:gosec // not applicable
}

// isSameDay checks if two timestamps are from the same day.
func isSameDay(time1, time2 time.Time) bool {
	return time1.Year() == time2.Year() &&
		time1.Month() == time2.Month() &&
		time1.Day() == time2.Day()
}

func PosthogCaptureEvent(event string, properties map[string]any) {
	if !posthogInitialized {
		return
	}
	if event == RunningEvent && isSameDay(lastLoggedTime, time.Now()) {
		return // skip duplicate daily RunningEvent
	}

	// Obtain the machine ID
	machineID, err := machineid.ProtectedID("world-cli")
	if err != nil {
		log.Err(err).Msg("Cannot get machine id")
		return
	}

	// Capture the event
	err = posthogClient.Enqueue(posthog.Capture{
		DistinctId: machineID,
		Timestamp:  time.Now(),
		Event:      event,
		Properties: properties,
	})
	if err != nil {
		log.Err(err).Msg("Cannot capture event")
		return
	}

	// Stamp the daily dedupe file only after the Running event has been
	// enqueued, so the day's quota is consumed by a capture that actually
	// queued an event rather than at init. Only the Running event is deduped,
	// so non-daily events (e.g. PostInstallationEvent) never stamp the file.
	if event == RunningEvent {
		lastLoggedTime = time.Now()
		if err := updateLastLoggedTime(lastLoggedTime); err != nil {
			log.Err(err).Msg("Cannot update last logged time")
		}
	}
}

func PosthogClose() {
	if posthogInitialized {
		err := posthogClient.Close()
		if err != nil {
			log.Err(err).Msg("Cannot close posthog client")
		}
		posthogInitialized = false
	}
}
