package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rnikrozoft/pramool-auction-service/internal/config"
	"github.com/rnikrozoft/pramool-auction-service/repository"
)

const platformPolicyCacheTTL = 60 * time.Second

type PlatformPolicyLoader struct {
	repo    *repository.PlatformSettingsRepository
	mu      sync.Mutex
	cached  config.PlatformPolicy
	expires time.Time
}

func NewPlatformPolicyLoader(ctx context.Context, repo *repository.PlatformSettingsRepository) (*PlatformPolicyLoader, error) {
	if repo == nil {
		return nil, fmt.Errorf("platform_settings repository is nil")
	}
	l := &PlatformPolicyLoader{repo: repo}
	if err := l.reload(ctx); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *PlatformPolicyLoader) Get(ctx context.Context) config.PlatformPolicy {
	l.mu.Lock()
	defer l.mu.Unlock()
	if time.Now().After(l.expires) {
		if err := l.reload(ctx); err != nil {
			panic("platform_settings reload failed: " + err.Error())
		}
	}
	return l.cached
}

func (l *PlatformPolicyLoader) reload(ctx context.Context) error {
	policy, err := l.repo.LoadPolicyStrict(ctx)
	if err != nil {
		return err
	}
	l.cached = policy
	l.expires = time.Now().Add(platformPolicyCacheTTL)
	return nil
}
