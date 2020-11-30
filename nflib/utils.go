package nflib

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

/*
Returns the IP address of the gateway.
*/
func GetGatewayIP() net.IP {
	file, err := os.Open(ROUTE_FILENAME)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	for i := 0; i < GATEWAY_LINE; i++ {
		scanner.Scan()
	}

	tokens := strings.Split(scanner.Text(), SEP)
	gatewayHex := "0x" + tokens[FIELD]

	d, _ := strconv.ParseInt(gatewayHex, 0, 64)
	d32 := uint32(d)
	ipd32 := make(net.IP, 4)
	binary.LittleEndian.PutUint32(ipd32, d32)
	return net.IP(ipd32)
}

/*
Returns the local IP address.
*/
func GetLocalIpAddr() (*net.IP, error) {
	addrs, err := net.InterfaceAddrs()

	if err != nil {
		fmt.Printf("Error obtaining the gateway address")
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil &&
			!ipnet.IP.IsLoopback() {
			return &ipnet.IP, nil
		}
	}

	return nil, nil
}

/*
Returns source and destination port from a packet given the related bytes pointers.
*/
func GetPortsFromBytes(packet []byte, ptr1, ptr2, ptr3, ptr4 int) (uint16, uint16, error) {
	if len(packet) < 4 {
		return 0, 0, fmt.Errorf("Error provided byte slice with lenght lower than 4")
	}

	var source uint16 = (uint16(packet[ptr1]|0) << 8) | uint16(packet[ptr2])
	var target uint16 = (uint16(packet[ptr3]|0) << 8) | uint16(packet[ptr4])

	return source, target, nil
}

/*
The function is a factory for a function that taken as input a slice of bytes returns source
and destination ip addresses.
*/
func GetIPsFromBytes(ptr1, ptr2 int) func([]byte) (net.IP, net.IP, error) {
	return func(pkt []byte) (net.IP, net.IP, error) {
		if len(pkt) < max(ptr1+3, ptr2+3) {
			return nil, nil, fmt.Errorf("Error byte buffer provided smaller than expected!")
		}
		return net.IPv4(pkt[ptr1], pkt[ptr1+1], pkt[ptr1+2], pkt[ptr1+3]),
			net.IPv4(pkt[ptr2], pkt[ptr2+1], pkt[ptr2+2], pkt[ptr2+3]),
			nil
	}
}

/*
Returns the nano seconds passed since 01/01/1970.
*/
func GetNanoSeconds() int64 {
	t := time.Now()
	return t.UnixNano()
}

/*
Returns source and destination ip address of a UDP packet taken as input a slice of
bytes.
*/
var GetIPsFromPkt func([]byte) (net.IP, net.IP, error) = GetIPsFromBytes(UDP_IP_SRC_FIRST_BYTE, UDP_IP_TRG_FIRST_BYTE)

/*
Send ping message to the gateway
*/
func SendPingMessageToRouter(debugLog *log.Logger, errLog *log.Logger) {
	rIp := GetGatewayIP()
	lIp, err := GetLocalIpAddr()
	if err != nil {
		errLog.Println(err)
	}

	port := 9082
	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{rIp, port, ""})
	if err != nil {
		errLog.Printf("Error opening UDP connection to ip %s and port %d. Action will be terminated.\n", rIp, port)
	}

	debugLog.Printf("Ping message sent to IP %s at port %d\n", rIp, port)

	pkt := NewMsg(lIp, 9826)
	tBuff := GetBytesFromMsg(*pkt)

	conn.Write(tBuff)
	conn.Close()
}

func max(v1, v2 int) int {
	if v1 > v2 {
		return v1
	} else {
		return v2
	}
}
