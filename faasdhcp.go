package main

import (
	"faasdhcp/dhcpdb"
	"log"
	"net"
	"time"

	"github.com/krolaw/dhcp4"
)

func main() {
	serverIp := &net.IP{192, 168, 1, 249}
	startIp := &net.IP{192, 168, 1, 115}
	subnetIp := &net.IP{255, 255, 255, 0}
	routerIp := &net.IP{192, 168, 1, 254}
	dnsIp := &net.IP{192, 168, 1, 254}
	client := dhcpdb.NewRedisClient("localhost", 6379)
	dhcpdb.InitAvailableIpRange(client, 50)

	handler := NewHandler(serverIp, startIp, subnetIp, routerIp, dnsIp, 50, time.Hour, client)
	log.Fatal(dhcp4.ListenAndServe(handler))
}
