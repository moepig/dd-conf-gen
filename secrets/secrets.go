package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// Identifies a Secrets Manager value and an optional top-level JSON string field.
type Reference struct {
	Region       string `yaml:"region"`
	SecretID     string `yaml:"secret_id"`
	JSONKey      string `yaml:"json_key,omitempty"`
	VersionID    string `yaml:"version_id,omitempty"`
	VersionStage string `yaml:"version_stage,omitempty"`
}

// Validates required settings without external access, returning an error for a missing region or secret ID.
func (r Reference) Validate() error {
	if r.Region == "" {
		return fmt.Errorf("region is required")
	}
	if r.SecretID == "" {
		return fmt.Errorf("secret_id is required")
	}
	return nil
}

// Provides cancellable secret retrieval.
type API interface {
	GetSecretValue(context.Context, *secretsmanager.GetSecretValueInput, ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

// Constructs an API client for a region, or returns a configuration error.
type ClientFactory func(context.Context, string) (API, error)

// Holds clients and fetched strings for one generation run. Concurrent use is not supported.
type Resolver struct {
	factory ClientFactory
	clients map[string]API
	values  map[Reference]string
}

// Creates a resolver with empty caches. A nil factory uses the AWS SDK's standard credentials.
func NewResolver(factory ClientFactory) *Resolver {
	if factory == nil {
		factory = func(ctx context.Context, region string) (API, error) {
			cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
			if err != nil {
				return nil, err
			}
			return secretsmanager.NewFromConfig(cfg), nil
		}
	}
	return &Resolver{factory: factory, clients: make(map[string]API), values: make(map[Reference]string)}
}

// Retrieves named strings in name order, reusing requests within this resolver. Returns no partial results on errors and excludes secret contents from error messages.
func (r *Resolver) Resolve(ctx context.Context, refs map[string]Reference) (map[string]string, error) {
	values := make(map[string]string, len(refs))
	for _, name := range slices.Sorted(maps.Keys(refs)) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ref := refs[name]
		if err := ref.Validate(); err != nil {
			return nil, fmt.Errorf("secret %q: %w", name, err)
		}
		value, err := r.fetch(ctx, ref)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("secret %q: %w", name, err)
		}
		if ref.JSONKey != "" {
			var object map[string]json.RawMessage
			if json.Unmarshal([]byte(value), &object) != nil || object == nil {
				return nil, fmt.Errorf("secret %q: expected a JSON object", name)
			}
			raw, ok := object[ref.JSONKey]
			if !ok {
				return nil, fmt.Errorf("secret %q: JSON field is missing", name)
			}
			var field interface{}
			if json.Unmarshal(raw, &field) != nil {
				return nil, fmt.Errorf("secret %q: invalid JSON field", name)
			}
			value, ok = field.(string)
			if !ok {
				return nil, fmt.Errorf("secret %q: JSON field must be a string", name)
			}
		}
		values[name] = value
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

// Fetches a textual secret with request caching. Returns sanitized errors for client, API, or unsupported response failures.
func (r *Resolver) fetch(ctx context.Context, ref Reference) (string, error) {
	ref.JSONKey = ""
	if value, ok := r.values[ref]; ok {
		return value, nil
	}
	client, ok := r.clients[ref.Region]
	if !ok {
		var err error
		client, err = r.factory(ctx, ref.Region)
		if err != nil || client == nil {
			return "", fmt.Errorf("failed to configure Secrets Manager client")
		}
		r.clients[ref.Region] = client
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	input := &secretsmanager.GetSecretValueInput{SecretId: aws.String(ref.SecretID)}
	if ref.VersionID != "" {
		input.VersionId = aws.String(ref.VersionID)
	}
	if ref.VersionStage != "" {
		input.VersionStage = aws.String(ref.VersionStage)
	}
	response, err := client.GetSecretValue(ctx, input)
	if err != nil {
		return "", fmt.Errorf("GetSecretValue failed")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if response == nil || response.SecretString == nil {
		return "", fmt.Errorf("SecretString is required; binary secrets are unsupported")
	}
	value := *response.SecretString
	r.values[ref] = value
	return value, nil
}
