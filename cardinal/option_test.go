package cardinal

import (
	"testing"
)

func TestOptionFunctionSignatures(_ *testing.T) {
	// This test is designed to keep API compatibility. If a compile error happens here it means a function signature to
	// public facing functions was changed.
	WithAdapter(nil)
	WithReceiptHistorySize(1)
	WithTickChannel(nil)
	WithTickDoneChannel(nil)
	WithStoreManager(nil)
	WithEventHub(nil)
	WithLoggingEventHub(nil)
	WithDisableSignatureVerification() //nolint:staticcheck //this test just looks for compile errors
}
