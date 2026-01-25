package ddconfgen

import (
	"encoding/json"
	"os"

	"gopkg.in/yaml.v3"
)

// 設定ファイル全体を表す構造体
type Config struct {
	GenerateConfig   GenerateConfig         `yaml:"generate_config"`
	InstanceTemplate interface{}            `yaml:"instance_template"`
	OtherConfigs     map[string]interface{} `yaml:",inline"`
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
	if tagsStr := os.Getenv("GENERATE_CONFIG_FIND_TAGS"); tagsStr != "" {
		var tags map[string]string
		// JSON形式の文字列をデコード
		if err := json.Unmarshal([]byte(tagsStr), &tags); err != nil {
			// デコードに失敗した場合は、エラーを返す
			return nil, err
		}
		config.GenerateConfig.FindTags = tags
	}

	return &config, nil
}
