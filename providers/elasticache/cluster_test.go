package elasticache

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	elasticachetypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	taggingtypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/moepig/dd-conf-gen/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Discovers multiple shards from mocked cache node pages, verifying endpoint identity, inherited tags, unknown roles, and filtered and unfiltered searches.
func TestDiscoverClusterMode(t *testing.T) {
	t.Parallel()
	for _, filtered := range []bool{false, true} {
		t.Run(map[bool]string{false: "all", true: "filtered"}[filtered], func(t *testing.T) {
			client, tagging := new(MockElastiCacheClient), new(MockResourceGroupsTaggingClient)
			client.Test(t)
			tagging.Test(t)
			group := elasticachetypes.ReplicationGroup{ReplicationGroupId: aws.String("redis"), ARN: aws.String("arn:redis"), ClusterEnabled: aws.Bool(true)}
			for _, id := range []string{"shard-a", "shard-b"} {
				group.NodeGroups = append(group.NodeGroups, elasticachetypes.NodeGroup{NodeGroupId: aws.String(id), NodeGroupMembers: []elasticachetypes.NodeGroupMember{
					{CacheClusterId: aws.String(id), CacheNodeId: aws.String("0001")},
					{CacheClusterId: aws.String(id), CacheNodeId: aws.String("0002")},
				}})
				client.On("DescribeCacheClusters", mock.Anything, mock.MatchedBy(func(in *elasticache.DescribeCacheClustersInput) bool {
					return aws.ToString(in.CacheClusterId) == id && aws.ToBool(in.ShowCacheNodeInfo) && in.Marker == nil
				}), mock.Anything).Return(&elasticache.DescribeCacheClustersOutput{Marker: aws.String("next"), CacheClusters: []elasticachetypes.CacheCluster{{CacheClusterId: aws.String(id), CacheNodes: []elasticachetypes.CacheNode{
					{CacheNodeId: aws.String("0002"), Endpoint: &elasticachetypes.Endpoint{Address: aws.String(id + "-2"), Port: aws.Int32(6379)}},
				}}}}, nil).Once()
				client.On("DescribeCacheClusters", mock.Anything, mock.MatchedBy(func(in *elasticache.DescribeCacheClustersInput) bool {
					return aws.ToString(in.CacheClusterId) == id && aws.ToBool(in.ShowCacheNodeInfo) && aws.ToString(in.Marker) == "next"
				}), mock.Anything).Return(&elasticache.DescribeCacheClustersOutput{CacheClusters: []elasticachetypes.CacheCluster{{CacheClusterId: aws.String(id), CacheNodes: []elasticachetypes.CacheNode{
					{CacheNodeId: aws.String("0001"), Endpoint: &elasticachetypes.Endpoint{Address: aws.String(id + "-1"), Port: aws.Int32(6379)}},
				}}}}, nil).Once()
			}
			tagging.On("GetResources", mock.Anything, mock.Anything, mock.Anything).Return(&resourcegroupstaggingapi.GetResourcesOutput{ResourceTagMappingList: []taggingtypes.ResourceTagMapping{{ResourceARN: aws.String("arn:redis"), Tags: []taggingtypes.Tag{{Key: aws.String("env"), Value: aws.String("prod")}}}}}, nil).Once()
			client.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{ReplicationGroups: []elasticachetypes.ReplicationGroup{group}}, nil).Once()
			cfg := providers.ProviderConfig{Region: "east"}
			if filtered {
				cfg.Filters = map[string]interface{}{"tags": map[string]interface{}{"env": "prod"}}
			}
			resources, err := discoverForTest(NewProviderWithClients(client, tagging), context.Background(), cfg)
			require.NoError(t, err)
			require.Len(t, resources, 4)
			for i, r := range resources {
				assert.Equal(t, []string{"shard-a-1", "shard-a-2", "shard-b-1", "shard-b-2"}[i], r.Host)
				assert.Equal(t, "prod", r.Tags["env"])
				assert.Equal(t, "redis", r.Metadata["ClusterName"])
				assert.Equal(t, false, r.Metadata["RoleKnown"])
				assert.NotContains(t, r.Metadata, "IsPrimary")
			}
			client.AssertExpectations(t)
			tagging.AssertExpectations(t)
		})
	}
}

// Injects cache API failures, repeated markers, cancellation, and incomplete endpoints; errors must discard partial results while absent endpoints are omitted.
func TestClusterEndpointFailures(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"api", "nil", "marker", "cancel", "missing", "identity"} {
		t.Run(scenario, func(t *testing.T) {
			client := new(MockElastiCacheClient)
			client.Test(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			group := elasticachetypes.ReplicationGroup{ClusterEnabled: aws.Bool(true), NodeGroups: []elasticachetypes.NodeGroup{{NodeGroupMembers: []elasticachetypes.NodeGroupMember{{CacheClusterId: aws.String("cluster"), CacheNodeId: aws.String("0001")}}}}}
			page := &elasticache.DescribeCacheClustersOutput{}
			switch scenario {
			case "api":
				client.On("DescribeCacheClusters", mock.Anything, mock.Anything, mock.Anything).Return(nil, assert.AnError).Once()
			case "nil":
				client.On("DescribeCacheClusters", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Once()
			case "marker":
				page.Marker = aws.String("repeat")
				client.On("DescribeCacheClusters", mock.Anything, mock.Anything, mock.Anything).Return(page, nil).Twice()
			case "cancel":
				cancel()
			case "missing":
				client.On("DescribeCacheClusters", mock.Anything, mock.Anything, mock.Anything).Return(page, nil).Once()
			case "identity":
				group.NodeGroups[0].NodeGroupMembers[0].CacheNodeId = nil
			}
			resources, err := NewProviderWithClients(client, nil).extractGroupNodes(ctx, group, nil)
			if scenario == "missing" {
				require.NoError(t, err)
				assert.Empty(t, resources)
			} else {
				require.Error(t, err)
				assert.Nil(t, resources)
			}
			if scenario == "cancel" {
				assert.ErrorIs(t, err, context.Canceled)
			}
			client.AssertExpectations(t)
		})
	}
}
