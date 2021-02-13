package main

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"time"

	"dhcpdb"
	"utils"

	"github.com/go-redis/redis/v8"
	"github.com/google/netstack/tcpip/header"
	dhcp "github.com/krolaw/dhcp4"
	"golang.org/x/net/ipv4"
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

func (h *DHCPHandler) Close() error {
	return h.sc.Close()
}

func (h *DHCPHandler) ServeDHCP(p dhcp.Packet, msgType dhcp.MessageType, options dhcp.Options) (d dhcp.Packet) {
	switch msgType {

	case dhcp.Discover:
		free, err := h.sc.GetFirstAvailableAddress()
		if err != nil {
			log.Println(err)
			return
		}

		utils.Log.Printf("Incoming DHCP Discover request from %s\n", p.CHAddr())
		utils.Log.Printf("IP address %s offered to %s\n", free, p.CHAddr())

		return dhcp.ReplyPacket(p, dhcp.Offer, h.ip, *free, h.leaseDuration,
			h.options.SelectOrderOrAll(options[dhcp.OptionParameterRequestList]))

	case dhcp.Request:

		utils.Log.Printf("Incoming DHCP Request request from %s\n", p.CHAddr())

		if server, ok := options[dhcp.OptionServerIdentifier]; ok && !net.IP(server).Equal(h.ip) {
			return nil // Message not for this dhcp server
		}
		reqIP := net.IP(options[dhcp.OptionRequestedIPAddress])
		if reqIP == nil {
			reqIP = net.IP(p.CIAddr())
		}

		utils.Log.Printf("Start processing of request for IP address %s made by %s\n", reqIP, p.CHAddr())

		if len(reqIP) == 4 && !reqIP.Equal(net.IPv4zero) {
			if leaseNum := dhcp.IPRange(h.start, reqIP) - 1; leaseNum >= 0 && leaseNum < h.leaseRange {

				hwAddr, err := h.sc.GetPortMACMapping(&reqIP)
				if err != nil && err != redis.Nil {
					utils.Log.Println(err)
					return
				} else if hwAddr == nil || hwAddr.String() == p.CHAddr().String() {
					hwAddress := p.CHAddr()
					err := h.sc.AddIPMACMapping(&reqIP, &hwAddress, h.leaseDuration)
					if err != nil {
						utils.Log.Println(err)
						return
					}

					utils.Log.Printf("Confirmed IP address %s for %s\n", reqIP, p.CHAddr())

					return dhcp.ReplyPacket(p, dhcp.ACK, h.ip, reqIP, h.leaseDuration,
						h.options.SelectOrderOrAll(options[dhcp.OptionParameterRequestList]))
				}

			}
		}
		return dhcp.ReplyPacket(p, dhcp.NAK, h.ip, nil, 0, nil)

	case dhcp.Release, dhcp.Decline:
		ipAddress := p.CIAddr()
		hwAddress := p.CHAddr()

		utils.Log.Printf("Incoming DHCP Release/Decline from %s [ip: %s]\n", hwAddress, ipAddress)

		if err := h.sc.RemoveIPMapping(&ipAddress, &hwAddress); err != nil {
			utils.Log.Println(err)
		}

		utils.Log.Printf("Mapping %s - %s released\n", hwAddress, ipAddress)

	}
	return nil
}

type SFServerConn struct {
	inConn  *net.UDPConn
	outConn *ipv4.PacketConn
}

func NewSFServerConn(udpSocketPort int) (*SFServerConn, error) {
	sfConn := new(SFServerConn)

	tmpConn, err := net.ListenUDP("udp", &net.UDPAddr{net.IPv4(0, 0, 0, 0), udpSocketPort, ""})
	if err != nil {
		return nil, fmt.Errorf("Error opening UDP socket on port %d: %s\n", udpSocketPort, err)
	}
	sfConn.inConn = tmpConn

	conn, err := net.ListenPacket("udp", ":67")
	if err != nil {
		return nil, fmt.Errorf("Error opening UDP socket on port 67: %s\n", err)
	}

	outConn := ipv4.NewPacketConn(conn)
	if err := outConn.SetControlMessage(ipv4.FlagInterface, true); err != nil {
		return nil, fmt.Errorf("Error opening UDP socket on port 67: %s\n", err)
	}
	sfConn.outConn = outConn

	return sfConn, nil
}

func (s *SFServerConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	// second parameter temporarily set to nil
	utils.Log.Println("DEBUG Before write to UDP socket")
	size, err := s.outConn.WriteTo(b, nil, addr)
	if err != nil {
		utils.Log.Printf("Error writing to UDP socket: %s\n", err)
	}
	return size, nil
}

type SFAddress struct {
	netStr  string
	addrStr string
}

func (a *SFAddress) Network() string {
	return a.netStr
}

func (a *SFAddress) String() string {
	return a.addrStr
}

func (s *SFServerConn) ReadFrom(b []byte) (int, net.Addr, error) {
	buff := make([]byte, 65535)
	utils.Log.Println("DEBUG Before read from UDP socket")
	size, err := s.inConn.Read(buff)
	if err != nil {
		utils.Log.Printf("Error reading incoming message from UDP socket: %s\n", err)
	}
	utils.Log.Println("DEBUG Packet read from UDP socket")

	ipPkt := header.IPv4(buff[:size])
	udpPkt := header.UDP(ipPkt.Payload())

	copy(b, udpPkt.Payload())

	return len(udpPkt.Payload()), &SFAddress{"udp", ipPkt.SourceAddress().String() + ":" + strconv.Itoa(int(udpPkt.SourcePort()))}, err
}

func (s *SFServerConn) Close() error {
	var errStr string
	if err := s.inConn.Close(); err != nil {
		errStr = fmt.Sprintf("Error closing incoming connection: %s\n", err)
	}

	if err := s.outConn.Close(); err != nil {
		errStr = fmt.Sprintf("%sError closing outgoing connection: %s\n", err)
	}

	return fmt.Errorf(errStr)
}
