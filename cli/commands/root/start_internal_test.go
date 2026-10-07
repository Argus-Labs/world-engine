package root

import (
	"context"
	"testing"

	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/require"

	errorspkg "github.com/argus-labs/world-engine/cli/internal/errors"
)

// errRealCluster and errRealBuild model non-silent failures that must always be
// surfaced to the user (and Sentry). They are stable sentinels so eris.Is can
// match them through rankStartErrors's eris.Wrap.
var (
	errRealCluster = eris.New("k3d cluster create failed: exit status 1")
	errRealBuild   = eris.New("docker build failed: Dockerfile parse error")
)

// errSilentCluster and errSilentBuild model Ctrl+C cancellations: phasebox.Box.Run
// converts a [context.Canceled]-wrapped op error into a SilentError
// (service.go:96-98), so IsSilent must report true and ShouldPrint false — the
// guarantee rankStartErrors relies on to keep a real failure from being
// swallowed by an interrupt.
var (
	errSilentCluster = errorspkg.NewSilent(
		eris.Wrap(eris.Wrapf(context.Canceled, "no Ready pod backing service"), "cluster start"),
	)
	errSilentBuild = errorspkg.NewSilent(context.Canceled)
)

// TestRankStartErrors is the contract for the runK8s error-precedence fix: a real
// failure must always win over a silent (Ctrl+C) cancellation regardless of
// which concurrent side it came from, mirroring reload.go's IsSilent-aware
// precedence. The table spans every (clusterErr, buildErr) combination the
// concurrent cluster-bring-up / shard-build race can produce.
func TestRankStartErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		clusterErr  error
		buildErr    error
		wantCluster bool // result is the exact clusterErr object (returned directly)
		wantBuild   bool // result wraps the buildErr (eris.Wrap "initial shard build")
		wantNil     bool // both phases succeeded; runK8s proceeds to deploy
		wantSilent  bool // result IsSilent == true (cancellation, not printed)
	}{
		{
			name:       "both succeed: proceed to deploy",
			clusterErr: nil,
			buildErr:   nil,
			wantNil:    true,
		},
		{
			name:        "real cluster fail, build ok: real cluster wins (original intent)",
			clusterErr:  errRealCluster,
			buildErr:    nil,
			wantCluster: true,
			wantSilent:  false,
		},
		{
			name:        "silent cluster cancel, build ok: cancellation surfaced but stays silent",
			clusterErr:  errSilentCluster,
			buildErr:    nil,
			wantCluster: true,
			wantSilent:  true,
		},
		{
			name:       "cluster ok, real build fail: real build wins and prints",
			clusterErr: nil,
			buildErr:   errRealBuild,
			wantBuild:  true,
			wantSilent: false,
		},
		{
			name:       "cluster ok, silent build cancel: build surfaced but stays silent",
			clusterErr: nil,
			buildErr:   errSilentBuild,
			wantBuild:  true,
			wantSilent: true,
		},
		{
			name:        "both real fail: real cluster wins (preserves original behavior)",
			clusterErr:  errRealCluster,
			buildErr:    errRealBuild,
			wantCluster: true,
			wantSilent:  false,
		},
		{
			name:        "real cluster fail + silent build cancel: real cluster wins over silent build",
			clusterErr:  errRealCluster,
			buildErr:    errSilentBuild,
			wantCluster: true,
			wantSilent:  false,
		},
		{
			name:       "BUG FIX: silent cluster cancel + real build fail: real build wins, not the cancel",
			clusterErr: errSilentCluster,
			buildErr:   errRealBuild,
			wantBuild:  true,
			wantSilent: false,
		},
		{
			name:       "both silent cancel: build surfaced but stays silent",
			clusterErr: errSilentCluster,
			buildErr:   errSilentBuild,
			wantBuild:  true,
			wantSilent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := rankStartErrors(tt.clusterErr, tt.buildErr)

			if tt.wantNil {
				require.Nil(t, got, "expected no error when both phases succeed")
				return
			}
			require.NotNil(t, got, "expected an error to be surfaced")

			// Identity: which phase's error did rankStartErrors surface?
			// cluster-wins returns the exact clusterErr; build-wins returns a fresh eris.Wrap.
			//nolint:errorlint // identity is the precise contract: cluster-wins returns the unwrapped clusterErr, so == (not errors.Is) is required
			require.Equal(t, tt.wantCluster, got == tt.clusterErr,
				"cluster-wins must return the exact cluster error object")
			if tt.buildErr != nil {
				require.Equal(t, tt.wantBuild, eris.Is(got, tt.buildErr),
					"build-wins must surface the build error through its wrap")
			}

			// Printability drives both stdout gates: main.go:141 (printer.Errorln +
			// sentry.CaptureException) and start.go:47 (the `world purge` hint). A real
			// failure must pass ShouldPrint; a silent cancellation must fail it.
			require.Equal(t, tt.wantSilent, errorspkg.IsSilent(got),
				"IsSilent must match the expected cancellation-vs-real classification")
			require.Equal(t, !tt.wantSilent, errorspkg.ShouldPrint(got),
				"ShouldPrint is the inverse of IsSilent")

			// build-wins carries the outermost wrap label that formatError prints to
			// stdout (main.go:49-51 returns the last ErrChain entry, i.e. "initial
			// shard build"), so the failure looks like a failure rather than a bare
			// cancellation.
			if tt.wantBuild {
				require.Contains(t, got.Error(), "initial shard build",
					"build-wins result must carry the 'initial shard build' wrap label")
			}
		})
	}
}

// TestSilentCancellationPropagatesThroughErisChain proves the IsSilent guards in
// rankStartErrors actually fire on the error shape the cluster-cancellation path
// produces. It reconstructs that shape from the traced path:
//
//	wait.PollUntilContextTimeout -> [context.Canceled]
//	readiness.waitForReadyPod:    eris.Wrapf(context.Canceled, "no Ready pod ...")
//	cluster.StartPlatform:        eris.Wrap(err, "wait for NATS to be ready")
//	phasebox.Box.Run:             errorspkg.NewSilent(opErr)  [eris.Is(opErr, context.Canceled)]
//	startCluster:                 eris.Wrap(err, "cluster start")
//
// and asserts each link rankStartErrors's precedence depends on. Without this,
// TestRankStartErrors only proves "if IsSilent returns X, rankStartErrors does
// Y"; this test proves IsSilent *does* return X for the real cancellation shape,
// using the real eris + errorspkg utilities composed the way the path does.
func TestSilentCancellationPropagatesThroughErisChain(t *testing.T) {
	t.Parallel()

	// The long-tail cancel target: wait.PollUntilContextCancel documents that it
	// returns ctx.Err() on a cancelled parent.
	cancelled := context.Canceled

	// waitForReadyPod wraps the poll's context.Canceled with the "no Ready pod"
	// detail; StartPlatform wraps that with "wait for NATS".
	startPlatformErr := eris.Wrap(
		eris.Wrapf(cancelled, "no Ready pod backing service %s/%s after %s", "nats-ns", "nats-svc", "5m"),
		"wait for NATS to be ready",
	)
	// Box.Run's silence trigger: eris.Is must reach context.Canceled through the
	// double wrap, otherwise Box.Run would return the error verbatim (non-silent)
	// and rankStartErrors's IsSilent guard would never see a silent cluster error.
	require.True(t, eris.Is(startPlatformErr, context.Canceled),
		"eris.Is must reach context.Canceled through the double wrap — Box.Run's silence trigger")

	// Box.Run converts this to a SilentError; startCluster wraps once more for the
	// "cluster start" label surfaced in dashboard scrollback.
	clusterErr := eris.Wrap(errorspkg.NewSilent(startPlatformErr), "cluster start")
	require.True(t, errorspkg.IsSilent(clusterErr),
		"cluster cancellation must remain silent after startCluster's eris.Wrap — "+
			"rankStartErrors's IsSilent guard and both ShouldPrint gates depend on it")
	require.False(t, errorspkg.ShouldPrint(clusterErr),
		"silent cancellation must fail ShouldPrint so main.go:141 skips "+
			"printer.Errorln + sentry.CaptureException and start.go:47 skips the purge hint")

	// The bug's exact ordering: a real build failure landed in buildDone first,
	// then a Ctrl+C silently cancelled the still-blocked cluster. The old block
	// returned clusterErr unconditionally on != nil, swallowing the build; the
	// fix must surface the build failure instead.
	realBuild := eris.New("docker build failed: Dockerfile parse error")
	surfaced := rankStartErrors(clusterErr, realBuild)

	//nolint:errorlint // identity is intentional: assert the fix does NOT return the silent clusterErr object
	require.False(t, surfaced == clusterErr,
		"fix: rankStartErrors must NOT return the silent cluster error when the build truly failed")
	require.True(t, eris.Is(surfaced, realBuild),
		"fix: rankStartErrors must surface the real build error through its 'initial shard build' wrap")
	require.False(t, errorspkg.IsSilent(surfaced),
		"fix: the surfaced build error must not be silent")
	require.True(t, errorspkg.ShouldPrint(surfaced),
		"fix: the surfaced build error must pass ShouldPrint and reach "+
			"printer.Errorln('✗ initial shard build') + sentry.CaptureException")
	require.Contains(t, surfaced.Error(), "initial shard build",
		"fix: the outermost wrap label that formatError prints to stdout (main.go:49-51) must be present")

	// Regression guard: the must-not-regress behaviors the original block got
	// right — a real cluster error still beats a silent build, and a silent
	// cluster with a successful build still surfaces the cancellation.
	//nolint:errorlint // identity is intentional: real cluster must be returned as the exact same object
	require.True(t, rankStartErrors(errRealCluster, errSilentBuild) == errRealCluster,
		"regression: a real cluster error must still beat a silent build")
	//nolint:errorlint // identity is intentional: silent cluster must be returned as the exact same object
	require.True(t, rankStartErrors(clusterErr, nil) == clusterErr,
		"regression: a silent cluster cancel with a successful build must still surface the cancel")
}
