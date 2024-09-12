package structure

import (
	"io"

	"github.com/go-bamboo/redissync/log"
)

func ReadByte(rd io.Reader) byte {
	b := ReadBytes(rd, 1)[0]
	return b
}

func ReadBytes(rd io.Reader, n int) []byte {
	buf := make([]byte, n)
	_, err := io.ReadFull(rd, buf)
	if err != nil {
		log.Panicf(err.Error())
	}
	return buf
}
