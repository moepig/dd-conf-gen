package elasticache

import (
	"context"
	"fmt"
	"maps"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	elasticachetypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"github.com/moepig/dd-conf-gen/internal/logging"
	"github.com/moepig/dd-conf-gen/internal/providers"
)

// Extracts a group's nodes, querying cache node endpoints for cluster mode. Returns no partial resources on API or identity errors.
func (p *Provider) extractGroupNodes(ctx context.Context, group elasticachetypes.ReplicationGroup, tags map[string]string) ([]providers.Resource, error) {
	id := aws.ToString(group.ReplicationGroupId)
	if !aws.ToBool(group.ClusterEnabled) {
		return extractNodesFromReplicationGroups(ctx, []elasticachetypes.ReplicationGroup{group}, id, tags), nil
	}
	cache := make(map[string]map[string]*elasticachetypes.Endpoint)
	var result []providers.Resource
	for _, shard := range group.NodeGroups {
		for _, member := range shard.NodeGroupMembers {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			clusterID, nodeID := aws.ToString(member.CacheClusterId), aws.ToString(member.CacheNodeId)
			if clusterID == "" || nodeID == "" {
				return nil, fmt.Errorf("cluster-mode member in replication group %s has no cache cluster or node ID", id)
			}
			nodes, ok := cache[clusterID]
			if !ok {
				var err error
				nodes, err = p.cacheEndpoints(ctx, clusterID)
				if err != nil {
					return nil, err
				}
				cache[clusterID] = nodes
			}
			endpoint := nodes[nodeID]
			if endpoint == nil || aws.ToString(endpoint.Address) == "" || aws.ToInt32(endpoint.Port) <= 0 || aws.ToInt32(endpoint.Port) > 65535 {
				logging.FromContext(ctx).Warn("Cache node has a missing or invalid endpoint", "cache_cluster_id", clusterID, "cache_node_id", nodeID)
				continue
			}
			// AWS restricts CurrentRole to cluster mode disabled: "only applicable for Valkey or Redis OSS (cluster mode disabled) replication groups."
			// https://docs.aws.amazon.com/AmazonElastiCache/latest/APIReference/API_NodeGroupMember.html
			result = append(result, providers.Resource{Host: aws.ToString(endpoint.Address), Port: int(aws.ToInt32(endpoint.Port)), Tags: maps.Clone(tags), Metadata: map[string]interface{}{
				"ClusterName": id, "ShardName": aws.ToString(shard.NodeGroupId), "CacheClusterID": clusterID, "RoleKnown": false,
			}})
		}
	}
	return result, nil
}

// Queries every cache cluster page with node details and indexes the requested cluster's endpoints by node ID. Returns nil on API, response, or pagination errors.
func (p *Provider) cacheEndpoints(ctx context.Context, clusterID string) (map[string]*elasticachetypes.Endpoint, error) {
	input := &elasticache.DescribeCacheClustersInput{CacheClusterId: aws.String(clusterID), ShowCacheNodeInfo: aws.Bool(true)}
	result := make(map[string]*elasticachetypes.Endpoint)
	seen := make(map[string]bool)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := p.elasticacheClient.DescribeCacheClusters(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to describe cache cluster %s: %w", clusterID, err)
		}
		if page == nil {
			return nil, fmt.Errorf("empty response describing cache cluster %s", clusterID)
		}
		for _, cluster := range page.CacheClusters {
			if aws.ToString(cluster.CacheClusterId) != clusterID {
				continue
			}
			for _, node := range cluster.CacheNodes {
				if id := aws.ToString(node.CacheNodeId); id != "" {
					result[id] = node.Endpoint
				}
			}
		}
		marker := aws.ToString(page.Marker)
		if marker == "" {
			return result, nil
		}
		if seen[marker] {
			return nil, fmt.Errorf("repeated pagination marker describing cache cluster %s", clusterID)
		}
		seen[marker] = true
		next := *input
		next.Marker = aws.String(marker)
		input = &next
	}
}
