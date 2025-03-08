package auth

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/golang-jwt/jwt"

	"pkg.world.dev/world-engine/assert"
)

const testJWTSecret = "JWTSecretKeyOnlyForTesting"

func TestValidateAndParseJWTHappyPath(t *testing.T) {
	claims := SupabaseClaims{
		StandardClaims: jwt.StandardClaims{
			Subject: "test-user-id",
		},
		UserMetaData: map[string]interface{}{},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	jwtString, err := token.SignedString([]byte(testJWTSecret))
	assert.Nil(t, err)

	hash := sha256.Sum256([]byte(jwtString))
	jwtHash := hex.EncodeToString(hash[:])

	_, err = validateAndParseJWT(t.Context(), jwtHash, jwtString, testJWTSecret)
	assert.Nil(t, err)
}

func TestValidateAndParseJWTWithWrongID(t *testing.T) {
	claims := SupabaseClaims{
		StandardClaims: jwt.StandardClaims{
			Subject: "test-user-id",
		},
		UserMetaData: map[string]interface{}{},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	jwtString, err := token.SignedString([]byte(testJWTSecret))
	assert.Nil(t, err)

	wrongHash := "invalidhashvalue"

	_, err = validateAndParseJWT(t.Context(), wrongHash, jwtString, testJWTSecret)
	assert.ErrorContains(t, err, ErrInvalidIDForJWT.Error())
}

func TestValidateAndParseJWTWithWrongSecret(t *testing.T) {
	claims := SupabaseClaims{
		StandardClaims: jwt.StandardClaims{
			Subject: "test-user-id",
		},
		UserMetaData: map[string]interface{}{},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	jwtString, err := token.SignedString([]byte("ThisIsNotTheRightSecret"))
	assert.Nil(t, err)

	hash := sha256.Sum256([]byte(jwtString))
	jwtHash := hex.EncodeToString(hash[:])

	_, err = validateAndParseJWT(t.Context(), jwtHash, jwtString, testJWTSecret)
	assert.ErrorContains(t, err, jwt.ErrSignatureInvalid.Error())
}

func TestValidateAndParseJWTWithWrongSigningMethod(t *testing.T) {
	claims := SupabaseClaims{
		StandardClaims: jwt.StandardClaims{
			Subject: "test-user-id",
		},
		UserMetaData: map[string]interface{}{},
	}

	_, privateKey, _ := ed25519.GenerateKey(nil)
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	jwtString, err := token.SignedString(privateKey)
	assert.Nil(t, err)

	hash := sha256.Sum256([]byte(jwtString))
	jwtHash := hex.EncodeToString(hash[:])

	_, err = validateAndParseJWT(t.Context(), jwtHash, jwtString, testJWTSecret)
	assert.ErrorContains(t, err, ErrInvalidJWTSigningMethod.Error())
}

func TestValidateAndParseJWTWithInvalidClaims(t *testing.T) {
	// Subject should be set
	claims := SupabaseClaims{
		StandardClaims: jwt.StandardClaims{},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	jwtString, err := token.SignedString([]byte(testJWTSecret))
	assert.Nil(t, err)

	hash := sha256.Sum256([]byte(jwtString))
	jwtHash := hex.EncodeToString(hash[:])

	_, err = validateAndParseJWT(t.Context(), jwtHash, jwtString, testJWTSecret)
	assert.ErrorContains(t, err, ErrInvalidJWTClaims.Error())
}
