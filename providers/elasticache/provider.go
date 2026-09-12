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

// Discovers ElastiCache Redis nodes using optional API client references.
type Provider struct {
	elasticacheClient ElastiCacheAPI
	taggingClient     ResourceGroupsTaggingAPI
}

// Provides replication group queries.
type ElastiCacheAPI interface {
	// Queries with params and optFns using ctx for cancellation, returning a response page or an API error.
	DescribeReplicationGroups(ctx context.Context, params *elasticache.DescribeReplicationGroupsInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeReplicationGroupsOutput, error)
}

// Provides resource tag queries.
type ResourceGroupsTaggingAPI interface {
	// Queries with params and optFns using ctx for cancellation, returning a resource tag page or an API error.
	GetResources(ctx context.Context, params *resourcegroupstaggingapi.GetResourcesInput, optFns ...func(*resourcegroupstaggingapi.Options)) (*resourcegroupstaggingapi.GetResourcesOutput, error)
}

// Returns an ElastiCache Redis provider with no API clients configured.
func NewProvider() *Provider {
	return &Provider{}
}

// Returns an ElastiCache Redis provider retaining elasticacheClient and taggingClient. Either client reference may be nil.
func NewProviderWithClients(elasticacheClient ElastiCacheAPI, taggingClient ResourceGroupsTaggingAPI) *Provider {
	return &Provider{elasticacheClient: elasticacheClient, taggingClient: taggingClient}
}

// Returns the resource type "elasticache_redis".
func (p *Provider) Type() string {
	return providerType
}

// Discovers Redis nodes using validated settings and ctx for cancellation and logging.
//
// Returns nodes matching settings.tags in settings.region, or all groups when tags are empty. Returns nil and an error on cancellation, client configuration failure, invalid responses, unsupported cluster mode, or API failure.
func (p *Provider) discover(ctx context.Context, settings discoveryConfig) ([]providers.Resource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	logging.FromContext(ctx).Debug("Starting ElastiCache Redis discovery", "region", settings.region)

	clientProvider, err := p.forRegion(ctx, settings.region)
	if err != nil {
		return nil, err
	}
	p = clientProvider

	tags := settings.tags
	logging.FromContext(ctx).Debug("Extracted tag filters", "tag_count", len(tags))

	resourceTagMappings, err := p.getReplicationGroupsByTags(ctx, tags)
	if err != nil {
		return nil, err
	}

	if len(resourceTagMappings) == 0 && len(tags) > 0 {
		logging.FromContext(ctx).Info("No replication groups found matching tag filters", "tag_count", len(tags))
		return []providers.Resource{}, nil
	}

	logging.FromContext(ctx).Info("Found replication groups by tags", "count", len(resourceTagMappings))

	if len(tags) == 0 {
		arnToTags := buildARNToTagsMap(resourceTagMappings)
		groups, err := p.describeReplicationGroups(ctx, &elasticache.DescribeReplicationGroupsInput{})
		if err != nil {
			return nil, err
		}
		var result []providers.Resource
		for _, group := range groups {
			if !settings.conditions.Matches(arnToTags[aws.ToString(group.ARN)]) {
				continue
			}
			result = append(result, extractNodesFromReplicationGroups(ctx,
				[]elasticachetypes.ReplicationGroup{group}, aws.ToString(group.ReplicationGroupId), arnToTags[aws.ToString(group.ARN)],
			)...)
		}
		return result, nil
	}

	var result []providers.Resource
	for _, mapping := range resourceTagMappings {
		if !settings.conditions.Matches(tagsFromMapping(mapping)) {
			continue
		}
		arn := aws.ToString(mapping.ResourceARN)
		if arn == "" {
			return nil, fmt.Errorf("resource mapping has no ARN")
		}
		id := arn[strings.LastIndexByte(arn, ':')+1:]
		if id == "" {
			return nil, fmt.Errorf("resource mapping ARN has no replication group ID")
		}
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

		nodes := extractNodesFromReplicationGroups(ctx, groups, id, tagsFromMapping(mapping))
		logging.FromContext(ctx).Debug("Extracted nodes from replication group",
			"replication_group_id", id,
			"nodes_count", len(nodes))
		result = append(result, nodes...)
	}

	logging.FromContext(ctx).Info("ElastiCache Redis discovery completed", "total_nodes", len(result))
	return result, nil
}

// Retrieves replication groups across pages using non-nil input and ctx for API cancellation.
//
// Returns all groups without modifying input, or nil and an error for API failure, a nil response, cluster mode, or repeated pagination markers.
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

// Returns a copy of the provider with missing clients initialized for region using ctx.
//
// Existing client references are retained and the receiver is unchanged. Returns nil and an error if AWS configuration cannot be loaded.
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

// Retrieves replication group tag mappings using tags as filters and ctx for cancellation and logging.
//
// Returns mappings from all pages, or nil and an error for an uninitialized tagging client, API failure, a nil response, or repeated pagination tokens. Empty tags apply no tag filters.
func (p *Provider) getReplicationGroupsByTags(ctx context.Context, tags map[string]string) ([]taggingtypes.ResourceTagMapping, error) {
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

// Returns one AWS tag filter per key-value pair in tags, with a single value per filter and unspecified order. Empty tags produce an empty slice.
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

// Indexes resourceTagMappings by nonempty ARN and returns independently owned tag maps.
//
// Mappings without an ARN and tag pairs missing a key or value are omitted. Later mappings replace earlier mappings with the same ARN.
func buildARNToTagsMap(resourceTagMappings []taggingtypes.ResourceTagMapping) map[string]map[string]string {
	arnToTags := make(map[string]map[string]string)
	for _, mapping := range resourceTagMappings {
		if aws.ToString(mapping.ResourceARN) == "" {
			continue
		}
		arnToTags[*mapping.ResourceARN] = tagsFromMapping(mapping)
	}
	return arnToTags
}

// Returns an independently owned map of complete tag pairs from mapping. Pairs with a nil key or value are omitted; later values replace earlier values for duplicate keys.
func tagsFromMapping(mapping taggingtypes.ResourceTagMapping) map[string]string {
	tags := make(map[string]string, len(mapping.Tags))
	for _, tag := range mapping.Tags {
		if tag.Key != nil && tag.Value != nil {
			tags[*tag.Key] = *tag.Value
		}
	}
	return tags
}

// Converts replicationGroups into node resources, using ctx for logging.
//
// Returns nodes with nonempty read addresses and positive ports, preserving group and member order. Each node receives an independent copy of tags and metadata containing clusterName, its shard ID, primary status, and cache cluster ID; incomplete endpoints are omitted.
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
				if member.ReadEndpoint != nil && aws.ToString(member.ReadEndpoint.Address) != "" && aws.ToInt32(member.ReadEndpoint.Port) > 0 {
					isPrimary := aws.ToString(member.CurrentRole) == "primary"

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
