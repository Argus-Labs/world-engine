package cardinal_test

import (
	"io"
	"os"
	"testing"

	"github.com/goccy/go-json"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

func TestOptionFunctionSignatures(_ *testing.T) {
	// This test is designed to keep API compatibility. If a compile error happens here it means a function signature to
	// public facing functions was changed.
	cardinal.WithReceiptHistorySize(1)
	cardinal.WithCustomLogger(zerolog.New(os.Stdout))
	cardinal.WithCustomMockRedis(nil)
	cardinal.WithPort("")
	cardinal.WithPrettyLog() //nolint:staticcheck // not applicable.
}

func TestWithPrettyLog_LogIsNotJSONFormatted(t *testing.T) {
	testutils.NewTestFixture(t, nil, cardinal.WithPrettyLog())

	// Create a pipe to capture the output
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Test log. WithPrettyLog overwrites the global logger, so this shouild be pretty printed too.
	log.Info().Msg("test")
	_ = w.Close()

	// Read the output and check that it is not JSON formatted (which is what a non-pretty logger would do)
	output, err := io.ReadAll(r)
	assert.NilError(t, err)
	assert.Assert(t, !isValidJSON(output))
}

// isValidJSON tests if a string is valid JSON.
func isValidJSON(bz []byte) bool {
	var js map[string]interface{}
	return json.Unmarshal(bz, &js) == nil
}
