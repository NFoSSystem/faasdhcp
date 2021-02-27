package nflib

import (
	"bytes"
	"encoding/binary"
	"net"
	"strconv"
	"strings"
)

type msg struct {
	Addr  [4]uint8
	Name  [10]byte
	Port  uint16
	CntId uint16
	Repl  bool
}

/*
Returns from an address and a port a message to send to the gateway.
*/
func NewMsg(addr *net.IP, actionName string, port uint16, cntId uint16, repl bool) *msg {
	res := new(msg)
	split := strings.Split(addr.String(), ".")
	if split == nil || len(split) != 4 {
		res.Addr = [4]uint8{}
	} else {
		for i, frag := range split {
			byteInWord, err := strconv.ParseUint(frag, 10, 8)
			if err != nil {
				res.Addr = [4]uint8{}
				break
			} else {
				res.Addr[i] = uint8(byteInWord)
			}
		}

		copy(res.Name[:], []byte(actionName))
		res.Port = port
		res.CntId = cntId
		res.Repl = repl
	}
	return res
}

/*
Returns a msg struct from a slice of byte.
*/
func GetMsgFromBytes(bSlice []byte) *msg {
	buff := &bytes.Buffer{}
	res := msg{}
	buff.Grow(len(bSlice))
	_, err := buff.Write(bSlice)
	if err != nil {
		panic(err)
	}
	err = binary.Read(buff, binary.BigEndian, &res)
	if err != nil {
		panic(err)
	}
	return &res
}

/*
Returns a slice of byte from a msg type.
*/
func GetBytesFromMsg(src msg) []byte {
	buff := &bytes.Buffer{}
	err := binary.Write(buff, binary.BigEndian, src)
	if err != nil {
		panic(err)
	}
	return buff.Bytes()
}
