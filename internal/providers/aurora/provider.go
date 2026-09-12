package aurora

import (
	"context"
	"fmt"
	"maps"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/moepig/dd-conf-gen/internal/logging"
	"github.com/moepig/dd-conf-gen/internal/providers"
)

// Discovers Aurora MySQL instance endpoints using an optional RDS client.
type Provider struct {
	client RDSAPI
}

// Provides DB cluster and instance queries.
type RDSAPI interface {
	// Queries clusters with params and optFns using ctx for cancellation, returning a page or an API error.
	DescribeDBClusters(ctx context.Context, params *rds.DescribeDBClustersInput, optFns ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error)
	// Queries instances with params and optFns using ctx for cancellation, returning a page or an API error.
	DescribeDBInstances(ctx context.Context, params *rds.DescribeDBInstancesInput, optFns ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
}

// Returns an Aurora MySQL provider with no API client configured.
func NewProvider() *Provider {
	return &Provider{}
}

// Returns an Aurora MySQL provider retaining client, which may be nil.
func NewProviderWithClient(client RDSAPI) *Provider {
	return &Provider{client: client}
}

// Returns the resource type "aurora_mysql".
func (p *Provider) Type() string {
	return "aurora_mysql"
}

// Searches settings.region for Aurora MySQL clusters matching all settings.tags and returns their instance endpoints using ctx for cancellation and logging.
//
// Empty filters include untagged clusters. Returns nil and an error on cancellation, invalid responses, AWS configuration failure, or API failure.
func (p *Provider) discover(ctx context.Context, settings discoveryConfig) ([]providers.Resource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	client := p.client
	if client == nil {
		cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(settings.region))
		if err != nil {
			return nil, fmt.Errorf("failed to load AWS config: %w (check AWS credentials and configuration)", err)
		}
		client = rds.NewFromConfig(cfg)
	}
	logger := logging.FromContext(ctx)
	logger.Debug("Starting Aurora MySQL discovery", "region", settings.region, "tag_count", len(settings.tags))
	clusters, err := describeClusters(ctx, client)
	if err != nil {
		return nil, err
	}
	var result []providers.Resource
	for _, cluster := range clusters {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if aws.ToString(cluster.Engine) != "aurora-mysql" {
			continue
		}
		tags := clusterTags(cluster.TagList)
		if !matchesTags(tags, settings.tags) || !settings.conditions.Matches(tags) {
			continue
		}
		id := aws.ToString(cluster.DBClusterIdentifier)
		if id == "" {
			return nil, fmt.Errorf("Aurora MySQL cluster has no identifier")
		}
		instances, err := describeInstances(ctx, client, id)
		if err != nil {
			return nil, fmt.Errorf("failed to discover instances for cluster %s: %w", id, err)
		}
		writers := make(map[string]bool, len(cluster.DBClusterMembers))
		for _, member := range cluster.DBClusterMembers {
			writers[aws.ToString(member.DBInstanceIdentifier)] = aws.ToBool(member.IsClusterWriter)
		}
		for _, instance := range instances {
			if aws.ToString(instance.Engine) != "aurora-mysql" || aws.ToString(instance.DBClusterIdentifier) != id {
				continue
			}
			instanceID := aws.ToString(instance.DBInstanceIdentifier)
			endpoint := instance.Endpoint
			if endpoint == nil || aws.ToString(endpoint.Address) == "" || aws.ToInt32(endpoint.Port) <= 0 || aws.ToInt32(endpoint.Port) > 65535 {
				logger.Warn("DB instance has a missing or invalid endpoint", "db_cluster_id", id, "db_instance_id", instanceID)
				continue
			}
			result = append(result, providers.Resource{
				Host: aws.ToString(endpoint.Address),
				Port: int(aws.ToInt32(endpoint.Port)),
				Tags: maps.Clone(tags),
				Metadata: map[string]interface{}{
					"ClusterName":      id,
					"DBInstanceID":     instanceID,
					"IsWriter":         writers[instanceID],
					"EngineVersion":    aws.ToString(instance.EngineVersion),
					"AvailabilityZone": aws.ToString(instance.AvailabilityZone),
				},
			})
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	logger.Info("Aurora MySQL discovery completed", "total_instances", len(result))
	return result, nil
}

// Retrieves Aurora MySQL cluster pages through client using ctx, returning all clusters or nil and an error on cancellation, API failure, nil responses, or repeated markers.
func describeClusters(ctx context.Context, client RDSAPI) ([]rdstypes.DBCluster, error) {
	input := &rds.DescribeDBClustersInput{Filters: []rdstypes.Filter{{Name: aws.String("engine"), Values: []string{"aurora-mysql"}}}}
	var result []rdstypes.DBCluster
	seen := make(map[string]bool)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := client.DescribeDBClusters(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to describe DB clusters: %w", err)
		}
		if page == nil {
			return nil, fmt.Errorf("empty response describing DB clusters")
		}
		result = append(result, page.DBClusters...)
		marker := aws.ToString(page.Marker)
		if marker == "" {
			return result, nil
		}
		if seen[marker] {
			return nil, fmt.Errorf("repeated pagination marker describing DB clusters")
		}
		seen[marker] = true
		next := *input
		next.Marker = aws.String(marker)
		input = &next
	}
}

// Retrieves instance pages for clusterID through client using ctx, returning all instances or nil and an error on cancellation, API failure, nil responses, or repeated markers.
func describeInstances(ctx context.Context, client RDSAPI, clusterID string) ([]rdstypes.DBInstance, error) {
	input := &rds.DescribeDBInstancesInput{Filters: []rdstypes.Filter{{Name: aws.String("db-cluster-id"), Values: []string{clusterID}}}}
	var result []rdstypes.DBInstance
	seen := make(map[string]bool)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := client.DescribeDBInstances(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to describe DB instances: %w", err)
		}
		if page == nil {
			return nil, fmt.Errorf("empty response describing DB instances")
		}
		result = append(result, page.DBInstances...)
		marker := aws.ToString(page.Marker)
		if marker == "" {
			return result, nil
		}
		if seen[marker] {
			return nil, fmt.Errorf("repeated pagination marker describing DB instances")
		}
		seen[marker] = true
		next := *input
		next.Marker = aws.String(marker)
		input = &next
	}
}

// Returns an independent map of complete tag pairs from tags; later duplicate keys replace earlier values.
func clusterTags(tags []rdstypes.Tag) map[string]string {
	result := make(map[string]string, len(tags))
	for _, tag := range tags {
		if tag.Key != nil && tag.Value != nil {
			result[*tag.Key] = *tag.Value
		}
	}
	return result
}

// Returns whether tags contains every filter key with its exact value, including empty values.
func matchesTags(tags, filters map[string]string) bool {
	for key, want := range filters {
		if got, exists := tags[key]; !exists || got != want {
			return false
		}
	}
	return true
}
