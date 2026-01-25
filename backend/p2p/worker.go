package p2p

import (
	"log"
	"time"

	"github.com/PratikkJadhav/Gorrent/backend/client"
)

func (t *Torrent) worker(
	results chan *pieceResult,
	workerDone chan struct{},
	peerDropped chan struct{},
) {
	defer func() {
		workerDone <- struct{}{}
	}()

	for {
		select {
		case mp, ok := <-t.peerQueue:
			if !ok {
				return
			}

			err := t.tryPeer(mp, results)
			if err != nil {
				mp.Failures++
				// Log the error but keep it short
				// log.Printf("Peer %s failed: %s", mp.Peer.IP, err)

				if mp.ShouldRetry() {
					time.Sleep(mp.BackoffDuration())
					t.peerQueue <- mp
				} else {
					log.Printf("Dropping peer %s after 3 failures", mp.Peer.IP)
					peerDropped <- struct{}{}
				}
				continue
			}

			if mp.State == PeerActive {
				go func(mp *ManagedPeer) {
					time.Sleep(2 * time.Second)
					select {
					case t.peerQueue <- mp:
					case <-t.done:
					}
				}(mp)
			}

		case <-t.done:
			return
		}
	}
}

func (t *Torrent) tryPeer(
	mp *ManagedPeer,
	results chan *pieceResult,
) error {
	c, err := client.New(mp.Peer, t.PeerID, t.InfoHash, len(t.PieceHashes))

	if err != nil {
		return err
	}
	defer c.Conn.Close()

	t.PieceMgr.RegisterBitfield(c.Bitfield.HasPiece)
	mp.State = PeerActive
	return t.downloadWithClient(c, results)
}
