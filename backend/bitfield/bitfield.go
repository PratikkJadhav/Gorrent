package bitfield

type Bitfield []byte

func (bf Bitfield) HasPiece(index int) bool {
	byteIndex := index / 8 //1 byte = 8 bits
	position := index % 8  //position of index in 1 byte

	if byteIndex < 0 || byteIndex >= len(bf) {
		return false
	}

	return bf[byteIndex]>>uint(7-position)&1 != 0
	//(7-position) because in byte numberings are 7 6 5 4 3 2 1 0
	//>>(right shift) moves index (target bit) to LSB
	//&1 masks everything except LSB
	//!=0 because we want true or false , !=0 -> true , ==0 -> false
	//unint() because for shifting we need unsigned integer

}

func (bf Bitfield) SetPiece(index int) {
	byteIndex := index / 8
	position := index % 8
	if byteIndex < 0 || byteIndex >= len(bf) {
		return
	}

	bf[byteIndex] |= 1 << uint(7-position)
	//1<<uint() creates bit mask
	//|= (OR) set bit to 1 ( compares bf[byteIndex] and 1<<uint() and sets 1 if any one of them contains 1 , like if I already have a  piece then set the piece to 1 or if I just got it from another user then also set it to 1)
}
