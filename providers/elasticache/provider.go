package elasticache

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	elasticachetypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	taggingtypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/moepig/dd-conf-gen/internal/logging"
	"github.com/moepig/dd-conf-gen/providers"
)

const providerType = "elasticache_redis"

// Provider implements the providers.Provider interface for ElastiCache Redis
type Provider struct {
	elasticacheClient ElastiCacheAPI
	taggingClient     ResourceGroupsTaggingAPI
}

// ElastiCacheAPI defines the ElastiCache API interface
type ElastiCacheAPI interface {
	DescribeReplicationGroups(ctx context.Context, params *elasticache.DescribeReplicationGroupsInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeReplicationGroupsOutput, error)
}

// ResourceGroupsTaggingAPI defines the Resource Groups Tagging API interface
type ResourceGroupsTaggingAPI interface {
	GetResources(ctx context.Context, params *resourcegroupstaggingapi.GetResourcesInput, optFns ...func(*resourcegroupstaggingapi.Options)) (*resourcegroupstaggingapi.GetResourcesOutput, error)
}

// NewProvider creates a new ElastiCache provider
func NewProvider() *Provider {
	return &Provider{}
}

// Type returns the resource type handled by this provider
func (p *Provider) Type() string {
	return providerType
}

// Discover retrieves ElastiCache Redis resources based on the configuration
func (p *Provider) Discover(ctx context.Context, cfg providers.ProviderConfig) ([]providers.Resource, error) {
	logging.FromContext(ctx).Debug("Starting ElastiCache Redis discovery", "region", cfg.Region)

	settings, err := parseConfig(cfg)
	if err != nil {
		return nil, err
	}

	clientProvider, err := p.forRegion(ctx, settings.region)
	if err != nil {
		return nil, err
	}
	p = clientProvider

	// Extract tag filters from config
	tags := settings.tags
	logging.FromContext(ctx).Debug("Extracted tag filters", "tag_count", len(tags), "tags", tags)

	// Get replication groups by tags
	resourceTagMappings, err := p.getReplicationGroupsByTags(ctx, tags)
	if err != nil {
		return nil, err
	}

	if len(resourceTagMappings) == 0 && len(tags) > 0 {
		logging.FromContext(ctx).Info("No replication groups found matching tag filters", "tags", tags)
		return []providers.Resource{}, nil
	}

	logging.FromContext(ctx).Info("Found replication groups by tags", "count", len(resourceTagMappings))

	// Build ARN to tags map
	arnToTags := buildARNToTagsMap(resourceTagMappings)
	if len(tags) == 0 {
		groups, err := p.describeReplicationGroups(ctx, &elasticache.DescribeReplicationGroupsInput{})
		if err != nil {
			return nil, err
		}
		var result []providers.Resource
		for _, group := range groups {
			result = append(result, extractNodesFromReplicationGroups(ctx,
				[]elasticachetypes.ReplicationGroup{group}, aws.ToString(group.ReplicationGroupId), arnToTags[aws.ToString(group.ARN)],
			)...)
		}
		return result, nil
	}

	// Extract replication group IDs
	var replicationGroupARNs []string
	for _, mapping := range resourceTagMappings {
		if aws.ToString(mapping.ResourceARN) == "" {
			return nil, fmt.Errorf("resource mapping has no ARN")
		}
		replicationGroupARNs = append(replicationGroupARNs, *mapping.ResourceARN)
	}
	replicationGroupIDs := extractReplicationGroupIDsFromARNs(replicationGroupARNs)
	logging.FromContext(ctx).Debug("Extracted replication group IDs", "ids", replicationGroupIDs)

	// Build ID to ARN map
	idToARN := make(map[string]string)
	for i, arn := range replicationGroupARNs {
		idToARN[replicationGroupIDs[i]] = arn
	}

	// Describe replication groups and extract nodes
	var result []providers.Resource
	for _, id := range replicationGroupIDs {
		logging.FromContext(ctx).Debug("Describing replication group", "replication_group_id", id)

		descInput := &elasticache.DescribeReplicationGroupsInput{
			ReplicationGroupId: aws.String(id),
		}

		groups, err := p.describeReplicationGroups(ctx, descInput)
		if err != nil {
			return nil, fmt.Errorf("failed to describe replication group %s: %w", id, err)
		}

		if len(groups) == 0 {
			logging.FromContext(ctx).Warn("No replication group details found", "replication_group_id", id)
			continue
		}

		logging.FromContext(ctx).Debug("Retrieved replication group details",
			"replication_group_id", id,
			"node_groups_count", len(groups[0].NodeGroups))

		// Get tags for this ARN (pass all tags as-is)
		arn := idToARN[id]
		clusterTags := arnToTags[arn]

		// Extract nodes from replication groups
		nodes := extractNodesFromReplicationGroups(ctx, groups, id, clusterTags)
		logging.FromContext(ctx).Debug("Extracted nodes from replication group",
			"replication_group_id", id,
			"nodes_count", len(nodes))
		result = append(result, nodes...)
	}

	logging.FromContext(ctx).Info("ElastiCache Redis discovery completed", "total_nodes", len(result))
	return result, nil
}

// Retrieves all replication group pages, returning no partial result on failure.
func (p *Provider) describeReplicationGroups(ctx context.Context, input *elasticache.DescribeReplicationGroupsInput) ([]elasticachetypes.ReplicationGroup, error) {
	var result []elasticachetypes.ReplicationGroup
	seenMarkers := make(map[string]bool)
	for {
		output, err := p.elasticacheClient.DescribeReplicationGroups(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to describe replication groups: %w", err)
		}
		if output == nil {
			return nil, fmt.Errorf("empty response describing replication groups")
		}
		for _, group := range output.ReplicationGroups {
			if aws.ToBool(group.ClusterEnabled) {
				return nil, fmt.Errorf("replication group %s uses unsupported cluster mode", aws.ToString(group.ReplicationGroupId))
			}
		}
		result = append(result, output.ReplicationGroups...)
		marker := aws.ToString(output.Marker)
		if marker == "" {
			return result, nil
		}
		if seenMarkers[marker] {
			return nil, fmt.Errorf("repeated pagination marker describing replication groups")
		}
		seenMarkers[marker] = true
		nextInput := *input
		nextInput.Marker = aws.String(marker)
		input = &nextInput
	}
}

// Returns clients for the requested region while preserving injected clients.
func (p *Provider) forRegion(ctx context.Context, region string) (*Provider, error) {
	local := *p
	if local.taggingClient != nil && local.elasticacheClient != nil {
		return &local, nil
	}
	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w (check AWS credentials and configuration)", err)
	}
	if local.taggingClient == nil {
		local.taggingClient = resourcegroupstaggingapi.NewFromConfig(awsCfg)
	}
	if local.elasticacheClient == nil {
		local.elasticacheClient = elasticache.NewFromConfig(awsCfg)
	}
	return &local, nil
}

// getReplicationGroupsByTags retrieves replication groups filtered by tags
func (p *Provider) getReplicationGroupsByTags(ctx context.Context, tags map[string]string) ([]taggingtypes.ResourceTagMapping, error) {
	// Ensure tagging client is initialized
	if p.taggingClient == nil {
		return nil, fmt.Errorf("tagging client is not initialized")
	}

	tagFilters := buildTagFilters(tags)
	logging.FromContext(ctx).Debug("Calling GetResources API",
		"resource_type", "elasticache:replicationgroup",
		"tag_filters_count", len(tagFilters))

	input := &resourcegroupstaggingapi.GetResourcesInput{
		ResourceTypeFilters: []string{"elasticache:replicationgroup"},
		TagFilters:          tagFilters,
	}

	var result []taggingtypes.ResourceTagMapping
	seenTokens := make(map[string]bool)
	for {
		output, err := p.taggingClient.GetResources(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to get resources by tags: %w", err)
		}
		if output == nil {
			return nil, fmt.Errorf("empty response getting resources by tags")
		}
		result = append(result, output.ResourceTagMappingList...)
		token := aws.ToString(output.PaginationToken)
		if token == "" {
			break
		}
		if seenTokens[token] {
			return nil, fmt.Errorf("repeated pagination token getting resources by tags")
		}
		seenTokens[token] = true
		nextInput := *input
		nextInput.PaginationToken = aws.String(token)
		input = &nextInput
	}
	return result, nil
}

// buildTagFilters converts a map of tags to AWS TagFilter array
func buildTagFilters(tags map[string]string) []taggingtypes.TagFilter {
	tagFilters := []taggingtypes.TagFilter{}
	for key, value := range tags {
		tagFilters = append(tagFilters, taggingtypes.TagFilter{
			Key:    aws.String(key),
			Values: []string{value},
		})
	}
	return tagFilters
}

// buildARNToTagsMap builds a map from ARN to tags
func buildARNToTagsMap(resourceTagMappings []taggingtypes.ResourceTagMapping) map[string]map[string]string {
	arnToTags := make(map[string]map[string]string)
	for _, mapping := range resourceTagMappings {
		if aws.ToString(mapping.ResourceARN) == "" {
			continue
		}
		arn := *mapping.ResourceARN
		tagsMap := make(map[string]string)
		for _, tag := range mapping.Tags {
			if tag.Key != nil && tag.Value != nil {
				tagsMap[*tag.Key] = *tag.Value
			}
		}
		arnToTags[arn] = tagsMap
	}
	return arnToTags
}

// extractReplicationGroupIDsFromARNs extracts replication group IDs from ARNs
func extractReplicationGroupIDsFromARNs(arns []string) []string {
	replicationGroupIDs := []string{}
	for _, arn := range arns {
		parts := strings.Split(arn, ":")
		replicationGroupIDs = append(replicationGroupIDs, parts[len(parts)-1])
	}
	return replicationGroupIDs
}

// extractNodesFromReplicationGroups extracts all nodes from replication groups
func extractNodesFromReplicationGroups(ctx context.Context, replicationGroups []elasticachetypes.ReplicationGroup, clusterName string, tags map[string]string) []providers.Resource {
	var result []providers.Resource

	for _, rg := range replicationGroups {
		logging.FromContext(ctx).Debug("Processing replication group",
			"replication_group_id", aws.ToString(rg.ReplicationGroupId),
			"node_groups_count", len(rg.NodeGroups))

		for _, ng := range rg.NodeGroups {
			shardName := aws.ToString(ng.NodeGroupId)
			logging.FromContext(ctx).Debug("Processing node group",
				"node_group_id", shardName,
				"members_count", len(ng.NodeGroupMembers))

			for _, member := range ng.NodeGroupMembers {
				// Get all node endpoints (both primary and replica)
				if member.ReadEndpoint != nil && aws.ToString(member.ReadEndpoint.Address) != "" && aws.ToInt32(member.ReadEndpoint.Port) > 0 {
					isPrimary := false
					if member.CurrentRole != nil && *member.CurrentRole == "primary" {
						isPrimary = true
					}

					resource := providers.Resource{
						Host: *member.ReadEndpoint.Address,
						Port: int(*member.ReadEndpoint.Port),
						Tags: maps.Clone(tags),
						Metadata: map[string]interface{}{
							"ClusterName":    clusterName,
							"ShardName":      shardName,
							"IsPrimary":      isPrimary,
							"CacheClusterID": aws.ToString(member.CacheClusterId),
						},
					}

					logging.FromContext(ctx).Debug("Extracted node",
						"host", resource.Host,
						"port", resource.Port,
						"is_primary", isPrimary,
						"shard", shardName)

					result = append(result, resource)
				} else {
					logging.FromContext(ctx).Warn("Node member has a missing or invalid read endpoint",
						"node_group_id", shardName,
						"cache_cluster_id", aws.ToString(member.CacheClusterId))
				}
			}
		}
	}

	return result
}
