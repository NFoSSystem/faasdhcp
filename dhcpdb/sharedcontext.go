package dhcpdb

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
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
	maxLeaseRange      uint32
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

func NewSharedContext(client *redis.Client, maxLeaseRange uint32, startIP *net.IP, maxTxRetryAttempts uint8) *SharedContext {
	return &SharedContext{
		client:             client,
		maxLeaseRange:      maxLeaseRange,
		rangeStartIp:       startIP,
		maxTxRetryAttempts: maxTxRetryAttempts,
	}
}

func getLastRangeByte(maxLeaseRange uint32) uint32 {
	res := maxLeaseRange >> 3
	if maxLeaseRange%8 == 0 {
		return res - 1
	} else {
		return res
	}
}

func (sc *SharedContext) Close() error {
	return sc.client.Close()
}

func (sc *SharedContext) GetFirstAvailableAddress() (*net.IP, error) {
	ctx := context.Background()

	var pos int64
	var err error

	for i := uint8(0); i < sc.maxTxRetryAttempts; i++ {
		res := sc.client.Watch(ctx, func(tx *redis.Tx) error {

			pos, err = tx.BitPos(ctx, LEASING_RANGE_BITSET, 0, 0).Result()

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

	res, err := sc.client.Get(ctx, "ip:"+ipAddr.String()).Result()
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
			pos, err := tx.BitPos(ctx, LEASING_RANGE_BITSET, 0, 0).Result()

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

			_, err = tx.Set(ctx, "ip:"+ipAddr.String(), hwAddr.String(), leaseTime).Result()
			return err
		}, "ip:"+ipAddr.String(), IP_MAC_MAPPING_SET, LEASING_RANGE_BITSET)

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

			res, err := tx.Get(ctx, "ip:"+ipAddr.String()).Result()
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

			_, err = tx.Del(ctx, "ip:"+ipAddr.String()).Result()
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
		}, "ip:"+ipAddr.String(), LEASING_RANGE_BITSET, IP_MAC_MAPPING_SET)

		if err == redis.TxFailedErr {
			continue
		} else {
			return err
		}
	}

	return fmt.Errorf("Error max retry transaction attempts exceeded (%d)", sc.maxTxRetryAttempts)
}

func CleanUpAvailableIpRange(client *redis.Client) error {
	ctx := context.Background()

	_, err := client.Del(ctx, LEASING_RANGE_BITSET).Result()
	if err != nil {
		return fmt.Errorf("Error deleting Redis set %s: %s", LEASING_RANGE_BITSET, err)
	}

	return nil
}

func CleanUpIpMacMapping(client *redis.Client) error {
	ctx := context.Background()

	_, err := client.Del(ctx, IP_MAC_MAPPING_SET).Result()
	if err != nil {
		return fmt.Errorf("Error deleting Redis set %s: %s", IP_MAC_MAPPING_SET, err)
	}

	return nil
}

func CleanUpIpSets(client *redis.Client) error {
	ctx := context.Background()

	sSlice, err := client.Keys(ctx, "ip:*").Result()
	if err != nil {
		return fmt.Errorf("Error obtaining keys from Redis: %s", err)
	}

	for _, keyStr := range sSlice {
		_, err = client.Del(ctx, keyStr).Result()
		if err != nil {
			return fmt.Errorf("Error deleting key %s from Redis: %s", keyStr, err)
		}
	}

	return nil
}

func InitAvailableIpRange(client *redis.Client, leasesRange uint8) error {
	ctx := context.Background()

	var bitsetStr string = ""
	for i := uint8(0); i < leasesRange; i++ {
		bitsetStr += "\x00"
	}

	_, err := client.Set(ctx, LEASING_RANGE_BITSET, bitsetStr, 0).Result()
	if err != nil {
		return fmt.Errorf("Error during init of BitSet %s\n", LEASING_RANGE_BITSET)
	}

	return nil
}

func (sc *SharedContext) CleanUpExpiredMappings(leaseTime, schedule time.Duration, logger *log.Logger) error {
	ctx := context.Background()
	ticker := time.NewTicker(schedule)

	for {
		<-ticker.C
		var sSlice []string
		score := fmt.Sprintf("%d", time.Duration(time.Now().UnixNano())-leaseTime)
		sSlice, err := sc.client.ZRangeByScore(ctx, IP_MAC_MAPPING_SET, &redis.ZRangeBy{Min: "-inf", Max: score}).Result()
		if err != nil {
			return fmt.Errorf("Error obtaining Redis set elements: %s", IP_MAC_MAPPING_SET)
		}

		for _, keyStr := range sSlice {
			pos := strings.IndexRune(keyStr, '-')
			if pos == -1 {
				continue
			}

			for i := uint8(0); i < sc.maxTxRetryAttempts; i++ {
				err := sc.client.Watch(ctx, func(tx *redis.Tx) error {
					// check present of mapping
					res, err := tx.Get(ctx, keyStr[:pos]).Result()
					if err != nil {
						return err
					}

					// check if the mac address corresponds with the one
					// assigned and now expired
					if keyStr[pos:] != res {
						// a new assignment has been performed between the
						// last two clean up
						return nil
					}

					// delete the assignment from the mapping set
					err = tx.Del(ctx, keyStr[:pos]).Err()
					if err != nil {
						return err
					}

					// set the bit related to the released ip to 0
					err = tx.SetBit(ctx, LEASING_RANGE_BITSET, int64(dhcp4.IPRange(*sc.rangeStartIp, net.ParseIP(keyStr[:pos]))-1),
						0).Err()
					if err != nil {
						return err
					}

					return nil
				}, "ip:"+keyStr[:pos], LEASING_RANGE_BITSET)

				if err == redis.Nil || err == nil {
					// if nothing has been found or nothing gone wrong
					// break the cycle
					break
				} else if err == redis.TxFailedErr {
					// if the optimistic lock failed, retry
					continue
				} else {
					// otherwise, show the error on provided logger
					logger.Println(err)
					break
				}
			}
		}

		for i := uint8(0); i < sc.maxTxRetryAttempts; i++ {
			// remove all keys from the mapping set
			err = sc.client.Watch(ctx, func(tx *redis.Tx) error {
				err = tx.ZRemRangeByScore(ctx, IP_MAC_MAPPING_SET, "-inf", score).Err()
				if err != nil {
					return fmt.Errorf("Error removing range from Redis set: %s", err)
				}

				return nil
			}, IP_MAC_MAPPING_SET)

			if err == redis.TxFailedErr {
				continue
			} else if err != nil && err != redis.Nil {
				break
			} else if err != nil {
				logger.Println(err)
				break
			}
		}

		logger.Println("DHCP db clean up performed")
	}
}
