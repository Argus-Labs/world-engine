package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

func generateRandomBytes(n int) ([]byte, error) {
	bytes := make([]byte, n)
	_, err := rand.Read(bytes)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func generateBetaKeys(n int) ([]string, error) {
	keys := make([]string, 0, n)
	for i := 0; i < n; i++ {
		randomBytes, err := generateRandomBytes(16) // 16 bytes for the desired format
		if err != nil {
			return nil, err
		}
		// Format the random bytes as a hyphen-separated string
		key := hex.EncodeToString(randomBytes)
		key = strings.ToUpper(key)
		key = fmt.Sprintf("%s-%s-%s-%s", key[0:4], key[4:8], key[8:12], key[12:16])
		keys = append(keys, key)
	}

	return keys, nil
}
