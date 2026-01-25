package torrentfile

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/PratikkJadhav/Gorrent/backend/p2p"
	"github.com/PratikkJadhav/Gorrent/backend/peers"
	bencode "github.com/jackpal/bencode-go"
)

const port uint16 = 55555

type TorrentFile struct {
	Announce     string `bencode:"announce"`
	AnnounceList [][]string
	InfoHash     [20]byte
	PieceHashes  [][20]byte
	PieceLength  int
	Length       int
	Name         string
}

type bencodeInfo struct {
	Pieces      string `bencode:"pieces"`
	PieceLength int    `bencode:"piece length"`
	Length      int    `bencode:"length"`
	Name        string `bencode:"name"`
}

type bencodeTorrent struct {
	Announce     string      `bencode:"announce"`
	AnnounceList [][]string  `bencode:"announce-list"`
	Info         bencodeInfo `bencode:"info"`
}

func (t *TorrentFile) DownloadToFile(path string) error {
	var peerID [20]byte
	const prefix = "-GR0001-"
	copy(peerID[:], prefix)
	_, err := rand.Read(peerID[len(prefix):])
	if err != nil {
		return err
	}

	peerList, err := t.requestPeersFromTrackers(peerID, port)
	if err != nil {
		log.Printf("tracker announce failed: %v\n", err)
		peerList = nil
	}

	localPeer := peers.Peer{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 12460,
	}
	peerList = append(peerList, localPeer)

	log.Printf("Calculated InfoHash: %x", t.InfoHash)
	log.Printf("Gorrent InfoHash: %x", t.InfoHash)

	torrent := p2p.Torrent{
		Peers:       peerList,
		PeerID:      peerID,
		InfoHash:    t.InfoHash,
		PieceHashes: t.PieceHashes,
		PieceLength: t.PieceLength,
		Length:      t.Length,
		Name:        t.Name,
	}

	buf, err := torrent.Download()
	if err != nil {
		return err
	}

	outFile, err := os.Create(path)
	if err != nil {
		return err
	}

	defer outFile.Close()
	_, err = outFile.Write(buf)
	if err != nil {
		return err
	}

	return nil
}

func Open(path string) (TorrentFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return TorrentFile{}, err
	}

	defer file.Close()

	bencodetorrent := bencodeTorrent{}
	err = bencode.Unmarshal(file, &bencodetorrent)
	if err != nil {
		return TorrentFile{}, err
	}

	return bencodetorrent.toTorrentFile()
}

func (i *bencodeInfo) hash() ([20]byte, error) {
	var buf bytes.Buffer
	err := bencode.Marshal(&buf, *i)
	if err != nil {
		return [20]byte{}, err
	}

	h := sha1.Sum(buf.Bytes())
	return h, nil
}

func (i *bencodeInfo) splitPieceHashes() ([][20]byte, error) {
	hashLen := 20
	buf := []byte(i.Pieces)
	if len(buf)%hashLen != 0 {
		err := fmt.Errorf("Received curropted pieces of lenght %d", len(buf))
		return nil, err
	}

	numHashes := len(buf) / hashLen
	hashes := make([][20]byte, numHashes)

	for i := 0; i < numHashes; i++ {
		copy(hashes[i][:], buf[i*hashLen:(i+1)*hashLen])
	}

	return hashes, nil
}

func (bto *bencodeTorrent) toTorrentFile() (TorrentFile, error) {
	infoHash, err := bto.Info.hash()
	if err != nil {
		return TorrentFile{}, err
	}

	pieceHashes, err := bto.Info.splitPieceHashes()
	if err != nil {
		return TorrentFile{}, err
	}

	t := TorrentFile{
		Announce:     bto.Announce,
		AnnounceList: bto.AnnounceList,
		InfoHash:     infoHash,
		PieceHashes:  pieceHashes,
		PieceLength:  bto.Info.PieceLength,
		Length:       bto.Info.Length,
		Name:         bto.Info.Name,
	}

	return t, nil
}
