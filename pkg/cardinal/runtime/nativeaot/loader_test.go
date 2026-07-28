//go:build cgo && (linux || darwin)

package nativeaot_test

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"testing"

	cardinalruntime "github.com/argus-labs/world-engine/pkg/cardinal/runtime"
	"github.com/argus-labs/world-engine/pkg/cardinal/runtime/nativeaot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	fixtureLibrary              string
	badABIFixtureLibrary        string
	badSizeFixtureLibrary       string
	badReservedFixtureLibraries [4]string
)

func TestMain(m *testing.M) {
	tempDirectory, err := os.MkdirTemp("", "cardinal-nativeaot-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create fixture directory: %v\n", err)
		os.Exit(1)
	}

	fixtureLibrary, err = compileFixture(tempDirectory, "fixture", nil)
	if err == nil {
		badABIFixtureLibrary, err = compileFixture(
			tempDirectory,
			"fixture-bad-abi",
			[]string{"-DFIXTURE_BAD_ABI=1"},
		)
	}
	if err == nil {
		badSizeFixtureLibrary, err = compileFixture(
			tempDirectory,
			"fixture-bad-size",
			[]string{"-DFIXTURE_BAD_SIZE=1"},
		)
	}
	for index := range badReservedFixtureLibraries {
		if err != nil {
			break
		}
		badReservedFixtureLibraries[index], err = compileFixture(
			tempDirectory,
			fmt.Sprintf("fixture-bad-reserved-%d", index),
			[]string{fmt.Sprintf("-DFIXTURE_BAD_RESERVED_INDEX=%d", index)},
		)
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

func TestRunnerLifecycle(t *testing.T) {
	runner := openFixture(t, []byte("config"))

	require.NoError(t, runner.Initialize(cardinalruntime.InitRequest{
		Snapshot: []byte("fresh"),
	}))

	input := []byte{0xCA, 0xFE, 0x01}
	output := make([]byte, 64)
	written, err := runner.Tick(cardinalruntime.TickRequest{
		Tick:         42,
		FixedDeltaNS: 50_000_000,
		Input:        input,
	}, output)
	require.NoError(t, err)
	require.Equal(t, 16+len(input), written)
	assert.Equal(t, uint64(42), binary.LittleEndian.Uint64(output[0:8]))
	assert.Equal(t, uint64(50_000_000), binary.LittleEndian.Uint64(output[8:16]))
	assert.Equal(t, input, output[16:written])

	queryInput := []byte("query")
	written, err = runner.Query(cardinalruntime.QueryRequest{
		Kind:  7,
		Input: queryInput,
	}, output)
	require.NoError(t, err)
	require.Equal(t, 4+len(queryInput), written)
	assert.Equal(t, uint32(7), binary.LittleEndian.Uint32(output[0:4]))
	assert.Equal(t, queryInput, output[4:written])

	snapshot := make([]byte, 16)
	written, err = runner.Snapshot(snapshot)
	require.NoError(t, err)
	require.Equal(t, len(snapshot), written)

	restored := openFixture(t, nil)
	require.NoError(t, restored.Restore(snapshot))
	restoredSnapshot := make([]byte, len(snapshot))
	written, err = restored.Snapshot(restoredSnapshot)
	require.NoError(t, err)
	assert.Equal(t, snapshot, restoredSnapshot[:written])

	require.NoError(t, runner.Close())
	require.NoError(t, runner.Close())
	_, err = runner.Tick(cardinalruntime.TickRequest{}, output)
	require.ErrorIs(t, err, cardinalruntime.ErrClosed)
}

func TestRunnerContract(t *testing.T) {
	runner := openFixture(t, nil)
	contract := runner.Contract()

	assert.Equal(t, cardinalruntime.ABIVersion, contract.ABIVersion)
	assert.Equal(t, "nativeaot-fixture", contract.Name)
	assert.Equal(t, "1.2.3", contract.Version)
	assert.True(t, contract.Supports(
		cardinalruntime.CapabilityInitialize|
			cardinalruntime.CapabilityTick|
			cardinalruntime.CapabilityQuery|
			cardinalruntime.CapabilitySnapshot|
			cardinalruntime.CapabilityRestore,
	))
	for index, value := range contract.SchemaHash {
		assert.Equal(t, byte(index), value)
	}
}

func TestOpenRejectsABIMismatch(t *testing.T) {
	runner, err := nativeaot.Open(badABIFixtureLibrary, nil)

	assert.Nil(t, runner)
	require.ErrorIs(t, err, cardinalruntime.ErrABIMismatch)
	require.ErrorContains(t, err, "module=2 host=1")
}

func TestOpenRejectsExtendedV1Contract(t *testing.T) {
	runner, err := nativeaot.Open(badSizeFixtureLibrary, nil)

	assert.Nil(t, runner)
	require.ErrorIs(t, err, cardinalruntime.ErrABIMismatch)
	require.ErrorContains(t, err, "contract size=184 expected=176")
}

func TestOpenRejectsNonzeroReservedContractWordsBeforeCreate(t *testing.T) {
	for index, library := range badReservedFixtureLibraries {
		t.Run(fmt.Sprintf("word_%d", index), func(t *testing.T) {
			runner, err := nativeaot.Open(library, []byte("fail-create"))

			assert.Nil(t, runner)
			require.ErrorIs(t, err, cardinalruntime.ErrABIMismatch)
			require.EqualError(
				t,
				err,
				fmt.Sprintf(
					"get contract: runtime ABI mismatch: reserved[%d]=1 expected=0",
					index,
				),
			)
		})
	}
}

func TestOpenValidatedRejectsContractBeforeCreate(t *testing.T) {
	requirement := fixtureContractRequirement()
	requirement.Version = "9.9.9"

	runner, err := nativeaot.OpenValidated(
		fixtureLibrary,
		[]byte("fail-create"),
		requirement,
	)

	assert.Nil(t, runner)
	require.ErrorIs(t, err, cardinalruntime.ErrContractMismatch)
	require.ErrorContains(t, err, `module version "1.2.3", want "9.9.9"`)
	assert.NotContains(t, err.Error(), "fixture create failure")
}

func TestOpenValidatedAcceptsMatchingContract(t *testing.T) {
	runner, err := nativeaot.OpenValidated(
		fixtureLibrary,
		nil,
		fixtureContractRequirement(),
	)

	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, runner.Close())
	})
	assert.Equal(t, "nativeaot-fixture", runner.Contract().Name)
}

func TestRunnerTranslatesModuleErrors(t *testing.T) {
	runner, err := nativeaot.Open(fixtureLibrary, []byte("fail-create"))
	assert.Nil(t, runner)
	require.ErrorIs(t, err, cardinalruntime.ErrExecutionFailed)
	require.ErrorContains(t, err, "fixture create failure")

	runner = openFixture(t, nil)
	_, err = runner.Tick(cardinalruntime.TickRequest{}, make([]byte, 32))
	require.ErrorIs(t, err, cardinalruntime.ErrInvalidState)
	require.ErrorContains(t, err, "fixture is not initialized")

	require.NoError(t, runner.Initialize(cardinalruntime.InitRequest{}))
	_, err = runner.Query(
		cardinalruntime.QueryRequest{Kind: ^uint32(0)},
		make([]byte, 32),
	)
	require.ErrorIs(t, err, cardinalruntime.ErrExecutionFailed)
	require.ErrorContains(t, err, "fixture query failure")

	err = runner.Restore([]byte("bad"))
	require.ErrorIs(t, err, cardinalruntime.ErrInvalidArgument)
	require.ErrorContains(t, err, "fixture snapshot must be 16 bytes")
}

func TestRunnerReportsRequiredBufferWithoutWriting(t *testing.T) {
	runner := openFixture(t, nil)
	require.NoError(t, runner.Initialize(cardinalruntime.InitRequest{}))

	output := []byte{0xA5, 0xA5, 0xA5, 0xA5}
	written, err := runner.Tick(cardinalruntime.TickRequest{
		Tick:         1,
		FixedDeltaNS: 2,
		Input:        []byte{3, 4, 5},
	}, output)

	assert.Zero(t, written)
	require.ErrorIs(t, err, cardinalruntime.ErrBufferTooSmall)
	var sizeError *cardinalruntime.BufferSizeError
	require.True(t, errors.As(err, &sizeError))
	assert.Equal(t, 19, sizeError.Required)
	assert.Equal(t, len(output), sizeError.Provided)
	assert.Equal(t, []byte{0xA5, 0xA5, 0xA5, 0xA5}, output)

	written, err = runner.Snapshot(nil)
	assert.Zero(t, written)
	require.ErrorIs(t, err, cardinalruntime.ErrBufferTooSmall)
	require.True(t, errors.As(err, &sizeError))
	assert.Equal(t, 16, sizeError.Required)
	assert.Zero(t, sizeError.Provided)
}

func TestRunnerSerializesCalls(t *testing.T) {
	runner := openFixture(t, nil)
	require.NoError(t, runner.Initialize(cardinalruntime.InitRequest{}))

	const callCount = 16
	start := make(chan struct{})
	errors := make(chan error, callCount)
	var waitGroup sync.WaitGroup
	waitGroup.Add(callCount)
	for range callCount {
		go func() {
			defer waitGroup.Done()
			<-start
			_, err := runner.Query(
				cardinalruntime.QueryRequest{Kind: 77},
				make([]byte, 4),
			)
			errors <- err
		}()
	}

	close(start)
	waitGroup.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
}

func BenchmarkRunnerTick(b *testing.B) {
	runner, err := nativeaot.Open(fixtureLibrary, nil)
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

	request := cardinalruntime.TickRequest{
		FixedDeltaNS: 50_000_000,
		Input:        []byte{1, 2, 3, 4},
	}
	output := make([]byte, 20)

	b.ReportAllocs()
	b.SetBytes(int64(len(request.Input)))
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

func openFixture(t testing.TB, config []byte) *nativeaot.Runner {
	t.Helper()

	runner, err := nativeaot.Open(fixtureLibrary, config)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, runner.Close())
	})
	return runner
}

func fixtureContractRequirement() cardinalruntime.ContractRequirement {
	var schemaHash [32]byte
	for index := range schemaHash {
		schemaHash[index] = byte(index)
	}
	return cardinalruntime.ContractRequirement{
		Name:       "nativeaot-fixture",
		Version:    "1.2.3",
		SchemaHash: schemaHash,
		Capabilities: cardinalruntime.CapabilityInitialize |
			cardinalruntime.CapabilityTick |
			cardinalruntime.CapabilityQuery |
			cardinalruntime.CapabilitySnapshot |
			cardinalruntime.CapabilityRestore,
	}
}

func compileFixture(
	outputDirectory string,
	name string,
	extraArguments []string,
) (string, error) {
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
	output := filepath.Join(outputDirectory, name+extension)

	compiler := strings.Fields(os.Getenv("CC"))
	if len(compiler) == 0 {
		compiler = []string{"cc"}
	}
	arguments := append([]string{}, compiler[1:]...)
	arguments = append(arguments, "-std=c11", "-O2", "-Wall", "-Wextra")
	arguments = append(arguments, linkArguments...)
	arguments = append(arguments, extraArguments...)
	arguments = append(
		arguments,
		"-I"+filepath.Join(workingDirectory, "include"),
		filepath.Join(workingDirectory, "testdata", "fixture.c"),
		"-o",
		output,
	)

	command := exec.Command(compiler[0], arguments...)
	combinedOutput, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w\n%s", err, combinedOutput)
	}
	return output, nil
}
