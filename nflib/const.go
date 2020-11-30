package nflib

const (
	TCP_SRC_FIRST_WORD    = 20
	TCP_TRG_FIRST_WORD    = 22
	UDP_SRC_FIRST_WORD    = 20
	UDP_TRG_FIRST_WORD    = 22
	LOG_FILE_PATH         = "function.log"
	ROUTE_FILENAME        = "/proc/net/route"
	GATEWAY_LINE          = 2    // line containing the gateway addr. (first line: 0)
	SEP                   = "\t" // field separator
	FIELD                 = 2
	REDIS_HOSTNAME        = "localhost"
	REDIS_PORT            = 6379
	UDP_IP_SRC_FIRST_BYTE = 12
	UDP_IP_TRG_FIRST_BYTE = 16
)
