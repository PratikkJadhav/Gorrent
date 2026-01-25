package torrentfile

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PratikkJadhav/Gorrent/backend/peers"
	bencode "github.com/jackpal/bencode-go"
)

type bencodeTrackerResp struct {
	Interval      int    `bencode:"interval"`
	Peers         string `bencode:"peers"`
	FailureReason string `bencode:"failure reason"`
}

func percentEncode(b []byte) string {
	var sb strings.Builder
	sb.Grow(len(b) * 3)

	for _, c := range b {
		sb.WriteByte('%')
		hex := strconv.FormatInt(int64(c), 16)
		if len(hex) == 1 {
			sb.WriteByte('0')
		}
		sb.WriteString(strings.ToLower(hex))
	}
	return sb.String()
}

func (t *TorrentFile) buildTrackerURL(peerID [20]byte, port uint16) (string, error) {
	base, err := url.Parse(t.Announce)
	if err != nil {
		return "", err
	}

	query := fmt.Sprintf(
		"info_hash=%s&peer_id=%s&port=%d&uploaded=0&downloaded=0&left=%d&compact=1",
		percentEncode(t.InfoHash[:]),
		percentEncode(peerID[:]),
		port,
		t.Length,
	)

	base.RawQuery = query
	return base.String(), nil
}

func (t *TorrentFile) requestPeers(trackerURL string, peerID [20]byte, port uint16) ([]peers.Peer, error) {
	trackerURL, err := t.buildTrackerURL(peerID, port)
	if err != nil {
		return nil, err
	}

	if strings.HasPrefix(trackerURL, "udp:") {
		return nil, fmt.Errorf("udp protocol not supported")
	}
	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	req, err := http.NewRequest("GET", trackerURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "qBittorrent/4.6.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tracker returned HTTP %d", resp.StatusCode)
	}

	trackerResp := bencodeTrackerResp{}
	if err := bencode.Unmarshal(resp.Body, &trackerResp); err != nil {
		return nil, err
	}

	if trackerResp.FailureReason != "" {
		return nil, fmt.Errorf("tracker failure: %s", trackerResp.FailureReason)
	}

	return peers.Unmarshal([]byte(trackerResp.Peers))
}
func (t *TorrentFile) requestPeersFromTrackers(
	peerID [20]byte,
	port uint16,
) ([]peers.Peer, error) {

	var lastErr error

	var tiers [][]string

	if len(t.AnnounceList) > 0 {
		tiers = t.AnnounceList
	} else {
		tiers = [][]string{{t.Announce}}
	}

	for _, tier := range tiers {
		for _, trackerURL := range tier {
			peers, err := t.requestPeers(trackerURL, peerID, port)
			if err != nil {
				lastErr = err
				continue
			}
			if len(peers) > 0 {
				return peers, nil
			}
		}
	}

	return nil, lastErr
}

func UnmarshalDict(list []interface{}) ([]peers.Peer, error) {
	var result []peers.Peer

	for _, item := range list {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		ipVal, ok1 := m["ip"].(string)
		portVal, ok2 := m["port"].(int64)
		if !ok1 || !ok2 {
			continue
		}

		result = append(result, peers.Peer{
			IP:   net.ParseIP(ipVal),
			Port: uint16(portVal),
		})
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no valid peers in non-compact list")
	}

	return result, nil
}
