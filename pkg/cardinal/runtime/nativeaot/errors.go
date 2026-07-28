package nativeaot

import "errors"

// ErrUnavailable reports that the NativeAOT loader is unavailable in this
// build. It requires cgo and a platform with dlopen.
var ErrUnavailable = errors.New("nativeaot runtime loader unavailable")
