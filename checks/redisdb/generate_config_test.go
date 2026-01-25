package redisdb

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// テスト内容: 単一の Redis ノードから Datadog Agent チェック設定を生成する
// 期待する結果: 正しい形式の YAML が生成される
func TestGenerateRedisDBConfig_SingleNode(t *testing.T) {
	// Arrange: テストデータの準備
	nodes := []RedisNode{
		{
			ClusterName: "test-cluster",
			ShardName:   "0001",
			Host:        "redis.example.com",
			Port:        6379,
			Tags: map[string]string{
				"Environment": "production",
				"Team":        "backend",
			},
		},
	}

	config := &Config{
		GenerateConfig: GenerateConfig{
			CheckTags: map[string]string{
				"env":  "Environment",
				"team": "Team",
			},
		},
		InstanceTemplate: map[string]interface{}{
			"username": "%%env_REDIS_USERNAME%%",
			"password": "%%env_REDIS_PASSWORD%%",
		},
		OtherConfigs: map[string]interface{}{
			"init_config": nil,
		},
	}

	// Act: 設定の生成
	yamlBytes, err := GenerateRedisDBConfig(nodes, config)
	require.NoError(t, err)

	// Assert: YAML のパース
	var result map[string]interface{}
	err = yaml.Unmarshal(yamlBytes, &result)
	require.NoError(t, err)

	// init_config の検証
	assert.Nil(t, result["init_config"])

	// instances の検証
	instances, ok := result["instances"].([]interface{})
	require.True(t, ok)
	require.Len(t, instances, 1)

	instance := instances[0].(map[string]interface{})
	assert.Equal(t, "redis.example.com", instance["host"])
	assert.Equal(t, 6379, instance["port"])
	assert.Equal(t, "%%env_REDIS_USERNAME%%", instance["username"])
	assert.Equal(t, "%%env_REDIS_PASSWORD%%", instance["password"])

	// tags の検証
	tags, ok := instance["tags"].([]interface{})
	require.True(t, ok)
	assert.Contains(t, tags, "env:production")
	assert.Contains(t, tags, "team:backend")
}

// テスト内容: 複数の Redis ノードから Datadog Agent チェック設定を生成する
// 期待する結果: 全てのノードに対応する instance が生成される
func TestGenerateRedisDBConfig_MultipleNodes(t *testing.T) {
	// Arrange: テストデータの準備
	nodes := []RedisNode{
		{
			ClusterName: "cluster-1",
			ShardName:   "0001",
			Host:        "redis1.example.com",
			Port:        6379,
			Tags: map[string]string{
				"Environment": "production",
			},
		},
		{
			ClusterName: "cluster-1",
			ShardName:   "0002",
			Host:        "redis2.example.com",
			Port:        6379,
			Tags: map[string]string{
				"Environment": "production",
			},
		},
	}

	config := &Config{
		GenerateConfig: GenerateConfig{
			CheckTags: map[string]string{
				"env": "Environment",
			},
		},
		InstanceTemplate: map[string]interface{}{},
		OtherConfigs:     map[string]interface{}{},
	}

	// Act: 設定の生成
	yamlBytes, err := GenerateRedisDBConfig(nodes, config)
	require.NoError(t, err)

	// Assert: YAML のパース
	var result map[string]interface{}
	err = yaml.Unmarshal(yamlBytes, &result)
	require.NoError(t, err)

	// instances の検証
	instances, ok := result["instances"].([]interface{})
	require.True(t, ok)
	require.Len(t, instances, 2)

	instance1 := instances[0].(map[string]interface{})
	assert.Equal(t, "redis1.example.com", instance1["host"])
	assert.Equal(t, 6379, instance1["port"])

	instance2 := instances[1].(map[string]interface{})
	assert.Equal(t, "redis2.example.com", instance2["host"])
	assert.Equal(t, 6379, instance2["port"])
}

// テスト内容: instance_template の tags と CheckTags が両方適用される
// 期待する結果: 両方のタグがマージされて出力される
func TestGenerateRedisDBConfig_WithTemplateTagsAndCheckTags(t *testing.T) {
	// Arrange: テストデータの準備
	nodes := []RedisNode{
		{
			ClusterName: "test-cluster",
			ShardName:   "0001",
			Host:        "redis.example.com",
			Port:        6379,
			Tags: map[string]string{
				"awsenv": "Production",
			},
		},
	}

	config := &Config{
		GenerateConfig: GenerateConfig{
			CheckTags: map[string]string{
				"env": "awsenv",
			},
		},
		InstanceTemplate: map[string]interface{}{
			"tags": []interface{}{
				"instancetag:bar",
			},
		},
		OtherConfigs: map[string]interface{}{
			"init_config": nil,
		},
	}

	// Act: 設定の生成
	yamlBytes, err := GenerateRedisDBConfig(nodes, config)
	require.NoError(t, err)

	// Assert: YAML のパース
	var result map[string]interface{}
	err = yaml.Unmarshal(yamlBytes, &result)
	require.NoError(t, err)

	// instances の検証
	instances := result["instances"].([]interface{})
	instance := instances[0].(map[string]interface{})

	// tags の検証
	tags, ok := instance["tags"].([]interface{})
	require.True(t, ok)
	assert.Len(t, tags, 2)
	assert.Contains(t, tags, "instancetag:bar")
	assert.Contains(t, tags, "env:Production")
}

// テスト内容: 空のノードリストから設定を生成する
// 期待する結果: instances が空の配列になる
func TestGenerateRedisDBConfig_EmptyNodes(t *testing.T) {
	// Arrange: 空のノードリスト
	nodes := []RedisNode{}

	config := &Config{
		GenerateConfig:   GenerateConfig{},
		InstanceTemplate: map[string]interface{}{},
		OtherConfigs: map[string]interface{}{
			"init_config": nil,
		},
	}

	// Act: 設定の生成
	yamlBytes, err := GenerateRedisDBConfig(nodes, config)
	require.NoError(t, err)

	// Assert: YAML のパース
	var result map[string]interface{}
	err = yaml.Unmarshal(yamlBytes, &result)
	require.NoError(t, err)

	// instances が空の配列
	instances := result["instances"].([]interface{})
	assert.Len(t, instances, 0)
}

// テスト内容: init_config が存在する場合に正しく出力される
// 期待する結果: init_config の内容が YAML に含まれる
func TestGenerateRedisDBConfig_WithInitConfig(t *testing.T) {
	// Arrange: init_config を含む設定
	nodes := []RedisNode{
		{
			ClusterName: "test-cluster",
			ShardName:   "0001",
			Host:        "redis.example.com",
			Port:        6379,
			Tags:        map[string]string{},
		},
	}

	config := &Config{
		GenerateConfig:   GenerateConfig{},
		InstanceTemplate: map[string]interface{}{},
		OtherConfigs: map[string]interface{}{
			"init_config": map[string]interface{}{
				"service": "redisdb",
			},
		},
	}

	// Act: 設定の生成
	yamlBytes, err := GenerateRedisDBConfig(nodes, config)
	require.NoError(t, err)

	// Assert: YAML のパース
	var result map[string]interface{}
	err = yaml.Unmarshal(yamlBytes, &result)
	require.NoError(t, err)

	// init_config の検証
	initConfig, ok := result["init_config"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "redisdb", initConfig["service"])
}

// テスト内容: CheckTags に該当するタグがノードに存在しない場合
// 期待する結果: 該当するタグは追加されない
func TestGenerateRedisDBConfig_CheckTagsNotFound(t *testing.T) {
	// Arrange: CheckTags に該当しないタグのみを持つノード
	nodes := []RedisNode{
		{
			ClusterName: "test-cluster",
			ShardName:   "0001",
			Host:        "redis.example.com",
			Port:        6379,
			Tags: map[string]string{
				"OtherTag": "value",
			},
		},
	}

	config := &Config{
		GenerateConfig: GenerateConfig{
			CheckTags: map[string]string{
				"env": "Environment",
			},
		},
		InstanceTemplate: map[string]interface{}{},
		OtherConfigs:     map[string]interface{}{},
	}

	// Act: 設定の生成
	yamlBytes, err := GenerateRedisDBConfig(nodes, config)
	require.NoError(t, err)

	// Assert: YAML のパース
	var result map[string]interface{}
	err = yaml.Unmarshal(yamlBytes, &result)
	require.NoError(t, err)

	// instances の検証
	instances := result["instances"].([]interface{})
	instance := instances[0].(map[string]interface{})

	// tags が存在しない（空のため追加されない）
	_, exists := instance["tags"]
	assert.False(t, exists)
}

// テスト内容: buildInstance が RedisNode から正しく instance を構築する
// 期待する結果: host, port, tags が正しく設定される
func TestBuildInstance(t *testing.T) {
	// Arrange: テストデータの準備
	node := RedisNode{
		ClusterName: "test-cluster",
		ShardName:   "0001",
		Host:        "redis.example.com",
		Port:        6379,
		Tags: map[string]string{
			"Environment": "production",
		},
	}

	config := &Config{
		GenerateConfig: GenerateConfig{
			CheckTags: map[string]string{
				"env": "Environment",
			},
		},
		InstanceTemplate: map[string]interface{}{
			"username": "user",
			"password": "pass",
		},
	}

	// Act: instance の構築
	instance, err := buildInstance(node, config)
	require.NoError(t, err)

	// Assert: 結果の検証
	assert.Equal(t, "redis.example.com", instance["host"])
	assert.Equal(t, 6379, instance["port"])
	assert.Equal(t, "user", instance["username"])
	assert.Equal(t, "pass", instance["password"])

	tags := instance["tags"].([]string)
	assert.Contains(t, tags, "env:production")
}

// テスト内容: buildTags が instance_template の tags を保持する
// 期待する結果: instance_template の tags が含まれる
func TestBuildTags_WithTemplateTags(t *testing.T) {
	// Arrange: テストデータの準備
	node := RedisNode{
		Tags: map[string]string{},
	}

	config := &Config{
		GenerateConfig: GenerateConfig{
			CheckTags: map[string]string{},
		},
	}

	instance := map[string]interface{}{
		"tags": []interface{}{
			"static:tag1",
			"static:tag2",
		},
	}

	// Act: タグの構築
	tags := buildTags(node, config, instance)

	// Assert: 結果の検証
	assert.Len(t, tags, 2)
	assert.Contains(t, tags, "static:tag1")
	assert.Contains(t, tags, "static:tag2")
}

// テスト内容: buildTags が CheckTags からタグを生成する
// 期待する結果: ノードのタグから Datadog タグが生成される
func TestBuildTags_WithCheckTags(t *testing.T) {
	// Arrange: テストデータの準備
	node := RedisNode{
		Tags: map[string]string{
			"awsenv":  "Production",
			"awsteam": "Backend",
		},
	}

	config := &Config{
		GenerateConfig: GenerateConfig{
			CheckTags: map[string]string{
				"env":  "awsenv",
				"team": "awsteam",
			},
		},
	}

	instance := map[string]interface{}{}

	// Act: タグの構築
	tags := buildTags(node, config, instance)

	// Assert: 結果の検証
	assert.Len(t, tags, 2)
	assert.Contains(t, tags, "env:Production")
	assert.Contains(t, tags, "team:Backend")
}

// テスト内容: buildTags が instance_template の tags と CheckTags を両方適用する
// 期待する結果: 両方のタグがマージされる
func TestBuildTags_WithBothTemplateAndCheckTags(t *testing.T) {
	// Arrange: テストデータの準備
	node := RedisNode{
		Tags: map[string]string{
			"Environment": "production",
		},
	}

	config := &Config{
		GenerateConfig: GenerateConfig{
			CheckTags: map[string]string{
				"env": "Environment",
			},
		},
	}

	instance := map[string]interface{}{
		"tags": []interface{}{
			"instancetag:bar",
		},
	}

	// Act: タグの構築
	tags := buildTags(node, config, instance)

	// Assert: 結果の検証
	assert.Len(t, tags, 2)
	assert.Contains(t, tags, "instancetag:bar")
	assert.Contains(t, tags, "env:production")
}

// テスト内容: buildTags でタグが全く存在しない場合
// 期待する結果: 空の配列が返される
func TestBuildTags_Empty(t *testing.T) {
	// Arrange: テストデータの準備
	node := RedisNode{
		Tags: map[string]string{},
	}

	config := &Config{
		GenerateConfig: GenerateConfig{
			CheckTags: map[string]string{},
		},
	}

	instance := map[string]interface{}{}

	// Act: タグの構築
	tags := buildTags(node, config, instance)

	// Assert: 結果の検証
	assert.Len(t, tags, 0)
}

// テスト内容: buildTags で CheckTags に該当するタグがノードに存在しない場合
// 期待する結果: instance_template のタグのみが返される
func TestBuildTags_CheckTagsNotFoundInNode(t *testing.T) {
	// Arrange: テストデータの準備
	node := RedisNode{
		Tags: map[string]string{
			"OtherTag": "value",
		},
	}

	config := &Config{
		GenerateConfig: GenerateConfig{
			CheckTags: map[string]string{
				"env": "Environment",
			},
		},
	}

	instance := map[string]interface{}{
		"tags": []interface{}{
			"static:tag",
		},
	}

	// Act: タグの構築
	tags := buildTags(node, config, instance)

	// Assert: 結果の検証
	assert.Len(t, tags, 1)
	assert.Contains(t, tags, "static:tag")
}
