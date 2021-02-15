package main

import (
	"log"
	"net"
	"os"
	"runtime/debug"
	"strconv"
	"time"

	"bitbucket.org/Manaphy91/faasdhcp/dhcpdb"
)

const (
	DEFAULT_DHCP_SERVER_PORT = 67
)

func main() {
	args := os.Args[1:]

	var port uint16 = DEFAULT_DHCP_SERVER_PORT
	if len(args) >= 1 {
		val, err := strconv.Atoi(args[0])
		if err == nil {
			port = uint16(val)
		}
	}

	go func() {
		tick := time.NewTicker(500 * time.Millisecond)

		for {
			<-tick.C
			debug.FreeOSMemory()
		}
	}()

	serverIp := &net.IP{192, 168, 1, 249}
	startIp := &net.IP{10, 0, 0, 1}
	subnetIp := &net.IP{255, 0, 0, 0}
	routerIp := &net.IP{192, 168, 1, 254}
	dnsIp := &net.IP{192, 168, 1, 254}
	client := dhcpdb.NewRedisClient("localhost", 6379)
	dhcpdb.CleanUpIpSets(client)
	dhcpdb.CleanUpAvailableIpRange(client)
	dhcpdb.CleanUpIpMacMapping(client)

	dhcpdb.InitAvailableIpRange(client, 50)

	handler := NewHandler(serverIp, startIp, subnetIp, routerIp, dnsIp, 1000000000, time.Hour, client)
	defer handler.Close()
	log.Printf("Starting DHCP Server - Listening port: %d - Starting IP: %s\nStart time: %d\n", port, startIp.String(), time.Now().UnixNano())
	log.Fatal(ListenAndServe(handler, port))
}
