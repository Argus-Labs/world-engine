# Protobuf NativeAOT counter fixture

This fixture uses generated input, output, and snapshot messages with one `int64`
field each. Tick adds the input value to the counter. The factory declares schema
names as constants. ABI version remains 1. Config is empty or an eight-byte
little-endian initial value; tick and snapshot payloads use protobuf.

## Reproduce

From the repository root, regenerate the checked-in Go and C# sources:

```bash
buf generate pkg/cardinal/runtime/nativeaot/testdata/fixture.proto \
  --template pkg/cardinal/runtime/nativeaot/testdata/buf.gen.yaml
```

Publish and run the existing integration tests on macOS ARM64:

```bash
dotnet publish sdk/csharp/testapps/CounterModule/CounterModule.csproj \
  --configuration Release --runtime osx-arm64 -p:TrimmerSingleWarn=false
CARDINAL_NATIVEAOT_TEST_LIBRARY="$PWD/sdk/csharp/testapps/CounterModule/bin/Release/net8.0/osx-arm64/publish/WorldEngine.Runtime.CounterFixture.dylib" \
  go test ./pkg/cardinal/runtime/nativeaot -run '^TestNativeAOT' -count=1 -v
```

`PublishAot=true` is set in the project. Do not pass it as a global command-line
property: that also applies it to the referenced Abstractions project's
`netstandard2.1` target, which fails with NETSDK1207.

## Result

Verified with .NET SDK 10.0.302, target framework `net8.0`, RID `osx-arm64`, and
Google.Protobuf 3.31.1:

- NativeAOT publish succeeded.
- The original step-0 descriptor probe passed. The current fixture uses schema
  constants and protobuf serialization instead of that probe.
- No trim warnings were emitted.
- AOT analysis emitted this warning:

  ```text
  IL3050: Google.Protobuf.Collections.RepeatedField`1.TryGetArrayAsSpanPinnedUnsafe(FieldCodec`1<T>,Span`1<Byte>&,GCHandle&): Using member 'System.Runtime.InteropServices.Marshal.SizeOf(Type)' which has 'RequiresDynamicCodeAttribute' can break functionality when AOT compiling. Marshalling code for the object might not be available. Use the SizeOf<T> overload instead.
  ```

- The native linker also warned that `-ld_classic` is deprecated.
- The Linux/x64 Docker and packaged-project builds were not verified in this check.

The warning comes from protobuf internals and still occurs with schema constants.
