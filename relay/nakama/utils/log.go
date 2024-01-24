package utils

import (
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
	nakamaerrors "pkg.world.dev/world-engine/relay/nakama/errors"
)

var (
	DebugEnabled bool
)

func LogDebugWithMessageAndCode(
	logger runtime.Logger,
	err error,
	code int,
	format string,
	v ...interface{},
) (string, error) {
	err = eris.Wrapf(err, format, v...)
	return LogDebug(logger, err, code)
}

func LogErrorWithMessageAndCode(
	logger runtime.Logger,
	err error,
	code int,
	format string,
	v ...interface{},
) (string, error) {
	err = eris.Wrapf(err, format, v...)
	return LogError(logger, err, code)
}

func LogErrorMessageFailedPrecondition(
	logger runtime.Logger,
	err error,
	format string,
	v ...interface{}) (string, error) {
	err = eris.Wrapf(err, format, v...)
	return LogErrorFailedPrecondition(logger, err)
}

func LogErrorFailedPrecondition(
	logger runtime.Logger,
	err error,
) (string, error) {
	return LogError(logger, err, nakamaerrors.FailedPrecondition)
}

func LogDebug(
	logger runtime.Logger,
	err error,
	code int,
) (string, error) {
	logger.Debug(eris.ToString(err, true))
	return "", errToNakamaError(err, code)
}

func LogError(
	logger runtime.Logger,
	err error,
	code int,
) (string, error) {
	logger.Error(eris.ToString(err, true))
	return "", errToNakamaError(err, code)
}

func errToNakamaError(err error, code int) error {
	if err != nil {
		if DebugEnabled {
			return runtime.NewError(eris.ToString(err, true), code)
		}
		return runtime.NewError(err.Error(), code)
	}
	return nil
}
