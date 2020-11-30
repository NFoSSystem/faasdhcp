package nflib

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/go-redis/redis/v8"
)

type RedisPubSubWriter struct {
	client  *redis.Client
	publish func(buff []byte) (int, error)
}

func NewRedisPubSubWriter(chanName string, hostname string, port int) (*RedisPubSubWriter, error) {
	ctx := context.Background()

	res := new(RedisPubSubWriter)
	res.client = redis.NewClient(&redis.Options{
		Network: "tcp",
		Addr:    hostname + ":" + strconv.Itoa(port),
	})

	pSClient := res.client.Subscribe(ctx, chanName)
	_, err := pSClient.Receive(ctx)

	if err != nil {
		return nil, fmt.Errorf("Error attempt opening Redis Pub/Sub Channel: %s", err)
	}

	res.publish = func(buff []byte) (int, error) {
		size, err := res.client.Publish(ctx, chanName, string(buff)).Result()
		return int(size), err
	}

	return res, nil
}

func (r *RedisPubSubWriter) Write(buff []byte) (int, error) {
	return r.publish(buff)
}

func (r *RedisPubSubWriter) Close() error {
	return r.client.Close()
}

/*
Returns a logger which one sends messages to a Redis Pub/Sub Channel.
*/
func NewRedisLogger(logPrefix, chanName, hostname string, port int) (*log.Logger, error) {
	rpsw, err := NewRedisPubSubWriter(chanName, hostname, port)
	if err != nil {
		return nil, fmt.Errorf("Error creating RedisPubSubWriter: %s", err)
	}

	return log.New(rpsw, logPrefix, log.Ldate|log.Lmicroseconds), nil
}
