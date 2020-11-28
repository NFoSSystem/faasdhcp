package main

import (
	"faasdhcp/dhcpdb"
	"log"
	"net"
	"time"

	"github.com/go-redis/redis/v8"
	dhcp "github.com/krolaw/dhcp4"
)

type lease struct {
	nic    string    // Client's CHAddr
	expiry time.Time // When the lease expires
}

type DHCPHandler struct {
	ip            net.IP        // Server IP to use
	options       dhcp.Options  // Options to send to DHCP Clients
	start         net.IP        // Start of IP range to distribute
	leaseRange    int           // Number of IPs to distribute (starting from start)
	leaseDuration time.Duration // Lease period
	leases        map[int]lease // Map to keep track of leases
	sc            *dhcpdb.SharedContext
}

func NewHandler(serverIP, startIP, subnet, router, serverDNS *net.IP, leaseRange int, leaseDuration time.Duration, client *redis.Client) *DHCPHandler {

	sc := dhcpdb.NewSharedContext(client, uint8(leaseRange), startIP, 5)

	return &DHCPHandler{
		ip:            *serverIP,
		leaseDuration: leaseDuration,
		start:         *startIP,
		leaseRange:    leaseRange,
		leases:        make(map[int]lease, 10),
		options: dhcp.Options{
			dhcp.OptionSubnetMask:       []byte(*subnet),
			dhcp.OptionRouter:           []byte(*router),
			dhcp.OptionDomainNameServer: []byte(*serverDNS),
		},
		sc: sc,
	}
}

func (h *DHCPHandler) ServeDHCP(p dhcp.Packet, msgType dhcp.MessageType, options dhcp.Options) (d dhcp.Packet) {
	switch msgType {

	case dhcp.Discover:
		free, err := h.sc.GetFirstAvailableAddress()
		if err != nil {
			log.Println(err)
			return
		}

		log.Printf("Incoming DHCP Discover request from %s\n", p.CHAddr())
		log.Printf("IP address %s offered to %s\n", free, p.CHAddr())

		return dhcp.ReplyPacket(p, dhcp.Offer, h.ip, *free, h.leaseDuration,
			h.options.SelectOrderOrAll(options[dhcp.OptionParameterRequestList]))

	case dhcp.Request:

		log.Printf("Incoming DHCP Request request from %s\n", p.CHAddr())

		if server, ok := options[dhcp.OptionServerIdentifier]; ok && !net.IP(server).Equal(h.ip) {
			return nil // Message not for this dhcp server
		}
		reqIP := net.IP(options[dhcp.OptionRequestedIPAddress])
		if reqIP == nil {
			reqIP = net.IP(p.CIAddr())
		}

		log.Printf("Start processing of request for IP address %s made by %s\n", reqIP, p.CHAddr())

		if len(reqIP) == 4 && !reqIP.Equal(net.IPv4zero) {
			if leaseNum := dhcp.IPRange(h.start, reqIP) - 1; leaseNum >= 0 && leaseNum < h.leaseRange {

				hwAddr, err := h.sc.GetPortMACMapping(&reqIP)
				if err != nil && err != redis.Nil {
					log.Println(err)
					return
				} else if hwAddr == nil || hwAddr.String() == p.CHAddr().String() {
					hwAddress := p.CHAddr()
					err := h.sc.AddIPMACMapping(&reqIP, &hwAddress, h.leaseDuration)
					if err != nil {
						log.Println(err)
						return
					}

					log.Printf("Confirmed IP address %s for %s\n", reqIP, p.CHAddr())

					return dhcp.ReplyPacket(p, dhcp.ACK, h.ip, reqIP, h.leaseDuration,
						h.options.SelectOrderOrAll(options[dhcp.OptionParameterRequestList]))
				}

			}
		}
		return dhcp.ReplyPacket(p, dhcp.NAK, h.ip, nil, 0, nil)

	case dhcp.Release, dhcp.Decline:
		ipAddress := p.CIAddr()
		hwAddress := p.CHAddr()

		log.Printf("Incoming DHCP Release/Decline from %s [ip: %s]\n", hwAddress, ipAddress)

		if err := h.sc.RemoveIPMapping(&ipAddress, &hwAddress); err != nil {
			log.Println(err)
		}

		log.Printf("Mapping %s - %s released\n", hwAddress, ipAddress)

	}
	return nil
}
