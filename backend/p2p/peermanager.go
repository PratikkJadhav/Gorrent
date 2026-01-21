package p2p

import (
	"time"

	"github.com/PratikkJadhav/Gorrent/backend/peers"
)

type PeerState int

const (
	PeerNew PeerState = iota
	PeerActive
	PeerBad
)

type ManagedPeer struct {
	Peer     peers.Peer
	State    PeerState
	Failures int
}

func (mp *ManagedPeer) ShouldRetry() bool {
	return mp.Failures < 3
}

func (mp *ManagedPeer) BackoffDuration() time.Duration {
	if mp.Failures <= 0 {
		return 0
	}

	return time.Second * time.Duration(mp.Failures)
}
