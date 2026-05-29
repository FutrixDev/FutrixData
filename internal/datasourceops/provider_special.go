package datasourceops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/browser"

	"futrixdata/platform/internal/commandutil"
)

type dynamoDBSSOClient interface {
	ListAccounts(ctx context.Context, params dynamoDBSSOListAccountsInput) (dynamoDBSSOListAccountsOutput, error)
	ListAccountRoles(ctx context.Context, params dynamoDBSSOListAccountRolesInput) (dynamoDBSSOListAccountRolesOutput, error)
	GetRoleCredentials(ctx context.Context, params dynamoDBSSOGetRoleCredentialsInput) (dynamoDBSSOGetRoleCredentialsOutput, error)
}

type dynamoDBSSOOIDCClient interface {
	RegisterClient(ctx context.Context, params dynamoDBSSOOIDCRegisterClientInput) (dynamoDBSSOOIDCRegisterClientOutput, error)
	StartDeviceAuthorization(ctx context.Context, params dynamoDBSSOOIDCStartDeviceAuthorizationInput) (dynamoDBSSOOIDCStartDeviceAuthorizationOutput, error)
	CreateToken(ctx context.Context, params dynamoDBSSOOIDCCreateTokenInput) (dynamoDBSSOOIDCCreateTokenOutput, error)
}

type dynamoDBSSOListAccountsInput struct {
	AccessToken string
	NextToken   string
	MaxResults  int32
}

type dynamoDBSSOListAccountsOutput struct {
	AccountList []dynamoDBSSOAccountInfo
	NextToken   string
}

type dynamoDBSSOAccountInfo struct {
	AccountID    string
	AccountName  string
	EmailAddress string
}

type dynamoDBSSOListAccountRolesInput struct {
	AccountID   string
	AccessToken string
	NextToken   string
	MaxResults  int32
}

type dynamoDBSSOListAccountRolesOutput struct {
	RoleList  []dynamoDBSSORoleInfo
	NextToken string
}

type dynamoDBSSORoleInfo struct {
	RoleName  string
	AccountID string
}

type dynamoDBSSOGetRoleCredentialsInput struct {
	AccountID   string
	RoleName    string
	AccessToken string
}

type dynamoDBSSOGetRoleCredentialsOutput struct {
	RoleCredentials *dynamoDBSSORoleCredentialsOutput
}

type dynamoDBSSORoleCredentialsOutput struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Expiration      int64
}

type dynamoDBSSOOIDCRegisterClientInput struct {
	ClientName string
	ClientType string
	GrantTypes []string
	Scopes     []string
}

type dynamoDBSSOOIDCRegisterClientOutput struct {
	ClientID     string
	ClientSecret string
}

type dynamoDBSSOOIDCStartDeviceAuthorizationInput struct {
	ClientID     string
	ClientSecret string
	StartURL     string
}

type dynamoDBSSOOIDCStartDeviceAuthorizationOutput struct {
	DeviceCode              string
	VerificationURI         string
	VerificationURIComplete string
	UserCode                string
	ExpiresIn               int32
	Interval                int32
}

type dynamoDBSSOOIDCCreateTokenInput struct {
	ClientID     string
	ClientSecret string
	GrantType    string
	DeviceCode   string
}

type dynamoDBSSOOIDCCreateTokenOutput struct {
	AccessToken string
	ExpiresIn   int32
}

type dynamoDBSSOHTTPClient struct {
	baseURL    string
	httpClient *http.Client
	initErr    error
}

type dynamoDBSSOOIDCHTTPClient struct {
	baseURL    string
	httpClient *http.Client
	initErr    error
}

type dynamoDBSSOAPIError struct {
	Service    string
	Code       string
	Message    string
	StatusCode int
}

func (e *dynamoDBSSOAPIError) Error() string {
	if e == nil {
		return ""
	}
	code := strings.TrimSpace(e.Code)
	msg := strings.TrimSpace(e.Message)
	switch {
	case code == "" && msg == "":
		return fmt.Sprintf("%s request failed", strings.TrimSpace(e.Service))
	case code == "":
		return msg
	case msg == "":
		return code
	default:
		return code + ": " + msg
	}
}

func (e *dynamoDBSSOAPIError) hasCode(code string) bool {
	if e == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(e.Code), strings.TrimSpace(code))
}

var resolveDynamoDBSSOPortalEndpoint = func(region string) (string, error) {
	return awsRegionalEndpoint("portal.sso", region)
}

var resolveDynamoDBSSOOIDCEndpoint = func(region string) (string, error) {
	return awsRegionalEndpoint("oidc", region)
}

var newDynamoDBSSOClient = func(region string, httpClient *http.Client) dynamoDBSSOClient {
	endpoint, err := resolveDynamoDBSSOPortalEndpoint(region)
	return &dynamoDBSSOHTTPClient{
		baseURL:    endpoint,
		httpClient: httpClient,
		initErr:    err,
	}
}

var newDynamoDBSSOOIDCClient = func(region string, httpClient *http.Client) dynamoDBSSOOIDCClient {
	endpoint, err := resolveDynamoDBSSOOIDCEndpoint(region)
	return &dynamoDBSSOOIDCHTTPClient{
		baseURL:    endpoint,
		httpClient: httpClient,
		initErr:    err,
	}
}

var openDynamoDBSSOVerificationURL = func(rawURL string) error {
	return browser.OpenURL(rawURL)
}

var waitDynamoDBSSOPollInterval = func(ctx context.Context, wait time.Duration) error {
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *Service) D1OAuthLogin(ctx context.Context) (D1OAuthSession, error) {
	session, err := s.d1OAuthLogin(ctxOrBackground(ctx), false)
	if err == nil {
		// Token refresh is best-effort; don't fail the login itself.
		_ = s.d1RefreshStoredTokens(session.AccountID, session.Token)
	}
	return session, err
}

func (s *Service) D1OAuthReLogin(ctx context.Context) (D1OAuthSession, error) {
	session, err := s.d1OAuthLogin(ctxOrBackground(ctx), true)
	if err == nil {
		_ = s.d1RefreshStoredTokens(session.AccountID, session.Token)
	}
	return session, err
}

// d1RefreshStoredTokens updates the apiToken on D1 datasources that use
// authMode=token and belong to the same account as the new OAuth session.
// Only datasources matching the given accountID are updated to avoid
// overwriting tokens for other Cloudflare accounts.
func (s *Service) d1RefreshStoredTokens(accountID, token string) error {
	if token == "" || s.store == nil {
		return nil
	}
	var errs []string
	for _, ds := range s.store.List() {
		if ds.Type != "d1" || ds.Options == nil {
			continue
		}
		mode, _ := ds.Options["authMode"].(string)
		if strings.ToLower(strings.TrimSpace(mode)) != "token" {
			continue
		}
		// Only update datasources belonging to the same account.
		if accountID != "" {
			dsAccount, _ := ds.Options["accountId"].(string)
			if dsAccount != "" && dsAccount != accountID {
				continue
			}
		}
		// Clone Options to avoid mutating the shared map outside the store lock.
		opts := make(map[string]any, len(ds.Options))
		for k, v := range ds.Options {
			opts[k] = v
		}
		opts["apiToken"] = token
		ds.Options = opts
		if _, err := s.store.Update(ds.ID, ds); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", ds.ID, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to update %d datasource(s): %s", len(errs), strings.Join(errs, "; "))
	}
	return nil
}

func (s *Service) D1IsWranglerInstalled(ctx context.Context) (bool, error) {
	_ = ctx
	return d1WranglerInstalled(), nil
}

func (s *Service) D1ListCloudDatabases(ctx context.Context, accountID, token string) ([]D1CloudDatabase, error) {
	accountID = strings.TrimSpace(accountID)
	token = strings.TrimSpace(token)
	if accountID == "" {
		return nil, errors.New("accountId is required")
	}
	if token == "" {
		return nil, errors.New("token is required")
	}
	return s.d1ListCloudDatabases(ctxOrBackground(ctx), accountID, token)
}

func (s *Service) D1CreateCloudDatabase(ctx context.Context, accountID, token, name string) (D1CloudDatabase, error) {
	accountID = strings.TrimSpace(accountID)
	token = strings.TrimSpace(token)
	name = strings.TrimSpace(name)
	if accountID == "" {
		return D1CloudDatabase{}, errors.New("accountId is required")
	}
	if token == "" {
		return D1CloudDatabase{}, errors.New("token is required")
	}
	if name == "" {
		return D1CloudDatabase{}, errors.New("database name is required")
	}
	endpoint := fmt.Sprintf("%s/accounts/%s/d1/database", d1CloudflareBaseURL(), url.PathEscape(accountID))
	body, err := json.Marshal(map[string]any{"name": name})
	if err != nil {
		return D1CloudDatabase{}, err
	}
	raw, err := s.d1CloudflareRequest(ctxOrBackground(ctx), http.MethodPost, endpoint, token, body)
	if err != nil {
		return D1CloudDatabase{}, err
	}
	var envelope struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Result struct {
			ID         string `json:"id"`
			UUID       string `json:"uuid"`
			DatabaseID string `json:"database_id"`
			Name       string `json:"name"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return D1CloudDatabase{}, err
	}
	if !envelope.Success {
		if len(envelope.Errors) > 0 && strings.TrimSpace(envelope.Errors[0].Message) != "" {
			return D1CloudDatabase{}, errors.New(strings.TrimSpace(envelope.Errors[0].Message))
		}
		return D1CloudDatabase{}, errors.New("create d1 database failed")
	}
	id := firstNonEmpty(envelope.Result.UUID, envelope.Result.ID, envelope.Result.DatabaseID)
	if id == "" {
		return D1CloudDatabase{}, errors.New("cloudflare create response missing database id")
	}
	return D1CloudDatabase{ID: id, Name: strings.TrimSpace(envelope.Result.Name)}, nil
}

func (s *Service) DynamoDBSSOListProfiles(ctx context.Context, configPath string) ([]DynamoDBSSOProfile, error) {
	_ = ctx
	resolvedPath, err := awsConfigPath(configPath)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("read aws config: %w", err)
	}
	profiles := awsProfilesFromConfig(string(raw))
	if len(profiles) == 0 {
		return nil, errors.New("no aws profiles found in ~/.aws/config")
	}
	return profiles, nil
}

func (s *Service) DynamoDBSSOLogin(ctx context.Context, profile, configPath string) (DynamoDBSSOLoginResult, error) {
	configPath, err := awsConfigPath(configPath)
	if err != nil {
		return DynamoDBSSOLoginResult{}, err
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return DynamoDBSSOLoginResult{}, fmt.Errorf("read aws config: %w", err)
	}
	profileConfig, err := awsResolveProfileConfig(string(raw), profile)
	if err != nil {
		return DynamoDBSSOLoginResult{}, err
	}
	if strings.TrimSpace(profileConfig.Name) == "" {
		return DynamoDBSSOLoginResult{}, errors.New("aws profile is required")
	}
	ssoRegion := strings.TrimSpace(profileConfig.SSORegion)
	if ssoRegion == "" {
		ssoRegion = strings.TrimSpace(profileConfig.Region)
	}
	if ssoRegion == "" {
		return DynamoDBSSOLoginResult{}, errors.New("sso_region or region is required for AWS SSO profile")
	}
	token, err := s.dynamoDBSSOEnsureAccessToken(ctxOrBackground(ctx), profileConfig, ssoRegion)
	if err != nil {
		return DynamoDBSSOLoginResult{}, err
	}
	return DynamoDBSSOLoginResult{AccessToken: token.AccessToken, ExpiresAt: token.ExpiresAt}, nil
}

func (s *Service) DynamoDBSSOOAuthAuthorize(ctx context.Context, profile, region, configPath string) (DynamoDBSSOOAuthResult, error) {
	resolvedPath, err := awsConfigPath(configPath)
	if err != nil {
		return DynamoDBSSOOAuthResult{}, err
	}
	raw, err := os.ReadFile(resolvedPath)
	if err != nil {
		return DynamoDBSSOOAuthResult{}, fmt.Errorf("read aws config: %w", err)
	}
	profileConfig, err := awsResolveProfileConfig(string(raw), profile)
	if err != nil {
		return DynamoDBSSOOAuthResult{}, err
	}

	selectedRegion := strings.TrimSpace(region)
	if selectedRegion == "" {
		selectedRegion = strings.TrimSpace(profileConfig.Region)
	}
	if selectedRegion == "" {
		selectedRegion = strings.TrimSpace(profileConfig.SSORegion)
	}
	if selectedRegion == "" {
		return DynamoDBSSOOAuthResult{}, errors.New("region is required")
	}
	ssoRegion := strings.TrimSpace(profileConfig.SSORegion)
	if ssoRegion == "" {
		ssoRegion = selectedRegion
	}
	token, err := s.dynamoDBSSOEnsureAccessToken(ctxOrBackground(ctx), profileConfig, ssoRegion)
	if err != nil {
		return DynamoDBSSOOAuthResult{}, err
	}

	accountID := strings.TrimSpace(profileConfig.AccountID)
	if accountID == "" {
		accounts, err := s.DynamoDBSSOListAccounts(ctxOrBackground(ctx), token.AccessToken, ssoRegion)
		if err != nil {
			return DynamoDBSSOOAuthResult{}, err
		}
		if len(accounts) != 1 {
			return DynamoDBSSOOAuthResult{}, errors.New("unable to resolve AWS account automatically; set sso_account_id in the selected profile")
		}
		accountID = strings.TrimSpace(accounts[0].AccountID)
	}
	if accountID == "" {
		return DynamoDBSSOOAuthResult{}, errors.New("accountId is required")
	}

	roleName := strings.TrimSpace(profileConfig.RoleName)
	if roleName == "" {
		roles, err := s.DynamoDBSSOListAccountRoles(ctxOrBackground(ctx), accountID, token.AccessToken, ssoRegion)
		if err != nil {
			return DynamoDBSSOOAuthResult{}, err
		}
		if len(roles) != 1 {
			return DynamoDBSSOOAuthResult{}, errors.New("unable to resolve AWS role automatically; set sso_role_name in the selected profile")
		}
		roleName = strings.TrimSpace(roles[0].RoleName)
	}
	if roleName == "" {
		return DynamoDBSSOOAuthResult{}, errors.New("roleName is required")
	}

	credentials, err := s.DynamoDBSSOGetRoleCredentials(ctxOrBackground(ctx), accountID, roleName, token.AccessToken, ssoRegion)
	if err != nil {
		return DynamoDBSSOOAuthResult{}, err
	}
	return DynamoDBSSOOAuthResult{
		Profile:         profileConfig.Name,
		Region:          selectedRegion,
		AccountID:       accountID,
		RoleName:        roleName,
		AccessKeyID:     strings.TrimSpace(credentials.AccessKeyID),
		SecretAccessKey: strings.TrimSpace(credentials.SecretAccessKey),
		SessionToken:    strings.TrimSpace(credentials.SessionToken),
		Expiration:      credentials.Expiration,
	}, nil
}

func (s *Service) DynamoDBSSOListAccounts(ctx context.Context, accessToken, region string) ([]DynamoDBSSOAccount, error) {
	accessToken = strings.TrimSpace(accessToken)
	region = strings.TrimSpace(region)
	if accessToken == "" {
		return nil, errors.New("accessToken is required")
	}
	if region == "" {
		return nil, errors.New("region is required")
	}
	client := newDynamoDBSSOClient(region, s.resolvedHTTPClient())
	out := make([]DynamoDBSSOAccount, 0, 8)
	nextToken := ""
	for {
		resp, err := client.ListAccounts(ctxOrBackground(ctx), dynamoDBSSOListAccountsInput{
			AccessToken: accessToken,
			NextToken:   nextToken,
			MaxResults:  100,
		})
		if err != nil {
			return nil, fmt.Errorf("aws sso list-accounts failed: %w", err)
		}
		for _, item := range resp.AccountList {
			accountID := strings.TrimSpace(item.AccountID)
			if accountID == "" {
				continue
			}
			accountName := strings.TrimSpace(item.AccountName)
			if accountName == "" {
				accountName = accountID
			}
			out = append(out, DynamoDBSSOAccount{
				AccountID:    accountID,
				AccountName:  accountName,
				EmailAddress: strings.TrimSpace(item.EmailAddress),
			})
		}
		if strings.TrimSpace(resp.NextToken) == "" {
			break
		}
		nextToken = strings.TrimSpace(resp.NextToken)
	}
	sort.Slice(out, func(i, j int) bool {
		left := strings.ToLower(strings.TrimSpace(out[i].AccountName))
		right := strings.ToLower(strings.TrimSpace(out[j].AccountName))
		if left == right {
			return out[i].AccountID < out[j].AccountID
		}
		return left < right
	})
	if len(out) == 0 {
		return nil, errors.New("aws sso list-accounts returned no accounts")
	}
	return out, nil
}

func (s *Service) DynamoDBSSOListAccountRoles(ctx context.Context, accountID, accessToken, region string) ([]DynamoDBSSORole, error) {
	accountID = strings.TrimSpace(accountID)
	accessToken = strings.TrimSpace(accessToken)
	region = strings.TrimSpace(region)
	if accountID == "" {
		return nil, errors.New("accountId is required")
	}
	if accessToken == "" {
		return nil, errors.New("accessToken is required")
	}
	if region == "" {
		return nil, errors.New("region is required")
	}
	client := newDynamoDBSSOClient(region, s.resolvedHTTPClient())
	out := make([]DynamoDBSSORole, 0, 8)
	nextToken := ""
	for {
		resp, err := client.ListAccountRoles(ctxOrBackground(ctx), dynamoDBSSOListAccountRolesInput{
			AccountID:   accountID,
			AccessToken: accessToken,
			NextToken:   nextToken,
			MaxResults:  100,
		})
		if err != nil {
			return nil, fmt.Errorf("aws sso list-account-roles failed: %w", err)
		}
		for _, item := range resp.RoleList {
			roleName := strings.TrimSpace(item.RoleName)
			if roleName == "" {
				continue
			}
			roleAccountID := strings.TrimSpace(item.AccountID)
			if roleAccountID == "" {
				roleAccountID = accountID
			}
			out = append(out, DynamoDBSSORole{RoleName: roleName, AccountID: roleAccountID})
		}
		if strings.TrimSpace(resp.NextToken) == "" {
			break
		}
		nextToken = strings.TrimSpace(resp.NextToken)
	}
	sort.Slice(out, func(i, j int) bool {
		left := strings.ToLower(strings.TrimSpace(out[i].RoleName))
		right := strings.ToLower(strings.TrimSpace(out[j].RoleName))
		if left == right {
			return out[i].AccountID < out[j].AccountID
		}
		return left < right
	})
	if len(out) == 0 {
		return nil, errors.New("aws sso list-account-roles returned no roles")
	}
	return out, nil
}

func (s *Service) DynamoDBSSOGetRoleCredentials(ctx context.Context, accountID, roleName, accessToken, region string) (DynamoDBSSORoleCredentials, error) {
	accountID = strings.TrimSpace(accountID)
	roleName = strings.TrimSpace(roleName)
	accessToken = strings.TrimSpace(accessToken)
	region = strings.TrimSpace(region)
	if accountID == "" {
		return DynamoDBSSORoleCredentials{}, errors.New("accountId is required")
	}
	if roleName == "" {
		return DynamoDBSSORoleCredentials{}, errors.New("roleName is required")
	}
	if accessToken == "" {
		return DynamoDBSSORoleCredentials{}, errors.New("accessToken is required")
	}
	if region == "" {
		return DynamoDBSSORoleCredentials{}, errors.New("region is required")
	}
	client := newDynamoDBSSOClient(region, s.resolvedHTTPClient())
	resp, err := client.GetRoleCredentials(ctxOrBackground(ctx), dynamoDBSSOGetRoleCredentialsInput{
		AccountID:   accountID,
		RoleName:    roleName,
		AccessToken: accessToken,
	})
	if err != nil {
		return DynamoDBSSORoleCredentials{}, fmt.Errorf("aws sso get-role-credentials failed: %w", err)
	}
	if resp.RoleCredentials == nil {
		return DynamoDBSSORoleCredentials{}, errors.New("aws sso get-role-credentials returned empty credentials")
	}
	creds := DynamoDBSSORoleCredentials{
		AccessKeyID:     strings.TrimSpace(resp.RoleCredentials.AccessKeyID),
		SecretAccessKey: strings.TrimSpace(resp.RoleCredentials.SecretAccessKey),
		SessionToken:    strings.TrimSpace(resp.RoleCredentials.SessionToken),
		Expiration:      resp.RoleCredentials.Expiration,
	}
	if creds.AccessKeyID == "" || creds.SecretAccessKey == "" || creds.SessionToken == "" {
		return DynamoDBSSORoleCredentials{}, errors.New("aws sso get-role-credentials returned incomplete credentials")
	}
	return creds, nil
}

func (s *Service) d1OAuthLogin(ctx context.Context, forceBrowserLogin bool) (D1OAuthSession, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	base := []string{"npx", "wrangler"}
	if !forceBrowserLogin {
		session, err := s.d1ResolveOAuthSession(timeoutCtx, base)
		if err == nil {
			return session, nil
		}
	}
	loginCommand := append(append([]string{}, base...), "login")
	if _, loginErr := s.runExternalCommand(timeoutCtx, loginCommand); loginErr != nil {
		if !strings.Contains(strings.ToLower(loginErr.Error()), "already logged in") {
			return D1OAuthSession{}, loginErr
		}
	}
	return s.d1ResolveOAuthSession(timeoutCtx, base)
}

func (s *Service) d1ResolveOAuthSession(ctx context.Context, baseCommand []string) (D1OAuthSession, error) {
	token, err := s.d1ResolveWranglerToken(ctx, baseCommand)
	if err != nil {
		return D1OAuthSession{}, err
	}
	accounts, accountID, err := s.d1ResolveWranglerAccounts(ctx, baseCommand)
	if err != nil {
		return D1OAuthSession{}, err
	}
	return D1OAuthSession{Accounts: accounts, AccountID: accountID, Token: token}, nil
}

func (s *Service) d1ListCloudDatabases(ctx context.Context, accountID, token string) ([]D1CloudDatabase, error) {
	endpoint := fmt.Sprintf("%s/accounts/%s/d1/database", d1CloudflareBaseURL(), url.PathEscape(accountID))
	raw, err := s.d1CloudflareRequest(ctx, http.MethodGet, endpoint, token, nil)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Result []struct {
			ID         string `json:"id"`
			UUID       string `json:"uuid"`
			DatabaseID string `json:"database_id"`
			Name       string `json:"name"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	if !envelope.Success {
		if len(envelope.Errors) > 0 && strings.TrimSpace(envelope.Errors[0].Message) != "" {
			return nil, errors.New(strings.TrimSpace(envelope.Errors[0].Message))
		}
		return nil, errors.New("list d1 databases failed")
	}
	out := make([]D1CloudDatabase, 0, len(envelope.Result))
	for _, item := range envelope.Result {
		id := firstNonEmpty(item.UUID, item.ID, item.DatabaseID)
		if id == "" {
			continue
		}
		out = append(out, D1CloudDatabase{ID: id, Name: strings.TrimSpace(item.Name)})
	}
	sort.Slice(out, func(i, j int) bool {
		left := strings.ToLower(strings.TrimSpace(out[i].Name))
		right := strings.ToLower(strings.TrimSpace(out[j].Name))
		if left == right {
			return out[i].ID < out[j].ID
		}
		return left < right
	})
	return out, nil
}

func d1CloudflareBaseURL() string {
	return "https://api.cloudflare.com/client/v4"
}

func (s *Service) d1CloudflareRequest(ctx context.Context, method, endpoint, token string, body []byte) ([]byte, error) {
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.resolvedHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		snippet := strings.TrimSpace(string(raw))
		if len(snippet) > 500 {
			snippet = snippet[:500] + "..."
		}
		if snippet != "" {
			return nil, fmt.Errorf("d1 api request failed: %s: %s", resp.Status, snippet)
		}
		return nil, fmt.Errorf("d1 api request failed: %s", resp.Status)
	}
	return raw, nil
}

func (s *Service) d1ResolveWranglerToken(ctx context.Context, baseCommand []string) (string, error) {
	command := append(append([]string{}, baseCommand...), "auth", "token", "--json")
	raw, err := s.runExternalCommand(ctx, command)
	if err != nil {
		return "", fmt.Errorf("wrangler auth token failed: %w", err)
	}
	var payload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", fmt.Errorf("parse wrangler auth token: %w", err)
	}
	if token := strings.TrimSpace(payload.Token); token != "" {
		return token, nil
	}
	return "", errors.New("wrangler auth token is empty")
}

func (s *Service) d1ResolveWranglerAccounts(ctx context.Context, baseCommand []string) ([]D1OAuthAccount, string, error) {
	command := append(append([]string{}, baseCommand...), "whoami", "--json")
	raw, err := s.runExternalCommand(ctx, command)
	if err != nil {
		return nil, "", fmt.Errorf("wrangler whoami failed: %w", err)
	}
	var payload struct {
		AccountID string `json:"account_id"`
		Accounts  []struct {
			ID         string `json:"id"`
			AccountTag string `json:"account_tag"`
			Name       string `json:"name"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, "", fmt.Errorf("parse wrangler whoami: %w", err)
	}
	selectedAccountID := strings.TrimSpace(payload.AccountID)
	accounts := make([]D1OAuthAccount, 0, len(payload.Accounts))
	seen := map[string]struct{}{}
	for _, account := range payload.Accounts {
		accountID := strings.TrimSpace(firstNonEmpty(account.ID, account.AccountTag))
		if accountID == "" {
			continue
		}
		if _, exists := seen[accountID]; exists {
			continue
		}
		seen[accountID] = struct{}{}
		accountName := strings.TrimSpace(account.Name)
		if accountName == "" {
			accountName = accountID
		}
		accounts = append(accounts, D1OAuthAccount{ID: accountID, Name: accountName})
	}
	if selectedAccountID != "" {
		if _, exists := seen[selectedAccountID]; !exists {
			accounts = append(accounts, D1OAuthAccount{ID: selectedAccountID, Name: selectedAccountID})
		}
	}
	sort.Slice(accounts, func(i, j int) bool {
		leftName := strings.ToLower(strings.TrimSpace(accounts[i].Name))
		rightName := strings.ToLower(strings.TrimSpace(accounts[j].Name))
		if leftName == rightName {
			return accounts[i].ID < accounts[j].ID
		}
		return leftName < rightName
	})
	if selectedAccountID == "" && len(accounts) == 1 {
		selectedAccountID = accounts[0].ID
	}
	if len(accounts) == 0 && selectedAccountID == "" {
		return nil, "", errors.New("wrangler account id not found")
	}
	return accounts, selectedAccountID, nil
}

func (s *Service) resolvedHTTPClient() *http.Client {
	if s.httpClient != nil {
		return s.httpClient
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (s *Service) runExternalCommand(ctx context.Context, command []string) ([]byte, error) {
	if s.runCommand != nil {
		return s.runCommand(ctx, command)
	}
	return defaultRunCommand(ctx, command)
}

func d1WranglerInstalled() bool {
	return d1WranglerInstalledWithLookup(exec.LookPath)
}

func d1WranglerInstalledWithLookup(lookup func(string) (string, error)) bool {
	if lookup == nil {
		return false
	}
	if _, err := lookup("wrangler"); err == nil {
		return true
	}
	if _, err := lookup("npx"); err == nil {
		return true
	}
	return false
}

func defaultRunCommand(ctx context.Context, command []string) ([]byte, error) {
	if len(command) == 0 {
		return nil, errors.New("command is required")
	}
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	commandutil.ApplyStableWorkingDir(cmd)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		snippet := strings.TrimSpace(stderr.String())
		if snippet == "" {
			snippet = strings.TrimSpace(stdout.String())
		}
		if len(snippet) > 800 {
			snippet = snippet[:800] + "..."
		}
		if snippet != "" {
			return nil, fmt.Errorf("%w: %s", err, snippet)
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}

func awsRegionalEndpoint(servicePrefix, region string) (string, error) {
	region = strings.TrimSpace(region)
	if region == "" {
		return "", errors.New("region is required")
	}
	suffix := "amazonaws.com"
	switch {
	case strings.HasPrefix(region, "cn-"):
		suffix = "amazonaws.com.cn"
	case strings.HasPrefix(region, "us-iso-"):
		suffix = "c2s.ic.gov"
	case strings.HasPrefix(region, "us-isob-"):
		suffix = "sc2s.sgov.gov"
	}
	return fmt.Sprintf("https://%s.%s.%s", servicePrefix, region, suffix), nil
}

func resolvedDynamoDBSSOHTTPClient(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func sanitizeDynamoDBSSOErrorCode(raw string) string {
	code := strings.TrimSpace(raw)
	if code == "" {
		return ""
	}
	if idx := strings.Index(code, ":"); idx >= 0 {
		code = code[:idx]
	}
	if idx := strings.LastIndex(code, "#"); idx >= 0 && idx < len(code)-1 {
		code = code[idx+1:]
	}
	return strings.TrimSpace(code)
}

func parseDynamoDBSSOAPIError(service string, statusCode int, header http.Header, body []byte) *dynamoDBSSOAPIError {
	code := sanitizeDynamoDBSSOErrorCode(header.Get("X-Amzn-Errortype"))
	msg := ""
	if len(body) > 0 {
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err == nil {
			for _, key := range []string{"message", "Message", "error_description"} {
				if raw, ok := payload[key]; ok && raw != nil {
					msg = strings.TrimSpace(fmt.Sprint(raw))
					if msg != "" {
						break
					}
				}
			}
			if code == "" {
				for _, key := range []string{"code", "Code", "__type", "error"} {
					if raw, ok := payload[key]; ok && raw != nil {
						code = sanitizeDynamoDBSSOErrorCode(fmt.Sprint(raw))
						if code != "" {
							break
						}
					}
				}
			}
		}
	}
	if code == "" {
		code = fmt.Sprintf("HTTP%d", statusCode)
	}
	return &dynamoDBSSOAPIError{Service: strings.TrimSpace(service), Code: code, Message: strings.TrimSpace(msg), StatusCode: statusCode}
}

func dynamoDBSSOAPIRequest(ctx context.Context, httpClient *http.Client, method, rawURL string, headers map[string]string, body any, out any, service string) error {
	var bodyReader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctxOrBackground(ctx), method, rawURL, bodyReader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			req.Header.Set(key, trimmed)
		}
	}
	resp, err := resolvedDynamoDBSSOHTTPClient(httpClient).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	rawResp, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseDynamoDBSSOAPIError(service, resp.StatusCode, resp.Header, rawResp)
	}
	if out == nil || len(rawResp) == 0 {
		return nil
	}
	if err := json.Unmarshal(rawResp, out); err != nil {
		return fmt.Errorf("decode %s response: %w", strings.TrimSpace(service), err)
	}
	return nil
}

func (c *dynamoDBSSOHTTPClient) validate() error {
	if c == nil {
		return errors.New("aws sso client is not initialized")
	}
	if c.initErr != nil {
		return c.initErr
	}
	if strings.TrimSpace(c.baseURL) == "" {
		return errors.New("aws sso endpoint is not configured")
	}
	return nil
}

func (c *dynamoDBSSOOIDCHTTPClient) validate() error {
	if c == nil {
		return errors.New("aws sso oidc client is not initialized")
	}
	if c.initErr != nil {
		return c.initErr
	}
	if strings.TrimSpace(c.baseURL) == "" {
		return errors.New("aws sso oidc endpoint is not configured")
	}
	return nil
}

func (c *dynamoDBSSOHTTPClient) ListAccounts(ctx context.Context, params dynamoDBSSOListAccountsInput) (dynamoDBSSOListAccountsOutput, error) {
	if err := c.validate(); err != nil {
		return dynamoDBSSOListAccountsOutput{}, err
	}
	endpoint := strings.TrimRight(c.baseURL, "/") + "/assignment/accounts"
	query := url.Values{}
	if params.MaxResults > 0 {
		query.Set("max_result", strconv.FormatInt(int64(params.MaxResults), 10))
	}
	if token := strings.TrimSpace(params.NextToken); token != "" {
		query.Set("next_token", token)
	}
	if encoded := query.Encode(); encoded != "" {
		endpoint = endpoint + "?" + encoded
	}
	var resp struct {
		AccountList []struct {
			AccountID    string `json:"accountId"`
			AccountName  string `json:"accountName"`
			EmailAddress string `json:"emailAddress"`
		} `json:"accountList"`
		NextToken string `json:"nextToken"`
	}
	if err := dynamoDBSSOAPIRequest(ctx, c.httpClient, http.MethodGet, endpoint, map[string]string{"X-Amz-Sso_bearer_token": strings.TrimSpace(params.AccessToken)}, nil, &resp, "aws sso list-accounts"); err != nil {
		return dynamoDBSSOListAccountsOutput{}, err
	}
	out := dynamoDBSSOListAccountsOutput{AccountList: make([]dynamoDBSSOAccountInfo, 0, len(resp.AccountList)), NextToken: strings.TrimSpace(resp.NextToken)}
	for _, item := range resp.AccountList {
		out.AccountList = append(out.AccountList, dynamoDBSSOAccountInfo{
			AccountID:    strings.TrimSpace(item.AccountID),
			AccountName:  strings.TrimSpace(item.AccountName),
			EmailAddress: strings.TrimSpace(item.EmailAddress),
		})
	}
	return out, nil
}

func (c *dynamoDBSSOHTTPClient) ListAccountRoles(ctx context.Context, params dynamoDBSSOListAccountRolesInput) (dynamoDBSSOListAccountRolesOutput, error) {
	if err := c.validate(); err != nil {
		return dynamoDBSSOListAccountRolesOutput{}, err
	}
	endpoint := strings.TrimRight(c.baseURL, "/") + "/assignment/roles"
	query := url.Values{}
	query.Set("account_id", strings.TrimSpace(params.AccountID))
	if params.MaxResults > 0 {
		query.Set("max_result", strconv.FormatInt(int64(params.MaxResults), 10))
	}
	if token := strings.TrimSpace(params.NextToken); token != "" {
		query.Set("next_token", token)
	}
	if encoded := query.Encode(); encoded != "" {
		endpoint = endpoint + "?" + encoded
	}
	var resp struct {
		RoleList []struct {
			RoleName  string `json:"roleName"`
			AccountID string `json:"accountId"`
		} `json:"roleList"`
		NextToken string `json:"nextToken"`
	}
	if err := dynamoDBSSOAPIRequest(ctx, c.httpClient, http.MethodGet, endpoint, map[string]string{"X-Amz-Sso_bearer_token": strings.TrimSpace(params.AccessToken)}, nil, &resp, "aws sso list-account-roles"); err != nil {
		return dynamoDBSSOListAccountRolesOutput{}, err
	}
	out := dynamoDBSSOListAccountRolesOutput{RoleList: make([]dynamoDBSSORoleInfo, 0, len(resp.RoleList)), NextToken: strings.TrimSpace(resp.NextToken)}
	for _, item := range resp.RoleList {
		out.RoleList = append(out.RoleList, dynamoDBSSORoleInfo{
			RoleName:  strings.TrimSpace(item.RoleName),
			AccountID: strings.TrimSpace(item.AccountID),
		})
	}
	return out, nil
}

func (c *dynamoDBSSOHTTPClient) GetRoleCredentials(ctx context.Context, params dynamoDBSSOGetRoleCredentialsInput) (dynamoDBSSOGetRoleCredentialsOutput, error) {
	if err := c.validate(); err != nil {
		return dynamoDBSSOGetRoleCredentialsOutput{}, err
	}
	endpoint := strings.TrimRight(c.baseURL, "/") + "/federation/credentials"
	query := url.Values{}
	query.Set("account_id", strings.TrimSpace(params.AccountID))
	query.Set("role_name", strings.TrimSpace(params.RoleName))
	if encoded := query.Encode(); encoded != "" {
		endpoint = endpoint + "?" + encoded
	}
	var resp struct {
		RoleCredentials *struct {
			AccessKeyID     string `json:"accessKeyId"`
			SecretAccessKey string `json:"secretAccessKey"`
			SessionToken    string `json:"sessionToken"`
			Expiration      int64  `json:"expiration"`
		} `json:"roleCredentials"`
	}
	if err := dynamoDBSSOAPIRequest(ctx, c.httpClient, http.MethodGet, endpoint, map[string]string{"X-Amz-Sso_bearer_token": strings.TrimSpace(params.AccessToken)}, nil, &resp, "aws sso get-role-credentials"); err != nil {
		return dynamoDBSSOGetRoleCredentialsOutput{}, err
	}
	out := dynamoDBSSOGetRoleCredentialsOutput{}
	if resp.RoleCredentials != nil {
		out.RoleCredentials = &dynamoDBSSORoleCredentialsOutput{
			AccessKeyID:     strings.TrimSpace(resp.RoleCredentials.AccessKeyID),
			SecretAccessKey: strings.TrimSpace(resp.RoleCredentials.SecretAccessKey),
			SessionToken:    strings.TrimSpace(resp.RoleCredentials.SessionToken),
			Expiration:      resp.RoleCredentials.Expiration,
		}
	}
	return out, nil
}

func (c *dynamoDBSSOOIDCHTTPClient) RegisterClient(ctx context.Context, params dynamoDBSSOOIDCRegisterClientInput) (dynamoDBSSOOIDCRegisterClientOutput, error) {
	if err := c.validate(); err != nil {
		return dynamoDBSSOOIDCRegisterClientOutput{}, err
	}
	endpoint := strings.TrimRight(c.baseURL, "/") + "/client/register"
	var resp struct {
		ClientID     string `json:"clientId"`
		ClientSecret string `json:"clientSecret"`
	}
	if err := dynamoDBSSOAPIRequest(ctx, c.httpClient, http.MethodPost, endpoint, nil, map[string]any{
		"clientName": params.ClientName,
		"clientType": params.ClientType,
		"grantTypes": params.GrantTypes,
		"scopes":     params.Scopes,
	}, &resp, "aws sso register-client"); err != nil {
		return dynamoDBSSOOIDCRegisterClientOutput{}, err
	}
	return dynamoDBSSOOIDCRegisterClientOutput{ClientID: strings.TrimSpace(resp.ClientID), ClientSecret: strings.TrimSpace(resp.ClientSecret)}, nil
}

func (c *dynamoDBSSOOIDCHTTPClient) StartDeviceAuthorization(ctx context.Context, params dynamoDBSSOOIDCStartDeviceAuthorizationInput) (dynamoDBSSOOIDCStartDeviceAuthorizationOutput, error) {
	if err := c.validate(); err != nil {
		return dynamoDBSSOOIDCStartDeviceAuthorizationOutput{}, err
	}
	endpoint := strings.TrimRight(c.baseURL, "/") + "/device_authorization"
	var resp dynamoDBSSOOIDCStartDeviceAuthorizationOutput
	if err := dynamoDBSSOAPIRequest(ctx, c.httpClient, http.MethodPost, endpoint, nil, map[string]any{
		"clientId":     params.ClientID,
		"clientSecret": params.ClientSecret,
		"startUrl":     params.StartURL,
	}, &resp, "aws sso start-device-authorization"); err != nil {
		return dynamoDBSSOOIDCStartDeviceAuthorizationOutput{}, err
	}
	return resp, nil
}

func (c *dynamoDBSSOOIDCHTTPClient) CreateToken(ctx context.Context, params dynamoDBSSOOIDCCreateTokenInput) (dynamoDBSSOOIDCCreateTokenOutput, error) {
	if err := c.validate(); err != nil {
		return dynamoDBSSOOIDCCreateTokenOutput{}, err
	}
	endpoint := strings.TrimRight(c.baseURL, "/") + "/token"
	var resp dynamoDBSSOOIDCCreateTokenOutput
	if err := dynamoDBSSOAPIRequest(ctx, c.httpClient, http.MethodPost, endpoint, nil, map[string]any{
		"clientId":     params.ClientID,
		"clientSecret": params.ClientSecret,
		"grantType":    params.GrantType,
		"deviceCode":   params.DeviceCode,
	}, &resp, "aws sso create-token"); err != nil {
		return dynamoDBSSOOIDCCreateTokenOutput{}, err
	}
	return resp, nil
}

type awsProfileConfig struct {
	Name      string
	Region    string
	SSORegion string
	StartURL  string
	AccountID string
	RoleName  string
}

type awsSSOCacheToken struct {
	AccessToken string
	ExpiresAt   string
	ExpiresTime time.Time
}

func (s *Service) dynamoDBSSOEnsureAccessToken(ctx context.Context, profileConfig awsProfileConfig, oidcRegion string) (awsSSOCacheToken, error) {
	if cacheDir, err := awsSSOCacheDir(); err == nil {
		if token, tokenErr := awsResolveSSOCacheToken(cacheDir, profileConfig.StartURL); tokenErr == nil {
			return token, nil
		}
	}
	return s.dynamoDBSSOAuthorizeWithDeviceCode(ctx, profileConfig, oidcRegion)
}

func (s *Service) dynamoDBSSOAuthorizeWithDeviceCode(ctx context.Context, profileConfig awsProfileConfig, oidcRegion string) (awsSSOCacheToken, error) {
	startURL := strings.TrimSpace(profileConfig.StartURL)
	if startURL == "" {
		return awsSSOCacheToken{}, errors.New("sso_start_url is required for AWS SSO profile")
	}
	oidcRegion = strings.TrimSpace(oidcRegion)
	if oidcRegion == "" {
		return awsSSOCacheToken{}, errors.New("sso_region or region is required for AWS SSO profile")
	}
	clientName := strings.TrimSpace(profileConfig.Name)
	if clientName == "" {
		clientName = "futrixdata-dynamodb-sso"
	}
	oidcClient := newDynamoDBSSOOIDCClient(oidcRegion, s.resolvedHTTPClient())
	registerResp, err := oidcClient.RegisterClient(ctxOrBackground(ctx), dynamoDBSSOOIDCRegisterClientInput{
		ClientName: clientName,
		ClientType: "public",
		GrantTypes: []string{"urn:ietf:params:oauth:grant-type:device_code"},
		Scopes:     []string{"sso:account:access"},
	})
	if err != nil {
		return awsSSOCacheToken{}, fmt.Errorf("aws sso register-client failed: %w", err)
	}
	clientID := strings.TrimSpace(registerResp.ClientID)
	clientSecret := strings.TrimSpace(registerResp.ClientSecret)
	if clientID == "" || clientSecret == "" {
		return awsSSOCacheToken{}, errors.New("aws sso register-client returned incomplete client credentials")
	}
	authorizeResp, err := oidcClient.StartDeviceAuthorization(ctxOrBackground(ctx), dynamoDBSSOOIDCStartDeviceAuthorizationInput{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		StartURL:     startURL,
	})
	if err != nil {
		return awsSSOCacheToken{}, fmt.Errorf("aws sso start-device-authorization failed: %w", err)
	}
	deviceCode := strings.TrimSpace(authorizeResp.DeviceCode)
	if deviceCode == "" {
		return awsSSOCacheToken{}, errors.New("aws sso start-device-authorization returned empty device code")
	}
	verifyURL := strings.TrimSpace(authorizeResp.VerificationURIComplete)
	if verifyURL == "" {
		verifyURL = strings.TrimSpace(authorizeResp.VerificationURI)
	}
	if verifyURL == "" {
		return awsSSOCacheToken{}, errors.New("aws sso start-device-authorization returned empty verification URL")
	}
	openURL := s.openURL
	if openURL == nil {
		openURL = openDynamoDBSSOVerificationURL
	}
	if err := openURL(verifyURL); err != nil {
		userCode := strings.TrimSpace(authorizeResp.UserCode)
		if userCode == "" {
			return awsSSOCacheToken{}, fmt.Errorf("open AWS SSO verification URL failed: %w", err)
		}
		return awsSSOCacheToken{}, fmt.Errorf("open AWS SSO verification URL failed: %w; manually open %s and enter code %s", err, verifyURL, userCode)
	}
	interval := time.Duration(authorizeResp.Interval) * time.Second
	if interval <= 0 {
		interval = 2 * time.Second
	}
	remaining := time.Duration(authorizeResp.ExpiresIn) * time.Second
	if remaining <= 0 {
		remaining = 5 * time.Minute
	}
	pollCtx, cancel := context.WithTimeout(ctxOrBackground(ctx), remaining)
	defer cancel()
	for {
		tokenResp, tokenErr := oidcClient.CreateToken(pollCtx, dynamoDBSSOOIDCCreateTokenInput{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			GrantType:    "urn:ietf:params:oauth:grant-type:device_code",
			DeviceCode:   deviceCode,
		})
		if tokenErr == nil {
			accessToken := strings.TrimSpace(tokenResp.AccessToken)
			if accessToken == "" {
				return awsSSOCacheToken{}, errors.New("aws sso create-token returned empty access token")
			}
			expiresIn := time.Duration(tokenResp.ExpiresIn) * time.Second
			if expiresIn <= 0 {
				expiresIn = time.Hour
			}
			expiresAt := time.Now().UTC().Add(expiresIn)
			return awsSSOCacheToken{AccessToken: accessToken, ExpiresAt: expiresAt.Format(time.RFC3339), ExpiresTime: expiresAt}, nil
		}
		var apiErr *dynamoDBSSOAPIError
		switch {
		case errors.As(tokenErr, &apiErr) && (apiErr.hasCode("AuthorizationPendingException") || apiErr.hasCode("authorization_pending")):
			if waitErr := waitDynamoDBSSOPollInterval(pollCtx, interval); waitErr != nil {
				return awsSSOCacheToken{}, fmt.Errorf("aws sso authorization timed out: %w", waitErr)
			}
			continue
		case errors.As(tokenErr, &apiErr) && (apiErr.hasCode("SlowDownException") || apiErr.hasCode("slow_down")):
			interval += time.Second
			if waitErr := waitDynamoDBSSOPollInterval(pollCtx, interval); waitErr != nil {
				return awsSSOCacheToken{}, fmt.Errorf("aws sso authorization timed out: %w", waitErr)
			}
			continue
		case errors.As(tokenErr, &apiErr) && (apiErr.hasCode("AccessDeniedException") || apiErr.hasCode("access_denied")):
			return awsSSOCacheToken{}, errors.New("aws sso authorization denied by user")
		case errors.As(tokenErr, &apiErr) && (apiErr.hasCode("ExpiredTokenException") || apiErr.hasCode("expired_token")):
			return awsSSOCacheToken{}, errors.New("aws sso device authorization expired; retry OAuth authorization")
		case errors.Is(tokenErr, context.DeadlineExceeded), errors.Is(tokenErr, context.Canceled):
			return awsSSOCacheToken{}, fmt.Errorf("aws sso authorization timed out: %w", tokenErr)
		default:
			return awsSSOCacheToken{}, fmt.Errorf("aws sso create-token failed: %w", tokenErr)
		}
	}
}

func awsConfigPath(configPath string) (string, error) {
	if explicit := strings.TrimSpace(configPath); explicit != "" {
		return explicit, nil
	}
	if envPath := strings.TrimSpace(os.Getenv("AWS_CONFIG_FILE")); envPath != "" {
		return envPath, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".aws", "config"), nil
}

func awsSSOCacheDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".aws", "sso", "cache"), nil
}

func awsProfilesFromConfig(content string) []DynamoDBSSOProfile {
	parsed := awsParseProfileConfigs(content)
	out := make([]DynamoDBSSOProfile, 0, len(parsed))
	for _, item := range parsed {
		out = append(out, DynamoDBSSOProfile{
			Name:      item.Name,
			Region:    item.Region,
			SSORegion: item.SSORegion,
			StartURL:  item.StartURL,
			AccountID: item.AccountID,
			RoleName:  item.RoleName,
		})
	}
	return out
}

func awsResolveProfileConfig(content, profile string) (awsProfileConfig, error) {
	profiles := awsParseProfileConfigs(content)
	if len(profiles) == 0 {
		return awsProfileConfig{}, errors.New("no aws profiles found in ~/.aws/config")
	}
	trimmedProfile := strings.TrimSpace(profile)
	if trimmedProfile == "" {
		for _, item := range profiles {
			if item.Name == "default" {
				trimmedProfile = "default"
				break
			}
		}
	}
	if trimmedProfile == "" && len(profiles) == 1 {
		return profiles[0], nil
	}
	if trimmedProfile == "" {
		return awsProfileConfig{}, errors.New("aws profile is required")
	}
	for _, item := range profiles {
		if item.Name == trimmedProfile {
			return item, nil
		}
	}
	return awsProfileConfig{}, fmt.Errorf("aws profile not found: %s", trimmedProfile)
}

func awsParseProfileConfigs(content string) []awsProfileConfig {
	type profileEntry struct {
		name, region, ssoRegion, start, accountID, roleName, session string
	}
	type sessionEntry struct {
		name, ssoRegion, start string
	}
	entries := map[string]*profileEntry{}
	sessions := map[string]*sessionEntry{}
	order := make([]string, 0, 8)
	currentProfile := ""
	currentSession := ""
	lines := strings.Split(content, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			switch {
			case section == "default":
				currentProfile, currentSession = "default", ""
			case strings.HasPrefix(section, "profile "):
				currentProfile, currentSession = strings.TrimSpace(strings.TrimPrefix(section, "profile ")), ""
			case strings.HasPrefix(section, "sso-session "):
				currentProfile, currentSession = "", strings.TrimSpace(strings.TrimPrefix(section, "sso-session "))
			default:
				currentProfile, currentSession = "", ""
			}
			if currentProfile != "" {
				if _, exists := entries[currentProfile]; !exists {
					entries[currentProfile] = &profileEntry{name: currentProfile}
					order = append(order, currentProfile)
				}
			} else if currentSession != "" {
				if _, exists := sessions[currentSession]; !exists {
					sessions[currentSession] = &sessionEntry{name: currentSession}
				}
			}
			continue
		}
		if currentProfile == "" && currentSession == "" {
			continue
		}
		sep := strings.Index(line, "=")
		if sep <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:sep]))
		value := strings.Trim(strings.TrimSpace(line[sep+1:]), `"`)
		if currentProfile != "" {
			current := entries[currentProfile]
			if current == nil {
				continue
			}
			switch key {
			case "region":
				current.region = value
			case "sso_region":
				current.ssoRegion = value
			case "sso_start_url":
				current.start = value
			case "sso_account_id":
				current.accountID = value
			case "sso_role_name":
				current.roleName = value
			case "sso_session":
				current.session = value
			}
			continue
		}
		current := sessions[currentSession]
		if current == nil {
			continue
		}
		switch key {
		case "sso_region":
			current.ssoRegion = value
		case "sso_start_url":
			current.start = value
		}
	}
	out := make([]awsProfileConfig, 0, len(order))
	for _, name := range order {
		entry := entries[name]
		if entry == nil || strings.TrimSpace(entry.name) == "" {
			continue
		}
		if sessionName := strings.TrimSpace(entry.session); sessionName != "" {
			if session := sessions[sessionName]; session != nil {
				if strings.TrimSpace(entry.start) == "" {
					entry.start = strings.TrimSpace(session.start)
				}
				if strings.TrimSpace(entry.ssoRegion) == "" {
					entry.ssoRegion = strings.TrimSpace(session.ssoRegion)
				}
			}
		}
		out = append(out, awsProfileConfig{
			Name:      strings.TrimSpace(entry.name),
			Region:    strings.TrimSpace(entry.region),
			SSORegion: strings.TrimSpace(entry.ssoRegion),
			StartURL:  strings.TrimSpace(entry.start),
			AccountID: strings.TrimSpace(entry.accountID),
			RoleName:  strings.TrimSpace(entry.roleName),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Name == "default" {
			return true
		}
		if out[j].Name == "default" {
			return false
		}
		return strings.ToLower(strings.TrimSpace(out[i].Name)) < strings.ToLower(strings.TrimSpace(out[j].Name))
	})
	return out
}

func awsResolveSSOCacheToken(cacheDir, startURL string) (awsSSOCacheToken, error) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return awsSSOCacheToken{}, fmt.Errorf("read aws sso cache directory: %w", err)
	}
	normalizedStartURL := strings.TrimSpace(startURL)
	now := time.Now().UTC()
	var best awsSSOCacheToken
	var matched bool
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(entry.Name()))
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(cacheDir, entry.Name()))
		if err != nil {
			continue
		}
		var payload struct {
			StartURL    string `json:"startUrl"`
			AccessToken string `json:"accessToken"`
			ExpiresAt   string `json:"expiresAt"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			continue
		}
		token := strings.TrimSpace(payload.AccessToken)
		expiresAtRaw := strings.TrimSpace(payload.ExpiresAt)
		if token == "" || expiresAtRaw == "" {
			continue
		}
		if normalizedStartURL != "" && strings.TrimSpace(payload.StartURL) != normalizedStartURL {
			continue
		}
		expiresAt, err := awsParseSSOExpiresAt(expiresAtRaw)
		if err != nil || expiresAt.Before(now) {
			continue
		}
		if !matched || expiresAt.After(best.ExpiresTime) {
			matched = true
			best = awsSSOCacheToken{AccessToken: token, ExpiresAt: expiresAtRaw, ExpiresTime: expiresAt}
		}
	}
	if !matched {
		if normalizedStartURL != "" {
			return awsSSOCacheToken{}, errors.New("no valid aws sso access token found for selected profile")
		}
		return awsSSOCacheToken{}, errors.New("no valid aws sso access token found")
	}
	return best, nil
}

func awsParseSSOExpiresAt(raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, errors.New("empty expiresAt")
	}
	layouts := []string{time.RFC3339, "2006-01-02T15:04:05Z0700", "2006-01-02T15:04:05-0700", "2006-01-02T15:04:05UTC"}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed.UTC(), nil
		}
	}
	if strings.HasSuffix(trimmed, "UTC") {
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSuffix(trimmed, "UTC")+"Z"); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid expiresAt: %s", trimmed)
}
