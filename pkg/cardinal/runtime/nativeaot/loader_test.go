//go:build cgo && (linux || darwin)

package nativeaot_test

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	cardinalruntime "github.com/argus-labs/world-engine/pkg/cardinal/runtime"
	"github.com/argus-labs/world-engine/pkg/cardinal/runtime/nativeaot"
	pb "github.com/argus-labs/world-engine/pkg/cardinal/runtime/nativeaot/testdata/fixturepb"
	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

const (
	fixtureName    = "nativeaot-fixture"
	fixtureVersion = "1.2.3"
)

var (
	fixtureLibrary       string
	badABIFixtureLibrary string
)

func TestMain(m *testing.M) {
	tempDirectory, err := os.MkdirTemp("", "cardinal-nativeaot-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create fixture directory: %v\n", err)
		os.Exit(1)
	}
	fixtureLibrary, err = compileFixture(tempDirectory, "fixture", nil)
	if err == nil {
		badABIFixtureLibrary, err = compileFixture(tempDirectory, "fixture-bad-abi", []string{"-DFIXTURE_BAD_ABI=1"})
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "compile NativeAOT C fixture: %v\n", err)
		_ = os.RemoveAll(tempDirectory)
		os.Exit(1)
	}
	exitCode := m.Run()
	_ = os.RemoveAll(tempDirectory)
	os.Exit(exitCode)
}

func TestFixtureOutputOwnership(t *testing.T) {
	executable, err := compileFixture(t.TempDir(), "self-test", []string{"-DFIXTURE_SELF_TEST"})
	require.NoError(t, err)
	output, err := exec.Command(executable).CombinedOutput()
	require.NoError(t, err, "%s", output)
}

func TestRunnerLifecycle(t *testing.T) {
	runner := openFixture(t, nil)
	require.NoError(t, runner.Initialize(&pb.FixtureSnapshot{Value: 300}))
	input := &pb.FixtureInput{Value: -173}
	output := &pb.FixtureOutput{Value: 999}
	require.NoError(t, runner.Tick(42, 50_000_000, input, output))
	assert.Equal(t, int64(127), output.GetValue())
	snapshot, err := runner.Snapshot()
	require.NoError(t, err)
	assert.Equal(t, int64(127), snapshot.GetValue())
	input.Value = 1
	require.NoError(t, runner.Tick(43, 50_000_000, input, output))
	assert.Equal(t, int64(128), output.GetValue())
	assert.Equal(t, int64(127), snapshot.GetValue(), "snapshot must not borrow native memory")
	restored := openFixture(t, nil)
	require.NoError(t, restored.Restore(snapshot))
	restoredSnapshot, err := restored.Snapshot()
	require.NoError(t, err)
	assert.Equal(t, snapshot.GetValue(), restoredSnapshot.GetValue())
}

func TestRunnerVarintValues(t *testing.T) {
	runner := openFixture(t, nil)
	require.NoError(t, runner.Initialize(nil))
	output := new(pb.FixtureOutput)
	for _, value := range []int64{0, 1, 127, 128, 16383, 16384, math.MaxInt64, -1, math.MinInt64} {
		t.Run(strconv.FormatInt(value, 10), func(t *testing.T) {
			require.NoError(t, runner.Restore(&pb.FixtureSnapshot{}))
			require.NoError(t, runner.Tick(1, 2, &pb.FixtureInput{Value: value}, output))
			assert.Equal(t, value, output.GetValue())
			snapshot, err := runner.Snapshot()
			require.NoError(t, err)
			assert.Equal(t, value, snapshot.GetValue())
		})
	}
}

func TestRunnerNilAndEmptyMessages(t *testing.T) {
	runner := openFixture(t, nil)
	require.NoError(t, runner.Initialize(nil))
	input := &pb.FixtureInput{Value: 17}
	output := new(pb.FixtureOutput)
	require.ErrorIs(t, runner.Tick(1, 2, nil, output), cardinalruntime.ErrInvalidArgument)
	require.ErrorIs(t, runner.Tick(1, 2, input, nil), cardinalruntime.ErrInvalidArgument)
	require.ErrorIs(t, runner.Restore(nil), cardinalruntime.ErrInvalidArgument)
	snapshot, err := runner.Snapshot()
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	assert.Zero(t, snapshot.GetValue(), "nil arguments must not execute the tick")
	require.NoError(t, runner.Tick(1, 2, input, output))
	assert.Equal(t, int64(17), output.GetValue())
	require.NoError(t, runner.Restore(&pb.FixtureSnapshot{}))
	output.ProtoReflect().SetUnknown([]byte{16, 1})
	require.NoError(t, runner.Tick(1, 2, &pb.FixtureInput{}, output))
	assert.Zero(t, output.GetValue(), "decode must reset absent fields")
	assert.Empty(t, output.ProtoReflect().GetUnknown())
}

func TestRunnerContract(t *testing.T) {
	contract := openFixture(t, nil).Contract()
	assert.Equal(t, cardinalruntime.Contract{
		ABIVersion: 1, Name: fixtureName, Version: fixtureVersion,
		InputType:    "worldengine.cardinal.fixture.v1.FixtureInput",
		OutputType:   "worldengine.cardinal.fixture.v1.FixtureOutput",
		SnapshotType: "worldengine.cardinal.fixture.v1.FixtureSnapshot",
	}, contract)
}

func TestRunnerRejectsUseAfterClose(t *testing.T) {
	runner, err := nativeaot.Open[*pb.FixtureInput, *pb.FixtureOutput, *pb.FixtureSnapshot](
		fixtureLibrary, nil, fixtureName, fixtureVersion)
	require.NoError(t, err)
	require.NoError(t, runner.Close())
	require.PanicsWithValue(t, "runtime runner is closed", func() { _ = runner.Close() })
	require.PanicsWithValue(t, "runtime runner is closed", func() { _ = runner.Tick(1, 2, nil, nil) })
}

func TestOpenRejectsABIMismatch(t *testing.T) {
	runner, err := nativeaot.Open[*pb.FixtureInput, *pb.FixtureOutput, *pb.FixtureSnapshot](
		badABIFixtureLibrary, nil, fixtureName, fixtureVersion)
	assert.Nil(t, runner)
	require.ErrorIs(t, err, cardinalruntime.ErrABIMismatch)
	require.ErrorContains(t, err, "module=2 host=1")
}

func TestOpenRejectsContractBeforeCreate(t *testing.T) {
	for _, field := range []string{"name", "version", "input_type", "output_type", "snapshot_type"} {
		t.Run(field, func(t *testing.T) {
			var err error
			config := []byte("fail-create")
			switch field {
			case "name", "version":
				name, version := fixtureName, fixtureVersion
				if field == "name" {
					name = "other"
				} else {
					version = "9.9.9"
				}
				_, err = nativeaot.Open[*pb.FixtureInput, *pb.FixtureOutput, *pb.FixtureSnapshot](
					fixtureLibrary, config, name, version)
			case "input_type":
				_, err = nativeaot.Open[*pb.FixtureOutput, *pb.FixtureOutput, *pb.FixtureSnapshot](
					fixtureLibrary, config, fixtureName, fixtureVersion)
			case "output_type":
				_, err = nativeaot.Open[*pb.FixtureInput, *pb.FixtureInput, *pb.FixtureSnapshot](
					fixtureLibrary, config, fixtureName, fixtureVersion)
			case "snapshot_type":
				_, err = nativeaot.Open[*pb.FixtureInput, *pb.FixtureOutput, *pb.FixtureInput](
					fixtureLibrary, config, fixtureName, fixtureVersion)
			}
			require.ErrorIs(t, err, cardinalruntime.ErrContractMismatch)
			var mismatch *cardinalruntime.ContractMismatchError
			require.ErrorAs(t, err, &mismatch)
			assert.Equal(t, field, mismatch.Field)
			assert.NotContains(t, err.Error(), "fixture create failure")
		})
	}
}

func TestOpenRejectsUnterminatedContract(t *testing.T) {
	for _, field := range []string{"name", "version", "input_type", "output_type", "snapshot_type"} {
		t.Run(field, func(t *testing.T) {
			library, err := compileFixture(t.TempDir(), field, []string{"-DFIXTURE_UNTERMINATED_FIELD=" + field})
			require.NoError(t, err)
			_, err = nativeaot.Open[*pb.FixtureInput, *pb.FixtureOutput, *pb.FixtureSnapshot](
				library, nil, fixtureName, fixtureVersion)
			require.ErrorIs(t, err, cardinalruntime.ErrABIMismatch)
			require.ErrorContains(t, err, field+" is not NUL-terminated")
		})
	}
}

func TestRunnerTranslatesModuleErrors(t *testing.T) {
	_, err := nativeaot.Open[*pb.FixtureInput, *pb.FixtureOutput, *pb.FixtureSnapshot](
		fixtureLibrary, []byte("fail-create"), fixtureName, fixtureVersion)
	require.ErrorIs(t, err, cardinalruntime.ErrExecutionFailed)
	require.ErrorContains(t, err, "fixture create failure")
	runner := openFixture(t, nil)
	input, output := new(pb.FixtureInput), new(pb.FixtureOutput)
	require.ErrorIs(t, runner.Tick(1, 2, input, output), cardinalruntime.ErrInvalidState)
	require.NoError(t, runner.Initialize(nil))
	err = runner.Tick(math.MaxUint64, 2, input, output)
	require.ErrorIs(t, err, cardinalruntime.ErrExecutionFailed)
	require.ErrorContains(t, err, "fixture tick failure")
	input.ProtoReflect().SetUnknown([]byte{16, 1})
	require.ErrorIs(t, runner.Tick(1, 2, input, output), cardinalruntime.ErrInvalidArgument)
	badSnapshot := new(pb.FixtureSnapshot)
	badSnapshot.ProtoReflect().SetUnknown([]byte{16, 1})
	require.ErrorIs(t, runner.Restore(badSnapshot), cardinalruntime.ErrInvalidArgument)
}

func TestRunnerRejectsInvalidOutput(t *testing.T) {
	for index, message := range []string{"nil output", "length too large", "unmarshal output"} {
		t.Run(message, func(t *testing.T) {
			runner := openFixture(t, []byte{byte(index + 1)})
			require.NoError(t, runner.Initialize(nil))
			err := runner.Tick(1, 2, &pb.FixtureInput{Value: 5}, new(pb.FixtureOutput))
			require.ErrorIs(t, err, cardinalruntime.ErrExecutionFailed)
			require.ErrorContains(t, err, message)
			snapshot, err := runner.Snapshot()
			assert.Nil(t, snapshot)
			require.ErrorIs(t, err, cardinalruntime.ErrExecutionFailed)
			require.ErrorContains(t, err, message)
		})
	}
}

func TestRunnerSerializesCalls(t *testing.T) {
	runner := openFixture(t, nil)
	require.NoError(t, runner.Initialize(nil))
	const callCount = 16
	start := make(chan struct{})
	errorsChannel := make(chan error, callCount)
	results := make(chan int64, callCount)
	var waitGroup sync.WaitGroup
	waitGroup.Add(callCount)
	for range callCount {
		go func() {
			defer waitGroup.Done()
			<-start
			output := new(pb.FixtureOutput)
			errorsChannel <- runner.Tick(77, 3, &pb.FixtureInput{Value: 1}, output)
			results <- output.GetValue()
		}()
	}
	close(start)
	waitGroup.Wait()
	close(errorsChannel)
	close(results)
	for err := range errorsChannel {
		require.NoError(t, err)
	}
	seen := make(map[int64]bool)
	for value := range results {
		seen[value] = true
	}
	for value := int64(1); value <= callCount; value++ {
		assert.True(t, seen[value])
	}
}

func TestRunnerTickAllocations(t *testing.T) {
	runner := openFixture(t, nil)
	require.NoError(t, runner.Initialize(nil))
	input, output := &pb.FixtureInput{Value: 1}, new(pb.FixtureOutput)
	var tickErr error
	allocations := testing.AllocsPerRun(100, func() { tickErr = runner.Tick(1, 2, input, output) })
	require.NoError(t, tickErr)
	assert.Zero(t, allocations)
	assert.Equal(t, int64(101), output.GetValue())
}

func BenchmarkRunnerTick(b *testing.B) {
	runner := openFixture(b, nil)
	require.NoError(b, runner.Initialize(nil))
	input, output := &pb.FixtureInput{Value: 1}, new(pb.FixtureOutput)
	require.NoError(b, runner.Tick(1, 50_000_000, input, output))
	b.ReportAllocs()
	b.SetBytes(int64(proto.Size(input)))
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if err := runner.Tick(1, 50_000_000, input, output); err != nil {
			b.Fatal(err)
		}
		if output.GetValue() != int64(index)+2 {
			b.Fatalf("unexpected output %d", output.GetValue())
		}
	}
}

func openFixture(
	t testing.TB,
	config []byte,
) *nativeaot.Runner[*pb.FixtureInput, *pb.FixtureOutput, *pb.FixtureSnapshot] {
	t.Helper()
	runner, err := nativeaot.Open[*pb.FixtureInput, *pb.FixtureOutput, *pb.FixtureSnapshot](
		fixtureLibrary, config, fixtureName, fixtureVersion)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runner.Close()) })
	return runner
}

func compileFixture(outputDirectory, name string, extraArguments []string) (string, error) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	extension := ".so"
	linkArguments := []string{"-shared", "-fPIC"}
	if goruntime.GOOS == "darwin" {
		extension = ".dylib"
		linkArguments = []string{"-dynamiclib"}
	}
	if len(extraArguments) == 1 && extraArguments[0] == "-DFIXTURE_SELF_TEST" {
		extension = ""
		linkArguments = nil
	}
	output := filepath.Join(outputDirectory, name+extension)
	compiler := strings.Fields(os.Getenv("CC"))
	if len(compiler) == 0 {
		compiler = []string{"cc"}
	}
	arguments := append([]string{}, compiler[1:]...)
	arguments = append(arguments, "-std=c11", "-O2", "-Wall", "-Wextra", "-Werror")
	arguments = append(arguments, linkArguments...)
	arguments = append(arguments, extraArguments...)
	arguments = append(arguments, "-I"+filepath.Join(workingDirectory, "include"),
		filepath.Join(workingDirectory, "testdata", "fixture.c"), "-o", output)
	combinedOutput, err := exec.Command(compiler[0], arguments...).CombinedOutput()
	if err != nil {
		return "", eris.Wrapf(err, "compile fixture:\n%s", combinedOutput)
	}
	return output, nil
}
