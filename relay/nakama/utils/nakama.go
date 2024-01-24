package utils

import (
	"context"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
)

// getUserID gets the Nakama UserID from the given context.
func GetUserID(ctx context.Context) (string, error) {
	userID, ok := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
	if !ok {
		return "", eris.New("unable to get user id from context")
	}
	return userID, nil
}
