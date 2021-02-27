package nflib

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"log"
	"net"
)

func Ipv4ToInt32(ip *net.IP) uint32 {
	ipB := []byte(*ip)
	return uint32(ipB[12])<<24 | uint32(ipB[13])<<16 | uint32(ipB[14])<<8 | uint32(ipB[15])
}

func Uint32ToIpv4(addr uint32) *net.IP {
	ip := net.IPv4(byte(addr>>24), byte((0xFFFFFF&addr)>>16), byte((0xFFFF&addr)>>8), byte(255&addr))
	return &ip
}

type Packet struct {
	Addr  uint32
	Crc16 uint16
	Port  uint16
}

func GetPacketFromBytes(bSlice []byte) *Packet {
	buff := &bytes.Buffer{}
	res := Packet{}
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

func GetBytesFromPacket(src Packet) []byte {
	buff := &bytes.Buffer{}
	err := binary.Write(buff, binary.BigEndian, src)
	if err != nil {
		panic(err)
	}
	return buff.Bytes()
}

func SendToRouter(pktChan chan Packet, addr *net.IP, port uint16) {
	conn, err := net.DialTCP("tcp", nil, &net.TCPAddr{*addr, int(port), ""})
	if err != nil {
		log.Fatalf("Error opening TCP socket to %s:%d: %s\n", addr.String(), port)
	}
	defer conn.Close()

	for {
		select {
		case pkt := <-pktChan:
			_, err := conn.Write(GetBytesFromPacket(pkt))
			if err != nil {
				log.Fatalf("Error writing mapping message to %s:%d: %s", addr.String(), port, err)
			}
		default:
			continue
		}
	}
}

func GetStringFromPortSlice(ps []uint16) string {
	inLen := len(ps)
	buff := make([]byte, inLen*2)
	for i := 0; i < inLen; i++ {
		binary.BigEndian.PutUint16(buff[i*2:], ps[i])
	}
	return base64.StdEncoding.EncodeToString(buff)
}

func GetPortSliceFromString(ps string) []uint16 {
	inBuff, err := base64.StdEncoding.DecodeString(ps)
	if err != nil {
		return []uint16{}
	}
	inLen := len(inBuff)
	res := make([]uint16, inLen/2)
	for i := 0; i < inLen/2; i++ {
		res[i] = binary.BigEndian.Uint16(inBuff[i*2:])
	}
	return res
}
