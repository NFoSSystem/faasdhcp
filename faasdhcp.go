package main

import (
	"fmt"
	"log"
	"net"
	"time"

	"bitbucket.org/Manaphy91/faasdhcp/dhcpdb"
	"bitbucket.org/Manaphy91/faasdhcp/utils"
	"bitbucket.org/Manaphy91/nflib"
	"github.com/krolaw/dhcp4"
)

func main() {
	/*serverIp := &net.IP{192, 168, 1, 249}
	startIp := &net.IP{192, 168, 1, 115}
	subnetIp := &net.IP{255, 255, 255, 0}
	routerIp := &net.IP{192, 168, 1, 254}
	dnsIp := &net.IP{192, 168, 1, 254}
	client := dhcpdb.NewRedisClient("localhost", 6379)
	dhcpdb.CleanUpIpSets(client)
	dhcpdb.CleanUpAvailableIpRange(client)
	dhcpdb.CleanUpIpMacMapping(client)

	dhcpdb.InitAvailableIpRange(client, 50)

	handler := NewHandler(serverIp, startIp, subnetIp, routerIp, dnsIp, 50, time.Hour, client)
	defer handler.Close()
	log.Fatal(dhcp4.ListenAndServe(handler))*/
	var m map[string]interface{}

	Main(m)
}

func Main(obj map[string]interface{}) map[string]interface{} {
	lIp, _ := nflib.GetLocalIpAddr()
	strPrefix := fmt.Sprintf("[%s] -> ", lIp.String())

	logger, err := nflib.NewRedisLogger(strPrefix, "logChan", lIp.String(), nflib.REDIS_PORT)
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
	client := dhcpdb.NewRedisClient("localhost", 6379)

	nflib.SendPingMessageToRouter(utils.Log, utils.Log)

	handler := NewHandler(&serverIp, startIp, subnetIp, routerIp, dnsIp, 50, time.Hour, client)
	defer handler.Close()
	utils.Log.Println("Starting accepting UDP packets ...")
	utils.Log.Println(dhcp4.ListenAndServe(handler))

	res := make(map[string]interface{})
	return res
}
