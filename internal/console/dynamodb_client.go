package console

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"futrixdata/platform/internal/datasource"
)

type dynamodbClient struct{}

func newDynamoDBClient() *dynamodbClient {
	return &dynamodbClient{}
}

func (c *dynamodbClient) newAPI(ctx context.Context, ds datasource.DataSource) (*dynamodb.Client, error) {
	region, err := dynamodbRegion(ds.Options)
	if err != nil {
		return nil, err
	}

	credsProvider, err := dynamodbCredentialsProvider(ds.Options)
	if err != nil {
		return nil, err
	}
	if _, err := credsProvider.Retrieve(ctx); err != nil {
		return nil, fmt.Errorf("resolve aws credentials: %w", err)
	}

	cfg := aws.Config{
		Region:      region,
		Credentials: aws.NewCredentialsCache(credsProvider),
	}
	if endpoint := dynamodbEndpoint(ds.Options); endpoint != "" {
		cfg.EndpointResolverWithOptions = aws.EndpointResolverWithOptionsFunc(
			func(service, region string, _ ...any) (aws.Endpoint, error) {
				if service != dynamodb.ServiceID {
					return aws.Endpoint{}, &aws.EndpointNotFoundError{}
				}
				return aws.Endpoint{
					URL:               endpoint,
					SigningRegion:     region,
					HostnameImmutable: true,
				}, nil
			},
		)
	}
	return dynamodb.NewFromConfig(cfg), nil
}

func dynamodbRegion(options map[string]any) (string, error) {
	if options == nil {
		return "", errors.New("region is required")
	}
	raw, ok := options["region"]
	if !ok {
		return "", errors.New("region is required")
	}
	region := strings.TrimSpace(fmt.Sprint(raw))
	if region == "" {
		return "", errors.New("region is required")
	}
	return region, nil
}

func dynamodbProfile(options map[string]any) string {
	if options == nil {
		return ""
	}
	if raw, ok := options["profile"]; ok && raw != nil {
		return strings.TrimSpace(fmt.Sprint(raw))
	}
	return ""
}

func dynamodbEndpoint(options map[string]any) string {
	if options == nil {
		return ""
	}
	if raw, ok := options["endpoint"]; ok && raw != nil {
		return strings.TrimSpace(fmt.Sprint(raw))
	}
	return ""
}

func dynamodbStaticCredentials(options map[string]any) (aws.CredentialsProvider, bool) {
	if options == nil {
		return nil, false
	}
	raw, ok := options["credentials"]
	if !ok || raw == nil {
		return nil, false
	}
	creds, ok := raw.(map[string]any)
	if !ok {
		return nil, false
	}
	accessKeyID := strings.TrimSpace(fmt.Sprint(creds["accessKeyId"]))
	secretAccessKey := strings.TrimSpace(fmt.Sprint(creds["secretAccessKey"]))
	sessionToken := strings.TrimSpace(fmt.Sprint(creds["sessionToken"]))
	if accessKeyID == "" || secretAccessKey == "" {
		return nil, false
	}
	return credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, sessionToken), true
}

func dynamodbCredentialsProvider(options map[string]any) (aws.CredentialsProvider, error) {
	if provider, ok := dynamodbStaticCredentials(options); ok {
		return provider, nil
	}

	if profile := dynamodbProfile(options); profile != "" {
		return dynamodbSharedCredentialsProvider(profile)
	}

	if provider, ok := dynamodbEnvCredentialsProvider(); ok {
		return provider, nil
	}

	if profile := dynamodbEnvProfile(); profile != "" {
		if provider, err := dynamodbSharedCredentialsProvider(profile); err == nil {
			return provider, nil
		}
	}

	if provider, err := dynamodbSharedCredentialsProvider("default"); err == nil {
		return provider, nil
	}

	return nil, errors.New("aws credentials are required (set static credentials, profile, or AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY)")
}

func dynamodbEnvProfile() string {
	if profile := strings.TrimSpace(os.Getenv("AWS_PROFILE")); profile != "" {
		return profile
	}
	return strings.TrimSpace(os.Getenv("AWS_DEFAULT_PROFILE"))
}

func dynamodbEnvCredentialsProvider() (aws.CredentialsProvider, bool) {
	accessKeyID := strings.TrimSpace(os.Getenv("AWS_ACCESS_KEY_ID"))
	secretAccessKey := strings.TrimSpace(os.Getenv("AWS_SECRET_ACCESS_KEY"))
	sessionToken := strings.TrimSpace(os.Getenv("AWS_SESSION_TOKEN"))
	if accessKeyID == "" || secretAccessKey == "" {
		return nil, false
	}
	return credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, sessionToken), true
}

func dynamodbSharedCredentialsProvider(profile string) (aws.CredentialsProvider, error) {
	selectedProfile := strings.TrimSpace(profile)
	if selectedProfile == "" {
		return nil, errors.New("aws profile is required")
	}

	credentialsPath, err := dynamodbSharedCredentialsPath()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("read aws credentials file %q: %w", credentialsPath, err)
	}

	profiles := dynamodbParseSharedCredentials(string(raw))
	creds, ok := profiles[selectedProfile]
	if !ok {
		return nil, fmt.Errorf("aws profile %q not found in credentials file %q", selectedProfile, credentialsPath)
	}
	if creds.accessKeyID == "" || creds.secretAccessKey == "" {
		return nil, fmt.Errorf("aws profile %q in credentials file %q is missing access key id or secret access key", selectedProfile, credentialsPath)
	}
	return credentials.NewStaticCredentialsProvider(creds.accessKeyID, creds.secretAccessKey, creds.sessionToken), nil
}

func dynamodbSharedCredentialsPath() (string, error) {
	if path := strings.TrimSpace(os.Getenv("AWS_SHARED_CREDENTIALS_FILE")); path != "" {
		return path, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home dir: %w", err)
	}
	return filepath.Join(homeDir, ".aws", "credentials"), nil
}

type dynamodbSharedCredentials struct {
	accessKeyID     string
	secretAccessKey string
	sessionToken    string
}

func dynamodbParseSharedCredentials(content string) map[string]dynamodbSharedCredentials {
	profiles := map[string]dynamodbSharedCredentials{}
	currentProfile := ""

	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentProfile = strings.TrimSpace(line[1 : len(line)-1])
			if currentProfile == "" {
				continue
			}
			if _, exists := profiles[currentProfile]; !exists {
				profiles[currentProfile] = dynamodbSharedCredentials{}
			}
			continue
		}

		if currentProfile == "" {
			continue
		}

		sep := strings.IndexAny(line, "=:")
		if sep < 0 {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(line[:sep]))
		value := strings.TrimSpace(line[sep+1:])
		item := profiles[currentProfile]

		switch key {
		case "aws_access_key_id", "access_key_id":
			item.accessKeyID = value
		case "aws_secret_access_key", "secret_access_key":
			item.secretAccessKey = value
		case "aws_session_token", "session_token":
			item.sessionToken = value
		}

		profiles[currentProfile] = item
	}

	return profiles
}
