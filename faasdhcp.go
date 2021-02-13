package main

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"time"

	"dhcpdb"
	"nflib"
	"utils"

	"github.com/krolaw/dhcp4"
)

// func main() {
// 	/*serverIp := &net.IP{192, 168, 1, 249}
// 	startIp := &net.IP{192, 168, 1, 115}
// 	subnetIp := &net.IP{255, 255, 255, 0}
// 	routerIp := &net.IP{192, 168, 1, 254}
// 	dnsIp := &net.IP{192, 168, 1, 254}
// 	client := dhcpdb.NewRedisClient("localhost", 6379)
// 	dhcpdb.CleanUpIpSets(client)
// 	dhcpdb.CleanUpAvailableIpRange(client)
// 	dhcpdb.CleanUpIpMacMapping(client)

// 	dhcpdb.InitAvailableIpRange(client, 50)

// 	handler := NewHandler(serverIp, startIp, subnetIp, routerIp, dnsIp, 50, time.Hour, client)
// 	defer handler.Close()
// 	log.Fatal(dhcp4.ListenAndServe(handler))*/
// 	var m map[string]interface{}

// 	Main(m)
// }

func Main(obj map[string]interface{}) map[string]interface{} {
	lIp, _ := nflib.GetLocalIpAddr()
	strPrefix := fmt.Sprintf("[%s] -> ", lIp.String())

	logger, err := nflib.NewRedisLogger(strPrefix, "logChan", nflib.GetGatewayIP().String(), nflib.REDIS_PORT)
	if err != nil {
		log.Fatalln(err)
	}
	utils.Log = logger

	utils.Log.Printf("Starting DHCP NF at %s ...", lIp)

	serverIp := nflib.GetGatewayIP()
	startIp := &net.IP{192, 168, 1, 115}
	subnetIp := &net.IP{255, 255, 255, 0}
	routerIp := &net.IP{192, 168, 1, 254}
	dnsIp := &net.IP{192, 168, 1, 254}
	client := dhcpdb.NewRedisClient(serverIp.String(), 6379)

	nflib.SendPingMessageToRouter("dhcp", utils.Log, utils.Log)

	handler := NewHandler(&serverIp, startIp, subnetIp, routerIp, dnsIp, 50, time.Hour, client)
	defer handler.Close()
	utils.Log.Println("Starting accepting UDP packets ...")
	utils.Log.Println(ListenAndServe(handler, 9826))

	utils.Log.Println("Function terminated due to an error")

	res := make(map[string]interface{})
	return res
}

func ListenAndServe(handler dhcp4.Handler, port int) error {
	conn, err := NewSFServerConn(port)
	if err != nil {
		return err
	}
	defer conn.Close()
	return Serve(conn, handler)
}

func Serve(conn dhcp4.ServeConn, handler dhcp4.Handler) error {
	buffer := make([]byte, 1500)
	for {
		n, addr, err := conn.ReadFrom(buffer)
		if err != nil {
			utils.Log.Println("DEBUG 1")
			return err
		}
		if n < 240 { // Packet too small to be DHCP
			utils.Log.Println("DEBUG 2")
			continue
		}
		req := dhcp4.Packet(buffer[:n])
		if req.HLen() > 16 { // Invalid size
			utils.Log.Println("DEBUG 3")
			continue
		}
		options := req.ParseOptions()
		var reqType dhcp4.MessageType
		if t := options[dhcp4.OptionDHCPMessageType]; len(t) != 1 {
			utils.Log.Println("DEBUG 4")
			continue
		} else {
			reqType = dhcp4.MessageType(t[0])
			if reqType < dhcp4.Discover || reqType > dhcp4.Inform {
				utils.Log.Println("DEBUG 5")
				continue
			}
		}
		if res := handler.ServeDHCP(req, reqType, options); res != nil {
			// If IP not available, broadcast
			ipStr, portStr, err := net.SplitHostPort(addr.String())
			if err != nil {
				utils.Log.Println("DEBUG 6")
				return err
			}

			if net.ParseIP(ipStr).Equal(net.IPv4zero) || req.Broadcast() {
				port, _ := strconv.Atoi(portStr)
				addr = &net.UDPAddr{IP: net.IPv4(192, 168, 1, 102), Port: port}
				utils.Log.Println("DEBUG 7")
			}
			if _, e := conn.WriteTo(res, addr); e != nil {
				utils.Log.Println("DEBUG 8")
				return e
			}
		}
	}
}
