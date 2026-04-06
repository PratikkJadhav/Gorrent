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

	outFile, err := os.Create(path)
	if err != nil {
		return err
	}
	defer outFile.Close()

	if err := outFile.Truncate(int64(t.Length)); err != nil {
		return err
	}

	torrent := p2p.Torrent{
		Peers:       peerList,
		PeerID:      peerID,
		InfoHash:    t.InfoHash,
		PieceHashes: t.PieceHashes,
		PieceLength: t.PieceLength,
		Length:      t.Length,
		Name:        t.Name,
	}

	return torrent.Download(outFile)
}

func Open(path string) (TorrentFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return TorrentFile{}, err
	}
	defer file.Close()

	// 1. Decode generic map to calculate the exact InfoHash without nil interfaces
	data, err := bencode.Decode(file)
	if err != nil {
		return TorrentFile{}, err
	}

	dict, ok := data.(map[string]interface{})
	if !ok {
		return TorrentFile{}, fmt.Errorf("invalid torrent file format")
	}

	infoDict, ok := dict["info"]
	if !ok {
		return TorrentFile{}, fmt.Errorf("missing info dictionary")
	}

	var infoBuf bytes.Buffer
	if err := bencode.Marshal(&infoBuf, infoDict); err != nil {
		return TorrentFile{}, err
	}
	infoHash := sha1.Sum(infoBuf.Bytes())

	// 2. Reset file pointer and unmarshal into our typed struct for ease of use
	file.Seek(0, 0)
	bto := bencodeTorrent{}
	if err := bencode.Unmarshal(file, &bto); err != nil {
		return TorrentFile{}, err
	}

	// 3. Extract the rest
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

func (i *bencodeInfo) splitPieceHashes() ([][20]byte, error) {
	hashLen := 20
	buf := []byte(i.Pieces)
	if len(buf)%hashLen != 0 {
		err := fmt.Errorf("Received corrupted pieces of length %d", len(buf))
		return nil, err
	}

	numHashes := len(buf) / hashLen
	hashes := make([][20]byte, numHashes)

	for i := 0; i < numHashes; i++ {
		copy(hashes[i][:], buf[i*hashLen:(i+1)*hashLen])
	}

	return hashes, nil
}
