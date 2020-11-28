package dhcpdb

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"time"

	"github.com/krolaw/dhcp4"

	"github.com/go-redis/redis/v8"
)

const (
	SECONDS_IN_HOUR      = 3600
	LEASING_RANGE_BITSET = "leasingRange"
	IP_MAC_MAPPING_SET   = "ipMacMapping"
)

type SharedContext struct {
	client             *redis.Client
	maxLeaseRange      uint8
	rangeStartIp       *net.IP
	maxTxRetryAttempts uint8
}

func NewRedisClient(hostname string, port uint16) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     hostname + ":" + strconv.Itoa(int(port)),
		Password: "",
		DB:       0,
	})
}

func NewSharedContext(client *redis.Client, maxLeaseRange uint8, startIP *net.IP, maxTxRetryAttempts uint8) *SharedContext {
	return &SharedContext{
		client:             client,
		maxLeaseRange:      maxLeaseRange,
		rangeStartIp:       startIP,
		maxTxRetryAttempts: maxTxRetryAttempts,
	}
}

func getLastRangeByte(maxLeaseRange uint8) uint8 {
	res := maxLeaseRange >> 3
	if maxLeaseRange%8 == 0 {
		return res - 1
	} else {
		return res
	}
}

func (sc *SharedContext) GetFirstAvailableAddress() (*net.IP, error) {
	ctx := context.Background()

	var pos int64
	var err error

	for i := uint8(0); i < sc.maxTxRetryAttempts; i++ {
		res := sc.client.Watch(ctx, func(tx *redis.Tx) error {

			pos, err = tx.BitPos(ctx, LEASING_RANGE_BITSET, 0, 0, int64(getLastRangeByte(sc.maxLeaseRange))).Result()

			if err != nil && err != redis.Nil {
				return err
			}

			if err == redis.Nil {
				return fmt.Errorf("Error Bitset %s not defined into remote database", LEASING_RANGE_BITSET)
			}

			return nil
		}, LEASING_RANGE_BITSET)

		if res == nil && pos != -1 {
			addr := dhcp4.IPAdd(*sc.rangeStartIp, int(pos))
			return &addr, nil
		} else if res == nil {
			return nil, fmt.Errorf("Error no more ip addresses available")
		} else if res == redis.TxFailedErr {
			continue
		} else {
			return nil, res
		}
	}

	return nil, fmt.Errorf("Error max retry transaction attempts exceeded (%d)", sc.maxTxRetryAttempts)
}

func (sc *SharedContext) GetPortMACMapping(ipAddr *net.IP) (*net.HardwareAddr, error) {
	ctx := context.Background()

	res, err := sc.client.Get(ctx, ipAddr.String()).Result()
	if err != nil {
		return nil, err
	} else {
		hwAddr, err := net.ParseMAC(res)
		return &hwAddr, err
	}
}

func (sc *SharedContext) AddIPMACMapping(ipAddr *net.IP, hwAddr *net.HardwareAddr, leaseTime time.Duration) error {
	ctx := context.Background()

	val := fmt.Sprintf("%s-%s", ipAddr, hwAddr)

	score := time.Now().UnixNano()

	for i := uint8(0); i < sc.maxTxRetryAttempts; i++ {
		res := sc.client.Watch(ctx, func(tx *redis.Tx) error {
			pos, err := tx.BitPos(ctx, LEASING_RANGE_BITSET, 0, 0, int64(getLastRangeByte(sc.maxLeaseRange))).Result()

			if err != nil && err != redis.Nil {
				return err
			}

			if err == redis.Nil {
				return fmt.Errorf("Error Bitset %s not defined into remote database", LEASING_RANGE_BITSET)
			}

			_, err = tx.SetBit(ctx, LEASING_RANGE_BITSET, pos, 1).Result()
			if err != nil && err != redis.Nil {
				return err
			}

			if err == redis.Nil {
				return fmt.Errorf("Error Bitset %s not defined into remote database", LEASING_RANGE_BITSET)
			}

			err = tx.ZAdd(ctx, IP_MAC_MAPPING_SET, &redis.Z{Score: float64(score), Member: val}).Err()
			if err != nil {
				return err
			}

			_, err = tx.Set(ctx, ipAddr.String(), hwAddr.String(), leaseTime).Result()
			return err
		}, ipAddr.String(), IP_MAC_MAPPING_SET, LEASING_RANGE_BITSET)

		if res == nil {
			return nil
		} else if res == redis.TxFailedErr {
			continue
		} else {
			return res
		}
	}

	return fmt.Errorf("Error max retry transaction attempts exceeded (%d)", sc.maxTxRetryAttempts)
}

func (sc *SharedContext) RemoveIPMapping(ipAddr *net.IP, hwAddr *net.HardwareAddr) error {
	ctx := context.Background()

	for i := uint8(0); i < sc.maxTxRetryAttempts; i++ {
		err := sc.client.Watch(ctx, func(tx *redis.Tx) error {
			_, err := tx.SetBit(ctx, LEASING_RANGE_BITSET, int64(dhcp4.IPRange(*sc.rangeStartIp, *ipAddr)-1),
				0).Result()
			if err == redis.Nil {
				return fmt.Errorf("Error no address mapping defined on db")
			}

			res, err := tx.Get(ctx, ipAddr.String()).Result()
			if err == redis.Nil {
				// nothing to remove, maybe someone provided an address not leased or
				// the timeout did the work for us
				return nil
			} else if err != nil {
				return err
			}

			// check if the stored hw address is equal with the one of the incoming request
			if res != hwAddr.String() {
				// and if they are different, the ip has been leased one more time to
				// someone else
				return nil
			}

			_, err = tx.Del(ctx, ipAddr.String()).Result()
			if err != nil {
				return err
			}

			_, err = tx.ZRem(ctx, IP_MAC_MAPPING_SET, ipAddr.String()).Result()
			if err != nil && err != redis.Nil {
				return err
			} else if err == redis.Nil {
				// nothing to do, some one else removed the mapping for us
				return nil
			}

			return err
		}, ipAddr.String(), LEASING_RANGE_BITSET, IP_MAC_MAPPING_SET)

		if err == redis.TxFailedErr {
			continue
		} else {
			return err
		}
	}

	return fmt.Errorf("Error max retry transaction attempts exceeded (%d)", sc.maxTxRetryAttempts)
}

func InitAvailableIpRange(client *redis.Client, leasesRange uint8) {
	ctx := context.Background()

	var bitsetStr string = ""
	for i := uint8(0); i < leasesRange; i++ {
		bitsetStr += "\x00"
	}

	_, err := client.Set(ctx, LEASING_RANGE_BITSET, bitsetStr, 0).Result()
	if err != nil {
		log.Printf("Error during init of BitSet %s\n", LEASING_RANGE_BITSET)
	}
}
