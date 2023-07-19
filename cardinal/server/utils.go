package server

import (
	"bytes"
	"encoding/json"
	"github.com/rs/zerolog/log"
	"net/http"
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
	log.Info().Msgf("unauthorized: %v", err)
}

func writeError(w http.ResponseWriter, msg string, err error) {
	w.WriteHeader(500)
	log.Info().Msgf("%s: %v", msg, err)
}

// writeResult takes in a json body string and writes it to the response writer.
func writeResult(w http.ResponseWriter, body json.RawMessage) {
	// Allow cors header
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Json content header
	w.Header().Set("Content-Type", "application/json")

	err := json.NewEncoder(w).Encode(body)
	if err != nil {
		writeError(w, "unable to encode body", err)
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
