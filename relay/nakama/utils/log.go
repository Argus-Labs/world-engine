package utils

import (
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
	"google.golang.org/grpc/codes"
)

var (
	DebugEnabled bool
)

func LogErrorWithMessageAndCode(
	logger runtime.Logger,
	err error,
	code codes.Code,
	format string,
	v ...interface{},
) (string, error) {
	err = eris.Wrapf(err, format, v...)
	return LogError(logger, err, code)
}

func LogError(
	logger runtime.Logger,
	err error,
	code codes.Code,
) (string, error) {
	logger.Error(eris.ToString(err, true))
	return "", errToNakamaError(err, code)
}

func errToNakamaError(err error, code codes.Code) error {
	if err != nil {
		if DebugEnabled {
			return runtime.NewError(eris.ToString(err, true), int(code))
		}
		return runtime.NewError(err.Error(), int(code))
	}
	return nil
}
