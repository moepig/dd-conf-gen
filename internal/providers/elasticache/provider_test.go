package elasticache

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	elasticachetypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	taggingtypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/moepig/dd-conf-gen/internal/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Holds mocked replication group API calls and results.
type MockElastiCacheClient struct {
	mock.Mock
}

// Records ctx, params, and optFns and returns the configured replication group response and error.
func (m *MockElastiCacheClient) DescribeReplicationGroups(ctx context.Context, params *elasticache.DescribeReplicationGroupsInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeReplicationGroupsOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*elasticache.DescribeReplicationGroupsOutput), args.Error(1)
}

// Records cache node queries and returns the configured page or error.
func (m *MockElastiCacheClient) DescribeCacheClusters(ctx context.Context, params *elasticache.DescribeCacheClustersInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeCacheClustersOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*elasticache.DescribeCacheClustersOutput), args.Error(1)
}

// Holds mocked resource tag API calls and results.
type MockResourceGroupsTaggingClient struct {
	mock.Mock
}

// Records ctx, params, and optFns and returns the configured resource tag response and error.
func (m *MockResourceGroupsTaggingClient) GetResources(ctx context.Context, params *resourcegroupstaggingapi.GetResourcesInput, optFns ...func(*resourcegroupstaggingapi.Options)) (*resourcegroupstaggingapi.GetResourcesOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*resourcegroupstaggingapi.GetResourcesOutput), args.Error(1)
}

// Prepares cfg with p and executes the search using ctx. Returns resources or a preparation or discovery error.
func discoverForTest(p *Provider, ctx context.Context, cfg providers.ProviderConfig) ([]providers.Resource, error) {
	discover, err := p.Prepare(cfg)
	if err != nil {
		return nil, err
	}
	return discover(ctx)
}

// Mutates raw filters and provider clients after preparation and verifies repeated searches use the original snapshot.
func TestPreparedDiscoveryOwnsSettings(t *testing.T) {
	t.Parallel()
	client := new(MockElastiCacheClient)
	tagging := new(MockResourceGroupsTaggingClient)
	client.Test(t)
	tagging.Test(t)
	p := NewProviderWithClients(client, tagging)
	tags := map[string]interface{}{"env": "prod"}
	cfg := providers.ProviderConfig{Region: "us-east-1", Filters: map[string]interface{}{"tags": tags}}
	discover, err := p.Prepare(cfg)
	require.NoError(t, err)
	require.NotNil(t, discover)
	assert.Empty(t, tagging.Calls)
	assert.Empty(t, client.Calls)
	tags["env"] = 123
	cfg.Filters["unsupported"] = true
	p.elasticacheClient = nil
	p.taggingClient = nil
	tagging.On("GetResources", mock.Anything, mock.MatchedBy(func(input *resourcegroupstaggingapi.GetResourcesInput) bool {
		return len(input.TagFilters) == 1 && aws.ToString(input.TagFilters[0].Key) == "env" && len(input.TagFilters[0].Values) == 1 && input.TagFilters[0].Values[0] == "prod"
	}), mock.Anything).Return(&resourcegroupstaggingapi.GetResourcesOutput{}, nil).Twice()
	for range 2 {
		resources, err := discover(context.Background())
		require.NoError(t, err)
		assert.Empty(t, resources)
	}
	tagging.AssertExpectations(t)
	client.AssertExpectations(t)
	invalid, err := p.Prepare(cfg)
	require.Error(t, err)
	assert.Nil(t, invalid)
}

// Constructs a provider and requires the resource type "elasticache_redis".
func TestProvider_Type(t *testing.T) {
	provider := NewProvider()
	assert.Equal(t, "elasticache_redis", provider.Type())
}

// Extracts two nodes and mutates input and output tag maps; requires independent tag values for each node and the input.
func TestExtractNodesOwnTags(t *testing.T) {
	tags := map[string]string{"env": "prod"}
	groups := []elasticachetypes.ReplicationGroup{{NodeGroups: []elasticachetypes.NodeGroup{{NodeGroupMembers: []elasticachetypes.NodeGroupMember{
		{ReadEndpoint: &elasticachetypes.Endpoint{Address: aws.String("first"), Port: aws.Int32(6379)}},
		{ReadEndpoint: &elasticachetypes.Endpoint{Address: aws.String("second"), Port: aws.Int32(6379)}},
	}}}}}
	nodes := extractNodesFromReplicationGroups(context.Background(), groups, "cluster", tags)
	require.Len(t, nodes, 2)
	nodes[0].Tags["env"] = "test"
	assert.Equal(t, "prod", nodes[1].Tags["env"])
	assert.Equal(t, "prod", tags["env"])
	tags["env"] = "staging"
	assert.Equal(t, "prod", nodes[1].Tags["env"])
}

// Supplies non-string tag values with API mocks that accept no calls and requires validation errors before API access.
func TestProvider_DiscoverRejectsInvalidTagValues(t *testing.T) {
	for _, value := range []interface{}{123, true, nil, []interface{}{"prod"}, map[string]interface{}{"env": "prod"}} {
		t.Run(fmt.Sprintf("%T", value), func(t *testing.T) {
			provider := NewProviderWithClients(new(MockElastiCacheClient), new(MockResourceGroupsTaggingClient))
			_, err := discoverForTest(provider, context.Background(), providers.ProviderConfig{
				Region:  "us-east-1",
				Filters: map[string]interface{}{"tags": map[string]interface{}{"env": value}},
			})
			require.ErrorContains(t, err, "filters.tags.env must be a string")
		})
	}
}

// Mocks two response pages and an optional second-page error; requires all mappings on success and no partial mappings on failure.
func TestProvider_GetReplicationGroupsByTagsPagination(t *testing.T) {
	for _, failSecondPage := range []bool{false, true} {
		t.Run(fmt.Sprintf("second page error=%t", failSecondPage), func(t *testing.T) {
			client := new(MockResourceGroupsTaggingClient)
			provider := NewProviderWithClients(nil, client)
			ctx := context.Background()
			first := taggingtypes.ResourceTagMapping{ResourceARN: aws.String("arn:aws:elasticache:us-east-1:123456789012:replicationgroup:first")}
			second := taggingtypes.ResourceTagMapping{ResourceARN: aws.String("arn:aws:elasticache:us-east-1:123456789012:replicationgroup:second")}
			client.On("GetResources", ctx, mock.MatchedBy(func(input *resourcegroupstaggingapi.GetResourcesInput) bool {
				return aws.ToString(input.PaginationToken) == ""
			}), mock.Anything).Return(&resourcegroupstaggingapi.GetResourcesOutput{
				ResourceTagMappingList: []taggingtypes.ResourceTagMapping{first},
				PaginationToken:        aws.String("next"),
			}, nil).Once()
			secondCall := client.On("GetResources", ctx, mock.MatchedBy(func(input *resourcegroupstaggingapi.GetResourcesInput) bool {
				return aws.ToString(input.PaginationToken) == "next" &&
					assert.Equal(t, []string{"elasticache:replicationgroup"}, input.ResourceTypeFilters) &&
					assert.Equal(t, buildTagFilters(map[string]string{"env": "prod"}), input.TagFilters)
			}), mock.Anything).Once()
			if failSecondPage {
				secondCall.Return(nil, assert.AnError)
			} else {
				secondCall.Return(&resourcegroupstaggingapi.GetResourcesOutput{
					ResourceTagMappingList: []taggingtypes.ResourceTagMapping{second},
				}, nil)
			}
			result, err := provider.getReplicationGroupsByTags(ctx, map[string]string{"env": "prod"})
			if failSecondPage {
				require.ErrorIs(t, err, assert.AnError)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, []taggingtypes.ResourceTagMapping{first, second}, result)
			}
			client.AssertExpectations(t)
		})
	}
}

// Mocks an empty page with a continuation token and a second page; requires continued pagination and rejection of a repeated token.
func TestProvider_GetReplicationGroupsByTagsEmptyPage(t *testing.T) {
	for _, repeated := range []bool{false, true} {
		t.Run(fmt.Sprintf("repeated token=%t", repeated), func(t *testing.T) {
			client := new(MockResourceGroupsTaggingClient)
			provider := NewProviderWithClients(nil, client)
			client.On("GetResources", mock.Anything, mock.MatchedBy(func(input *resourcegroupstaggingapi.GetResourcesInput) bool {
				return aws.ToString(input.PaginationToken) == ""
			}), mock.Anything).Return(&resourcegroupstaggingapi.GetResourcesOutput{PaginationToken: aws.String("next")}, nil).Once()
			mapping := taggingtypes.ResourceTagMapping{ResourceARN: aws.String("arn:aws:elasticache:us-east-1:123456789012:replicationgroup:cluster")}
			output := &resourcegroupstaggingapi.GetResourcesOutput{ResourceTagMappingList: []taggingtypes.ResourceTagMapping{mapping}}
			if repeated {
				output.PaginationToken = aws.String("next")
			}
			client.On("GetResources", mock.Anything, mock.MatchedBy(func(input *resourcegroupstaggingapi.GetResourcesInput) bool {
				return aws.ToString(input.PaginationToken) == "next"
			}), mock.Anything).Return(output, nil).Once()
			result, err := provider.getReplicationGroupsByTags(context.Background(), nil)
			if repeated {
				require.ErrorContains(t, err, "repeated pagination token")
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, []taggingtypes.ResourceTagMapping{mapping}, result)
			}
			client.AssertExpectations(t)
		})
	}
}

// Selects a missing profile in empty AWS configuration files and requires a configuration error with no resources.
func TestProvider_DiscoverAWSConfigError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aws-config")
	require.NoError(t, os.WriteFile(path, nil, 0600))
	t.Setenv("AWS_CONFIG_FILE", path)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", path)
	t.Setenv("AWS_PROFILE", "missing-profile")
	result, err := discoverForTest(NewProvider(), context.Background(), providers.ProviderConfig{Region: "us-east-1"})
	require.ErrorContains(t, err, "failed to load AWS config")
	assert.Nil(t, result)
}

// Injects API mocks while selecting a missing AWS profile and requires successful discovery through those clients.
func TestProvider_DiscoverWithInjectedClients(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "aws-config")
	require.NoError(t, os.WriteFile(configPath, nil, 0600))
	t.Setenv("AWS_CONFIG_FILE", configPath)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", configPath)
	t.Setenv("AWS_PROFILE", "missing-profile")
	client := new(MockResourceGroupsTaggingClient)
	client.On("GetResources", mock.Anything, mock.Anything, mock.Anything).
		Return(&resourcegroupstaggingapi.GetResourcesOutput{}, nil).Once()
	provider := NewProviderWithClients(new(MockElastiCacheClient), client)
	_, err := discoverForTest(provider, context.Background(), providers.ProviderConfig{
		Region:  "us-east-1",
		Filters: map[string]interface{}{"tags": map[string]interface{}{"env": "prod"}},
	})
	require.NoError(t, err)
	client.AssertExpectations(t)
}

// Initializes clients repeatedly for different regions and verifies each client region and unchanged client references on the original provider.
func TestProvider_ClientsUseRequestedRegion(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "aws-config")
	require.NoError(t, os.WriteFile(configPath, nil, 0600))
	t.Setenv("AWS_CONFIG_FILE", configPath)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", configPath)
	t.Setenv("AWS_PROFILE", "")
	provider := NewProvider()
	for _, region := range []string{"us-east-1", "ap-northeast-1", "us-east-1"} {
		local, err := provider.forRegion(context.Background(), region)
		require.NoError(t, err)
		assert.Equal(t, region, local.taggingClient.(*resourcegroupstaggingapi.Client).Options().Region)
		assert.Equal(t, region, local.elasticacheClient.(*elasticache.Client).Options().Region)
	}
	assert.Nil(t, provider.taggingClient)
	assert.Nil(t, provider.elasticacheClient)
}

// Supplies complete and incomplete endpoints and requires only nodes with a nonempty address and positive port.
func TestExtractNodesWithIncompleteEndpoints(t *testing.T) {
	groups := []elasticachetypes.ReplicationGroup{{NodeGroups: []elasticachetypes.NodeGroup{{
		NodeGroupMembers: []elasticachetypes.NodeGroupMember{
			{},
			{ReadEndpoint: &elasticachetypes.Endpoint{}},
			{ReadEndpoint: &elasticachetypes.Endpoint{Address: aws.String("host")}},
			{ReadEndpoint: &elasticachetypes.Endpoint{Port: aws.Int32(6379)}},
			{ReadEndpoint: &elasticachetypes.Endpoint{Address: aws.String("host"), Port: aws.Int32(0)}},
			{ReadEndpoint: &elasticachetypes.Endpoint{Address: aws.String("host"), Port: aws.Int32(6379)}},
		},
	}}}}
	result := extractNodesFromReplicationGroups(context.Background(), groups, "cluster", nil)
	require.Len(t, result, 1)
	assert.Equal(t, "host", result[0].Host)
	assert.Equal(t, 6379, result[0].Port)
	assert.Equal(t, "", result[0].Metadata["ShardName"])
}

// Mocks tagged and untagged groups across pages with absent or empty tag filters; requires both groups and preservation of their tags and cluster names.
func TestProvider_DiscoverWithoutTagFilters(t *testing.T) {
	for _, filters := range []map[string]interface{}{nil, {"tags": map[string]interface{}{}}} {
		t.Run(fmt.Sprint(filters), func(t *testing.T) {
			tagging := new(MockResourceGroupsTaggingClient)
			client := new(MockElastiCacheClient)
			provider := NewProviderWithClients(client, tagging)
			taggedARN := "arn:aws:elasticache:us-east-1:123456789012:replicationgroup:tagged"
			tagging.On("GetResources", mock.Anything, mock.Anything, mock.Anything).Return(&resourcegroupstaggingapi.GetResourcesOutput{
				ResourceTagMappingList: []taggingtypes.ResourceTagMapping{{
					ResourceARN: aws.String(taggedARN),
					Tags:        []taggingtypes.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
				}},
			}, nil).Once()
			for i, id := range []string{"tagged", "untagged"} {
				inputMarker, outputMarker := "", "next"
				if i == 1 {
					inputMarker, outputMarker = "next", ""
				}
				client.On("DescribeReplicationGroups", mock.Anything, mock.MatchedBy(func(input *elasticache.DescribeReplicationGroupsInput) bool {
					return input.ReplicationGroupId == nil && aws.ToString(input.Marker) == inputMarker
				}), mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
					Marker: aws.String(outputMarker),
					ReplicationGroups: []elasticachetypes.ReplicationGroup{{
						ReplicationGroupId: aws.String(id),
						ARN:                aws.String("arn:aws:elasticache:us-east-1:123456789012:replicationgroup:" + id),
						NodeGroups: []elasticachetypes.NodeGroup{{
							NodeGroupId: aws.String("0001"),
							NodeGroupMembers: []elasticachetypes.NodeGroupMember{{
								ReadEndpoint: &elasticachetypes.Endpoint{Address: aws.String(id + ".example.com"), Port: aws.Int32(6379)},
							}},
						}},
					}},
				}, nil).Once()
			}
			result, err := discoverForTest(provider, context.Background(), providers.ProviderConfig{Region: "us-east-1", Filters: filters})
			require.NoError(t, err)
			require.Len(t, result, 2)
			assert.Equal(t, "tagged.example.com", result[0].Host)
			assert.Equal(t, "prod", result[0].Tags["env"])
			assert.Equal(t, "tagged", result[0].Metadata["ClusterName"])
			assert.Equal(t, "untagged.example.com", result[1].Host)
			assert.Empty(t, result[1].Tags)
			assert.Equal(t, "untagged", result[1].Metadata["ClusterName"])
			tagging.AssertExpectations(t)
			client.AssertExpectations(t)
		})
	}
}

// Mocks an empty tagging result and an untagged replication group; requires discovery of the group without tag filters.
func TestProvider_DiscoverOnlyUntaggedResources(t *testing.T) {
	tagging := new(MockResourceGroupsTaggingClient)
	client := new(MockElastiCacheClient)
	tagging.On("GetResources", mock.Anything, mock.Anything, mock.Anything).
		Return(&resourcegroupstaggingapi.GetResourcesOutput{}, nil).Once()
	client.On("DescribeReplicationGroups", mock.Anything, mock.MatchedBy(func(input *elasticache.DescribeReplicationGroupsInput) bool {
		return input.ReplicationGroupId == nil
	}), mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
		ReplicationGroups: []elasticachetypes.ReplicationGroup{{
			ReplicationGroupId: aws.String("untagged"),
			NodeGroups: []elasticachetypes.NodeGroup{{NodeGroupMembers: []elasticachetypes.NodeGroupMember{{
				ReadEndpoint: &elasticachetypes.Endpoint{Address: aws.String("untagged.example.com"), Port: aws.Int32(6379)},
			}}}},
		}},
	}, nil).Once()
	provider := NewProviderWithClients(client, tagging)
	result, err := discoverForTest(provider, context.Background(), providers.ProviderConfig{Region: "us-east-1"})
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "untagged.example.com", result[0].Host)
	assert.Equal(t, "untagged", result[0].Metadata["ClusterName"])
	tagging.AssertExpectations(t)
	client.AssertExpectations(t)
}

// Mocks missing ARNs, missing group IDs, and nil API responses; requires the corresponding errors and no resources.
func TestProvider_DiscoverIncompleteResponses(t *testing.T) {
	for _, name := range []string{"nil tagging response", "missing ARN", "empty ARN", "missing group ID", "nil describe response"} {
		t.Run(name, func(t *testing.T) {
			tagging := new(MockResourceGroupsTaggingClient)
			client := new(MockElastiCacheClient)
			provider := NewProviderWithClients(client, tagging)
			var taggingOutput *resourcegroupstaggingapi.GetResourcesOutput
			expectedError := "empty response getting resources"
			if name != "nil tagging response" {
				mapping := taggingtypes.ResourceTagMapping{}
				expectedError = "resource mapping has no ARN"
				if name == "empty ARN" {
					mapping.ResourceARN = aws.String("")
				} else if name == "missing group ID" {
					mapping.ResourceARN = aws.String("arn:aws:elasticache:us-east-1:123456789012:replicationgroup:")
					expectedError = "resource mapping ARN has no replication group ID"
				} else if name == "nil describe response" {
					mapping.ResourceARN = aws.String("arn:aws:elasticache:us-east-1:123456789012:replicationgroup:cluster")
					expectedError = "empty response describing replication groups"
					client.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Once()
				}
				taggingOutput = &resourcegroupstaggingapi.GetResourcesOutput{ResourceTagMappingList: []taggingtypes.ResourceTagMapping{mapping}}
			}
			tagging.On("GetResources", mock.Anything, mock.Anything, mock.Anything).Return(taggingOutput, nil).Once()
			result, err := discoverForTest(provider, context.Background(), providers.ProviderConfig{
				Region:  "us-east-1",
				Filters: map[string]interface{}{"tags": map[string]interface{}{"env": "prod"}},
			})
			require.ErrorContains(t, err, expectedError)
			assert.Nil(t, result)
			tagging.AssertExpectations(t)
			client.AssertExpectations(t)
		})
	}
}

// Mocks a successful first page followed by an API error, nil response, or repeated marker; requires an error and no partial groups.
func TestProvider_DescribeReplicationGroupsPaginationFailure(t *testing.T) {
	for _, name := range []string{"API error", "nil response", "repeated marker"} {
		t.Run(name, func(t *testing.T) {
			client := new(MockElastiCacheClient)
			provider := NewProviderWithClients(client, nil)
			client.On("DescribeReplicationGroups", mock.Anything, mock.MatchedBy(func(input *elasticache.DescribeReplicationGroupsInput) bool {
				return aws.ToString(input.Marker) == ""
			}), mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
				Marker:            aws.String("next"),
				ReplicationGroups: []elasticachetypes.ReplicationGroup{{ReplicationGroupId: aws.String("cluster")}},
			}, nil).Once()
			call := client.On("DescribeReplicationGroups", mock.Anything, mock.MatchedBy(func(input *elasticache.DescribeReplicationGroupsInput) bool {
				return aws.ToString(input.Marker) == "next" && aws.ToString(input.ReplicationGroupId) == "cluster"
			}), mock.Anything).Once()
			switch name {
			case "API error":
				call.Return(nil, assert.AnError)
			case "nil response":
				call.Return(nil, nil)
			case "repeated marker":
				call.Return(&elasticache.DescribeReplicationGroupsOutput{Marker: aws.String("next")}, nil)
			}
			result, err := provider.describeReplicationGroups(context.Background(), &elasticache.DescribeReplicationGroupsInput{ReplicationGroupId: aws.String("cluster")})
			require.Error(t, err)
			if name == "API error" {
				assert.ErrorIs(t, err, assert.AnError)
			}
			assert.Nil(t, result)
			client.AssertExpectations(t)
		})
	}
}

// Supplies valid settings, a missing region, and a non-map tags filter; requires acceptance of valid settings and the corresponding validation errors.
func TestProvider_Prepare(t *testing.T) {
	provider := NewProvider()

	t.Run("valid config", func(t *testing.T) {
		cfg := providers.ProviderConfig{
			Region: "us-east-1",
		}
		_, err := provider.Prepare(cfg)
		assert.NoError(t, err)
	})

	t.Run("missing region", func(t *testing.T) {
		cfg := providers.ProviderConfig{}
		_, err := provider.Prepare(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "region is required")
	})

	t.Run("valid with tags filter", func(t *testing.T) {
		cfg := providers.ProviderConfig{
			Region: "us-east-1",
			Filters: map[string]interface{}{
				"tags": map[string]interface{}{
					"Environment": "production",
				},
			},
		}
		_, err := provider.Prepare(cfg)
		assert.NoError(t, err)
	})

	t.Run("invalid tags filter type", func(t *testing.T) {
		cfg := providers.ProviderConfig{
			Region: "us-east-1",
			Filters: map[string]interface{}{
				"tags": "invalid",
			},
		}
		_, err := provider.Prepare(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "filters.tags must be a map")
	})
}

// Uses mocked API responses to verify node endpoints, original tags, and metadata, empty results for unmatched filters, and errors for invalid settings or API failures.
func TestProvider_Discover(t *testing.T) {
	t.Run("successful discovery", func(t *testing.T) {
		mockTagging := new(MockResourceGroupsTaggingClient)
		mockElastiCache := new(MockElastiCacheClient)
		ctx := context.Background()

		provider := NewProviderWithClients(mockElastiCache, mockTagging)

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
									CacheClusterId: aws.String("my-cluster-0001-001"),
									CurrentRole:    aws.String("primary"),
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

		cfg := providers.ProviderConfig{
			Region: "ap-northeast-1",
			Filters: map[string]interface{}{
				"tags": map[string]interface{}{
					"Environment": "production",
				},
			},
		}

		result, err := discoverForTest(provider, ctx, cfg)
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
		assert.Equal(t, "my-cluster-0001-001", resource.Metadata["CacheClusterID"])

		mockTagging.AssertExpectations(t)
		mockElastiCache.AssertExpectations(t)
	})

	t.Run("no matching resources", func(t *testing.T) {
		mockTagging := new(MockResourceGroupsTaggingClient)
		mockElastiCache := new(MockElastiCacheClient)
		ctx := context.Background()

		provider := NewProviderWithClients(mockElastiCache, mockTagging)

		taggingOutput := &resourcegroupstaggingapi.GetResourcesOutput{
			ResourceTagMappingList: []taggingtypes.ResourceTagMapping{},
		}
		mockTagging.On("GetResources", ctx, mock.Anything, mock.Anything).Return(taggingOutput, nil)

		cfg := providers.ProviderConfig{
			Region: "us-east-1",
			Filters: map[string]interface{}{
				"tags": map[string]interface{}{
					"Environment": "test",
				},
			},
		}

		result, err := discoverForTest(provider, ctx, cfg)
		require.NoError(t, err)
		assert.Len(t, result, 0)

		mockTagging.AssertExpectations(t)
		mockElastiCache.AssertNotCalled(t, "DescribeReplicationGroups")
	})

	t.Run("multiple shards", func(t *testing.T) {
		mockTagging := new(MockResourceGroupsTaggingClient)
		mockElastiCache := new(MockElastiCacheClient)
		ctx := context.Background()

		provider := NewProviderWithClients(mockElastiCache, mockTagging)

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

		cfg := providers.ProviderConfig{
			Region: "ap-northeast-1",
			Filters: map[string]interface{}{
				"tags": map[string]interface{}{
					"env": "prod",
				},
			},
		}

		result, err := discoverForTest(provider, ctx, cfg)
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

		provider := NewProviderWithClients(mockElastiCache, mockTagging)

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

		cfg := providers.ProviderConfig{
			Region: "ap-northeast-1",
		}

		result, err := discoverForTest(provider, ctx, cfg)
		require.NoError(t, err)
		require.Len(t, result, 2, "Should return both primary and replica")

		hosts := []string{result[0].Host, result[1].Host}
		assert.Contains(t, hosts, "primary.cache.amazonaws.com")
		assert.Contains(t, hosts, "replica.cache.amazonaws.com")

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
		cfg := providers.ProviderConfig{}

		_, err := discoverForTest(provider, context.Background(), cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "region is required")
	})

	t.Run("tagging client error", func(t *testing.T) {
		mockTagging := new(MockResourceGroupsTaggingClient)
		ctx := context.Background()

		provider := NewProviderWithClients(new(MockElastiCacheClient), mockTagging)

		mockTagging.On("GetResources", ctx, mock.Anything, mock.Anything).Return(nil, assert.AnError)

		cfg := providers.ProviderConfig{
			Region: "ap-northeast-1",
			Filters: map[string]interface{}{
				"tags": map[string]interface{}{
					"Environment": "production",
				},
			},
		}

		_, err := discoverForTest(provider, ctx, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get resources by tags")

		mockTagging.AssertExpectations(t)
	})

	t.Run("elasticache client error", func(t *testing.T) {
		mockTagging := new(MockResourceGroupsTaggingClient)
		mockElastiCache := new(MockElastiCacheClient)
		ctx := context.Background()

		provider := NewProviderWithClients(mockElastiCache, mockTagging)

		taggingOutput := &resourcegroupstaggingapi.GetResourcesOutput{
			ResourceTagMappingList: []taggingtypes.ResourceTagMapping{
				{
					ResourceARN: aws.String("arn:aws:elasticache:ap-northeast-1:123456789012:replicationgroup:my-cluster"),
					Tags:        []taggingtypes.Tag{},
				},
			},
		}
		mockTagging.On("GetResources", ctx, mock.Anything, mock.Anything).Return(taggingOutput, nil)
		mockElastiCache.On("DescribeReplicationGroups", ctx, mock.Anything, mock.Anything).Return(nil, assert.AnError)

		cfg := providers.ProviderConfig{
			Region: "ap-northeast-1",
		}

		_, err := discoverForTest(provider, ctx, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to describe replication group")

		mockTagging.AssertExpectations(t)
		mockElastiCache.AssertExpectations(t)
	})
}

// Converts populated and empty tag maps and requires one single-value filter per input pair, without imposing filter order.
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

// Mocks two filtered groups with distinct tags and incomplete tag pairs; verifies that node IDs retain the associated complete tags and cluster names.
func TestFilteredDiscoveryKeepsMappingTags(t *testing.T) {
	t.Parallel()
	client := new(MockElastiCacheClient)
	tagging := new(MockResourceGroupsTaggingClient)
	var mappings []taggingtypes.ResourceTagMapping
	for _, id := range []string{"cluster-2", "cluster-1"} {
		mappings = append(mappings, taggingtypes.ResourceTagMapping{
			ResourceARN: aws.String("arn:aws:elasticache:us-east-1:123456789012:replicationgroup:" + id),
			Tags:        []taggingtypes.Tag{{Key: aws.String("cluster"), Value: aws.String(id)}, {Key: aws.String("incomplete")}},
		})
		client.On("DescribeReplicationGroups", mock.Anything, &elasticache.DescribeReplicationGroupsInput{ReplicationGroupId: aws.String(id)}, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
			ReplicationGroups: []elasticachetypes.ReplicationGroup{{ReplicationGroupId: aws.String(id), NodeGroups: []elasticachetypes.NodeGroup{{NodeGroupMembers: []elasticachetypes.NodeGroupMember{
				{ReadEndpoint: &elasticachetypes.Endpoint{Address: aws.String(id), Port: aws.Int32(6379)}},
			}}}}},
		}, nil).Once()
	}
	tagging.On("GetResources", mock.Anything, mock.Anything, mock.Anything).Return(&resourcegroupstaggingapi.GetResourcesOutput{ResourceTagMappingList: mappings}, nil).Once()
	result, err := discoverForTest(NewProviderWithClients(client, tagging), context.Background(), providers.ProviderConfig{Region: "us-east-1", Filters: map[string]interface{}{"tags": map[string]interface{}{"env": "prod"}}})
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "cluster-2", result[0].Host)
	assert.Equal(t, "cluster-1", result[1].Host)
	for _, node := range result {
		assert.Equal(t, map[string]string{"cluster": node.Host}, node.Tags)
		assert.Equal(t, node.Host, node.Metadata["ClusterName"])
	}
	client.AssertExpectations(t)
	tagging.AssertExpectations(t)
}

// Applies exclusion conditions to tagged and untagged groups from mocked APIs and verifies that untagged nodes remain discoverable.
func TestDiscoveryTagConditions(t *testing.T) {
	t.Parallel()
	client, tagging := new(MockElastiCacheClient), new(MockResourceGroupsTaggingClient)
	tagging.On("GetResources", mock.Anything, mock.Anything, mock.Anything).Return(&resourcegroupstaggingapi.GetResourcesOutput{
		ResourceTagMappingList: []taggingtypes.ResourceTagMapping{{ResourceARN: aws.String("arn:disabled"), Tags: []taggingtypes.Tag{{Key: aws.String("disabled"), Value: aws.String("")}}}},
	}, nil).Once()
	var groups []elasticachetypes.ReplicationGroup
	for _, id := range []string{"disabled", "untagged"} {
		groups = append(groups, elasticachetypes.ReplicationGroup{ARN: aws.String("arn:" + id), ReplicationGroupId: aws.String(id), NodeGroups: []elasticachetypes.NodeGroup{{NodeGroupMembers: []elasticachetypes.NodeGroupMember{{ReadEndpoint: &elasticachetypes.Endpoint{Address: aws.String(id), Port: aws.Int32(6379)}}}}}})
	}
	client.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{ReplicationGroups: groups}, nil).Once()
	resources, err := discoverForTest(NewProviderWithClients(client, tagging), context.Background(), providers.ProviderConfig{Region: "east", Filters: map[string]interface{}{"tag_conditions": []interface{}{map[string]interface{}{"key": "disabled", "operator": "not_exists"}}}})
	require.NoError(t, err)
	require.Len(t, resources, 1)
	assert.Equal(t, "untagged", resources[0].Host)
	client.AssertExpectations(t)
	tagging.AssertExpectations(t)
}
