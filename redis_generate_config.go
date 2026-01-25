package ddconfgen

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Datadog Agent チェック設定ファイルを生成
func GenerateRedisDBConfig(nodes []RedisNode, config *Config) ([]byte, error) {
	// 出力する YAML の構造を作成
	output := make(map[string]interface{})

	// init_config を設定（config.OtherConfigs から取得、存在しない場合は nil）
	if initConfig, exists := config.OtherConfigs["init_config"]; exists {
		output["init_config"] = initConfig
	} else {
		output["init_config"] = nil
	}

	// instances を生成
	instances := make([]interface{}, 0, len(nodes))
	for _, node := range nodes {
		instance, err := buildInstance(node, config)
		if err != nil {
			return nil, fmt.Errorf("failed to build instance for node %s:%d: %w", node.Host, node.Port, err)
		}
		instances = append(instances, instance)
	}
	output["instances"] = instances

	// YAML にマーシャル
	yamlBytes, err := yaml.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config to YAML: %w", err)
	}

	return yamlBytes, nil
}

// RedisNode と Config から1つの instance 設定を構築する
func buildInstance(node RedisNode, config *Config) (map[string]interface{}, error) {
	// instance_template をベースとしてコピー
	instance := make(map[string]interface{})

	// instance_template が map の場合、その内容をコピー
	if template, ok := config.InstanceTemplate.(map[string]interface{}); ok {
		for k, v := range template {
			instance[k] = v
		}
	}

	// host と port を設定
	instance["host"] = node.Host
	instance["port"] = node.Port

	// tags を生成
	tags := buildTags(node, config, instance)
	if len(tags) > 0 {
		instance["tags"] = tags
	}

	return instance, nil
}

// RedisNode のタグと CheckTags から Datadog タグのリストを生成する
func buildTags(node RedisNode, config *Config, instance map[string]interface{}) []string {
	tags := []string{}

	// instance_template の tags があれば、それを先に追加
	if existingTags, ok := instance["tags"].([]interface{}); ok {
		for _, tag := range existingTags {
			if tagStr, ok := tag.(string); ok {
				tags = append(tags, tagStr)
			}
		}
	}

	// CheckTags に基づいて、ノードのタグから Datadog タグを生成
	for checkTagKey, nodeTagKey := range config.GenerateConfig.CheckTags {
		if nodeTagValue, exists := node.Tags[nodeTagKey]; exists {
			tags = append(tags, fmt.Sprintf("%s:%s", checkTagKey, nodeTagValue))
		}
	}

	return tags
}
