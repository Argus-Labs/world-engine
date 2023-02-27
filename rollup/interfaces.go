package rollup

// Application describes the functions that can be performed on the rollup application.
type Application interface {
	// Start starts the rollup application in process.
	Start() error
}
