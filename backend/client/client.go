package client

import (
	"bytes"
	"fmt"
	"net"
	"time"

	"github.com/PratikkJadhav/Gorrent/backend/bitfield"
	"github.com/PratikkJadhav/Gorrent/backend/handshake"
	"github.com/PratikkJadhav/Gorrent/backend/message"
	"github.com/PratikkJadhav/Gorrent/backend/peers"
)

type Client struct {
	Conn     net.Conn
	Choked   bool
	Bitfield bitfield.Bitfield
	peer     peers.Peer
	infoHash [20]byte
	peerID   [20]byte
}

func completeHandshake(conn net.Conn, infohash, peerID [20]byte) (*handshake.Handshake, error) {
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	defer conn.SetDeadline(time.Time{})

	req := handshake.New(infohash, peerID)
	if _, err := conn.Write(req.Serialize()); err != nil {
		return nil, err
	}

	res, err := handshake.Read(conn)
	if err != nil {
		return nil, err
	}

	if !bytes.Equal(res.InfoHash[:], infohash[:]) {
		return nil, fmt.Errorf("expected infohash %x but got %x", infohash, res.InfoHash)
	}

	return res, nil
}

// recvInitialMessages reads messages until:
// - bitfield is received OR
// - a non-bitfield message arrives
func recvInitialMessages(conn net.Conn, numPieces int) (bitfield.Bitfield, error) {
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer conn.SetReadDeadline(time.Time{})

	bf := bitfield.New(numPieces)

	for {
		msg, err := message.Read(conn)
		if err != nil {
			return nil, err
		}

		if msg == nil {
			continue // keep-alive
		}

		switch msg.ID {
		case message.MsgBitField:
			return msg.Payload, nil

		case message.MsgHave:
			index, err := message.ParseHave(msg)
			if err != nil {
				return nil, err
			}
			bf.SetPiece(index)

		default:
			// First non-bitfield message → assume no bitfield
			return bf, nil
		}
	}
}

func New(peer peers.Peer, peerID, infoHash [20]byte, numPieces int) (*Client, error) {
	conn, err := net.DialTimeout("tcp", peer.String(), 3*time.Second)
	if err != nil {
		return nil, err
	}

	if _, err := completeHandshake(conn, infoHash, peerID); err != nil {
		conn.Close()
		return nil, err
	}

	bf, err := recvInitialMessages(conn, numPieces)
	if err != nil {
		conn.Close()
		return nil, err
	}

	c := &Client{
		Conn:     conn,
		Choked:   true,
		Bitfield: bf,
		peer:     peer,
		infoHash: infoHash,
		peerID:   peerID,
	}

	// 🔑 CRITICAL: tell peer we are interested
	if hasAnyPiece(bf) {
		if err := c.SendInterested(); err != nil {
			conn.Close()
			return nil, err
		}
	}

	return c, nil
}

func hasAnyPiece(bf bitfield.Bitfield) bool {
	for _, b := range bf {
		if b != 0 {
			return true
		}
	}
	return false
}

func (c *Client) Read() (*message.Message, error) {
	return message.Read(c.Conn)
}

func (c *Client) SendRequest(index, begin, length int) error {
	req := message.FormatRequestMessage(index, begin, length)
	_, err := c.Conn.Write(req.Serialize())
	return err
}

func (c *Client) SendInterested() error {
	msg := message.Message{ID: message.MsgInterested}
	_, err := c.Conn.Write(msg.Serialize())
	return err
}

func (c *Client) SendNotInterested() error {
	msg := message.Message{ID: message.MsgNotInterested}
	_, err := c.Conn.Write(msg.Serialize())
	return err
}

func (c *Client) SendHave(index int) error {
	msg := message.FormatHaveMessage(index)
	_, err := c.Conn.Write(msg.Serialize())
	return err
}
