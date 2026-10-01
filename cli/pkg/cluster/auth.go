package cluster

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"

	"connectrpc.com/connect"

	"github.com/argus-labs/world-engine/cli/proto/gen/go/cardinal/operator/v1/operatorv1connect"
)

// TokenSource supplies a bearer token for a remote operator. Taken as an
// interface so this package stays free of any credential store.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// operatorClient builds a client for cfg.OperatorEndpoint, attaching Argus auth
// when one is configured. A k3d operator runs unauthenticated, so a local
// endpoint is left alone even if a token source is present.
func (c *Client) operatorClient() operatorv1connect.OperatorServiceClient {
	if c.cfg.TokenSource == nil || isLocalEndpoint(c.cfg.OperatorEndpoint) {
		return operatorv1connect.NewOperatorServiceClient(http.DefaultClient, c.cfg.OperatorEndpoint)
	}
	return operatorv1connect.NewOperatorServiceClient(
		http.DefaultClient,
		c.cfg.OperatorEndpoint,
		connect.WithInterceptors(bearerInterceptor{tokens: c.cfg.TokenSource}),
	)
}

// isLocalEndpoint reports whether the operator is the unauthenticated one a
// developer runs in k3d. Hostnames are matched by name because they are not IPs
// that ParseIP can classify.
func isLocalEndpoint(endpoint string) bool {
	if !strings.Contains(endpoint, "://") {
		// url.Parse would read a bare "localhost:8090" as scheme "localhost".
		endpoint = "http://" + endpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "localhost" || host == "host.docker.internal" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type bearerInterceptor struct {
	tokens TokenSource
}

func (i bearerInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if err := i.setAuth(ctx, req.Header()); err != nil {
			return nil, err
		}
		return next(ctx, req)
	}
}

// WrapStreamingClient covers StreamPodLogs and the other server streams.
func (i bearerInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		if err := i.setAuth(ctx, conn.RequestHeader()); err != nil {
			return failedConn{StreamingClientConn: conn, err: err}
		}
		return conn
	}
}

// WrapStreamingHandler is server-side; this interceptor is client-only.
func (i bearerInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

func (i bearerInterceptor) setAuth(ctx context.Context, header http.Header) error {
	token, err := i.tokens.Token(ctx)
	if err != nil {
		return connect.NewError(connect.CodeUnauthenticated, err)
	}
	header.Set("Authorization", "Bearer "+token)
	return nil
}

// failedConn reports a token error on the first use of the stream, since
// WrapStreamingClient cannot return one directly. CloseRequest and
// CloseResponse are overridden too: connect's CallServerStream cleanup calls
// them after a failed Send, and the embedded live conn would otherwise flush an
// unauthenticated POST (duplexHTTPCall.CloseWrite -> makeRequest) before the
// error is returned. Both must be overridden — CloseRequest alone would still
// leave CloseResponse blocking forever on responseReady, which makeRequest
// would never close.
type failedConn struct {
	connect.StreamingClientConn

	err error
}

func (c failedConn) Send(_ any) error     { return c.err }
func (c failedConn) Receive(_ any) error  { return c.err }
func (c failedConn) CloseRequest() error  { return c.err }
func (c failedConn) CloseResponse() error { return c.err }
