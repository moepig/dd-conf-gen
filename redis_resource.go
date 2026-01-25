package ddconfgen

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
)

type RedisNode struct {
	ClusterName string
	ShardName   string
	Endpoint    string
	Tags        map[string]string
}

// ElastiCache API のインターフェース
type ElastiCacheAPI interface {
	DescribeReplicationGroups(ctx context.Context, params *elasticache.DescribeReplicationGroupsInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeReplicationGroupsOutput, error)
}

// Resource Groups Tagging API のインターフェース
type ResourceGroupsTaggingAPI interface {
	GetResources(ctx context.Context, params *resourcegroupstaggingapi.GetResourcesInput, optFns ...func(*resourcegroupstaggingapi.Options)) (*resourcegroupstaggingapi.GetResourcesOutput, error)
}

// Datadog Agent チェック設定の組み立てに必要なRedis ノード情報の取得
func GetRedisNodes(ctx context.Context, region string, tags map[string]string) ([]RedisNode, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	taggingClient := resourcegroupstaggingapi.NewFromConfig(cfg)
	elasticacheClient := elasticache.NewFromConfig(cfg)

	return getRedisNodesWithClients(ctx, taggingClient, elasticacheClient, tags)
}

// Redis ノード情報の取得
func getRedisNodesWithClients(ctx context.Context, taggingClient ResourceGroupsTaggingAPI, elasticacheClient ElastiCacheAPI, tags map[string]string) ([]RedisNode, error) {
	// Replication Group の ARN とタグを取得
	resourceTagMappings, err := getReplicationGroupARNsByTagsWithClient(ctx, taggingClient, tags)
	if err != nil {
		return nil, err
	}

	if len(resourceTagMappings) == 0 {
		return []RedisNode{}, nil
	}

	// ARN からタグマップを作成
	arnToTags := make(map[string]map[string]string)
	var replicationGroupARNs []string
	for _, mapping := range resourceTagMappings {
		arn := *mapping.ResourceARN
		replicationGroupARNs = append(replicationGroupARNs, arn)

		// タグを map に変換
		tagsMap := make(map[string]string)
		for _, tag := range mapping.Tags {
			if tag.Key != nil && tag.Value != nil {
				tagsMap[*tag.Key] = *tag.Value
			}
		}
		arnToTags[arn] = tagsMap
	}

	// ARN から Replication Group ID を抽出
	replicationGroupIDs := extractReplicationGroupIDsFromARNs(replicationGroupARNs)

	// ARN と ID のマッピングを作成
	idToARN := make(map[string]string)
	for i, arn := range replicationGroupARNs {
		idToARN[replicationGroupIDs[i]] = arn
	}

	// Replication Group ID を使って詳細を取得
	var redisNodes []RedisNode

	// DescribeReplicationGroups は複数の ID を一度に受け付けないので、ループ処理する
	for _, id := range replicationGroupIDs {
		descInput := &elasticache.DescribeReplicationGroupsInput{
			ReplicationGroupId: aws.String(id),
		}
		resp, err := elasticacheClient.DescribeReplicationGroups(ctx, descInput)
		if err != nil {
			return nil, fmt.Errorf("failed to describe replication group %s: %w", id, err)
		}

		// 該当する ARN のタグ情報を取得
		arn := idToARN[id]
		clusterTags := arnToTags[arn]

		nodes := extractNodesFromReplicationGroups(resp.ReplicationGroups, clusterTags)
		redisNodes = append(redisNodes, nodes...)
	}

	return redisNodes, nil
}

// Replication Group から全てのノードを抽出
func extractNodesFromReplicationGroups(replicationGroups []elasticachetypes.ReplicationGroup, clusterTags map[string]string) []RedisNode {
	var redisNodes []RedisNode

	for _, rg := range replicationGroups {
		clusterName := *rg.ReplicationGroupId
		for _, ng := range rg.NodeGroups {
			// Redis/Redis 7 以降は Shard 名が "0001" ~ ではなく、shard-000001 のような名前になることがある
			// NodeGroupIdは "0001" のような形式なので、これを ShardName として使う
			shardName := *ng.NodeGroupId

			for _, member := range ng.NodeGroupMembers {
				// 全てのノードのエンドポイントを取得（プライマリとレプリカ両方）
				if member.ReadEndpoint != nil {
					endpoint := fmt.Sprintf("%s:%d", *member.ReadEndpoint.Address, *member.ReadEndpoint.Port)
					redisNodes = append(redisNodes, RedisNode{
						ClusterName: clusterName,
						ShardName:   shardName,
						Endpoint:    endpoint,
						Tags:        clusterTags,
					})
				}
			}
		}
	}

	return redisNodes
}

// タグの map を AWS TagFilter の配列に変換
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

// ARN から Replication Group ID を抽出する
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

// 指定されたタグに一致する ElastiCache Replication Group の ARN のリストを得る
func getReplicationGroupARNsByTags(ctx context.Context, cfg aws.Config, tags map[string]string) ([]taggingtypes.ResourceTagMapping, error) {
	taggingClient := resourcegroupstaggingapi.NewFromConfig(cfg)
	return getReplicationGroupARNsByTagsWithClient(ctx, taggingClient, tags)
}

// ElastiCache Replication Group の ARN をタグでフィルタリングして取得
func getReplicationGroupARNsByTagsWithClient(ctx context.Context, taggingClient ResourceGroupsTaggingAPI, tags map[string]string) ([]taggingtypes.ResourceTagMapping, error) {
	tagFilters := buildTagFilters(tags)

	getResourcesInput := &resourcegroupstaggingapi.GetResourcesInput{
		ResourceTypeFilters: []string{"elasticache:replicationgroup"},
		TagFilters:          tagFilters,
	}

	// 最初のページを取得
	output, err := taggingClient.GetResources(ctx, getResourcesInput)
	if err != nil {
		return nil, fmt.Errorf("failed to get resources by tags: %w", err)
	}

	return output.ResourceTagMappingList, nil
}
