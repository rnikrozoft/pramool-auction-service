package auctionlive

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/rnikrozoft/pramool-auction-service/model/dto"
	"github.com/redis/go-redis/v9"
)

const wsFanoutChannel = "pramool:auction:ws"

type wsEnvelope struct {
	AuctionID string               `json:"auction_id"`
	Message   dto.AuctionWSMessage `json:"message"`
}

// WSDeliver forwards a decoded message to local WebSocket clients.
type WSDeliver func(auctionID string, message dto.AuctionWSMessage)

// WSFanout publishes auction WebSocket events across instances (Redis Pub/Sub).
// When disabled, Publish calls deliver directly on the local instance only.
type WSFanout interface {
	Enabled() bool
	Publish(ctx context.Context, auctionID string, message dto.AuctionWSMessage, deliver WSDeliver) error
	Run(ctx context.Context, deliver WSDeliver) error
	Close() error
}

type redisWSFanout struct {
	client *redis.Client
	mu     sync.Mutex
	pubsub *redis.PubSub
}

type localWSFanout struct{}

// LocalWSFanout returns a fan-out that only broadcasts on the current instance.
func LocalWSFanout() WSFanout { return localWSFanout{} }

func (localWSFanout) Enabled() bool { return false }

func (localWSFanout) Publish(_ context.Context, auctionID string, message dto.AuctionWSMessage, deliver WSDeliver) error {
	if deliver == nil {
		return nil
	}
	deliver(auctionID, message)
	return nil
}

func (localWSFanout) Run(context.Context, WSDeliver) error { return nil }

func (localWSFanout) Close() error { return nil }

func (redisWSFanout) Enabled() bool { return true }

func (c redisWSFanout) Publish(ctx context.Context, auctionID string, message dto.AuctionWSMessage, _ WSDeliver) error {
	auctionID = strings.TrimSpace(auctionID)
	if auctionID == "" {
		return nil
	}
	raw, err := json.Marshal(wsEnvelope{AuctionID: auctionID, Message: message})
	if err != nil {
		return err
	}
	return c.client.Publish(ctx, wsFanoutChannel, raw).Err()
}

func (c *redisWSFanout) Run(ctx context.Context, deliver WSDeliver) error {
	if deliver == nil {
		return errors.New("ws fanout: deliver is nil")
	}
	pubsub := c.client.Subscribe(ctx, wsFanoutChannel)
	c.mu.Lock()
	c.pubsub = pubsub
	c.mu.Unlock()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			var env wsEnvelope
			if err := json.Unmarshal([]byte(msg.Payload), &env); err != nil {
				continue
			}
			if strings.TrimSpace(env.AuctionID) == "" {
				continue
			}
			deliver(env.AuctionID, env.Message)
		}
	}
}

func (c *redisWSFanout) Close() error {
	c.mu.Lock()
	ps := c.pubsub
	c.pubsub = nil
	c.mu.Unlock()
	if ps != nil {
		_ = ps.Close()
	}
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// NewWSFanoutFromURL returns Redis-backed fan-out, or a local-only noop when url is empty.
func NewWSFanoutFromURL(url string) (WSFanout, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return localWSFanout{}, nil
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	client := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return &redisWSFanout{client: client}, nil
}

// CloseWSFanout closes Redis resources when fan-out is enabled.
func CloseWSFanout(f WSFanout) error {
	if f == nil {
		return nil
	}
	return f.Close()
}
