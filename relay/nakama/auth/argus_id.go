package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/golang-jwt/jwt"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
	"go.opentelemetry.io/otel"
	otelcode "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"

	"pkg.world.dev/world-engine/relay/nakama/utils"
)

var (
	ErrInvalidIDForJWT         = errors.New("ID doesn't match JWT hash")
	ErrInvalidJWTSigningMethod = errors.New("invalid JWT signing algorithm")
	ErrInvalidJWT              = errors.New("invalid JWT Token")
	ErrInvalidJWTClaims        = errors.New("invalid JWT claims format")
)

// The body (claims) of the JWT is decided by Supabase's GoTrue, so we'll have to update this code
// if it were to change in the future.
// src: https://github.com/supabase/auth/blob/master/internal/api/token.go#L24
type SupabaseClaims struct {
	// Supabase uses jwt.RegisteredClaims from golang-jwt/jwt/v5, but it's still based on the same
	// RFC (https://datatracker.ietf.org/doc/html/rfc7519) as this version's jwt.StandardClaims.
	jwt.StandardClaims
	UserMetaData map[string]interface{} `json:"user_metadata"`
}

func validateAndParseJWT(
	ctx context.Context,
	jwtHash string,
	jwtString string,
	jwtSecret string,
) (*SupabaseClaims, error) {
	_, span := otel.Tracer("nakama.auth").Start(ctx, "Validating and Parsing JWT")
	defer span.End()

	span.AddEvent("Comparing given JWT hash with actual JWT hash")
	computedHash := sha256.Sum256([]byte(jwtString))
	computedHashString := hex.EncodeToString(computedHash[:])
	if computedHashString != jwtHash {
		span.RecordError(ErrInvalidIDForJWT)
		span.SetStatus(otelcode.Error, "Given JWT hash does not match computed hash")
		return nil, ErrInvalidIDForJWT
	}

	span.AddEvent("Parsing JWT Claims")
	token, err := jwt.ParseWithClaims(
		jwtString,
		&SupabaseClaims{},
		func(token *jwt.Token) (interface{}, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, eris.Wrapf(ErrInvalidJWTSigningMethod, "Unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(jwtSecret), nil
		})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcode.Error, "Failed to parse JWT")
		return nil, eris.Wrap(err, "Failed to parse JWT")
	}
	if !token.Valid {
		span.RecordError(ErrInvalidJWT)
		span.SetStatus(otelcode.Error, "Invalid JWT token")
		return nil, ErrInvalidJWT
	}

	claims, ok := token.Claims.(*SupabaseClaims)
	// Make sure claims has a subject (the user ID set by Supabase)
	if !ok || claims.Subject == "" {
		span.RecordError(ErrInvalidJWTClaims)
		span.SetStatus(otelcode.Error, "Invalid JWT claims")
		return nil, ErrInvalidJWTClaims
	}

	span.SetStatus(otelcode.Ok, "Successfully parsed and validated JWT")
	return claims, nil
}

// The AuthenticateCustom request should be called with the sha256 hash of the JWT as the ID and
// include the JWT as a request variable. This is done because the JWTs are often longer than the
// max length of AuthenticateCustom IDs (128 characters).
func authWithArgusID(
	ctx context.Context,
	logger runtime.Logger,
	_ runtime.NakamaModule,
	in *api.AuthenticateCustomRequest,
) (*api.AuthenticateCustomRequest, error) {
	span := trace.SpanFromContext(ctx)

	jwtHash := in.GetAccount().GetId()
	jwt := in.GetAccount().GetVars()["jwt"]
	claims, err := validateAndParseJWT(ctx, jwtHash, jwt, GlobalJWTSecret)
	if err != nil {
		_, err = utils.LogErrorWithMessageAndCode(logger, err, codes.InvalidArgument, "Failed to validate and parse JWT")
		return nil, err
	}

	if err = claims.Valid(); err != nil {
		_, err = utils.LogErrorWithMessageAndCode(logger, err, codes.InvalidArgument, "JWT is not valid")
		return nil, err
	}

	span.AddEvent("Setting user ID and metadata to request")
	// Set account with user id (claims.Subject) and metadata. Nakama account metadata only supports
	// string values, so we should also limit the values of user metadata to be only strings.
	in.Account.Id = claims.Subject
	for key, value := range claims.UserMetaData {
		if strValue, ok := value.(string); ok {
			in.Account.Vars[key] = strValue
		} else {
			logger.Warn("Found non-string value in user metadata: %v", value)
		}
	}

	return in, nil
}

func linkWithArgusID(
	ctx context.Context,
	logger runtime.Logger,
	_ runtime.NakamaModule,
	in *api.AccountCustom,
) (*api.AccountCustom, error) {
	span := trace.SpanFromContext(ctx)

	jwtHash := in.GetId()
	jwt := in.GetVars()["jwt"]
	claims, err := validateAndParseJWT(ctx, jwtHash, jwt, GlobalJWTSecret)
	if err != nil {
		_, err = utils.LogErrorWithMessageAndCode(logger, err, codes.InvalidArgument, "Failed to parse and verify JWT")
		return nil, err
	}

	if err = claims.Valid(); err != nil {
		_, err = utils.LogErrorWithMessageAndCode(logger, err, codes.InvalidArgument, "JWT is not valid")
		return nil, err
	}

	span.AddEvent("Setting user ID and metadata to request")
	in.Id = claims.Subject
	for key, value := range claims.UserMetaData {
		if strValue, ok := value.(string); ok {
			in.Vars[key] = strValue
		} else {
			logger.Warn("Found non-string value in user metadata for key: %s", key)
		}
	}

	return in, nil
}
