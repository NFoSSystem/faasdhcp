module action

go 1.15

replace nflib => ./nflib

replace dhcpdb => ./dhcpdb

replace utils => ./utils

require (
	dhcpdb v0.0.0-00010101000000-000000000000
	github.com/go-redis/redis/v8 v8.4.0
	github.com/krolaw/dhcp4 v0.0.0-20190909130307-a50d88189771
	nflib v0.0.0-00010101000000-000000000000
	utils v0.0.0-00010101000000-000000000000
)
