package elasticache

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	elasticachetypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	taggingtypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/moepig/dd-conf-gen/resources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockElastiCacheClient is a mock implementation of ElastiCacheAPI
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

// MockResourceGroupsTaggingClient is a mock implementation of ResourceGroupsTaggingAPI
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

func TestProvider_Type(t *testing.T) {
	provider := NewProvider()
	assert.Equal(t, "elasticache_redis", provider.Type())
}

func TestProvider_ValidateConfig(t *testing.T) {
	provider := NewProvider()

	t.Run("valid config", func(t *testing.T) {
		cfg := resources.ProviderConfig{
			Region: "us-east-1",
		}
		err := provider.ValidateConfig(cfg)
		assert.NoError(t, err)
	})

	t.Run("missing region", func(t *testing.T) {
		cfg := resources.ProviderConfig{}
		err := provider.ValidateConfig(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "region is required")
	})

	t.Run("valid with tags filter", func(t *testing.T) {
		cfg := resources.ProviderConfig{
			Region: "us-east-1",
			Filters: map[string]interface{}{
				"tags": map[string]interface{}{
					"Environment": "production",
				},
			},
		}
		err := provider.ValidateConfig(cfg)
		assert.NoError(t, err)
	})

	t.Run("invalid tags filter type", func(t *testing.T) {
		cfg := resources.ProviderConfig{
			Region: "us-east-1",
			Filters: map[string]interface{}{
				"tags": "invalid",
			},
		}
		err := provider.ValidateConfig(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "filters.tags must be a map")
	})
}

func TestProvider_Discover(t *testing.T) {
	t.Run("successful discovery", func(t *testing.T) {
		mockTagging := new(MockResourceGroupsTaggingClient)
		mockElastiCache := new(MockElastiCacheClient)
		ctx := context.Background()

		provider := NewProvider()
		provider.taggingClient = mockTagging
		provider.elasticacheClient = mockElastiCache

		// Setup mocks
		taggingOutput := &resourcegroupstaggingapi.GetResourcesOutput{
			ResourceTagMappingList: []taggingtypes.ResourceTagMapping{
				{
					ResourceARN: aws.String("arn:aws:elasticache:ap-northeast-1:123456789012:replicationgroup:my-cluster"),
					Tags: []taggingtypes.Tag{
						{Key: aws.String("Environment"), Value: aws.String("production")},
						{Key: aws.String("Team"), Value: aws.String("backend")},
					},
				},
			},
		}
		mockTagging.On("GetResources", ctx, mock.Anything, mock.Anything).Return(taggingOutput, nil)

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
		mockElastiCache.On("DescribeReplicationGroups", ctx, mock.Anything, mock.Anything).Return(elasticacheOutput, nil)

		cfg := resources.ProviderConfig{
			Region: "ap-northeast-1",
			Filters: map[string]interface{}{
				"tags": map[string]interface{}{
					"Environment": "production",
				},
			},
		}

		result, err := provider.Discover(ctx, cfg)
		require.NoError(t, err)
		require.Len(t, result, 1)

		resource := result[0]
		assert.Equal(t, "my-cluster.abc123.0001.apne1.cache.amazonaws.com", resource.Host)
		assert.Equal(t, 6379, resource.Port)
		assert.Equal(t, "production", resource.Tags["Environment"])
		assert.Equal(t, "backend", resource.Tags["Team"])
		assert.Equal(t, "my-cluster", resource.Metadata["ClusterName"])
		assert.Equal(t, "0001", resource.Metadata["ShardName"])
		assert.Equal(t, true, resource.Metadata["IsPrimary"])

		mockTagging.AssertExpectations(t)
		mockElastiCache.AssertExpectations(t)
	})

	t.Run("no matching resources", func(t *testing.T) {
		mockTagging := new(MockResourceGroupsTaggingClient)
		mockElastiCache := new(MockElastiCacheClient)
		ctx := context.Background()

		provider := NewProvider()
		provider.taggingClient = mockTagging
		provider.elasticacheClient = mockElastiCache

		taggingOutput := &resourcegroupstaggingapi.GetResourcesOutput{
			ResourceTagMappingList: []taggingtypes.ResourceTagMapping{},
		}
		mockTagging.On("GetResources", ctx, mock.Anything, mock.Anything).Return(taggingOutput, nil)

		cfg := resources.ProviderConfig{
			Region: "us-east-1",
			Filters: map[string]interface{}{
				"tags": map[string]interface{}{
					"Environment": "test",
				},
			},
		}

		result, err := provider.Discover(ctx, cfg)
		require.NoError(t, err)
		assert.Len(t, result, 0)

		mockTagging.AssertExpectations(t)
		mockElastiCache.AssertNotCalled(t, "DescribeReplicationGroups")
	})

	t.Run("multiple shards", func(t *testing.T) {
		mockTagging := new(MockResourceGroupsTaggingClient)
		mockElastiCache := new(MockElastiCacheClient)
		ctx := context.Background()

		provider := NewProvider()
		provider.taggingClient = mockTagging
		provider.elasticacheClient = mockElastiCache

		taggingOutput := &resourcegroupstaggingapi.GetResourcesOutput{
			ResourceTagMappingList: []taggingtypes.ResourceTagMapping{
				{
					ResourceARN: aws.String("arn:aws:elasticache:ap-northeast-1:123456789012:replicationgroup:multi-shard"),
					Tags: []taggingtypes.Tag{
						{Key: aws.String("env"), Value: aws.String("prod")},
					},
				},
			},
		}
		mockTagging.On("GetResources", ctx, mock.Anything, mock.Anything).Return(taggingOutput, nil)

		elasticacheOutput := &elasticache.DescribeReplicationGroupsOutput{
			ReplicationGroups: []elasticachetypes.ReplicationGroup{
				{
					ReplicationGroupId: aws.String("multi-shard"),
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
		mockElastiCache.On("DescribeReplicationGroups", ctx, mock.Anything, mock.Anything).Return(elasticacheOutput, nil)

		cfg := resources.ProviderConfig{
			Region: "ap-northeast-1",
			Filters: map[string]interface{}{
				"tags": map[string]interface{}{
					"env": "prod",
				},
			},
		}

		result, err := provider.Discover(ctx, cfg)
		require.NoError(t, err)
		require.Len(t, result, 2)

		assert.Equal(t, "shard1.cache.amazonaws.com", result[0].Host)
		assert.Equal(t, "shard2.cache.amazonaws.com", result[1].Host)
		assert.Equal(t, "0001", result[0].Metadata["ShardName"])
		assert.Equal(t, "0002", result[1].Metadata["ShardName"])

		mockTagging.AssertExpectations(t)
		mockElastiCache.AssertExpectations(t)
	})

	t.Run("primary and replica nodes", func(t *testing.T) {
		mockTagging := new(MockResourceGroupsTaggingClient)
		mockElastiCache := new(MockElastiCacheClient)
		ctx := context.Background()

		provider := NewProvider()
		provider.taggingClient = mockTagging
		provider.elasticacheClient = mockElastiCache

		taggingOutput := &resourcegroupstaggingapi.GetResourcesOutput{
			ResourceTagMappingList: []taggingtypes.ResourceTagMapping{
				{
					ResourceARN: aws.String("arn:aws:elasticache:ap-northeast-1:123456789012:replicationgroup:test-cluster"),
					Tags:        []taggingtypes.Tag{},
				},
			},
		}
		mockTagging.On("GetResources", ctx, mock.Anything, mock.Anything).Return(taggingOutput, nil)

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
		mockElastiCache.On("DescribeReplicationGroups", ctx, mock.Anything, mock.Anything).Return(elasticacheOutput, nil)

		cfg := resources.ProviderConfig{
			Region: "ap-northeast-1",
		}

		result, err := provider.Discover(ctx, cfg)
		require.NoError(t, err)
		require.Len(t, result, 2, "Should return both primary and replica")

		hosts := []string{result[0].Host, result[1].Host}
		assert.Contains(t, hosts, "primary.cache.amazonaws.com")
		assert.Contains(t, hosts, "replica.cache.amazonaws.com")

		// Check IsPrimary flag
		for _, res := range result {
			if res.Host == "primary.cache.amazonaws.com" {
				assert.Equal(t, true, res.Metadata["IsPrimary"])
			} else {
				assert.Equal(t, false, res.Metadata["IsPrimary"])
			}
		}

		mockTagging.AssertExpectations(t)
		mockElastiCache.AssertExpectations(t)
	})

	t.Run("invalid config", func(t *testing.T) {
		provider := NewProvider()
		cfg := resources.ProviderConfig{}

		_, err := provider.Discover(context.Background(), cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "region is required")
	})
}

func TestExtractTagFilters(t *testing.T) {
	t.Run("extract tags from filters", func(t *testing.T) {
		filters := map[string]interface{}{
			"tags": map[string]interface{}{
				"Environment": "production",
				"Team":        "backend",
			},
		}

		result := extractTagFilters(filters)
		assert.Len(t, result, 2)
		assert.Equal(t, "production", result["Environment"])
		assert.Equal(t, "backend", result["Team"])
	})

	t.Run("no tags in filters", func(t *testing.T) {
		filters := map[string]interface{}{
			"other": "value",
		}

		result := extractTagFilters(filters)
		assert.Len(t, result, 0)
	})

	t.Run("nil filters", func(t *testing.T) {
		result := extractTagFilters(nil)
		assert.Len(t, result, 0)
	})

	t.Run("non-string tag values", func(t *testing.T) {
		filters := map[string]interface{}{
			"tags": map[string]interface{}{
				"String": "value",
				"Number": 123,
				"Bool":   true,
			},
		}

		result := extractTagFilters(filters)
		assert.Len(t, result, 1)
		assert.Equal(t, "value", result["String"])
	})
}

func TestBuildTagFilters(t *testing.T) {
	t.Run("build tag filters", func(t *testing.T) {
		tags := map[string]string{
			"Environment": "production",
			"Team":        "backend",
		}

		tagFilters := buildTagFilters(tags)
		assert.Len(t, tagFilters, 2)

		tagMap := make(map[string]string)
		for _, filter := range tagFilters {
			require.NotNil(t, filter.Key)
			require.Len(t, filter.Values, 1)
			tagMap[*filter.Key] = filter.Values[0]
		}

		assert.Equal(t, "production", tagMap["Environment"])
		assert.Equal(t, "backend", tagMap["Team"])
	})

	t.Run("empty tags", func(t *testing.T) {
		tags := map[string]string{}
		tagFilters := buildTagFilters(tags)
		assert.Len(t, tagFilters, 0)
	})
}

func TestExtractReplicationGroupIDsFromARNs(t *testing.T) {
	t.Run("extract IDs from ARNs", func(t *testing.T) {
		arns := []string{
			"arn:aws:elasticache:ap-northeast-1:123456789012:replicationgroup:cluster-1",
			"arn:aws:elasticache:us-east-1:999999999999:replicationgroup:cluster-2",
		}

		ids := extractReplicationGroupIDsFromARNs(arns)
		require.Len(t, ids, 2)
		assert.Equal(t, "cluster-1", ids[0])
		assert.Equal(t, "cluster-2", ids[1])
	})

	t.Run("empty ARNs", func(t *testing.T) {
		arns := []string{}
		ids := extractReplicationGroupIDsFromARNs(arns)
		assert.Len(t, ids, 0)
	})
}
