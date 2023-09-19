package shutdown

import (
	"time"
)

type SignalObject struct {
	receiverChannel chan bool
	clientChannel   chan bool
}

// Send returns true when a signal is received false when it does not
func (s *SignalObject) Send(secondsToWait int) bool {
	s.receiverChannel <- true
	select {
	case <-s.clientChannel:
		return true
	case <-time.After(time.Duration(secondsToWait) * time.Second):
		return false
	}
}

// HandleReceive returns true when a signal is received false when it is not. It does not block
func (s *SignalObject) HandleReceive(secondsToWait int) bool {
	select {
	case <-s.receiverChannel:
		return true
	case <-time.After(time.Duration(secondsToWait) * time.Second):
		return false
	}
}

type ShutdownManager struct {
	ServerShutDownSignaler   SignalObject
	GameLoopShutDownSignaler SignalObject
}

func (s *ShutdownManager) ShutdownServer() bool {
	return s.ServerShutDownSignaler.Send(5)
}

func (s *ShutdownManager) ShutdownGameLoop() bool {
	return s.GameLoopShutDownSignaler.Send(5)
}

// HandleServerShutDown boolean return value indicates whether a shutdown signal was received
func (s *ShutdownManager) HandleServerShutDown(waitTime int, handler func() error) (bool, error) {
	if s.ServerShutDownSignaler.HandleReceive(waitTime) {
		err := handler()
		if err != nil {
			return true, err
		}
		s.ServerShutDownSignaler.receiverChannel <- true
		return true, err
	}
	return false, nil
}

// HandleGameLoopShutdown boolean return value indicates whether a shutdown signal was received
func (s *ShutdownManager) HandleGameLoopShutdown(waitTime int, handler func() error) (bool, error) {
	if s.GameLoopShutDownSignaler.HandleReceive(waitTime) {
		err := handler()
		if err != nil {
			return true, err
		}
		s.GameLoopShutDownSignaler.receiverChannel <- true
		return false, nil
	}
	return false, nil
}

func (s *ShutdownManager) Shutdown() bool {
	if s.ShutdownServer() {
		if s.ShutdownGameLoop() {
			return true
		}
	}
	return false
}
