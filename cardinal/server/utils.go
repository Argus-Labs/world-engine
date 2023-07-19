package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
)

// fixes a path to contain a leading slash.
// if the path already contains a leading slash, it is simply returned as is.
func conformPath(p string) string {
	if p[0] != '/' {
		p = "/" + p
	}
	return p
}

func writeUnauthorized(w http.ResponseWriter, err error) {
	w.WriteHeader(401)
	fmt.Fprintf(w, "unauthorized: %v", err)
}

func writeError(w http.ResponseWriter, msg string, err error) {
	w.WriteHeader(500)
	fmt.Fprintf(w, "%s: %v", msg, err)
}

func writeResult(w http.ResponseWriter, v any) {
	// Allow cors
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if reflect.TypeOf(v).String() == "[]uint8" {
		// Handle JSON bytes
		_, err := w.Write(v.([]byte))
		if err != nil {
			writeError(w, "can't write", err)
			return
		}
	} else {
		// Handle anything else
		o, err := json.Marshal(v)
		if err != nil {
			writeError(w, "can't marshal", err)
			return
		}
		_, err = w.Write(o)
		if err != nil {
			writeError(w, "can't write", err)
			return
		}
	}
}

func decode[T any](buf []byte) (T, error) {
	dec := json.NewDecoder(bytes.NewBuffer(buf))
	dec.DisallowUnknownFields()
	var val T
	if err := dec.Decode(&val); err != nil {
		return val, err
	}
	return val, nil
}
