package secrets

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Records secret API calls and provides configured responses.
type mockClient struct{ mock.Mock }

// Returns the mocked response and error for a secret request.
func (m *mockClient) GetSecretValue(ctx context.Context, in *secretsmanager.GetSecretValueInput, opts ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	args := m.Called(ctx, in, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*secretsmanager.GetSecretValueOutput), args.Error(1)
}

// Resolves multiple JSON fields and whole strings using a mocked API, verifying request reuse within a run and fresh retrieval in the next run.
func TestResolveCaching(t *testing.T) {
	t.Parallel()
	client := new(mockClient)
	client.Test(t)
	client.On("GetSecretValue", mock.Anything, mock.MatchedBy(func(in *secretsmanager.GetSecretValueInput) bool {
		return aws.ToString(in.SecretId) == "database" && in.VersionStage == nil && in.VersionId == nil
	}), mock.Anything).Return(&secretsmanager.GetSecretValueOutput{SecretString: aws.String(`{"username":"reader","password":"a: b\n\"c"}`)}, nil).Twice()
	factoryCalls := 0
	factory := func(ctx context.Context, region string) (API, error) {
		assert.Equal(t, "east", region)
		factoryCalls++
		return client, nil
	}
	refs := map[string]Reference{
		"username": {Region: "east", SecretID: "database", JSONKey: "username"},
		"password": {Region: "east", SecretID: "database", JSONKey: "password"},
		"whole":    {Region: "east", SecretID: "database"},
	}
	for range 2 {
		resolver := NewResolver(factory)
		for range 2 {
			values, err := resolver.Resolve(context.Background(), refs)
			require.NoError(t, err)
			assert.Equal(t, "reader", values["username"])
			assert.Equal(t, "a: b\n\"c", values["password"])
			assert.JSONEq(t, `{"username":"reader","password":"a: b\n\"c"}`, values["whole"])
		}
	}
	assert.Equal(t, 2, factoryCalls)
	client.AssertExpectations(t)
}

// Checks region and version forwarding with independent cached requests.
func TestResolveVersions(t *testing.T) {
	t.Parallel()
	client := new(mockClient)
	client.Test(t)
	for _, id := range []string{"one", "two"} {
		client.On("GetSecretValue", mock.Anything, mock.MatchedBy(func(in *secretsmanager.GetSecretValueInput) bool {
			return aws.ToString(in.SecretId) == "db" && aws.ToString(in.VersionId) == id && aws.ToString(in.VersionStage) == "AWSPREVIOUS"
		}), mock.Anything).Return(&secretsmanager.GetSecretValueOutput{SecretString: aws.String(id)}, nil).Once()
	}
	regions := []string{}
	resolver := NewResolver(func(ctx context.Context, region string) (API, error) {
		regions = append(regions, region)
		return client, nil
	})
	values, err := resolver.Resolve(context.Background(), map[string]Reference{
		"a": {Region: "east", SecretID: "db", VersionID: "one", VersionStage: "AWSPREVIOUS"},
		"b": {Region: "west", SecretID: "db", VersionID: "two", VersionStage: "AWSPREVIOUS"},
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "one", "b": "two"}, values)
	assert.Equal(t, []string{"east", "west"}, regions)
	client.AssertExpectations(t)
}

// Supplies malformed, binary, missing, and failing responses; all failures discard partial values and keep secret contents out of errors.
func TestResolveFailures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, value, key string
	}{
		{name: "malformed", value: "SECRET_VALUE{", key: "password"},
		{name: "not object", value: `"SECRET_VALUE"`, key: "password"},
		{name: "missing", value: `{"other":"SECRET_VALUE"}`, key: "password"},
		{name: "number", value: `{"password":42}`, key: "password"},
		{name: "null", value: `{"password":null}`, key: "password"},
		{name: "object", value: `{"password":{"data":"SECRET_VALUE"}}`, key: "password"},
		{name: "API"},
		{name: "nil"}, {name: "binary"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := new(mockClient)
			client.Test(t)
			var response *secretsmanager.GetSecretValueOutput
			var apiErr error
			switch tc.name {
			case "API":
				apiErr = errors.New("SECRET_VALUE")
			case "nil":
			case "binary":
				response = &secretsmanager.GetSecretValueOutput{SecretBinary: []byte("SECRET_VALUE")}
			default:
				response = &secretsmanager.GetSecretValueOutput{SecretString: aws.String(tc.value)}
			}
			client.On("GetSecretValue", mock.Anything, mock.Anything, mock.Anything).Return(response, apiErr).Once()
			resolver := NewResolver(func(context.Context, string) (API, error) { return client, nil })
			values, err := resolver.Resolve(context.Background(), map[string]Reference{"password": {Region: "east", SecretID: "db", JSONKey: tc.key}})
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "SECRET_VALUE")
			assert.Nil(t, values)
			client.AssertExpectations(t)
		})
	}
}

// Cancels before retrieval and inside a mocked API request and verifies context errors with no partial results.
func TestResolveCancellation(t *testing.T) {
	t.Parallel()
	for _, before := range []bool{true, false} {
		ctx, cancel := context.WithCancel(context.Background())
		client := new(mockClient)
		client.Test(t)
		if before {
			cancel()
		} else {
			client.On("GetSecretValue", mock.Anything, mock.Anything, mock.Anything).Return(&secretsmanager.GetSecretValueOutput{SecretString: aws.String("secret")}, nil).Run(func(mock.Arguments) { cancel() }).Once()
		}
		resolver := NewResolver(func(context.Context, string) (API, error) { return client, nil })
		values, err := resolver.Resolve(ctx, map[string]Reference{"password": {Region: "east", SecretID: "db"}})
		require.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, values)
		client.AssertExpectations(t)
		cancel()
	}
}

// Rejects invalid references and failed client construction without exposing factory error contents, while accepting empty string secrets.
func TestResolveValidationAndClientErrors(t *testing.T) {
	t.Parallel()
	for _, ref := range []Reference{{SecretID: "db"}, {Region: "east"}} {
		resolver := NewResolver(func(context.Context, string) (API, error) { t.Fatal("unexpected client construction"); return nil, nil })
		values, err := resolver.Resolve(context.Background(), map[string]Reference{"value": ref})
		require.Error(t, err)
		assert.Nil(t, values)
	}
	for _, factory := range []ClientFactory{
		func(context.Context, string) (API, error) { return nil, errors.New("SECRET_VALUE") },
		func(context.Context, string) (API, error) { return nil, nil },
	} {
		values, err := NewResolver(factory).Resolve(context.Background(), map[string]Reference{"value": {Region: "east", SecretID: "db"}})
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "SECRET_VALUE")
		assert.Nil(t, values)
	}
	client := new(mockClient)
	client.Test(t)
	client.On("GetSecretValue", mock.Anything, mock.Anything, mock.Anything).Return(&secretsmanager.GetSecretValueOutput{SecretString: aws.String("")}, nil).Once()
	values, err := NewResolver(func(context.Context, string) (API, error) { return client, nil }).Resolve(context.Background(), map[string]Reference{"value": {Region: "east", SecretID: "db"}})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"value": ""}, values)
	client.AssertExpectations(t)
}
