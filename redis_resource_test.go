package ddconfgen

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	elasticachetypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	taggingtypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockElastiCacheClient struct {
	mock.Mock
}

func (m *MockElastiCacheClient) DescribeReplicationGroups(ctx context.Context, params *elasticache.DescribeReplicationGroupsInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeReplicationGroupsOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*elasticache.DescribeReplicationGroupsOutput), args.Error(1)
}

func createMockReplicationGroup(id, nodeGroupID, address string, port int32, role string) elasticachetypes.ReplicationGroup {
	return elasticachetypes.ReplicationGroup{
		ReplicationGroupId: aws.String(id),
		NodeGroups: []elasticachetypes.NodeGroup{
			{
				NodeGroupId: aws.String(nodeGroupID),
				NodeGroupMembers: []elasticachetypes.NodeGroupMember{
					{
						CurrentRole: aws.String(role),
						ReadEndpoint: &elasticachetypes.Endpoint{
							Address: aws.String(address),
							Port:    aws.Int32(port),
						},
					},
				},
			},
		},
	}
}

// テスト内容: MockElastiCacheClient が正しく動作することを検証する
// 期待する結果: モックが期待通りの応答を返し、呼び出しが検証される
func TestMockElastiCacheClient_Usage(t *testing.T) {
	// Arrange: モックとテストデータの準備
	mockClient := new(MockElastiCacheClient)
	ctx := context.Background()

	expectedOutput := &elasticache.DescribeReplicationGroupsOutput{
		ReplicationGroups: []elasticachetypes.ReplicationGroup{
			createMockReplicationGroup("test-cluster", "0001", "test.cache.amazonaws.com", 6379, "primary"),
		},
	}

	mockClient.On("DescribeReplicationGroups", ctx, mock.Anything, mock.Anything).Return(expectedOutput, nil)

	// Act: モックを使用して実行
	output, err := mockClient.DescribeReplicationGroups(ctx, &elasticache.DescribeReplicationGroupsInput{
		ReplicationGroupId: aws.String("test-cluster"),
	}, nil)

	// Assert: 結果の検証
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Len(t, output.ReplicationGroups, 1)
	assert.Equal(t, "test-cluster", *output.ReplicationGroups[0].ReplicationGroupId)

	mockClient.AssertExpectations(t)
}

// テスト内容: モック用の ReplicationGroup データ構造が正しく生成される
// 期待する結果: 生成されたモックデータの各フィールドが期待通りの値になる
func TestCreateReplicationGroupMock(t *testing.T) {
	// Arrange & Act: モックデータの生成
	mockRG := createMockReplicationGroup(
		"test-cluster",
		"0001",
		"test.cache.amazonaws.com",
		6379,
		"primary",
	)

	// Assert: 生成されたデータの検証
	assert.Equal(t, "test-cluster", *mockRG.ReplicationGroupId)
	require.Len(t, mockRG.NodeGroups, 1)
	assert.Equal(t, "0001", *mockRG.NodeGroups[0].NodeGroupId)

	nodeGroup := mockRG.NodeGroups[0]
	require.Len(t, nodeGroup.NodeGroupMembers, 1)

	member := nodeGroup.NodeGroupMembers[0]
	require.NotNil(t, member.CurrentRole)
	assert.Equal(t, "primary", *member.CurrentRole)
	require.NotNil(t, member.ReadEndpoint)
	assert.Equal(t, "test.cache.amazonaws.com", *member.ReadEndpoint.Address)
	require.NotNil(t, member.ReadEndpoint.Port)
	assert.Equal(t, int32(6379), *member.ReadEndpoint.Port)
}

type MockResourceGroupsTaggingClient struct {
	mock.Mock
}

func (m *MockResourceGroupsTaggingClient) GetResources(ctx context.Context, params *resourcegroupstaggingapi.GetResourcesInput, optFns ...func(*resourcegroupstaggingapi.Options)) (*resourcegroupstaggingapi.GetResourcesOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*resourcegroupstaggingapi.GetResourcesOutput), args.Error(1)
}

// テスト内容: MockResourceGroupsTaggingClient が正しく動作することを検証する
// 期待する結果: モックが期待通りの応答を返し、呼び出しが検証される
func TestMockResourceGroupsTaggingClient_Usage(t *testing.T) {
	// Arrange: モックとテストデータの準備
	mockClient := new(MockResourceGroupsTaggingClient)
	ctx := context.Background()

	expectedOutput := &resourcegroupstaggingapi.GetResourcesOutput{
		ResourceTagMappingList: []taggingtypes.ResourceTagMapping{
			{
				ResourceARN: aws.String("arn:aws:elasticache:ap-northeast-1:123456789012:replicationgroup:test-cluster"),
				Tags: []taggingtypes.Tag{
					{
						Key:   aws.String("Environment"),
						Value: aws.String("test"),
					},
				},
			},
		},
	}

	mockClient.On("GetResources", ctx, mock.Anything, mock.Anything).Return(expectedOutput, nil)

	// Act: モックを使用して実行
	output, err := mockClient.GetResources(ctx, &resourcegroupstaggingapi.GetResourcesInput{
		ResourceTypeFilters: []string{"elasticache:replicationgroup"},
	}, nil)

	// Assert: 結果の検証
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Len(t, output.ResourceTagMappingList, 1)
	assert.Equal(t, "arn:aws:elasticache:ap-northeast-1:123456789012:replicationgroup:test-cluster", *output.ResourceTagMappingList[0].ResourceARN)

	mockClient.AssertExpectations(t)
}

// テスト内容: タグの map から AWS TagFilter の配列を正しく構築する
// 期待する結果: 各タグが正しく TagFilter 構造体に変換される
func TestBuildTagFilters(t *testing.T) {
	// Arrange: テストデータの準備
	tags := map[string]string{
		"Environment": "production",
		"Team":        "backend",
	}

	// Act: タグフィルタの構築
	tagFilters := buildTagFilters(tags)

	// Assert: 結果の検証
	assert.Len(t, tagFilters, 2)

	tagMap := make(map[string]string)
	for _, filter := range tagFilters {
		require.NotNil(t, filter.Key)
		require.Len(t, filter.Values, 1)
		tagMap[*filter.Key] = filter.Values[0]
	}

	assert.Equal(t, "production", tagMap["Environment"])
	assert.Equal(t, "backend", tagMap["Team"])
}

// テスト内容: 空のタグ map から空の TagFilter 配列が構築される
// 期待する結果: 空の配列が返される
func TestBuildTagFilters_EmptyTags(t *testing.T) {
	// Arrange: 空のタグマップ
	tags := map[string]string{}

	// Act: タグフィルタの構築
	tagFilters := buildTagFilters(tags)

	// Assert: 空の配列が返される
	assert.Len(t, tagFilters, 0)
}

// テスト内容: モックを使用して GetRedisNodes が正しく動作することを検証する
// 期待する結果: タグに一致する Redis ノードが正しく取得される
func TestGetRedisNodesWithClients(t *testing.T) {
	// Arrange: モックとテストデータの準備
	mockTaggingClient := new(MockResourceGroupsTaggingClient)
	mockElastiCacheClient := new(MockElastiCacheClient)
	ctx := context.Background()

	tags := map[string]string{
		"Environment": "production",
	}

	taggingOutput := &resourcegroupstaggingapi.GetResourcesOutput{
		ResourceTagMappingList: []taggingtypes.ResourceTagMapping{
			{
				ResourceARN: aws.String("arn:aws:elasticache:ap-northeast-1:123456789012:replicationgroup:my-cluster"),
				Tags: []taggingtypes.Tag{
					{
						Key:   aws.String("Environment"),
						Value: aws.String("production"),
					},
					{
						Key:   aws.String("Team"),
						Value: aws.String("backend"),
					},
				},
			},
		},
	}
	mockTaggingClient.On("GetResources", ctx, mock.Anything, mock.Anything).Return(taggingOutput, nil)

	elasticacheOutput := &elasticache.DescribeReplicationGroupsOutput{
		ReplicationGroups: []elasticachetypes.ReplicationGroup{
			{
				ReplicationGroupId: aws.String("my-cluster"),
				NodeGroups: []elasticachetypes.NodeGroup{
					{
						NodeGroupId: aws.String("0001"),
						NodeGroupMembers: []elasticachetypes.NodeGroupMember{
							{
								CurrentRole: aws.String("primary"),
								ReadEndpoint: &elasticachetypes.Endpoint{
									Address: aws.String("my-cluster.abc123.0001.apne1.cache.amazonaws.com"),
									Port:    aws.Int32(6379),
								},
							},
						},
					},
				},
			},
		},
	}
	mockElastiCacheClient.On("DescribeReplicationGroups", ctx, mock.Anything, mock.Anything).Return(elasticacheOutput, nil)

	// Act: Redis ノードの取得
	nodes, err := getRedisNodesWithClients(ctx, mockTaggingClient, mockElastiCacheClient, tags)

	// Assert: 結果の検証
	require.NoError(t, err)
	require.Len(t, nodes, 1)

	node := nodes[0]
	assert.Equal(t, "my-cluster", node.ClusterName)
	assert.Equal(t, "0001", node.ShardName)
	assert.Equal(t, "my-cluster.abc123.0001.apne1.cache.amazonaws.com", node.Host)
	assert.Equal(t, 6379, node.Port)
	assert.Equal(t, map[string]string{"Environment": "production", "Team": "backend"}, node.Tags)

	mockTaggingClient.AssertExpectations(t)
	mockElastiCacheClient.AssertExpectations(t)
}

// テスト内容: タグに一致するリソースがない場合の挙動を検証する
// 期待する結果: 空の配列が返される
func TestGetRedisNodesWithClients_NoMatchingResources(t *testing.T) {
	// Arrange: モックとテストデータの準備
	mockTaggingClient := new(MockResourceGroupsTaggingClient)
	mockElastiCacheClient := new(MockElastiCacheClient)
	ctx := context.Background()

	tags := map[string]string{
		"Environment": "test",
	}

	taggingOutput := &resourcegroupstaggingapi.GetResourcesOutput{
		ResourceTagMappingList: []taggingtypes.ResourceTagMapping{},
	}
	mockTaggingClient.On("GetResources", ctx, mock.Anything, mock.Anything).Return(taggingOutput, nil)

	// Act: Redis ノードの取得
	nodes, err := getRedisNodesWithClients(ctx, mockTaggingClient, mockElastiCacheClient, tags)

	// Assert: 結果の検証
	require.NoError(t, err)
	assert.Len(t, nodes, 0)

	mockTaggingClient.AssertExpectations(t)
	mockElastiCacheClient.AssertNotCalled(t, "DescribeReplicationGroups")
}

// テスト内容: 複数のシャードを持つクラスターから複数のノードが取得される
// 期待する結果: 各シャードのプライマリノードが全て取得される
func TestGetRedisNodesWithClients_MultipleShards(t *testing.T) {
	// Arrange: モックとテストデータの準備
	mockTaggingClient := new(MockResourceGroupsTaggingClient)
	mockElastiCacheClient := new(MockElastiCacheClient)
	ctx := context.Background()

	tags := map[string]string{
		"Environment": "production",
	}

	taggingOutput := &resourcegroupstaggingapi.GetResourcesOutput{
		ResourceTagMappingList: []taggingtypes.ResourceTagMapping{
			{
				ResourceARN: aws.String("arn:aws:elasticache:ap-northeast-1:123456789012:replicationgroup:multi-shard-cluster"),
				Tags: []taggingtypes.Tag{
					{
						Key:   aws.String("Environment"),
						Value: aws.String("production"),
					},
					{
						Key:   aws.String("App"),
						Value: aws.String("multi-shard"),
					},
				},
			},
		},
	}
	mockTaggingClient.On("GetResources", ctx, mock.Anything, mock.Anything).Return(taggingOutput, nil)

	elasticacheOutput := &elasticache.DescribeReplicationGroupsOutput{
		ReplicationGroups: []elasticachetypes.ReplicationGroup{
			{
				ReplicationGroupId: aws.String("multi-shard-cluster"),
				NodeGroups: []elasticachetypes.NodeGroup{
					{
						NodeGroupId: aws.String("0001"),
						NodeGroupMembers: []elasticachetypes.NodeGroupMember{
							{
								CurrentRole: aws.String("primary"),
								ReadEndpoint: &elasticachetypes.Endpoint{
									Address: aws.String("shard1.cache.amazonaws.com"),
									Port:    aws.Int32(6379),
								},
							},
						},
					},
					{
						NodeGroupId: aws.String("0002"),
						NodeGroupMembers: []elasticachetypes.NodeGroupMember{
							{
								CurrentRole: aws.String("primary"),
								ReadEndpoint: &elasticachetypes.Endpoint{
									Address: aws.String("shard2.cache.amazonaws.com"),
									Port:    aws.Int32(6379),
								},
							},
						},
					},
				},
			},
		},
	}
	mockElastiCacheClient.On("DescribeReplicationGroups", ctx, mock.Anything, mock.Anything).Return(elasticacheOutput, nil)

	// Act: Redis ノードの取得
	nodes, err := getRedisNodesWithClients(ctx, mockTaggingClient, mockElastiCacheClient, tags)

	// Assert: 結果の検証
	require.NoError(t, err)
	require.Len(t, nodes, 2)

	assert.Equal(t, "multi-shard-cluster", nodes[0].ClusterName)
	assert.Equal(t, "0001", nodes[0].ShardName)
	assert.Equal(t, "shard1.cache.amazonaws.com", nodes[0].Host)
	assert.Equal(t, 6379, nodes[0].Port)
	assert.Equal(t, map[string]string{"Environment": "production", "App": "multi-shard"}, nodes[0].Tags)

	assert.Equal(t, "multi-shard-cluster", nodes[1].ClusterName)
	assert.Equal(t, "0002", nodes[1].ShardName)
	assert.Equal(t, "shard2.cache.amazonaws.com", nodes[1].Host)
	assert.Equal(t, 6379, nodes[1].Port)
	assert.Equal(t, map[string]string{"Environment": "production", "App": "multi-shard"}, nodes[1].Tags)

	mockTaggingClient.AssertExpectations(t)
	mockElastiCacheClient.AssertExpectations(t)
}

// テスト内容: プライマリとレプリカの両方のノードが取得される
// 期待する結果: プライマリとレプリカの両方が返される
func TestGetRedisNodesWithClients_PrimaryAndReplicaNodes(t *testing.T) {
	// Arrange: モックとテストデータの準備
	mockTaggingClient := new(MockResourceGroupsTaggingClient)
	mockElastiCacheClient := new(MockElastiCacheClient)
	ctx := context.Background()

	tags := map[string]string{
		"Environment": "production",
	}

	taggingOutput := &resourcegroupstaggingapi.GetResourcesOutput{
		ResourceTagMappingList: []taggingtypes.ResourceTagMapping{
			{
				ResourceARN: aws.String("arn:aws:elasticache:ap-northeast-1:123456789012:replicationgroup:test-cluster"),
				Tags: []taggingtypes.Tag{
					{
						Key:   aws.String("Environment"),
						Value: aws.String("production"),
					},
					{
						Key:   aws.String("Service"),
						Value: aws.String("api"),
					},
				},
			},
		},
	}
	mockTaggingClient.On("GetResources", ctx, mock.Anything, mock.Anything).Return(taggingOutput, nil)

	elasticacheOutput := &elasticache.DescribeReplicationGroupsOutput{
		ReplicationGroups: []elasticachetypes.ReplicationGroup{
			{
				ReplicationGroupId: aws.String("test-cluster"),
				NodeGroups: []elasticachetypes.NodeGroup{
					{
						NodeGroupId: aws.String("0001"),
						NodeGroupMembers: []elasticachetypes.NodeGroupMember{
							{
								CurrentRole: aws.String("primary"),
								ReadEndpoint: &elasticachetypes.Endpoint{
									Address: aws.String("primary.cache.amazonaws.com"),
									Port:    aws.Int32(6379),
								},
							},
							{
								CurrentRole: aws.String("replica"),
								ReadEndpoint: &elasticachetypes.Endpoint{
									Address: aws.String("replica.cache.amazonaws.com"),
									Port:    aws.Int32(6379),
								},
							},
						},
					},
				},
			},
		},
	}
	mockElastiCacheClient.On("DescribeReplicationGroups", ctx, mock.Anything, mock.Anything).Return(elasticacheOutput, nil)

	// Act: Redis ノードの取得
	nodes, err := getRedisNodesWithClients(ctx, mockTaggingClient, mockElastiCacheClient, tags)

	// Assert: 結果の検証
	require.NoError(t, err)
	require.Len(t, nodes, 2, "プライマリとレプリカの両方が返されるべき")

	hosts := []string{nodes[0].Host, nodes[1].Host}
	assert.Contains(t, hosts, "primary.cache.amazonaws.com")
	assert.Contains(t, hosts, "replica.cache.amazonaws.com")

	expectedTags := map[string]string{"Environment": "production", "Service": "api"}
	for _, node := range nodes {
		assert.Equal(t, "test-cluster", node.ClusterName)
		assert.Equal(t, "0001", node.ShardName)
		assert.Equal(t, 6379, node.Port)
		assert.Equal(t, expectedTags, node.Tags)
	}

	mockTaggingClient.AssertExpectations(t)
	mockElastiCacheClient.AssertExpectations(t)
}

// テスト内容: ARN から Replication Group ID を正しく抽出する
// 期待する結果: ARN の最後の部分が ID として抽出される
func TestExtractReplicationGroupIDsFromARNs(t *testing.T) {
	// Arrange: テスト用の ARN リスト
	arns := []string{
		"arn:aws:elasticache:ap-northeast-1:123456789012:replicationgroup:cluster-1",
		"arn:aws:elasticache:us-east-1:999999999999:replicationgroup:cluster-2",
	}

	// Act: ID の抽出
	ids := extractReplicationGroupIDsFromARNs(arns)

	// Assert: 結果の検証
	require.Len(t, ids, 2)
	assert.Equal(t, "cluster-1", ids[0])
	assert.Equal(t, "cluster-2", ids[1])
}

// テスト内容: 空の ARN リストから空の ID リストが返される
// 期待する結果: 空の配列が返される
func TestExtractReplicationGroupIDsFromARNs_Empty(t *testing.T) {
	// Arrange: 空の ARN リスト
	arns := []string{}

	// Act: ID の抽出
	ids := extractReplicationGroupIDsFromARNs(arns)

	// Assert: 空の配列が返される
	assert.Len(t, ids, 0)
}

// テスト内容: 実際の AWS 環境から Redis ノード情報を取得する (統合テスト)
// 期待する結果: AWS 認証情報と実際のリソースがある場合、エラーなくノード情報が取得される
func TestGetRedisNodes_Integration(t *testing.T) {
	t.Skip("統合テスト - AWS 認証情報と実際のリソースが必要")

	ctx := context.Background()
	region := "ap-northeast-1"
	tags := map[string]string{
		"Environment": "test",
	}

	nodes, err := GetRedisNodes(ctx, region, tags)
	require.NoError(t, err)
	assert.NotNil(t, nodes)
}
