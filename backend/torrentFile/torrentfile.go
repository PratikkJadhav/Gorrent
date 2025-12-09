package torrentfile

const port uint16 = 6881

type torrentfile struct {
	Announce    string
	InfoHash    [20]byte
	PieceHash   [][20]byte
	PieceLength int
	Length      int
	Name        string
}
