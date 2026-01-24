package ddconfgen

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

	yamlContent := `
generate_config:
  region: "ap-northeast-1"
  tags:
    env: "test"
    service: "test-service"
other_key:
  foo: "bar"
`
	filePath := createTempFile(t, yamlContent)
	defer os.Remove(filePath)

	config, err := LoadConfig(filePath)
	require.NoError(t, err, "LoadConfig should not return an error")

	expected := &Config{
		GenerateConfig: GenerateConfig{
			Region: "ap-northeast-1",
			Tags:   map[string]string{"env": "test", "service": "test-service"},
		},
		OtherConfigs: map[string]interface{}{"other_key": map[string]interface{}{"foo": "bar"}},
	}

	assert.Equal(t, expected, config)
}

// テスト内容: YAML ファイルの内容を環境変数で上書きする
// 期待する結果: 環境変数の値が優先され、Config 構造体に反映される
func TestLoadConfig_OverrideWithEnvironmentVariables(t *testing.T) {
	yamlContent := `
generate_config:
  region: "ap-northeast-1"
  tags:
    env: "test"
other_key:
  foo: "bar"
`
	filePath := createTempFile(t, yamlContent)
	defer os.Remove(filePath)

	t.Setenv("GENERATE_CONFIG_REGION", "us-west-2")
	t.Setenv("GENERATE_CONFIG_TAGS", `{"env":"prod","team":"backend"}`)

	config, err := LoadConfig(filePath)
	require.NoError(t, err)

	expected := &Config{
		GenerateConfig: GenerateConfig{
			Region: "us-west-2",
			Tags:   map[string]string{"env": "prod", "team": "backend"},
		},
		OtherConfigs: map[string]interface{}{"other_key": map[string]interface{}{"foo": "bar"}},
	}

	assert.Equal(t, expected, config)
}

// テスト内容: 環境変数からのみ設定を読み込む
// 期待する結果: 環境変数の値が正しく Config 構造体にパースされる
func TestLoadConfig_FromEnvironmentVariablesOnly(t *testing.T) {
	t.Setenv("GENERATE_CONFIG_REGION", "eu-central-1")
	t.Setenv("GENERATE_CONFIG_TAGS", `{"team":"frontend"}`)

	config, err := LoadConfig("") // ファイルパスを空にする
	require.NoError(t, err)

	expected := &Config{
		GenerateConfig: GenerateConfig{
			Region: "eu-central-1",
			Tags:   map[string]string{"team": "frontend"},
		},
		OtherConfigs: nil, // 明示的にnilであることを期待
	}

	assert.Equal(t, expected, config)
}

// テスト内容: tags に不正な JSON が設定された環境変数で読み込むケース
// 期待する結果: エラーを返す
func TestLoadConfig_InvalidJSONInEnvironmentVariable(t *testing.T) {
	yamlContent := `
generate_config:
  tags:
    from: "file"
`
	filePath := createTempFile(t, yamlContent)
	defer os.Remove(filePath)

	t.Setenv("GENERATE_CONFIG_TAGS", `{"invalid"}`) // 不正なJSON

	_, err := LoadConfig(filePath)
	require.Error(t, err)
}

// テスト内容: 存在しないファイルパスを指定して読み込む
// 期待する結果: エラーにならず、空の Config 構造体が返される
func TestLoadConfig_FileNotFound(t *testing.T) {
	config, err := LoadConfig("non-existent-file.yaml")
	require.NoError(t, err, "LoadConfig should not return error for non-existent file")

	assert.Equal(t, &Config{}, config)
}

// テスト内容: 不正な形式の YAML ファイルを読み込む
// 期待する結果: `yaml.Unmarshal` エラーが返される
func TestLoadConfig_InvalidYAMLFile(t *testing.T) {
	yamlContent := "this: is: invalid: yaml"
	filePath := createTempFile(t, yamlContent)
	defer os.Remove(filePath)

	_, err := LoadConfig(filePath)
	require.Error(t, err, "Expected error for invalid YAML file")
}
