package p2p

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/PratikkJadhav/Gorrent/backend/client"
	"github.com/PratikkJadhav/Gorrent/backend/message"
	"github.com/PratikkJadhav/Gorrent/backend/peers"
	"github.com/PratikkJadhav/Gorrent/backend/piece"
)

const MaxBacklog = 5
const MaxBlockSize = 16384

type Torrent struct {
	Peers       []peers.Peer
	PeerID      [20]byte
	InfoHash    [20]byte
	PieceHashes [][20]byte
	PieceLength int
	Length      int
	Name        string

	PieceMgr *piece.Manager

	peerQueue chan *ManagedPeer
	done      chan struct{}
}

type pieceWork struct {
	index  int
	hash   [20]byte
	length int
}

type pieceResult struct {
	index int
	buf   []byte
}

type pieceProgress struct {
	index      int
	client     *client.Client
	buf        []byte
	downloaded int
	requested  int
	backlog    int

	pieceMgr *piece.Manager
}

func (state *pieceProgress) readMessage() error {
	msg, err := state.client.Read()
	if err != nil {
		return err
	}

	if msg == nil {
		return nil
	}

	switch msg.ID {
	case message.MsgUnchoke:
		state.client.Choked = false

	case message.MsgChoke:
		state.client.Choked = true

	case message.MsgHave:
		index, err := message.ParseHave(msg)
		if err != nil {
			return err
		}
		state.client.Bitfield.SetPiece(index)
		state.pieceMgr.RegisterHave(index)

	case message.MsgPiece:
		n, err := message.ParsePiece(state.index, state.buf, msg)
		if err != nil {
			return err
		}
		state.downloaded += n
		state.backlog--
	}

	return nil
}

func attemptDownloadedPiece(c *client.Client, pw *pieceWork, pm *piece.Manager) ([]byte, error) {
	state := pieceProgress{
		index:    pw.index,
		client:   c,
		buf:      make([]byte, pw.length),
		pieceMgr: pm,
	}

	c.Conn.SetDeadline(time.Now().Add(30 * time.Second))
	defer c.Conn.SetDeadline(time.Time{})

	for state.downloaded < pw.length {
		if !state.client.Choked {
			for state.backlog < MaxBacklog && state.requested < pw.length {
				blockSize := MaxBlockSize
				if pw.length-state.requested < blockSize {
					blockSize = pw.length - state.requested
				}

				if err := c.SendRequest(pw.index, state.requested, blockSize); err != nil {
					return nil, err
				}

				state.backlog++
				state.requested += blockSize
			}
		}

		if err := state.readMessage(); err != nil {
			return nil, err
		}
	}

	return state.buf, nil
}

func checkIntegrity(pw *pieceWork, buf []byte) error {
	hash := sha1.Sum(buf)
	if !bytes.Equal(hash[:], pw.hash[:]) {
		return fmt.Errorf("piece %d failed integrity check", pw.index)
	}
	return nil
}

func (t *Torrent) Download() ([]byte, error) {
	t.PieceMgr = piece.New(t.PieceHashes, t.PieceLength, t.Length)
	if len(t.Peers) == 0 {
		return nil, fmt.Errorf("no peers available from tracker")
	}
	log.Printf("Starting download for %s\n", t.Name)

	results := make(chan *pieceResult)
	t.peerQueue = make(chan *ManagedPeer, 64)
	t.done = make(chan struct{})

	for _, p := range t.Peers {
		t.peerQueue <- &ManagedPeer{
			Peer:  p,
			State: PeerNew,
		}
	}
	// close(peerQueue)

	const MaxWorkers = 5
	workerDone := make(chan struct{})

	for i := 0; i < MaxWorkers; i++ {
		go t.worker(results, workerDone)
	}

	buf := make([]byte, t.Length)
	donePieces := 0
	activeWorkers := MaxWorkers

	for donePieces < len(t.PieceHashes) {
		select {
		case res := <-results:
			begin := res.index * t.PieceLength
			end := begin + len(res.buf)
			copy(buf[begin:end], res.buf)

			donePieces++
			percent := float64(donePieces) / float64(len(t.PieceHashes)) * 100
			log.Printf("(%0.2f%%) Downloaded piece #%d (%d goroutines)\n",
				percent, res.index, runtime.NumGoroutine()-1)

		case <-workerDone:
			activeWorkers--
			log.Printf("Worker exited, remaining: %d\n", activeWorkers)
			if activeWorkers == 0 {
				return nil, fmt.Errorf("download failed: no active peers left")
			}
		}
	}

	close(t.done)
	return buf, nil
}

func (t *Torrent) downloadWithClient(
	c *client.Client,
	results chan *pieceResult,
) error {

	for {
		pw := t.PieceMgr.NextPiece(c.Bitfield.HasPiece)
		if pw == nil {
			return nil
		}

		buf, err := attemptDownloadedPiece(c, &pieceWork{
			index:  pw.Index,
			hash:   pw.Hash,
			length: pw.Length,
		},
			t.PieceMgr,
		)
		if err != nil {
			t.PieceMgr.MarkFailed(pw.Index)
			return err
		}

		if err := checkIntegrity(&pieceWork{
			index: pw.Index,
			hash:  pw.Hash,
		}, buf); err != nil {
			t.PieceMgr.MarkFailed(pw.Index)
			continue
		}

		t.PieceMgr.MarkDone(pw.Index)
		c.SendHave(pw.Index)
		results <- &pieceResult{pw.Index, buf}
	}
}

func (t *Torrent) AddPeers(newPeers []peers.Peer) {
	for _, p := range newPeers {
		mp := &ManagedPeer{
			Peer:  p,
			State: PeerNew,
		}
		t.peerQueue <- mp
	}
}
