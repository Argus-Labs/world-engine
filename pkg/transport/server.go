package transport

import (
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
)

// clientService implements CardinalService: commands from clients (command.go), and event streams and
// publishing (event.go).
type clientService struct {
	address      *micro.ServiceAddress
	replyTimeout time.Duration // Zero means SendCommandWithReply waits without a cap
	dispatch     Handler
	log          zerolog.Logger
	subscribers  map[string]*streamSubscriber
	replyWaiters map[string]map[string][]chan *iscv1.Event // event name -> player ID -> waiters
	mu           sync.RWMutex
	stopped      chan struct{} // Closed by stop; ends open event streams and reply waits
	stopOnce     sync.Once
}

var _ cardinalv1connect.CardinalServiceHandler = (*clientService)(nil)

func newClientService(
	address *micro.ServiceAddress, replyTimeout time.Duration, dispatch Handler, log zerolog.Logger,
) *clientService {
	return &clientService{
		address:      address,
		replyTimeout: replyTimeout,
		dispatch:     dispatch,
		log:          log,
		subscribers:  make(map[string]*streamSubscriber),
		replyWaiters: make(map[string]map[string][]chan *iscv1.Event),
		stopped:      make(chan struct{}),
	}
}

// stop ends every open event stream and reply wait, and any started later, as unavailable.
func (s *clientService) stop() {
	s.stopOnce.Do(func() { close(s.stopped) })
}

// dispatchError reports a dispatch error to the client: unavailable during shutdown, invalid otherwise.
func dispatchError(err error) error {
	code := connect.CodeInvalidArgument
	if eris.Is(err, errStopping) {
		code = connect.CodeUnavailable
	}
	return connect.NewError(code, eris.Wrap(err, "failed to enqueue command"))
}
