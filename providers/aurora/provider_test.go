package aurora

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/moepig/dd-conf-gen/providers"
	"github.com/moepig/dd-conf-gen/renderer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Holds mocked RDS API calls and results.
type mockRDSClient struct{ mock.Mock }

// Records ctx, input, and options and returns the configured cluster page and error.
func (m *mockRDSClient) DescribeDBClusters(ctx context.Context, input *rds.DescribeDBClustersInput, options ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error) {
	args := m.Called(ctx, input, options)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rds.DescribeDBClustersOutput), args.Error(1)
}

// Records ctx, input, and options and returns the configured instance page and error.
func (m *mockRDSClient) DescribeDBInstances(ctx context.Context, input *rds.DescribeDBInstancesInput, options ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error) {
	args := m.Called(ctx, input, options)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rds.DescribeDBInstancesOutput), args.Error(1)
}

// Returns an API mock that rejects unexpected calls and checks expectations at cleanup.
func newMockClient(t *testing.T) *mockRDSClient {
	t.Helper()
	client := new(mockRDSClient)
	client.Test(t)
	t.Cleanup(func() { client.AssertExpectations(t) })
	return client
}

// Returns an Aurora MySQL cluster fixture with writer and reader membership and cluster tags.
func testCluster() rdstypes.DBCluster {
	return rdstypes.DBCluster{
		DBClusterIdentifier: aws.String("production"), Engine: aws.String("aurora-mysql"),
		TagList: []rdstypes.Tag{{Key: aws.String("env"), Value: aws.String("prod")}, {Key: aws.String("service"), Value: aws.String("api")}},
		DBClusterMembers: []rdstypes.DBClusterMember{
			{DBInstanceIdentifier: aws.String("writer"), IsClusterWriter: aws.Bool(true)},
			{DBInstanceIdentifier: aws.String("reader"), IsClusterWriter: aws.Bool(false)},
		},
	}
}

// Returns an Aurora MySQL instance fixture with the supplied identifier and endpoint.
func testInstance(id string) rdstypes.DBInstance {
	return rdstypes.DBInstance{
		DBInstanceIdentifier: aws.String(id), DBClusterIdentifier: aws.String("production"), Engine: aws.String("aurora-mysql"),
		EngineVersion: aws.String("8.0.mysql_aurora.3.10.1"), AvailabilityZone: aws.String("ap-northeast-1a"),
		Endpoint: &rdstypes.Endpoint{Address: aws.String(id + ".example.rds.amazonaws.com"), Port: aws.Int32(3306)},
		TagList:  []rdstypes.Tag{{Key: aws.String("env"), Value: aws.String("instance-tag")}},
	}
}

// Validates raw configuration with an API mock accepting no calls; requires errors for invalid settings and a usable search for valid settings.
func TestPrepareValidation(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		cfg  providers.ProviderConfig
		err  string
	}{
		{name: "missing region", err: "region is required"},
		{name: "no filters", cfg: providers.ProviderConfig{Region: "us-east-1"}},
		{name: "empty tags", cfg: providers.ProviderConfig{Region: "us-east-1", Filters: map[string]interface{}{"tags": map[string]interface{}{}}}},
		{name: "unknown filter", cfg: providers.ProviderConfig{Region: "us-east-1", Filters: map[string]interface{}{"engine": "mysql"}}, err: "unsupported filter: engine"},
		{name: "non-map tags", cfg: providers.ProviderConfig{Region: "us-east-1", Filters: map[string]interface{}{"tags": "prod"}}, err: "filters.tags must be a map"},
		{name: "numeric tag", cfg: providers.ProviderConfig{Region: "us-east-1", Filters: map[string]interface{}{"tags": map[string]interface{}{"env": 123}}}, err: "filters.tags.env must be a string"},
		{name: "boolean tag", cfg: providers.ProviderConfig{Region: "us-east-1", Filters: map[string]interface{}{"tags": map[string]interface{}{"env": true}}}, err: "filters.tags.env must be a string"},
		{name: "null tag", cfg: providers.ProviderConfig{Region: "us-east-1", Filters: map[string]interface{}{"tags": map[string]interface{}{"env": nil}}}, err: "filters.tags.env must be a string"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := NewProviderWithClient(newMockClient(t))
			assert.Equal(t, "aurora_mysql", p.Type())
			discover, err := p.Prepare(tt.cfg)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				assert.Nil(t, discover)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, discover)
			}
		})
	}
}

// Mutates raw settings and the provider after preparation; repeated mocked searches must retain the original tag filter and client, returning independent writer and reader resources.
func TestPreparedDiscovery(t *testing.T) {
	t.Parallel()
	client := newMockClient(t)
	cluster := testCluster()
	client.On("DescribeDBClusters", mock.Anything, mock.MatchedBy(func(input *rds.DescribeDBClustersInput) bool {
		return assert.Equal(t, []rdstypes.Filter{{Name: aws.String("engine"), Values: []string{"aurora-mysql"}}}, input.Filters)
	}), mock.Anything).Return(&rds.DescribeDBClustersOutput{DBClusters: []rdstypes.DBCluster{cluster}}, nil).Twice()
	client.On("DescribeDBInstances", mock.Anything, mock.MatchedBy(func(input *rds.DescribeDBInstancesInput) bool {
		return assert.Equal(t, []rdstypes.Filter{{Name: aws.String("db-cluster-id"), Values: []string{"production"}}}, input.Filters)
	}), mock.Anything).Return(&rds.DescribeDBInstancesOutput{DBInstances: []rdstypes.DBInstance{testInstance("writer"), testInstance("reader")}}, nil).Twice()
	rawTags := map[string]interface{}{"env": "prod", "service": "api"}
	cfg := providers.ProviderConfig{Region: "ap-northeast-1", Filters: map[string]interface{}{"tags": rawTags}}
	p := NewProviderWithClient(client)
	discover, err := p.Prepare(cfg)
	require.NoError(t, err)
	assert.Empty(t, client.Calls)
	rawTags["env"] = 123
	cfg.Filters["invalid"] = true
	p.client = nil
	for range 2 {
		resources, err := discover(context.Background())
		require.NoError(t, err)
		require.Len(t, resources, 2)
		assert.Equal(t, "writer.example.rds.amazonaws.com", resources[0].Host)
		assert.Equal(t, 3306, resources[0].Port)
		assert.Equal(t, map[string]string{"env": "prod", "service": "api"}, resources[0].Tags)
		assert.Equal(t, map[string]interface{}{"ClusterName": "production", "DBInstanceID": "writer", "IsWriter": true, "EngineVersion": "8.0.mysql_aurora.3.10.1", "AvailabilityZone": "ap-northeast-1a"}, resources[0].Metadata)
		assert.Equal(t, false, resources[1].Metadata["IsWriter"])
		resources[0].Tags["env"] = "changed"
		resources[0].Metadata["IsWriter"] = false
		assert.Equal(t, "prod", resources[1].Tags["env"])
		assert.Equal(t, "prod", aws.ToString(cluster.TagList[0].Value))
	}
}

// Supplies clusters with different engines and tag sets; only Aurora MySQL clusters matching every exact tag pair may trigger instance queries.
func TestDiscoveryFilters(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		engine  string
		tags    []rdstypes.Tag
		filters map[string]interface{}
		match   bool
	}{
		{name: "untagged without filters", engine: "aurora-mysql", match: true},
		{name: "tagged without filters", engine: "aurora-mysql", tags: testCluster().TagList, match: true},
		{name: "postgres excluded", engine: "aurora-postgresql"},
		{name: "RDS mysql excluded", engine: "mysql"},
		{name: "missing engine"},
		{name: "all filters required", engine: "aurora-mysql", tags: testCluster().TagList, filters: map[string]interface{}{"env": "prod", "service": "other"}},
		{name: "case sensitive", engine: "aurora-mysql", tags: testCluster().TagList, filters: map[string]interface{}{"env": "Prod"}},
		{name: "missing is not empty", engine: "aurora-mysql", filters: map[string]interface{}{"env": ""}},
		{name: "empty value matches", engine: "aurora-mysql", tags: []rdstypes.Tag{{Key: aws.String("env"), Value: aws.String("")}}, filters: map[string]interface{}{"env": ""}, match: true},
		{name: "incomplete tags omitted", engine: "aurora-mysql", tags: []rdstypes.Tag{{Key: aws.String("env")}, {Value: aws.String("prod")}}, filters: map[string]interface{}{"env": ""}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newMockClient(t)
			cluster := testCluster()
			cluster.Engine, cluster.TagList = aws.String(tt.engine), tt.tags
			client.On("DescribeDBClusters", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBClustersOutput{DBClusters: []rdstypes.DBCluster{cluster}}, nil).Once()
			if tt.match {
				client.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{DBInstances: []rdstypes.DBInstance{testInstance("writer")}}, nil).Once()
			}
			cfg := providers.ProviderConfig{Region: "us-east-1"}
			if tt.filters != nil {
				cfg.Filters = map[string]interface{}{"tags": tt.filters}
			}
			discover, err := NewProviderWithClient(client).Prepare(cfg)
			require.NoError(t, err)
			resources, err := discover(context.Background())
			require.NoError(t, err)
			if tt.match {
				require.Len(t, resources, 1)
			} else {
				assert.Empty(t, resources)
			}
		})
	}
}

// Mocks paginated cluster and instance results, including empty pages; requires complete results or no partial resources on API errors, nil responses, repeated markers, and cancellation.
func TestDiscoveryPaginationAndFailures(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"clusters", "instances"} {
		for _, scenario := range []string{"success", "empty page", "API error", "nil response", "repeated marker", "marker cycle", "cancelled"} {
			t.Run(stage+"/"+scenario, func(t *testing.T) {
				client := newMockClient(t)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				method := "DescribeDBClusters"
				var first, second, third interface{}
				if stage == "clusters" {
					first = &rds.DescribeDBClustersOutput{DBClusters: []rdstypes.DBCluster{testCluster()}, Marker: aws.String("next")}
					second = &rds.DescribeDBClustersOutput{}
					third = &rds.DescribeDBClustersOutput{Marker: aws.String("next")}
					if scenario == "empty page" {
						first = &rds.DescribeDBClustersOutput{Marker: aws.String("next")}
						second = &rds.DescribeDBClustersOutput{DBClusters: []rdstypes.DBCluster{testCluster()}}
					}
					if scenario == "success" || scenario == "empty page" {
						client.On("DescribeDBInstances", ctx, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{DBInstances: []rdstypes.DBInstance{testInstance("writer")}}, nil).Once()
					}
				} else {
					method = "DescribeDBInstances"
					client.On("DescribeDBClusters", ctx, mock.Anything, mock.Anything).Return(&rds.DescribeDBClustersOutput{DBClusters: []rdstypes.DBCluster{testCluster()}}, nil).Once()
					first = &rds.DescribeDBInstancesOutput{DBInstances: []rdstypes.DBInstance{testInstance("writer")}, Marker: aws.String("next")}
					second = &rds.DescribeDBInstancesOutput{}
					third = &rds.DescribeDBInstancesOutput{Marker: aws.String("next")}
					if scenario == "empty page" {
						first = &rds.DescribeDBInstancesOutput{Marker: aws.String("next")}
						second = &rds.DescribeDBInstancesOutput{DBInstances: []rdstypes.DBInstance{testInstance("writer")}}
					}
				}
				var apiErr error
				switch scenario {
				case "API error":
					second, apiErr = nil, assert.AnError
				case "nil response":
					second = nil
				case "repeated marker", "marker cycle":
					marker := "next"
					if scenario == "marker cycle" {
						marker = "third"
					}
					if stage == "clusters" {
						second = &rds.DescribeDBClustersOutput{Marker: aws.String(marker)}
					} else {
						second = &rds.DescribeDBInstancesOutput{Marker: aws.String(marker)}
					}
				}
				markerMatches := func(marker string) interface{} {
					if stage == "clusters" {
						return mock.MatchedBy(func(input *rds.DescribeDBClustersInput) bool {
							return aws.ToString(input.Marker) == marker && len(input.Filters) == 1 && input.Filters[0].Values[0] == "aurora-mysql"
						})
					}
					return mock.MatchedBy(func(input *rds.DescribeDBInstancesInput) bool {
						return aws.ToString(input.Marker) == marker && len(input.Filters) == 1 && input.Filters[0].Values[0] == "production"
					})
				}
				call := client.On(method, ctx, markerMatches(""), mock.Anything).Return(first, nil).Once()
				if scenario == "cancelled" {
					call.Run(func(mock.Arguments) { cancel() })
				} else {
					client.On(method, ctx, markerMatches("next"), mock.Anything).Return(second, apiErr).Once()
					if scenario == "marker cycle" {
						client.On(method, ctx, markerMatches("third"), mock.Anything).Return(third, nil).Once()
					}
				}
				discover, err := NewProviderWithClient(client).Prepare(providers.ProviderConfig{Region: "us-east-1"})
				require.NoError(t, err)
				resources, err := discover(ctx)
				if scenario == "success" || scenario == "empty page" {
					require.NoError(t, err)
					require.Len(t, resources, 1)
				} else {
					require.Error(t, err)
					assert.Nil(t, resources)
					switch scenario {
					case "API error":
						assert.ErrorIs(t, err, assert.AnError)
					case "nil response":
						assert.ErrorContains(t, err, "empty response")
					case "repeated marker", "marker cycle":
						assert.ErrorContains(t, err, "repeated pagination marker")
					case "cancelled":
						assert.ErrorIs(t, err, context.Canceled)
					}
				}
			})
		}
	}
}

// Supplies invalid endpoints and instances belonging to other engines or clusters; only the valid Aurora MySQL instance is returned.
func TestDiscoverySkipsInvalidInstances(t *testing.T) {
	t.Parallel()
	client := newMockClient(t)
	instances := []rdstypes.DBInstance{testInstance("writer")}
	for _, endpoint := range []*rdstypes.Endpoint{nil, {}, {Address: aws.String("host")}, {Address: aws.String("host"), Port: aws.Int32(-1)}, {Address: aws.String("host"), Port: aws.Int32(65536)}, {Port: aws.Int32(3306)}} {
		instance := testInstance("invalid")
		instance.Endpoint = endpoint
		instances = append(instances, instance)
	}
	wrongEngine, wrongCluster := testInstance("mysql"), testInstance("other")
	wrongEngine.Engine = aws.String("mysql")
	wrongCluster.DBClusterIdentifier = aws.String("other")
	instances = append(instances, wrongEngine, wrongCluster)
	client.On("DescribeDBClusters", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBClustersOutput{DBClusters: []rdstypes.DBCluster{testCluster()}}, nil).Once()
	client.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{DBInstances: instances}, nil).Once()
	discover, err := NewProviderWithClient(client).Prepare(providers.ProviderConfig{Region: "us-east-1"})
	require.NoError(t, err)
	resources, err := discover(context.Background())
	require.NoError(t, err)
	require.Len(t, resources, 1)
	assert.Equal(t, "writer.example.rds.amazonaws.com", resources[0].Host)
}

// Mocks two clusters with a valid first result and an invalid identifier or failed second query; requires no partial resources after either failure.
func TestDiscoveryDiscardsEarlierClustersOnFailure(t *testing.T) {
	t.Parallel()
	for _, invalidID := range []bool{false, true} {
		name := "API failure"
		if invalidID {
			name = "missing cluster identifier"
		}
		t.Run(name, func(t *testing.T) {
			client := newMockClient(t)
			second := testCluster()
			second.DBClusterIdentifier = aws.String("second")
			if invalidID {
				second.DBClusterIdentifier = nil
			}
			client.On("DescribeDBClusters", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBClustersOutput{DBClusters: []rdstypes.DBCluster{testCluster(), second}}, nil).Once()
			client.On("DescribeDBInstances", mock.Anything, mock.MatchedBy(func(input *rds.DescribeDBInstancesInput) bool {
				return len(input.Filters) == 1 && input.Filters[0].Values[0] == "production"
			}), mock.Anything).Return(&rds.DescribeDBInstancesOutput{DBInstances: []rdstypes.DBInstance{testInstance("writer")}}, nil).Once()
			if !invalidID {
				client.On("DescribeDBInstances", mock.Anything, mock.MatchedBy(func(input *rds.DescribeDBInstancesInput) bool {
					return len(input.Filters) == 1 && input.Filters[0].Values[0] == "second"
				}), mock.Anything).Return(nil, assert.AnError).Once()
			}
			discover, err := NewProviderWithClient(client).Prepare(providers.ProviderConfig{Region: "us-east-1"})
			require.NoError(t, err)
			resources, err := discover(context.Background())
			assert.Nil(t, resources)
			if invalidID {
				assert.ErrorContains(t, err, "cluster has no identifier")
			} else {
				assert.ErrorIs(t, err, assert.AnError)
			}
		})
	}
}

// Uses an already cancelled context with an unconfigured provider; requires cancellation before AWS configuration or API access.
func TestDiscoveryCancelledBeforeStart(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	discover, err := NewProvider().Prepare(providers.ProviderConfig{Region: "us-east-1"})
	require.NoError(t, err)
	resources, err := discover(ctx)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, resources)
}

// Selects a missing profile in empty AWS files; requires a configuration error without a client and successful discovery with an injected client.
func TestDiscoveryAWSConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aws-config")
	require.NoError(t, os.WriteFile(path, nil, 0600))
	t.Setenv("AWS_CONFIG_FILE", path)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", path)
	t.Setenv("AWS_PROFILE", "missing-profile")
	discover, err := NewProvider().Prepare(providers.ProviderConfig{Region: "us-east-1"})
	require.NoError(t, err)
	resources, err := discover(context.Background())
	require.ErrorContains(t, err, "failed to load AWS config")
	assert.Nil(t, resources)
	client := newMockClient(t)
	client.On("DescribeDBClusters", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBClustersOutput{}, nil).Once()
	discover, err = NewProviderWithClient(client).Prepare(providers.ProviderConfig{Region: "us-east-1"})
	require.NoError(t, err)
	resources, err = discover(context.Background())
	require.NoError(t, err)
	assert.Empty(t, resources)
}

// Discovers mocked writer and reader endpoints and renders the distributed MySQL template; requires valid YAML with both hosts, cluster tags, roles, and preserved credential placeholders.
func TestMySQLTemplate(t *testing.T) {
	t.Parallel()
	client := newMockClient(t)
	client.On("DescribeDBClusters", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBClustersOutput{DBClusters: []rdstypes.DBCluster{testCluster()}}, nil).Once()
	client.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{DBInstances: []rdstypes.DBInstance{testInstance("writer"), testInstance("reader")}}, nil).Once()
	discover, err := NewProviderWithClient(client).Prepare(providers.ProviderConfig{Region: "us-east-1"})
	require.NoError(t, err)
	resources, err := discover(context.Background())
	require.NoError(t, err)
	template, err := renderer.Compile(context.Background(), "../../examples/templates/mysql.yaml.tmpl")
	require.NoError(t, err)
	content, err := template.Render(context.Background(), renderer.TemplateData{Resources: resources})
	require.NoError(t, err)
	var cfg struct {
		Instances []struct {
			Host               string
			Port               int
			Username, Password string
			Tags               []string
		}
	}
	require.NoError(t, yaml.Unmarshal(content, &cfg))
	require.Len(t, cfg.Instances, 2)
	assert.Equal(t, "writer.example.rds.amazonaws.com", cfg.Instances[0].Host)
	assert.Equal(t, "reader.example.rds.amazonaws.com", cfg.Instances[1].Host)
	for _, instance := range cfg.Instances {
		assert.Equal(t, 3306, instance.Port)
		assert.Equal(t, "%%env_MYSQL_USERNAME%%", instance.Username)
		assert.Equal(t, "%%env_MYSQL_PASSWORD%%", instance.Password)
		assert.Contains(t, instance.Tags, "env:prod")
		assert.Contains(t, instance.Tags, "cluster:production")
	}
	assert.Contains(t, cfg.Instances[0].Tags, "role:writer")
	assert.Contains(t, cfg.Instances[1].Tags, "role:reader")
	content, err = template.Render(context.Background(), renderer.TemplateData{})
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(content, &cfg))
	assert.Empty(t, cfg.Instances)
}
