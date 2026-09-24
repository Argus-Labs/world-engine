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
	pb "github.com/argus-labs/world-engine/pkg/cardinal/runtime/nativeaot/testdata/fixturepb"
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

	config := make([]byte, 8)
	binary.LittleEndian.PutUint64(config, 300)
	runner, err := nativeaot.Open[*pb.FixtureInput, *pb.FixtureOutput, *pb.FixtureSnapshot](
		libraryPath, config, moduleName, integrationContractVersion)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, runner.Close())
	})

	contract := runner.Contract()
	assert.Equal(t, moduleName, contract.Name)
	assert.Equal(t, integrationContractVersion, contract.Version)
	assert.Equal(t, cardinalruntime.ABIVersion, contract.ABIVersion)

	require.NoError(t, runner.Initialize(nil))
	input := &pb.FixtureInput{Value: -173}
	output := new(pb.FixtureOutput)
	require.NoError(t, runner.Tick(42, 50_000_000, input, output))
	assert.Equal(t, int64(127), output.GetValue())
	snapshot, err := runner.Snapshot()
	require.NoError(t, err)
	input.Value = 1
	require.NoError(t, runner.Tick(43, 50_000_000, input, output))
	assert.Equal(t, int64(128), output.GetValue())
	assert.Equal(t, int64(127), snapshot.GetValue())
	require.NoError(t, runner.Restore(new(pb.FixtureSnapshot)))
	input.Value = 0
	require.NoError(t, runner.Tick(44, 50_000_000, input, output))
	assert.Zero(t, output.GetValue())

	restored, err := nativeaot.Open[*pb.FixtureInput, *pb.FixtureOutput, *pb.FixtureSnapshot](
		libraryPath, nil, moduleName, integrationContractVersion)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, restored.Close())
	})
	require.NoError(t, restored.Restore(snapshot))

	restoredSnapshot, err := restored.Snapshot()
	require.NoError(t, err)
	assert.Equal(t, int64(127), restoredSnapshot.GetValue())
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

			_, err := nativeaot.Open[*pb.FixtureInput, *pb.FixtureOutput, *pb.FixtureSnapshot](
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

	runner, err := nativeaot.Open[*pb.FixtureInput, *pb.FixtureOutput, *pb.FixtureSnapshot](
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
	if err = runner.Initialize(nil); err != nil {
		b.Fatal(err)
	}

	input := &pb.FixtureInput{Value: 1}
	output := new(pb.FixtureOutput)
	require.NoError(b, runner.Tick(1, 50_000_000, input, output))

	b.ReportAllocs()
	b.SetBytes(2)
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		tickErr := runner.Tick(uint64(index), 50_000_000, input, output)
		if tickErr != nil {
			b.Fatal(tickErr)
		}
		if output.GetValue() != int64(index)+2 {
			b.Fatalf("unexpected output %d", output.GetValue())
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
