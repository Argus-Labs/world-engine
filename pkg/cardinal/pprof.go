package cardinal

import (
	"context"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/argus-labs/world-engine/pkg/telemetry"
)

// pprofModule serves Go runtime profiling endpoints via net/http/pprof.
type pprofModule struct {
	server *http.Server
	tel    telemetry.Telemetry
}

// newPprofModule creates a pprof server with handlers mounted on a private mux to avoid
// http.DefaultServeMux contamination.
func newPprofModule(tel telemetry.Telemetry) *pprofModule {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	return &pprofModule{
		tel: tel,
		server: &http.Server{
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
		},
	}
}

// Init starts the pprof HTTP server on the given address.
func (p *pprofModule) Init(addr string) {
	if p == nil {
		return
	}

	logger := p.tel.GetLogger("pprof")

	p.server.Addr = addr
	logger.Info().Str("addr", addr).Msg("pprof server initialized")

	go func() {
		// Surface bind failures and unexpected shutdown errors. ErrServerClosed
		// is the normal exit path from Shutdown(); anything else is unexpected
		// (port collision, OS-revoked socket) and the operator's Profile RPC
		// will start failing with connection-refused — the log line is what
		// lets you correlate that to a misconfiguration.
		if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Warn().Err(err).Msg("pprof server stopped unexpectedly")
		}
	}()
}

// Shutdown gracefully shuts down the pprof server.
func (p *pprofModule) Shutdown(ctx context.Context) error {
	if p == nil || p.server == nil {
		return nil
	}
	return p.server.Shutdown(ctx)
}
