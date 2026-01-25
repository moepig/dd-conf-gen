package redisdb

import (
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// アプリケーション設定ファイル全体を表す構造体
type Config struct {
	// チェック設定生成のための設定
	GenerateConfig GenerateConfig `yaml:"generate_config"`

	// .instances[] の要素のテンプレート
	InstanceTemplate interface{} `yaml:"instance_template"`

	// その他の設定 そのまま出力物に含まれる
	OtherConfigs map[string]interface{} `yaml:",inline"`
}

type GenerateConfig struct {
	// Redis ノードの検索に使用する AWS リソースタグ
	// このタグに一致したリソースのみを対象として、Datadog チェック設定が生成される
	FindTags map[string]string `yaml:"find_tags"`

	// チェック設定に追加するタグ
	// 例えば、Redis ノードに "awsenv: production" タグが付与されている場合、
	// check_tags に "env: awsenv" のように指定すると、Datadog チェック設定に "env: production" タグが追加される
	CheckTags map[string]string `yaml:"check_tags"`

	// AWS リージョン
	Region string `yaml:"region"`
}

// 環境変数からJSON形式の文字列を読み込み、指定された型にデコードする
// 環境変数が設定されていない場合は nil を返し、デコードに失敗した場合はエラーを返す
func loadJSONFromEnv(envKey string, target interface{}) error {
	value := os.Getenv(envKey)
	if value == "" {
		return nil
	}
	return json.Unmarshal([]byte(value), target)
}

// 指定されたパス・環境変数から設定を読み込む
func LoadConfig(filePath string) (*Config, error) {
	var config Config

	// ファイルが指定されていれば読み込む
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			// ファイルが存在しない場合はエラーとせず、設定が空の状態で続ける
			if !os.IsNotExist(err) {
				return nil, err
			}
		} else {
			if err := yaml.Unmarshal(data, &config); err != nil {
				return nil, err
			}
		}
	}

	// 環境変数で .generate_config の値を上書きする
	if region := os.Getenv("GENERATE_CONFIG_REGION"); region != "" {
		config.GenerateConfig.Region = region
	}

	var findTags map[string]string
	if err := loadJSONFromEnv("GENERATE_CONFIG_FIND_TAGS", &findTags); err != nil {
		return nil, err
	}
	if findTags != nil {
		config.GenerateConfig.FindTags = findTags
	}

	var checkTags map[string]string
	if err := loadJSONFromEnv("GENERATE_CONFIG_CHECK_TAGS", &checkTags); err != nil {
		return nil, err
	}
	if checkTags != nil {
		config.GenerateConfig.CheckTags = checkTags
	}

	// 環境変数で instance_template を上書きする
	var instanceTemplate interface{}
	if err := loadJSONFromEnv("INSTANCE_TEMPLATE", &instanceTemplate); err != nil {
		return nil, err
	}
	if instanceTemplate != nil {
		config.InstanceTemplate = instanceTemplate
	}

	// 環境変数で other_configs を上書きする
	var otherConfigs map[string]interface{}
	if err := loadJSONFromEnv("OTHER_CONFIGS", &otherConfigs); err != nil {
		return nil, err
	}
	if otherConfigs != nil {
		if config.OtherConfigs == nil {
			config.OtherConfigs = make(map[string]interface{})
		}
		for k, v := range otherConfigs {
			config.OtherConfigs[k] = v
		}
	}

	// 設定の検証
	if config.GenerateConfig.Region == "" {
		return nil, fmt.Errorf("region is not specified in config or environment variable")
	}

	return &config, nil
}
