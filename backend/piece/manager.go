package piece

import "sync"

type State int

const (
	Missing State = iota
	Downloading
	Done
)

type Piece struct {
	Index       int
	Hash        [20]byte
	Length      int
	State       State
	Downloaders int
}

type Manager struct {
	mu           sync.Mutex
	pieces       []*Piece
	availability []int
}

func New(pieceHashes [][20]byte, pieceLength int, totalLength int) *Manager {
	pieces := make([]*Piece, len(pieceHashes))
	availability := make([]int, len(pieceHashes))

	for i, hash := range pieceHashes {
		length := pieceLength
		if i == len(pieceHashes)-1 {
			remaining := totalLength - (i * pieceLength)
			if remaining > 0 {
				length = remaining
			}
		}

		pieces[i] = &Piece{
			Index:  i,
			Hash:   hash,
			Length: length,
			State:  Missing,
		}
	}

	return &Manager{
		pieces:       pieces,
		availability: availability,
	}
}

func (m *Manager) RegisterBitfield(bfHasPiece func(int) bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.availability {
		if bfHasPiece(i) {
			m.availability[i]++
		}
	}
}

const EndgameThreshold = 5

func (m *Manager) NextPiece(hasPiece func(index int) bool) *Piece {
	m.mu.Lock()
	defer m.mu.Unlock()

	endgame := m.remainingPieces() <= EndgameThreshold

	var selected *Piece
	minAvailability := int(^uint(0) >> 1) // max int

	// 1️⃣ Normal rarest-first selection
	for _, p := range m.pieces {
		if p.State != Missing {
			continue
		}
		if !hasPiece(p.Index) {
			continue
		}
		if endgame && p.Downloaders >= 3 {
			continue
		}

		avail := m.availability[p.Index]
		if avail < minAvailability {
			minAvailability = avail
			selected = p
		}
	}

	if selected == nil {
		for _, p := range m.pieces {
			if p.State == Missing && hasPiece(p.Index) {
				selected = p
				break
			}
		}
	}

	if selected != nil {
		selected.State = Downloading
		selected.Downloaders++
	}

	return selected
}

func (m *Manager) MarkDone(index int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if index < 0 || index >= len(m.pieces) {
		return
	}
	p := m.pieces[index]
	p.State = Done
	p.Downloaders = 0
}

func (m *Manager) MarkFailed(index int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if index < 0 || index >= len(m.pieces) {
		return
	}
	p := m.pieces[index]

	if p.Downloaders > 0 {
		p.Downloaders--
	}

	if p.Downloaders == 0 && p.State != Done {
		p.State = Missing
	}
}

func (m *Manager) RegisterHave(index int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if index >= 0 && index < len(m.availability) {
		m.availability[index]++
	}
}

func (m *Manager) remainingPieces() int {
	count := 0
	for _, p := range m.pieces {
		if p.State != Done {
			count++
		}
	}
	return count
}

func (m *Manager) HasPendingWork() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, p := range m.pieces {
		if p.State == Missing {
			return true
		}
	}
	return false
}
