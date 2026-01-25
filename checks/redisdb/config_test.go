package redisdb

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTempFile(t *testing.T, content string) string {
	t.Helper()
	tmpfile, err := os.CreateTemp("", "test-*.yaml")
	require.NoError(t, err, "Failed to create temp file")

	_, err = tmpfile.Write([]byte(content))
	require.NoError(t, err, "Failed to write to temp file")

	err = tmpfile.Close()
	require.NoError(t, err, "Failed to close temp file")

	return tmpfile.Name()
}

// テスト内容: YAML ファイルからのみ設定を読み込む
// 期待する結果: ファイルの内容が正しく Config 構造体にパースされる
func TestLoadConfig_FromFileOnly(t *testing.T) {
	// Arrange: テスト用 YAML ファイルの作成
	yamlContent := `
generate_config:
  region: "ap-northeast-1"
  find_tags:
    env: "test"
    service: "test-service"
  check_tags:
    environment: "env"
    team: "service"
other_key:
  foo: "bar"
`
	filePath := createTempFile(t, yamlContent)
	defer os.Remove(filePath)

	// Act: 設定の読み込み
	config, err := LoadConfig(filePath)
	require.NoError(t, err, "LoadConfig should not return an error")

	// Assert: 結果の検証
	expected := &Config{
		GenerateConfig: GenerateConfig{
			Region:    "ap-northeast-1",
			FindTags:  map[string]string{"env": "test", "service": "test-service"},
			CheckTags: map[string]string{"environment": "env", "team": "service"},
		},
		OtherConfigs: map[string]interface{}{"other_key": map[string]interface{}{"foo": "bar"}},
	}

	assert.Equal(t, expected, config)
}

// テスト内容: YAML ファイルの内容を環境変数で上書きする
// 期待する結果: 環境変数の値が優先され、Config 構造体に反映される
func TestLoadConfig_OverrideWithEnvironmentVariables(t *testing.T) {
	// Arrange: YAML ファイルと環境変数の準備
	yamlContent := `
generate_config:
  region: "ap-northeast-1"
  find_tags:
    env: "test"
  check_tags:
    environment: "env"
other_key:
  foo: "bar"
`
	filePath := createTempFile(t, yamlContent)
	defer os.Remove(filePath)

	t.Setenv("GENERATE_CONFIG_REGION", "us-west-2")
	t.Setenv("GENERATE_CONFIG_FIND_TAGS", `{"env":"prod","team":"backend"}`)

	// Act: 設定の読み込み
	config, err := LoadConfig(filePath)
	require.NoError(t, err)

	// Assert: 結果の検証
	expected := &Config{
		GenerateConfig: GenerateConfig{
			Region:    "us-west-2",
			FindTags:  map[string]string{"env": "prod", "team": "backend"},
			CheckTags: map[string]string{"environment": "env"},
		},
		OtherConfigs: map[string]interface{}{"other_key": map[string]interface{}{"foo": "bar"}},
	}

	assert.Equal(t, expected, config)
}

// テスト内容: 環境変数からのみ設定を読み込む
// 期待する結果: 環境変数の値が正しく Config 構造体にパースされる
func TestLoadConfig_FromEnvironmentVariablesOnly(t *testing.T) {
	// Arrange: 環境変数の設定
	t.Setenv("GENERATE_CONFIG_REGION", "eu-central-1")
	t.Setenv("GENERATE_CONFIG_FIND_TAGS", `{"team":"frontend"}`)

	// Act: 設定の読み込み（ファイルパスを空にする）
	config, err := LoadConfig("")
	require.NoError(t, err)

	// Assert: 結果の検証
	expected := &Config{
		GenerateConfig: GenerateConfig{
			Region:    "eu-central-1",
			FindTags:  map[string]string{"team": "frontend"},
			CheckTags: nil,
		},
		OtherConfigs: nil,
	}

	assert.Equal(t, expected, config)
}

// テスト内容: find_tags に不正な JSON が設定された環境変数を読み込むケース
// 期待する結果: エラーを返す
func TestLoadConfig_InvalidJSONInEnvironmentVariable(t *testing.T) {
	// Arrange: 正しい YAML ファイルと不正な JSON 環境変数の準備
	yamlContent := `
generate_config:
  region: "ap-northeast-1"
  find_tags:
    from: "file"
  check_tags:
    environment: "env"
`
	filePath := createTempFile(t, yamlContent)
	defer os.Remove(filePath)

	t.Setenv("GENERATE_CONFIG_FIND_TAGS", `{"invalid"}`)

	// Act: 設定の読み込み
	_, err := LoadConfig(filePath)

	// Assert: エラーが返される
	require.Error(t, err)
}

// テスト内容: 存在しないファイルパスを指定して読み込む
// 期待する結果: region が設定されていないためエラーが返される
func TestLoadConfig_FileNotFound(t *testing.T) {
	// Arrange & Act: 存在しないファイルパスで設定を読み込む
	_, err := LoadConfig("non-existent-file.yaml")

	// Assert: region が必須のためエラーが返される
	require.Error(t, err)
	assert.Contains(t, err.Error(), "region is not specified")
}

// テスト内容: 不正な形式の YAML ファイルを読み込む
// 期待する結果: `yaml.Unmarshal` エラーが返される
func TestLoadConfig_InvalidYAMLFile(t *testing.T) {
	// Arrange: 不正な YAML ファイルの作成
	yamlContent := "this: is: invalid: yaml"
	filePath := createTempFile(t, yamlContent)
	defer os.Remove(filePath)

	// Act: 設定の読み込み
	_, err := LoadConfig(filePath)

	// Assert: エラーが返される
	require.Error(t, err, "Expected error for invalid YAML file")
}

// テスト内容: CheckTags, InstanceTemplate, OtherConfigs を環境変数で上書きする
// 期待する結果: 環境変数の値が優先され、Config 構造体に反映される
func TestLoadConfig_OverrideCheckTagsInstanceTemplateAndOtherConfigs(t *testing.T) {
	// Arrange: YAML ファイルと環境変数の準備
	yamlContent := `
generate_config:
  region: "ap-northeast-1"
  find_tags:
    env: "test"
  check_tags:
    environment: "env"
instance_template:
  host: "localhost"
  port: 6379
init_config:
  foo: "bar"
`
	filePath := createTempFile(t, yamlContent)
	defer os.Remove(filePath)

	t.Setenv("GENERATE_CONFIG_CHECK_TAGS", `{"team":"service","env":"environment"}`)
	t.Setenv("INSTANCE_TEMPLATE", `{"host":"redis.example.com","port":6380,"ssl":true}`)
	t.Setenv("OTHER_CONFIGS", `{"init_config":{"baz":"qux"},"new_key":"new_value"}`)

	// Act: 設定の読み込み
	config, err := LoadConfig(filePath)
	require.NoError(t, err)

	// Assert: 結果の検証
	expectedCheckTags := map[string]string{"team": "service", "env": "environment"}
	expectedInstanceTemplate := map[string]interface{}{
		"host": "redis.example.com",
		"port": float64(6380),
		"ssl":  true,
	}
	expectedOtherConfigs := map[string]interface{}{
		"init_config": map[string]interface{}{"baz": "qux"},
		"new_key":     "new_value",
	}

	assert.Equal(t, expectedCheckTags, config.GenerateConfig.CheckTags)
	assert.Equal(t, expectedInstanceTemplate, config.InstanceTemplate)
	assert.Equal(t, expectedOtherConfigs, config.OtherConfigs)
}

// テスト内容: CheckTags に不正な JSON が設定された環境変数を読み込むケース
// 期待する結果: エラーを返す
func TestLoadConfig_InvalidJSONInCheckTags(t *testing.T) {
	// Arrange
	yamlContent := `
generate_config:
  region: "ap-northeast-1"
`
	filePath := createTempFile(t, yamlContent)
	defer os.Remove(filePath)

	t.Setenv("GENERATE_CONFIG_CHECK_TAGS", `{"invalid"}`)

	// Act
	_, err := LoadConfig(filePath)

	// Assert
	require.Error(t, err)
}

// テスト内容: InstanceTemplate に不正な JSON が設定された環境変数を読み込むケース
// 期待する結果: エラーを返す
func TestLoadConfig_InvalidJSONInInstanceTemplate(t *testing.T) {
	// Arrange
	yamlContent := `
generate_config:
  region: "ap-northeast-1"
`
	filePath := createTempFile(t, yamlContent)
	defer os.Remove(filePath)

	t.Setenv("INSTANCE_TEMPLATE", `{"invalid"}`)

	// Act
	_, err := LoadConfig(filePath)

	// Assert
	require.Error(t, err)
}

// テスト内容: OtherConfigs に不正な JSON が設定された環境変数を読み込むケース
// 期待する結果: エラーを返す
func TestLoadConfig_InvalidJSONInOtherConfigs(t *testing.T) {
	// Arrange
	yamlContent := `
generate_config:
  region: "ap-northeast-1"
`
	filePath := createTempFile(t, yamlContent)
	defer os.Remove(filePath)

	t.Setenv("OTHER_CONFIGS", `{"invalid"}`)

	// Act
	_, err := LoadConfig(filePath)

	// Assert
	require.Error(t, err)
}
