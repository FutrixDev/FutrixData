package console

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestDynamoDBCredentialsProvider_UsesStaticCredentialsFromOptions(t *testing.T) {
	provider, err := dynamodbCredentialsProvider(map[string]any{
		"credentials": map[string]any{
			"accessKeyId":     "AKIA_STATIC",
			"secretAccessKey": "SECRET_STATIC",
			"sessionToken":    "TOKEN_STATIC",
		},
	})
	if err != nil {
		t.Fatalf("dynamodbCredentialsProvider: %v", err)
	}
	creds := retrieveCredentials(t, provider)
	if creds.AccessKeyID != "AKIA_STATIC" {
		t.Fatalf("expected AccessKeyID from static credentials, got %q", creds.AccessKeyID)
	}
	if creds.SecretAccessKey != "SECRET_STATIC" {
		t.Fatalf("expected SecretAccessKey from static credentials, got %q", creds.SecretAccessKey)
	}
	if creds.SessionToken != "TOKEN_STATIC" {
		t.Fatalf("expected SessionToken from static credentials, got %q", creds.SessionToken)
	}
}

func TestDynamoDBCredentialsProvider_UsesProfileFromSharedCredentialsFile(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIA_ENV")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "SECRET_ENV")
	t.Setenv("AWS_SESSION_TOKEN", "TOKEN_ENV")

	credentialsFile := filepath.Join(t.TempDir(), "credentials")
	content := strings.Join([]string{
		"[default]",
		"aws_access_key_id = AKIA_DEFAULT",
		"aws_secret_access_key = SECRET_DEFAULT",
		"",
		"[dev]",
		"aws_access_key_id = AKIA_PROFILE",
		"aws_secret_access_key = SECRET_PROFILE",
		"aws_session_token = TOKEN_PROFILE",
		"",
	}, "\n")
	if err := os.WriteFile(credentialsFile, []byte(content), 0o600); err != nil {
		t.Fatalf("write credentials file: %v", err)
	}
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credentialsFile)

	provider, err := dynamodbCredentialsProvider(map[string]any{"profile": "dev"})
	if err != nil {
		t.Fatalf("dynamodbCredentialsProvider: %v", err)
	}
	creds := retrieveCredentials(t, provider)
	if creds.AccessKeyID != "AKIA_PROFILE" {
		t.Fatalf("expected profile AccessKeyID, got %q", creds.AccessKeyID)
	}
	if creds.SecretAccessKey != "SECRET_PROFILE" {
		t.Fatalf("expected profile SecretAccessKey, got %q", creds.SecretAccessKey)
	}
	if creds.SessionToken != "TOKEN_PROFILE" {
		t.Fatalf("expected profile SessionToken, got %q", creds.SessionToken)
	}
}

func TestDynamoDBCredentialsProvider_FallsBackToEnvironmentCredentials(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIA_ENV")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "SECRET_ENV")
	t.Setenv("AWS_SESSION_TOKEN", "TOKEN_ENV")

	provider, err := dynamodbCredentialsProvider(map[string]any{})
	if err != nil {
		t.Fatalf("dynamodbCredentialsProvider: %v", err)
	}
	creds := retrieveCredentials(t, provider)
	if creds.AccessKeyID != "AKIA_ENV" {
		t.Fatalf("expected environment AccessKeyID, got %q", creds.AccessKeyID)
	}
	if creds.SecretAccessKey != "SECRET_ENV" {
		t.Fatalf("expected environment SecretAccessKey, got %q", creds.SecretAccessKey)
	}
	if creds.SessionToken != "TOKEN_ENV" {
		t.Fatalf("expected environment SessionToken, got %q", creds.SessionToken)
	}
}

func TestDynamoDBCredentialsProvider_UsesAWSProfileFromEnvironment(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_PROFILE", "dev")
	t.Setenv("AWS_DEFAULT_PROFILE", "")

	credentialsFile := writeDynamoDBCredentialsFileForTest(t, strings.Join([]string{
		"[default]",
		"aws_access_key_id = AKIA_DEFAULT",
		"aws_secret_access_key = SECRET_DEFAULT",
		"",
		"[dev]",
		"aws_access_key_id = AKIA_PROFILE",
		"aws_secret_access_key = SECRET_PROFILE",
		"aws_session_token = TOKEN_PROFILE",
		"",
	}, "\n"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credentialsFile)

	provider, err := dynamodbCredentialsProvider(map[string]any{})
	if err != nil {
		t.Fatalf("dynamodbCredentialsProvider: %v", err)
	}
	creds := retrieveCredentials(t, provider)
	if creds.AccessKeyID != "AKIA_PROFILE" {
		t.Fatalf("expected AWS_PROFILE AccessKeyID, got %q", creds.AccessKeyID)
	}
	if creds.SecretAccessKey != "SECRET_PROFILE" {
		t.Fatalf("expected AWS_PROFILE SecretAccessKey, got %q", creds.SecretAccessKey)
	}
	if creds.SessionToken != "TOKEN_PROFILE" {
		t.Fatalf("expected AWS_PROFILE SessionToken, got %q", creds.SessionToken)
	}
}

func TestDynamoDBCredentialsProvider_UsesAWSDefaultProfileFromEnvironment(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_DEFAULT_PROFILE", "dev")

	credentialsFile := writeDynamoDBCredentialsFileForTest(t, strings.Join([]string{
		"[default]",
		"aws_access_key_id = AKIA_DEFAULT",
		"aws_secret_access_key = SECRET_DEFAULT",
		"",
		"[dev]",
		"aws_access_key_id = AKIA_PROFILE",
		"aws_secret_access_key = SECRET_PROFILE",
		"aws_session_token = TOKEN_PROFILE",
		"",
	}, "\n"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credentialsFile)

	provider, err := dynamodbCredentialsProvider(map[string]any{})
	if err != nil {
		t.Fatalf("dynamodbCredentialsProvider: %v", err)
	}
	creds := retrieveCredentials(t, provider)
	if creds.AccessKeyID != "AKIA_PROFILE" {
		t.Fatalf("expected AWS_DEFAULT_PROFILE AccessKeyID, got %q", creds.AccessKeyID)
	}
	if creds.SecretAccessKey != "SECRET_PROFILE" {
		t.Fatalf("expected AWS_DEFAULT_PROFILE SecretAccessKey, got %q", creds.SecretAccessKey)
	}
	if creds.SessionToken != "TOKEN_PROFILE" {
		t.Fatalf("expected AWS_DEFAULT_PROFILE SessionToken, got %q", creds.SessionToken)
	}
}

func TestDynamoDBCredentialsProvider_ErrorsWhenCredentialsMissing(t *testing.T) {
	credentialsFile := filepath.Join(t.TempDir(), "credentials")
	if err := os.WriteFile(credentialsFile, []byte("[default]\n"), 0o600); err != nil {
		t.Fatalf("write credentials file: %v", err)
	}
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credentialsFile)
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_SESSION_TOKEN", "")

	_, err := dynamodbCredentialsProvider(map[string]any{})
	if err == nil {
		t.Fatalf("expected missing credentials error")
	}
	if !strings.Contains(err.Error(), "aws credentials are required") {
		t.Fatalf("expected missing credentials error, got %v", err)
	}
}

func retrieveCredentials(t *testing.T, provider aws.CredentialsProvider) aws.Credentials {
	t.Helper()
	creds, err := provider.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	return creds
}

func writeDynamoDBCredentialsFileForTest(t *testing.T, content string) string {
	t.Helper()
	credentialsFile := filepath.Join(t.TempDir(), "credentials")
	if err := os.WriteFile(credentialsFile, []byte(content), 0o600); err != nil {
		t.Fatalf("write credentials file: %v", err)
	}
	return credentialsFile
}
