package elasticache

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	elasticachetypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	taggingtypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/moepig/dd-conf-gen/resources"
)

const providerType = "elasticache_redis"

// Provider implements the resources.Provider interface for ElastiCache Redis
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

// ValidateConfig checks if the provider configuration is valid
func (p *Provider) ValidateConfig(cfg resources.ProviderConfig) error {
	if cfg.Region == "" {
		return fmt.Errorf("region is required")
	}

	// Check if filters contains tags
	if cfg.Filters != nil {
		if _, ok := cfg.Filters["tags"]; ok {
			// tags should be a map
			if _, ok := cfg.Filters["tags"].(map[string]interface{}); !ok {
				return fmt.Errorf("filters.tags must be a map")
			}
		}
	}

	return nil
}

// Discover retrieves ElastiCache Redis resources based on the configuration
func (p *Provider) Discover(ctx context.Context, cfg resources.ProviderConfig) ([]resources.Resource, error) {
	if err := p.ValidateConfig(cfg); err != nil {
		return nil, err
	}

	// Load AWS config
	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(cfg.Region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Initialize clients if not set (for testing, they can be injected)
	if p.taggingClient == nil {
		p.taggingClient = resourcegroupstaggingapi.NewFromConfig(awsCfg)
	}
	if p.elasticacheClient == nil {
		p.elasticacheClient = elasticache.NewFromConfig(awsCfg)
	}

	// Extract tag filters from config
	tags := extractTagFilters(cfg.Filters)

	// Get replication groups by tags
	resourceTagMappings, err := p.getReplicationGroupsByTags(ctx, tags)
	if err != nil {
		return nil, err
	}

	if len(resourceTagMappings) == 0 {
		return []resources.Resource{}, nil
	}

	// Build ARN to tags map
	arnToTags := buildARNToTagsMap(resourceTagMappings)

	// Extract replication group IDs
	var replicationGroupARNs []string
	for _, mapping := range resourceTagMappings {
		replicationGroupARNs = append(replicationGroupARNs, *mapping.ResourceARN)
	}
	replicationGroupIDs := extractReplicationGroupIDsFromARNs(replicationGroupARNs)

	// Build ID to ARN map
	idToARN := make(map[string]string)
	for i, arn := range replicationGroupARNs {
		idToARN[replicationGroupIDs[i]] = arn
	}

	// Describe replication groups and extract nodes
	var result []resources.Resource
	for _, id := range replicationGroupIDs {
		descInput := &elasticache.DescribeReplicationGroupsInput{
			ReplicationGroupId: aws.String(id),
		}
		resp, err := p.elasticacheClient.DescribeReplicationGroups(ctx, descInput)
		if err != nil {
			return nil, fmt.Errorf("failed to describe replication group %s: %w", id, err)
		}

		// Get tags for this ARN
		arn := idToARN[id]
		clusterTags := arnToTags[arn]

		// Apply tag mapping
		mappedTags := applyTagMapping(clusterTags, cfg.TagMapping)

		// Extract nodes from replication groups
		nodes := extractNodesFromReplicationGroups(resp.ReplicationGroups, id, mappedTags)
		result = append(result, nodes...)
	}

	return result, nil
}

// extractTagFilters extracts tag filters from the filters map
func extractTagFilters(filters map[string]interface{}) map[string]string {
	tags := make(map[string]string)
	if filters == nil {
		return tags
	}

	if tagsInterface, ok := filters["tags"]; ok {
		if tagsMap, ok := tagsInterface.(map[string]interface{}); ok {
			for k, v := range tagsMap {
				if strVal, ok := v.(string); ok {
					tags[k] = strVal
				}
			}
		}
	}

	return tags
}

// getReplicationGroupsByTags retrieves replication groups filtered by tags
func (p *Provider) getReplicationGroupsByTags(ctx context.Context, tags map[string]string) ([]taggingtypes.ResourceTagMapping, error) {
	tagFilters := buildTagFilters(tags)

	input := &resourcegroupstaggingapi.GetResourcesInput{
		ResourceTypeFilters: []string{"elasticache:replicationgroup"},
		TagFilters:          tagFilters,
	}

	output, err := p.taggingClient.GetResources(ctx, input, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get resources by tags: %w", err)
	}

	return output.ResourceTagMappingList, nil
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
		if len(parts) > 0 {
			replicationGroupIDs = append(replicationGroupIDs, parts[len(parts)-1])
		}
	}
	return replicationGroupIDs
}

// applyTagMapping applies tag mapping rules to convert AWS tags to Datadog tags
func applyTagMapping(awsTags map[string]string, tagMapping map[string]string) map[string]string {
	result := make(map[string]string)
	for ddTag, awsTag := range tagMapping {
		if value, exists := awsTags[awsTag]; exists {
			result[ddTag] = value
		}
	}
	return result
}

// extractNodesFromReplicationGroups extracts all nodes from replication groups
func extractNodesFromReplicationGroups(replicationGroups []elasticachetypes.ReplicationGroup, clusterName string, tags map[string]string) []resources.Resource {
	var result []resources.Resource

	for _, rg := range replicationGroups {
		for _, ng := range rg.NodeGroups {
			shardName := *ng.NodeGroupId

			for _, member := range ng.NodeGroupMembers {
				// Get all node endpoints (both primary and replica)
				if member.ReadEndpoint != nil {
					isPrimary := false
					if member.CurrentRole != nil && *member.CurrentRole == "primary" {
						isPrimary = true
					}

					resource := resources.Resource{
						Host: *member.ReadEndpoint.Address,
						Port: int(*member.ReadEndpoint.Port),
						Tags: tags,
						Metadata: map[string]interface{}{
							"ClusterName": clusterName,
							"ShardName":   shardName,
							"IsPrimary":   isPrimary,
						},
					}
					result = append(result, resource)
				}
			}
		}
	}

	return result
}
