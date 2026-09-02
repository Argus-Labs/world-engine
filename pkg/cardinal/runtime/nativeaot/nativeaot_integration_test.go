//go:build cgo && (linux || darwin)

package nativeaot_test

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	cardinalruntime "github.com/argus-labs/world-engine/pkg/cardinal/runtime"
	"github.com/argus-labs/world-engine/pkg/cardinal/runtime/nativeaot"
	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const integrationContractVersion = "1.0.0"

func TestNativeAOTModule(t *testing.T) {
	libraryPath := os.Getenv("CARDINAL_NATIVEAOT_TEST_LIBRARY")
	if libraryPath == "" {
		t.Skip("CARDINAL_NATIVEAOT_TEST_LIBRARY is not set")
	}
	moduleName := integrationContractName()

	runner, err := nativeaot.Open(libraryPath, nil, moduleName, integrationContractVersion)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, runner.Close())
	})

	contract := runner.Contract()
	assert.Equal(t, moduleName, contract.Name)
	assert.Equal(t, integrationContractVersion, contract.Version)
	assert.Equal(t, cardinalruntime.ABIVersion, contract.ABIVersion)

	require.NoError(t, runner.Initialize(cardinalruntime.InitRequest{}))

	increment := make([]byte, 8)
	binary.LittleEndian.PutUint64(increment, 5)
	request := cardinalruntime.TickRequest{
		Tick:         42,
		FixedDeltaNS: 50_000_000,
		Input:        increment,
	}

	undersized := []byte{0xA5, 0xA5, 0xA5, 0xA5}
	written, err := runner.Tick(request, undersized)
	require.ErrorIs(t, err, cardinalruntime.ErrBufferTooSmall)
	assert.Zero(t, written)
	assert.Equal(t, []byte{0xA5, 0xA5, 0xA5, 0xA5}, undersized)

	output := make([]byte, 8)
	written, err = runner.Tick(request, output)
	require.NoError(t, err)
	require.Equal(t, len(output), written)
	assert.Equal(t, uint64(5), binary.LittleEndian.Uint64(output))

	snapshot := make([]byte, 8)
	written, err = runner.Snapshot(snapshot)
	require.NoError(t, err)
	require.Equal(t, len(snapshot), written)

	restored, err := nativeaot.Open(libraryPath, nil, moduleName, integrationContractVersion)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, restored.Close())
	})
	require.NoError(t, restored.Restore(snapshot))

	written, err = restored.Query(
		cardinalruntime.QueryRequest{Kind: 1},
		output,
	)
	require.NoError(t, err)
	require.Equal(t, len(output), written)
	assert.Equal(t, uint64(5), binary.LittleEndian.Uint64(output))
}

func TestNativeAOTConcurrentOpenErrors(t *testing.T) {
	libraryPath := os.Getenv("CARDINAL_NATIVEAOT_TEST_LIBRARY")
	if libraryPath == "" {
		t.Skip("CARDINAL_NATIVEAOT_TEST_LIBRARY is not set")
	}
	if os.Getenv("CARDINAL_NATIVEAOT_TEST_NAME") != "" {
		t.Skip("error fixture contract is specific to CounterModule")
	}

	const callCount = 28
	moduleName := integrationContractName()
	start := make(chan struct{})
	errorsChannel := make(chan error, callCount)
	var waitGroup sync.WaitGroup
	waitGroup.Add(callCount)

	for index := range callCount {
		configLength := index%7 + 1
		go func() {
			defer waitGroup.Done()
			<-start

			_, err := nativeaot.Open(
				libraryPath,
				make([]byte, configLength),
				moduleName,
				integrationContractVersion,
			)
			if err == nil {
				errorsChannel <- eris.Errorf("config length %d unexpectedly succeeded", configLength)
				return
			}
			expected := fmt.Sprintf("received %d", configLength)
			if !strings.Contains(err.Error(), expected) {
				errorsChannel <- eris.Wrapf(
					err,
					"config length %d received another call's diagnostic",
					configLength,
				)
			}
		}()
	}

	close(start)
	waitGroup.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		require.NoError(t, err)
	}
}

func BenchmarkNativeAOTTick(b *testing.B) {
	libraryPath := os.Getenv("CARDINAL_NATIVEAOT_TEST_LIBRARY")
	if libraryPath == "" {
		b.Skip("CARDINAL_NATIVEAOT_TEST_LIBRARY is not set")
	}

	runner, err := nativeaot.Open(
		libraryPath,
		nil,
		integrationContractName(),
		integrationContractVersion,
	)
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		if closeErr := runner.Close(); closeErr != nil {
			b.Error(closeErr)
		}
	}()
	if err = runner.Initialize(cardinalruntime.InitRequest{}); err != nil {
		b.Fatal(err)
	}

	input := make([]byte, 8)
	binary.LittleEndian.PutUint64(input, 1)
	output := make([]byte, 8)
	request := cardinalruntime.TickRequest{
		FixedDeltaNS: 50_000_000,
		Input:        input,
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		request.Tick = uint64(index)
		written, tickErr := runner.Tick(request, output)
		if tickErr != nil {
			b.Fatal(tickErr)
		}
		if written != len(output) {
			b.Fatalf("tick wrote %d bytes, want %d", written, len(output))
		}
	}
}

func integrationContractName() string {
	name := os.Getenv("CARDINAL_NATIVEAOT_TEST_NAME")
	if name == "" {
		return "counter-fixture"
	}
	return name
}
