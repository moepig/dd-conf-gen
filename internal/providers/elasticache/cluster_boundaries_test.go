package elasticache

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	elasticachetypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Returns a valid endpoint before a later page or member query fails, requiring the original error and no partial resources.
func TestClusterEndpointsDiscardPartialResults(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"page", "member"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			client := new(MockElastiCacheClient)
			client.Test(t)
			group := elasticachetypes.ReplicationGroup{
				ReplicationGroupId: aws.String("redis"), ClusterEnabled: aws.Bool(true),
				NodeGroups: []elasticachetypes.NodeGroup{{NodeGroupMembers: []elasticachetypes.NodeGroupMember{
					{CacheClusterId: aws.String("first"), CacheNodeId: aws.String("0001")},
					{CacheClusterId: aws.String("second"), CacheNodeId: aws.String("0001")},
				}}},
			}
			page := &elasticache.DescribeCacheClustersOutput{CacheClusters: []elasticachetypes.CacheCluster{{
				CacheClusterId: aws.String("first"), CacheNodes: []elasticachetypes.CacheNode{{
					CacheNodeId: aws.String("0001"), Endpoint: &elasticachetypes.Endpoint{Address: aws.String("first.example"), Port: aws.Int32(6379)},
				}},
			}}}
			if stage == "page" {
				page.Marker = aws.String("next")
			}
			first := client.On("DescribeCacheClusters", mock.Anything, mock.MatchedBy(func(in *elasticache.DescribeCacheClustersInput) bool {
				return aws.ToString(in.CacheClusterId) == "first" && in.Marker == nil && aws.ToBool(in.ShowCacheNodeInfo)
			}), mock.Anything).Return(page, nil).Once()
			client.On("DescribeCacheClusters", mock.Anything, mock.MatchedBy(func(in *elasticache.DescribeCacheClustersInput) bool {
				if stage == "page" {
					return aws.ToString(in.CacheClusterId) == "first" && aws.ToString(in.Marker) == "next"
				}
				return aws.ToString(in.CacheClusterId) == "second" && in.Marker == nil
			}), mock.Anything).Return(nil, assert.AnError).Once().NotBefore(first)
			resources, err := NewProviderWithClients(client, nil).extractGroupNodes(context.Background(), group, nil)
			require.ErrorIs(t, err, assert.AnError)
			assert.Nil(t, resources)
			client.AssertExpectations(t)
		})
	}
}

// Supplies unrelated cache identities and endpoint boundary values, requiring only valid requested nodes and independently owned tags.
func TestClusterEndpointIdentityAndPortBoundaries(t *testing.T) {
	t.Parallel()
	client := new(MockElastiCacheClient)
	client.Test(t)
	endpoints := map[string]*elasticachetypes.Endpoint{
		"nil":             nil,
		"missing address": {Port: aws.Int32(6379)},
		"empty address":   {Address: aws.String(""), Port: aws.Int32(6379)},
		"missing port":    {Address: aws.String("node.example")},
		"zero":            {Address: aws.String("node.example"), Port: aws.Int32(0)},
		"negative":        {Address: aws.String("node.example"), Port: aws.Int32(-1)},
		"overflow":        {Address: aws.String("node.example"), Port: aws.Int32(65536)},
		"minimum":         {Address: aws.String("node.example"), Port: aws.Int32(1)},
		"maximum":         {Address: aws.String("node.example"), Port: aws.Int32(65535)},
	}
	group := elasticachetypes.ReplicationGroup{ClusterEnabled: aws.Bool(true), NodeGroups: []elasticachetypes.NodeGroup{{}}}
	cluster := elasticachetypes.CacheCluster{CacheClusterId: aws.String("requested")}
	for id, endpoint := range endpoints {
		group.NodeGroups[0].NodeGroupMembers = append(group.NodeGroups[0].NodeGroupMembers, elasticachetypes.NodeGroupMember{CacheClusterId: aws.String("requested"), CacheNodeId: aws.String(id)})
		cluster.CacheNodes = append(cluster.CacheNodes, elasticachetypes.CacheNode{CacheNodeId: aws.String(id), Endpoint: endpoint})
	}
	group.NodeGroups[0].NodeGroupMembers = append(group.NodeGroups[0].NodeGroupMembers, elasticachetypes.NodeGroupMember{CacheClusterId: aws.String("requested"), CacheNodeId: aws.String("absent")})
	cluster.CacheNodes = append(cluster.CacheNodes, elasticachetypes.CacheNode{Endpoint: endpoints["minimum"]})
	client.On("DescribeCacheClusters", mock.Anything, mock.MatchedBy(func(in *elasticache.DescribeCacheClustersInput) bool {
		return aws.ToString(in.CacheClusterId) == "requested" && aws.ToBool(in.ShowCacheNodeInfo)
	}), mock.Anything).Return(&elasticache.DescribeCacheClustersOutput{CacheClusters: []elasticachetypes.CacheCluster{
		cluster,
		{CacheClusterId: aws.String("unrelated"), CacheNodes: []elasticachetypes.CacheNode{{CacheNodeId: aws.String("absent"), Endpoint: endpoints["minimum"]}}},
	}}, nil).Once()
	tags := map[string]string{"env": "prod"}
	resources, err := NewProviderWithClients(client, nil).extractGroupNodes(context.Background(), group, tags)
	require.NoError(t, err)
	require.Len(t, resources, 2)
	assert.ElementsMatch(t, []int{1, 65535}, []int{resources[0].Port, resources[1].Port})
	resources[0].Tags["env"] = "changed"
	assert.Equal(t, "prod", resources[1].Tags["env"])
	assert.Equal(t, "prod", tags["env"])
	client.AssertExpectations(t)
}
