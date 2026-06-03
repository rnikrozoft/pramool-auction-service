package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rnikrozoft/pramool-auction-service/internal/config"
	"github.com/uptrace/bun"
)

type PlatformSettingsRepository struct {
	db *bun.DB
}

func NewPlatformSettingsRepository(db *bun.DB) *PlatformSettingsRepository {
	return &PlatformSettingsRepository{db: db}
}

type platformSettingRow struct {
	SettingKey   string          `bun:"setting_key"`
	SettingValue json.RawMessage `bun:"setting_value"`
}

func (r *PlatformSettingsRepository) loadMaps(ctx context.Context) (map[string]map[string]any, error) {
	var rows []platformSettingRow
	err := r.db.NewSelect().TableExpr("platform_settings").
		Column("setting_key", "setting_value").
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("platform_settings: table empty — run backoffice-core migrate")
	}
	out := make(map[string]map[string]any, len(rows))
	for _, row := range rows {
		if len(row.SettingValue) == 0 {
			return nil, fmt.Errorf("platform_settings.%s is empty", row.SettingKey)
		}
		var m map[string]any
		if err := json.Unmarshal(row.SettingValue, &m); err != nil {
			return nil, fmt.Errorf("platform_settings.%s invalid json: %w", row.SettingKey, err)
		}
		if m == nil {
			return nil, fmt.Errorf("platform_settings.%s is null", row.SettingKey)
		}
		out[row.SettingKey] = m
	}
	return out, nil
}

func (r *PlatformSettingsRepository) LoadPolicyStrict(ctx context.Context) (config.PlatformPolicy, error) {
	m, err := r.loadMaps(ctx)
	if err != nil {
		return config.PlatformPolicy{}, err
	}
	return config.ParsePlatformPolicy(m)
}
