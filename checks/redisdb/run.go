package redisdb

import (
	"context"
	"fmt"
)

// Run は Redis チェック設定の生成を実行し、YAML データを返す
func Run(ctx context.Context, configPath string) ([]byte, error) {
	// 設定ファイルの読み込み
	config, err := LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Redis ノードの取得
	nodes, err := GetRedisNodes(ctx, config.GenerateConfig.Region, config.GenerateConfig.FindTags)
	if err != nil {
		return nil, fmt.Errorf("failed to get Redis nodes: %w", err)
	}

	// Datadog チェック設定の生成
	yamlData, err := GenerateRedisDBConfig(nodes, config)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Redis config: %w", err)
	}

	return yamlData, nil
}
