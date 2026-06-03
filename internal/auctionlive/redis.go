package auctionlive

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	maxLiveBiddersPerAuction = 100
	ttlAfterEndBuffer        = 24 * time.Hour
	keyPrefix                = "pramool:auction:"
)

type bidderMetaJSON struct {
	PlacedAt  string `json:"placed_at"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type redisCache struct {
	client *redis.Client
}

// NewRedisFromURL opens a Redis client; returns Noop if url is empty.
func NewRedisFromURL(url string) (Cache, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return Noop(), nil
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	c := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, err
	}
	return redisCache{client: c}, nil
}

func (redisCache) Enabled() bool { return true }

func (c redisCache) rankKey(auctionID string) string {
	return keyPrefix + auctionID + ":rank"
}

func (c redisCache) metaKey(auctionID string) string {
	return keyPrefix + auctionID + ":meta"
}

func (c redisCache) UpsertBidder(ctx context.Context, auctionID string, auctionEndAt time.Time, entry BidderEntry) error {
	if strings.TrimSpace(auctionID) == "" || strings.TrimSpace(entry.BidderUserID) == "" {
		return nil
	}
	meta, err := json.Marshal(bidderMetaJSON{
		PlacedAt:  entry.PlacedAt.UTC().Format(time.RFC3339),
		FirstName: entry.FirstName,
		LastName:  entry.LastName,
	})
	if err != nil {
		return err
	}
	rankKey := c.rankKey(auctionID)
	metaKey := c.metaKey(auctionID)
	pipe := c.client.Pipeline()
	pipe.ZAdd(ctx, rankKey, redis.Z{Score: float64(entry.BidAmount), Member: entry.BidderUserID})
	pipe.HSet(ctx, metaKey, entry.BidderUserID, string(meta))
	// Trim lowest ranks if over cap (keep top N by bid amount).
	pipe.ZRemRangeByRank(ctx, rankKey, 0, int64(-(maxLiveBiddersPerAuction + 1)))
	expireAt := auctionEndAt.UTC().Add(ttlAfterEndBuffer)
	if expireAt.Before(time.Now().UTC()) {
		expireAt = time.Now().UTC().Add(ttlAfterEndBuffer)
	}
	pipe.ExpireAt(ctx, rankKey, expireAt)
	pipe.ExpireAt(ctx, metaKey, expireAt)
	_, err = pipe.Exec(ctx)
	return err
}

func (c redisCache) ListBidders(ctx context.Context, auctionID string, limit int) ([]BidderEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > maxLiveBiddersPerAuction {
		limit = maxLiveBiddersPerAuction
	}
	rankKey := c.rankKey(auctionID)
	metaKey := c.metaKey(auctionID)
	zs, err := c.client.ZRevRangeWithScores(ctx, rankKey, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, err
	}
	if len(zs) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(zs))
	for _, z := range zs {
		ids = append(ids, fmt.Sprint(z.Member))
	}
	metaRows, err := c.client.HMGet(ctx, metaKey, ids...).Result()
	if err != nil {
		return nil, err
	}
	out := make([]BidderEntry, 0, len(zs))
	for i, z := range zs {
		e := BidderEntry{
			BidderUserID: fmt.Sprint(z.Member),
			BidAmount:    int64(z.Score),
			PlacedAt:     time.Now().UTC(),
		}
		if i < len(metaRows) && metaRows[i] != nil {
			if raw, ok := metaRows[i].(string); ok && raw != "" {
				var m bidderMetaJSON
				if json.Unmarshal([]byte(raw), &m) == nil {
					e.FirstName = m.FirstName
					e.LastName = m.LastName
					if t, err := time.Parse(time.RFC3339, m.PlacedAt); err == nil {
						e.PlacedAt = t
					}
				}
			}
		}
		out = append(out, e)
	}
	return out, nil
}

func (c redisCache) RemoveBidder(ctx context.Context, auctionID, bidderUserID string) error {
	if strings.TrimSpace(auctionID) == "" || strings.TrimSpace(bidderUserID) == "" {
		return nil
	}
	pipe := c.client.Pipeline()
	pipe.ZRem(ctx, c.rankKey(auctionID), bidderUserID)
	pipe.HDel(ctx, c.metaKey(auctionID), bidderUserID)
	_, err := pipe.Exec(ctx)
	return err
}

func (c redisCache) ClearAuction(ctx context.Context, auctionID string) error {
	if strings.TrimSpace(auctionID) == "" {
		return nil
	}
	return c.client.Del(ctx, c.rankKey(auctionID), c.metaKey(auctionID)).Err()
}

// Close releases the Redis client (optional on shutdown).
func Close(c Cache) error {
	rc, ok := c.(redisCache)
	if !ok || rc.client == nil {
		return nil
	}
	return rc.client.Close()
}

// RedisURLFromEnv reads REDIS_URL or REDIS_ADDR (host:port) for local dev.
func RedisURLFromEnv() string {
	if u := strings.TrimSpace(os.Getenv("REDIS_URL")); u != "" {
		return u
	}
	addr := strings.TrimSpace(os.Getenv("REDIS_ADDR"))
	if addr == "" {
		return ""
	}
	return "redis://" + addr + "/0"
}
