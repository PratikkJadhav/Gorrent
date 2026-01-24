package p2p

import (
	"time"

	"github.com/PratikkJadhav/Gorrent/backend/client"
)

func (t *Torrent) worker(
	results chan *pieceResult,
	workerDone chan struct{},
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
			if mp == nil {
				continue
			}

			err := t.tryPeer(mp, results)
			if err == ErrNoWork {
				return
			}
			if err != nil {
				mp.Failures++
				if mp.ShouldRetry() {
					time.Sleep(mp.BackoffDuration())
					t.peerQueue <- mp
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
