// Package commandtest registers wire codecs for the shared testutils command fixtures so that command
// tests across pkg/cardinal can enqueue them. The fixtures live in pkg/testutils (which can't import
// the internal command package), so their codecs live here instead. Test binaries blank-import this
// package to get the registration via init, mirroring how generated code registers real commands.
package commandtest

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/rotisserie/eris"
)

//nolint:gochecknoinits // registers test-fixture codecs on package load, mirroring generated code
func init() {
	command.RegisterCodec("simple_command", simpleCodec{})
	command.RegisterCodec("command_a", aCodec{})
	command.RegisterCodec("command_b", bCodec{})
	command.RegisterCodec("command_c", cCodec{})
}

// Codecs hand-roll the wire format with encoding/binary (no msgpack — commands never use msgpack).
// Unmarshal returns a fresh value, so every method is a value receiver.

type simpleCodec struct{}

func (simpleCodec) Marshal(p command.Payload) ([]byte, error) {
	c, ok := p.(testutils.SimpleCommand)
	if !ok {
		return nil, eris.Errorf("expected SimpleCommand, got %T", p)
	}
	var b bytes.Buffer
	err := binary.Write(&b, binary.LittleEndian, int64(c.Value))
	return b.Bytes(), err
}

func (simpleCodec) Unmarshal(data []byte) (command.Payload, error) {
	var v int64
	if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &v); err != nil {
		return nil, err
	}
	return testutils.SimpleCommand{Value: int(v)}, nil
}

type aCodec struct{}

func (aCodec) Marshal(p command.Payload) ([]byte, error) {
	c, ok := p.(testutils.CommandA)
	if !ok {
		return nil, eris.Errorf("expected CommandA, got %T", p)
	}
	var b bytes.Buffer
	err := binary.Write(&b, binary.LittleEndian, c)
	return b.Bytes(), err
}

func (aCodec) Unmarshal(data []byte) (command.Payload, error) {
	var c testutils.CommandA
	if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &c); err != nil {
		return nil, err
	}
	return c, nil
}

type bCodec struct{}

func (bCodec) Marshal(p command.Payload) ([]byte, error) {
	c, ok := p.(testutils.CommandB)
	if !ok {
		return nil, eris.Errorf("expected CommandB, got %T", p)
	}
	var b bytes.Buffer
	if err := binary.Write(&b, binary.LittleEndian, c.ID); err != nil {
		return nil, err
	}
	if err := binary.Write(&b, binary.LittleEndian, c.Enabled); err != nil {
		return nil, err
	}
	// Label is variable-length, so it goes last and consumes the rest on decode.
	if err := binary.Write(&b, binary.LittleEndian, []byte(c.Label)); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (bCodec) Unmarshal(data []byte) (command.Payload, error) {
	b := bytes.NewReader(data)
	var c testutils.CommandB
	if err := binary.Read(b, binary.LittleEndian, &c.ID); err != nil {
		return nil, err
	}
	if err := binary.Read(b, binary.LittleEndian, &c.Enabled); err != nil {
		return nil, err
	}
	label := make([]byte, b.Len())
	if _, err := io.ReadFull(b, label); err != nil {
		return nil, err
	}
	c.Label = string(label)
	return c, nil
}

type cCodec struct{}

func (cCodec) Marshal(p command.Payload) ([]byte, error) {
	c, ok := p.(testutils.CommandC)
	if !ok {
		return nil, eris.Errorf("expected CommandC, got %T", p)
	}
	var b bytes.Buffer
	err := binary.Write(&b, binary.LittleEndian, c)
	return b.Bytes(), err
}

func (cCodec) Unmarshal(data []byte) (command.Payload, error) {
	var c testutils.CommandC
	if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &c); err != nil {
		return nil, err
	}
	return c, nil
}
