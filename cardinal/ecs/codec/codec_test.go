package codec_test

import (
	"testing"

	"pkg.world.dev/world-engine/cardinal/ecs/codec"
)

// Define a dummy struct for benchmarking.
type ExampleStruct struct {
	ID   int
	Name string
}

// Benchmark the Decode function.
func BenchmarkDecode(b *testing.B) {
	// Prepare a byte slice to decode
	data := []byte(`{"ID": 1, "Name": "Example"}`)

	b.ResetTimer() // Reset the timer

	// Run the benchmark
	for i := 0; i < b.N; i++ {
		_, err := codec.Decode[ExampleStruct](data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark the Encode function.
func BenchmarkEncode(b *testing.B) {
	// Prepare an example struct to encode
	example := ExampleStruct{
		ID:   1,
		Name: "Example",
	}

	// Reset the timer
	b.ResetTimer()

	// Run the benchmark
	for i := 0; i < b.N; i++ {
		_, err := codec.Encode(example)
		if err != nil {
			b.Fatal(err)
		}
	}
}
