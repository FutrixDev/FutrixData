package main

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
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/browser"

	"futrixdata/platform/internal/commandutil"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/secrets"
)

type DataSourcePayload struct {
	Name       string                       `json:"name"`
	Type       datasource.DataSourceType    `json:"type"`
	Host       string                       `json:"host"`
	Port       int                          `json:"port"`
	Username   string                       `json:"username"`
	Password   string                       `json:"password"`
	Database   string                       `json:"database"`
	AuthSource string                       `json:"authSource"`
	Options    map[string]any               `json:"options"`
	SecretRefs map[string]secrets.SecretRef `json:"secretRefs,omitempty"`
}

type D1OAuthSession struct {
	Accounts  []D1OAuthAccount `json:"accounts"`
	AccountID string           `json:"accountId"`
	Token     string           `json:"token"`
}

type D1OAuthAccount struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type D1CloudDatabase struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type DynamoDBSSOProfile struct {
	Name      string `json:"name"`
	Region    string `json:"region"`
	SSORegion string `json:"ssoRegion"`
	StartURL  string `json:"startUrl"`
	AccountID string `json:"accountId"`
	RoleName  string `json:"roleName"`
}

type DynamoDBSSOLoginResult struct {
	AccessToken string `json:"accessToken"`
	ExpiresAt   string `json:"expiresAt"`
}

type DynamoDBSSOAccount struct {
	AccountID    string `json:"accountId"`
	AccountName  string `json:"accountName"`
	EmailAddress string `json:"emailAddress"`
}

type DynamoDBSSORole struct {
	RoleName  string `json:"roleName"`
	AccountID string `json:"accountId"`
}

type DynamoDBSSORoleCredentials struct {
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	SessionToken    string `json:"sessionToken"`
	Expiration      int64  `json:"expiration"`
}

type DynamoDBSSOOAuthResult struct {
	Profile         string `json:"profile"`
	Region          string `json:"region"`
	AccountID       string `json:"accountId"`
	RoleName        string `json:"roleName"`
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	SessionToken    string `json:"sessionToken"`
	Expiration      int64  `json:"expiration"`
}

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
	return &dynamoDBSSOAPIError{
		Service:    strings.TrimSpace(service),
		Code:       code,
		Message:    strings.TrimSpace(msg),
		StatusCode: statusCode,
	}
}

func dynamoDBSSOAPIRequest(
	ctx context.Context,
	httpClient *http.Client,
	method string,
	rawURL string,
	headers map[string]string,
	body any,
	out any,
	service string,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var bodyReader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
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
	if err := dynamoDBSSOAPIRequest(
		ctx,
		c.httpClient,
		http.MethodGet,
		endpoint,
		map[string]string{"X-Amz-Sso_bearer_token": strings.TrimSpace(params.AccessToken)},
		nil,
		&resp,
		"aws sso list-accounts",
	); err != nil {
		return dynamoDBSSOListAccountsOutput{}, err
	}
	out := dynamoDBSSOListAccountsOutput{
		AccountList: make([]dynamoDBSSOAccountInfo, 0, len(resp.AccountList)),
		NextToken:   strings.TrimSpace(resp.NextToken),
	}
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
	if err := dynamoDBSSOAPIRequest(
		ctx,
		c.httpClient,
		http.MethodGet,
		endpoint,
		map[string]string{"X-Amz-Sso_bearer_token": strings.TrimSpace(params.AccessToken)},
		nil,
		&resp,
		"aws sso list-account-roles",
	); err != nil {
		return dynamoDBSSOListAccountRolesOutput{}, err
	}
	out := dynamoDBSSOListAccountRolesOutput{
		RoleList:  make([]dynamoDBSSORoleInfo, 0, len(resp.RoleList)),
		NextToken: strings.TrimSpace(resp.NextToken),
	}
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
	if err := dynamoDBSSOAPIRequest(
		ctx,
		c.httpClient,
		http.MethodGet,
		endpoint,
		map[string]string{"X-Amz-Sso_bearer_token": strings.TrimSpace(params.AccessToken)},
		nil,
		&resp,
		"aws sso get-role-credentials",
	); err != nil {
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
	if err := dynamoDBSSOAPIRequest(
		ctx,
		c.httpClient,
		http.MethodPost,
		endpoint,
		nil,
		map[string]any{
			"clientName": params.ClientName,
			"clientType": params.ClientType,
			"grantTypes": params.GrantTypes,
			"scopes":     params.Scopes,
		},
		&resp,
		"aws sso register-client",
	); err != nil {
		return dynamoDBSSOOIDCRegisterClientOutput{}, err
	}
	return dynamoDBSSOOIDCRegisterClientOutput{
		ClientID:     strings.TrimSpace(resp.ClientID),
		ClientSecret: strings.TrimSpace(resp.ClientSecret),
	}, nil
}

func (c *dynamoDBSSOOIDCHTTPClient) StartDeviceAuthorization(ctx context.Context, params dynamoDBSSOOIDCStartDeviceAuthorizationInput) (dynamoDBSSOOIDCStartDeviceAuthorizationOutput, error) {
	if err := c.validate(); err != nil {
		return dynamoDBSSOOIDCStartDeviceAuthorizationOutput{}, err
	}
	endpoint := strings.TrimRight(c.baseURL, "/") + "/device_authorization"
	var resp struct {
		DeviceCode              string `json:"deviceCode"`
		VerificationURI         string `json:"verificationUri"`
		VerificationURIComplete string `json:"verificationUriComplete"`
		UserCode                string `json:"userCode"`
		ExpiresIn               int32  `json:"expiresIn"`
		Interval                int32  `json:"interval"`
	}
	if err := dynamoDBSSOAPIRequest(
		ctx,
		c.httpClient,
		http.MethodPost,
		endpoint,
		nil,
		map[string]any{
			"clientId":     params.ClientID,
			"clientSecret": params.ClientSecret,
			"startUrl":     params.StartURL,
		},
		&resp,
		"aws sso start-device-authorization",
	); err != nil {
		return dynamoDBSSOOIDCStartDeviceAuthorizationOutput{}, err
	}
	return dynamoDBSSOOIDCStartDeviceAuthorizationOutput{
		DeviceCode:              strings.TrimSpace(resp.DeviceCode),
		VerificationURI:         strings.TrimSpace(resp.VerificationURI),
		VerificationURIComplete: strings.TrimSpace(resp.VerificationURIComplete),
		UserCode:                strings.TrimSpace(resp.UserCode),
		ExpiresIn:               resp.ExpiresIn,
		Interval:                resp.Interval,
	}, nil
}

func (c *dynamoDBSSOOIDCHTTPClient) CreateToken(ctx context.Context, params dynamoDBSSOOIDCCreateTokenInput) (dynamoDBSSOOIDCCreateTokenOutput, error) {
	if err := c.validate(); err != nil {
		return dynamoDBSSOOIDCCreateTokenOutput{}, err
	}
	endpoint := strings.TrimRight(c.baseURL, "/") + "/token"
	var resp struct {
		AccessToken string `json:"accessToken"`
		ExpiresIn   int32  `json:"expiresIn"`
	}
	if err := dynamoDBSSOAPIRequest(
		ctx,
		c.httpClient,
		http.MethodPost,
		endpoint,
		nil,
		map[string]any{
			"clientId":     params.ClientID,
			"clientSecret": params.ClientSecret,
			"grantType":    params.GrantType,
			"deviceCode":   params.DeviceCode,
		},
		&resp,
		"aws sso create-token",
	); err != nil {
		return dynamoDBSSOOIDCCreateTokenOutput{}, err
	}
	return dynamoDBSSOOIDCCreateTokenOutput{
		AccessToken: strings.TrimSpace(resp.AccessToken),
		ExpiresIn:   resp.ExpiresIn,
	}, nil
}

func (p DataSourcePayload) toDataSource(id string) datasource.DataSource {
	return datasource.DataSource{
		ID:         id,
		Name:       strings.TrimSpace(p.Name),
		Type:       p.Type,
		Host:       strings.TrimSpace(p.Host),
		Port:       p.Port,
		Username:   strings.TrimSpace(p.Username),
		Password:   p.Password,
		Database:   strings.TrimSpace(p.Database),
		AuthSource: strings.TrimSpace(p.AuthSource),
		Options:    p.Options,
		SecretRefs: datasource.PruneSecretRefs(p.SecretRefs),
	}
}

func validateDataSourcePayload(p DataSourcePayload) error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("name is required")
	}
	if p.Type == "" {
		return errors.New("type is required")
	}
	switch p.Type {
	case datasource.TypeMySQL, datasource.TypePostgreSQL, datasource.TypeMongoDB, datasource.TypeRedis, datasource.TypeElasticsearch, datasource.TypeChromaDB, datasource.TypeDynamoDB, datasource.TypeD1:
	default:
		return errors.New("unsupported type")
	}
	if err := datasource.ValidateSecretRefs(p.SecretRefs); err != nil {
		return err
	}
	if p.Type == datasource.TypeMongoDB {
		// A uri/hosts-based connection (including a secret-backed options.uri ref)
		// supplies addressing out of band, so don't require host/port — and don't
		// gate the exemption on host/port being empty, since the form may submit the
		// type's default port (27017) for a ref-backed datasource with no host UI.
		// An inline options.uri is stripped on save when a password ref shadows it,
		// so it only counts as addressing when it will survive. Hosts and a delegated
		// options.uri ref are never stripped, so they always satisfy addressing.
		inlineURIUsable := hasSQLOptionsURI(p.Options) && !datasource.InlineOptionURIWillBeStripped(p.SecretRefs)
		if hasMongoOptionsHosts(p.Options) || inlineURIUsable || datasource.HasResolvableOptionURIRef(p.SecretRefs) {
			// allow MongoDB uri/hosts without host/port
		} else {
			if strings.TrimSpace(p.Host) == "" {
				return errors.New("host is required")
			}
			if p.Port <= 0 {
				return errors.New("port is required")
			}
		}
	} else if p.Type == datasource.TypeRedis {
		if strings.TrimSpace(p.Host) == "" || p.Port <= 0 {
			if !hasRedisOptionsNodes(p.Options) {
				if strings.TrimSpace(p.Host) == "" {
					return errors.New("host is required")
				}
				if p.Port <= 0 {
					return errors.New("port is required")
				}
			}
		}
	} else if p.Type == datasource.TypeDynamoDB {
		if !hasDynamoDBRegion(p.Options) {
			return errors.New("region is required")
		}
	} else if p.Type == datasource.TypeD1 {
		if err := validateD1Options(p.Options, p.SecretRefs); err != nil {
			return err
		}
	} else if p.Type == datasource.TypeMySQL || p.Type == datasource.TypePostgreSQL {
		// An inline options.uri only counts as addressing if it will survive the save.
		// When a password ref is present, ClearInlineSecretsForRefs strips the inline
		// uri (it shadows the ref), so the persisted record would have no uri and no
		// host/port — require host/port or a delegated options.uri ref instead.
		inlineURIUsable := hasSQLOptionsURI(p.Options) && !datasource.InlineOptionURIWillBeStripped(p.SecretRefs)
		if !inlineURIUsable && !datasource.HasResolvableOptionURIRef(p.SecretRefs) {
			if strings.TrimSpace(p.Host) == "" {
				return errors.New("host is required")
			}
			if p.Port <= 0 {
				return errors.New("port is required")
			}
		}
	} else {
		if strings.TrimSpace(p.Host) == "" {
			return errors.New("host is required")
		}
		if p.Port <= 0 {
			return errors.New("port is required")
		}
	}
	if p.Port < 0 {
		return errors.New("port must be >= 0")
	}
	if p.Port < 0 || p.Port > 65535 {
		return errors.New("port out of range")
	}
	return nil
}

// hasMongoOptionsHosts reports whether options carries an explicit hosts list.
// Unlike an inline options.uri, the hosts list is never stripped on save, so it
// always satisfies MongoDB addressing regardless of any password ref.
func hasMongoOptionsHosts(options map[string]any) bool {
	if options == nil {
		return false
	}
	hostsRaw, ok := options["hosts"]
	if !ok {
		return false
	}
	switch v := hostsRaw.(type) {
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				return true
			}
		}
	case []string:
		for _, s := range v {
			if strings.TrimSpace(s) != "" {
				return true
			}
		}
	}
	return false
}

func hasSQLOptionsURI(options map[string]any) bool {
	if options == nil {
		return false
	}
	if uri, ok := options["uri"].(string); ok && strings.TrimSpace(uri) != "" {
		return true
	}
	return false
}

func hasRedisOptionsNodes(options map[string]any) bool {
	if options == nil {
		return false
	}
	nodesRaw, ok := options["nodes"]
	if !ok {
		return false
	}
	switch v := nodesRaw.(type) {
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				return true
			}
		}
	case []string:
		for _, s := range v {
			if strings.TrimSpace(s) != "" {
				return true
			}
		}
	case string:
		return strings.TrimSpace(v) != ""
	}
	return false
}

func hasDynamoDBRegion(options map[string]any) bool {
	if options == nil {
		return false
	}
	if region, ok := options["region"].(string); ok && strings.TrimSpace(region) != "" {
		return true
	}
	return false
}

func validateD1Options(options map[string]any, refs map[string]secrets.SecretRef) error {
	mode := strings.ToLower(strings.TrimSpace(optionAnyString(options, "mode")))
	databaseID := strings.TrimSpace(optionAnyString(options, "databaseId"))
	if databaseID == "" {
		return errors.New("databaseId is required for d1")
	}

	if mode == "local" {
		if strings.TrimSpace(optionAnyString(options, "binding")) == "" {
			return errors.New("binding is required for local mode")
		}
		return nil
	}

	accountID := strings.TrimSpace(optionAnyString(options, "accountId"))
	if accountID == "" {
		return errors.New("accountId is required for d1")
	}

	if mode == "" {
		if strings.TrimSpace(optionAnyString(options, "databaseName")) == "" {
			return errors.New("databaseName is required for d1")
		}
		return nil
	}
	if mode != "cloud" {
		return errors.New("mode must be local or cloud when provided")
	}

	authMode := strings.ToLower(strings.TrimSpace(optionAnyString(options, "authMode")))
	if authMode == "" {
		authMode = "wrangler"
	}
	if authMode != "wrangler" && authMode != "token" {
		return errors.New("authMode must be wrangler or token")
	}
	if authMode == "token" &&
		strings.TrimSpace(optionAnyString(options, "apiToken")) == "" &&
		!datasource.HasResolvableOptionRef(refs, "options.apiToken") {
		// The token may be delegated to a secret provider (resolved read-only at
		// execution time), in which case the inline value is absent by design.
		return errors.New("apiToken is required when authMode=token")
	}
	return nil
}

func optionAnyString(options map[string]any, key string) string {
	if options == nil {
		return ""
	}
	raw, ok := options[key]
	if !ok || raw == nil {
		return ""
	}
	switch typed := raw.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		rendered := strings.TrimSpace(fmt.Sprint(typed))
		if rendered == "<nil>" {
			return ""
		}
		return rendered
	}
}

func (a *App) ListDatasources() ([]datasource.DataSource, error) {
	items := a.store.List()
	out := make([]datasource.DataSource, 0, len(items))
	for _, item := range items {
		out = append(out, datasource.RedactDatasource(item))
	}
	return out, nil
}

func (a *App) GetDatasource(id string) (datasource.DataSource, error) {
	item, ok := a.store.Get(id)
	if !ok {
		return datasource.DataSource{}, errors.New("datasource not found")
	}
	return datasource.RedactDatasource(item), nil
}

func (a *App) CreateDatasource(payload DataSourcePayload) (datasource.DataSource, error) {
	if err := validateDataSourcePayload(payload); err != nil {
		return datasource.DataSource{}, err
	}
	ds := payload.toDataSource("")
	if ds.Type == datasource.TypeD1 {
		if strings.TrimSpace(ds.ID) == "" {
			ds.ID = newDatasourceID()
		}
	}
	createCheck := a.datasourceCreateCheck()
	created, err := a.store.CreateChecked(ds, func(input *datasource.DataSource, count int) error {
		if createCheck != nil {
			if err := createCheck(count); err != nil {
				return err
			}
		}
		*input = a.withRedisClusterNodesDiscovered(*input)
		if input.Type != datasource.TypeD1 {
			next, err := a.externalizeDatasourceSecrets(*input)
			if err != nil {
				return err
			}
			*input = next
			return nil
		}
		next, err := a.withD1MetadataPrepared(*input)
		if err != nil {
			return err
		}
		next, err = a.externalizeDatasourceSecrets(next)
		if err != nil {
			return err
		}
		*input = next
		return nil
	})
	if err != nil {
		return datasource.DataSource{}, err
	}
	return datasource.RedactDatasource(created), nil
}

func (a *App) UpdateDatasource(id string, payload DataSourcePayload) (datasource.DataSource, error) {
	if err := validateDataSourcePayload(payload); err != nil {
		return datasource.DataSource{}, err
	}
	ds := payload.toDataSource(id)
	existing, ok := a.store.Get(id)
	if !ok {
		return datasource.DataSource{}, errors.New("datasource not found")
	}
	ds = datasource.RestoreRedactedDatasource(ds, existing)
	if ds.Type == datasource.TypeD1 {
		ds.Options = d1CarryLegacyDevMetadataOnUpdate(ds.Options, existing.Options)
		next, err := a.withD1MetadataPrepared(ds)
		if err != nil {
			return datasource.DataSource{}, err
		}
		ds = next
	}
	ds = a.withRedisClusterNodesDiscovered(ds)
	ds, err := a.externalizeDatasourceSecrets(ds)
	if err != nil {
		return datasource.DataSource{}, err
	}
	updated, err := a.store.Update(id, ds)
	if err != nil {
		return datasource.DataSource{}, err
	}
	return datasource.RedactDatasource(updated), nil
}

func (a *App) externalizeDatasourceSecrets(ds datasource.DataSource) (datasource.DataSource, error) {
	if a == nil || a.datasourceSecrets == nil {
		return ds, nil
	}
	return a.datasourceSecrets.ExternalizeDatasourceSecrets(context.Background(), ds)
}

// SetDatasourceTrustLevel updates the per-datasource trust level. The four
// recognised values (approval, cautious, trusted, danger) control how far AI
// Chat / MCP / CLI go before asking for explicit approval. Returns the updated
// datasource. Unknown values are coerced to the default (cautious).
func (a *App) SetDatasourceTrustLevel(id string, trustLevel string) (datasource.DataSource, error) {
	existing, ok := a.store.Get(id)
	if !ok {
		return datasource.DataSource{}, errors.New("datasource not found")
	}
	// Clone options before mutating so we do not race with other readers/writers
	// holding a reference to the map returned by store.Get. Drop the legacy
	// `dangerous` flag on write so it cannot resurface if a third-party client
	// left it in place — MigrateOptions now ignores the legacy key when an
	// explicit trustLevel is set, but stripping here makes the persisted shape
	// unambiguous too.
	opts := make(map[string]any, len(existing.Options)+1)
	for k, v := range existing.Options {
		if k == datasource.LegacyDangerousOptionKey {
			continue
		}
		opts[k] = v
	}
	opts[datasource.TrustLevelOptionKey] = string(datasource.NormalizeTrustLevel(trustLevel))
	existing.Options = opts
	return a.store.Update(id, existing)
}

func (a *App) DeleteDatasource(id string) (bool, error) {
	if a.toolService == nil {
		// Fallback for code paths that bypass the shared service layer; the
		// cascade clean-up below is the same one the service runs.
		if err := a.store.Delete(id); err != nil {
			return false, err
		}
		if a.redisProtoStore != nil {
			if _, err := a.redisProtoStore.DeleteByDatasource(strings.TrimSpace(id)); err != nil && a.errorLog != nil {
				a.errorLog.Printf("delete redis protobuf schemas for %s: %v", id, err)
			}
		}
		return true, nil
	}
	return a.toolService.DeleteDatasource(context.Background(), id)
}

func (a *App) TestDatasource(id string) (bool, error) {
	ds, ok := a.store.Get(id)
	if !ok {
		return false, errors.New("datasource not found")
	}
	ctx, finishTiming := a.beginDatasourceTiming(context.Background(), "app.test_datasource", ds, "", console.ExecuteOptions{}, false)
	err := a.manager.TestConnection(ctx, ds)
	finishTiming(err)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (a *App) TestDatasourcePayload(payload DataSourcePayload, existingID string) (bool, error) {
	if err := validateDataSourcePayload(payload); err != nil {
		return false, err
	}
	ds := payload.toDataSource("")
	existingID = strings.TrimSpace(existingID)
	var existing datasource.DataSource
	haveExisting := false
	if existingID != "" {
		if found, ok := a.store.Get(existingID); ok {
			existing = found
			haveExisting = true
			ds = datasource.RestoreRedactedDatasource(ds, existing)
		}
	}
	// Reference-only secret binding. A SecretRef may only be resolved against the
	// stored datasource that owns it — never against a target the caller supplies in
	// this payload. Otherwise a renderer that listed a ref-backed datasource could
	// reuse its secretRefs in a new/edited payload pointed at an attacker-controlled
	// host, driving the backend to resolve the secret and AUTH outward to that host
	// (the agent surface rejects the same pattern outright in Service.TestDatasourcePayload).
	// The decision keys off the *restored* refs: RestoreRedactedDatasource re-adds the
	// stored ref when the secret is unchanged ([REDACTED]) and drops it when the user
	// switches that field back to a typed value, so a real ref here means this very
	// connection still delegates to a secret. A switch-to-manual edit carries no real
	// ref and is tested with the newly typed credential against the form target.
	if datasource.HasRealSecretRefs(ds.SecretRefs) {
		// A brand-new datasource cannot be tested with a ref attached: it must be saved
		// first so the ref is bound to a concrete, operator-visible target, then tested by id.
		if !haveExisting {
			return false, errors.New("save the datasource before testing a secret-backed connection")
		}
		// Only the unchanged stored connection may be resolved here. If the caller edited
		// the reference (provider/key/version) or the target (host/database/options), the
		// stored secret must not be resolved toward the new, unsaved target — that is the
		// exfiltration shape — and testing the stored record would falsely report success
		// for settings Save will replace. Require a save first so the new binding is
		// committed (then it is tested by id via TestDatasource); otherwise test the
		// stored datasource as persisted.
		if !secretBackedConnectionMatchesStored(ds, existing) {
			return false, errors.New("save the datasource before testing the updated secret-backed connection")
		}
		ds = existing
	}
	ctx, finishTiming := a.beginDatasourceTiming(context.Background(), "app.test_datasource_payload", ds, "", console.ExecuteOptions{}, false)
	err := a.manager.TestConnection(ctx, ds)
	finishTiming(err)
	if err != nil {
		return false, err
	}
	return true, nil
}

// secretBackedConnectionMatchesStored reports whether the restored, secret-backed
// payload describes the same reference and connection target as the stored record.
// Only then may the stored secret be resolved for a Test: an edited reference or
// target must be saved first so the secret is never resolved toward an unverified
// destination and so the test reflects what Save will persist. The comparison is
// conservative — any meaningful connection difference fails it — because a false
// "changed" only asks the operator to save first, while a false "unchanged" would
// resolve a secret toward an unintended target.
func secretBackedConnectionMatchesStored(restored, stored datasource.DataSource) bool {
	if restored.Type != stored.Type ||
		restored.Host != stored.Host ||
		restored.Port != stored.Port ||
		strings.TrimSpace(restored.Username) != strings.TrimSpace(stored.Username) ||
		strings.TrimSpace(restored.Database) != strings.TrimSpace(stored.Database) ||
		strings.TrimSpace(restored.AuthSource) != strings.TrimSpace(stored.AuthSource) {
		return false
	}
	if !reflect.DeepEqual(restored.SecretRefs, stored.SecretRefs) {
		return false
	}
	return optionsEqual(restored.Options, stored.Options)
}

// optionsEqual compares two option maps, treating nil and empty as equal so a
// round-tripped payload that drops an empty map does not read as a change.
func optionsEqual(a, b map[string]any) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

func (a *App) withRedisClusterNodesDiscovered(ds datasource.DataSource) datasource.DataSource {
	if ds.Type != datasource.TypeRedis {
		return ds
	}
	if hasRedisOptionsNodes(ds.Options) {
		return ds
	}
	if strings.TrimSpace(ds.Host) == "" || ds.Port <= 0 {
		return ds
	}
	if a == nil || a.manager == nil {
		return ds
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := a.manager.ExecuteInternal(ctx, ds, "CLUSTER NODES", console.ExecuteOptions{})
	if err != nil {
		return ds
	}
	raw, ok := queryResultText(result)
	if !ok {
		return ds
	}
	nodes := parseRedisClusterNodes(raw)
	if len(nodes) == 0 {
		return ds
	}

	next := ds
	next.Options = copyDatasourceOptions(ds.Options)
	next.Options["nodes"] = nodes
	return next
}

func (a *App) withD1MetadataPrepared(ds datasource.DataSource) (datasource.DataSource, error) {
	if ds.Type != datasource.TypeD1 {
		return ds, nil
	}
	mode := strings.ToLower(strings.TrimSpace(optionAnyString(ds.Options, "mode")))
	databaseID := strings.TrimSpace(optionAnyString(ds.Options, "databaseId"))
	if databaseID == "" {
		return datasource.DataSource{}, errors.New("databaseId is required for d1")
	}
	databaseName := strings.TrimSpace(optionAnyString(ds.Options, "databaseName"))
	if databaseName == "" {
		databaseName = strings.TrimSpace(ds.Database)
	}
	if databaseName == "" {
		if mode == "local" || mode == "cloud" {
			databaseName = strings.TrimSpace(databaseID)
		} else {
			return datasource.DataSource{}, errors.New("databaseName is required for d1")
		}
	}
	binding := strings.TrimSpace(optionAnyString(ds.Options, "binding"))
	if binding == "" {
		binding = d1BindingFromDatabaseName(databaseName)
	}
	if binding == "" {
		return datasource.DataSource{}, errors.New("binding is required for d1")
	}
	devProjectPath, err := d1NormalizeProjectPath(optionAnyString(ds.Options, "devProjectPath"))
	if err != nil {
		return datasource.DataSource{}, err
	}
	supportDev := optionAnyBool(ds.Options, "supportDev") && devProjectPath != ""
	legacyWranglerConfigPath := strings.TrimSpace(optionAnyString(ds.Options, "wranglerConfigPath"))
	previousDatabaseID := strings.TrimSpace(optionAnyString(ds.Options, "previousDatabaseId"))
	previousBinding := strings.TrimSpace(optionAnyString(ds.Options, "previousBinding"))
	migrationDir := filepath.ToSlash(filepath.Join("migrations", d1MigrationDirName(databaseName, databaseID)))

	next := ds
	next.Database = databaseName
	next.Options = copyDatasourceOptions(ds.Options)
	next.Options["databaseId"] = databaseID
	next.Options["databaseName"] = databaseName
	next.Options["binding"] = binding
	next.Options["supportDev"] = supportDev
	if supportDev {
		configPath, err := a.ensureD1WranglerConfig(devProjectPath, d1WranglerDatabaseEntry{
			Binding:       binding,
			DatabaseName:  databaseName,
			DatabaseID:    databaseID,
			MigrationsDir: migrationDir,
		}, previousDatabaseID, previousBinding)
		if err != nil {
			return datasource.DataSource{}, err
		}
		next.Options["devProjectPath"] = devProjectPath
		next.Options["wranglerConfigPath"] = configPath
		next.Options["migrationsDir"] = migrationDir
	} else if mode != "local" {
		// New remote-mode datasources should not persist autogenerated wrangler files.
		delete(next.Options, "devProjectPath")
		if legacyWranglerConfigPath == "" {
			delete(next.Options, "wranglerConfigPath")
			delete(next.Options, "migrationsDir")
		} else {
			configPath, err := a.ensureD1WranglerConfig(filepath.Dir(legacyWranglerConfigPath), d1WranglerDatabaseEntry{
				Binding:       binding,
				DatabaseName:  databaseName,
				DatabaseID:    databaseID,
				MigrationsDir: migrationDir,
			}, previousDatabaseID, previousBinding)
			if err != nil {
				if errors.Is(err, errD1DevProjectPathMissing) || errors.Is(err, errD1DevProjectPathNotDir) {
					delete(next.Options, "wranglerConfigPath")
					delete(next.Options, "migrationsDir")
					return next, nil
				}
				return datasource.DataSource{}, err
			}
			next.Options["wranglerConfigPath"] = configPath
			next.Options["migrationsDir"] = migrationDir
		}
	}
	delete(next.Options, "previousDatabaseId")
	delete(next.Options, "previousBinding")
	return next, nil
}

type d1WranglerDatabaseEntry struct {
	Binding       string
	DatabaseName  string
	DatabaseID    string
	MigrationsDir string
}

var (
	errD1DevProjectPathMissing = errors.New("devProjectPath does not exist")
	errD1DevProjectPathNotDir  = errors.New("devProjectPath must be a directory")
)

func d1MigrationDirName(databaseName, databaseID string) string {
	base := d1NormalizeMigrationSegment(databaseName)
	if base == "" {
		base = "datasource"
	}

	identifier := d1NormalizeMigrationSegment(databaseID)
	if identifier == "" || base == identifier {
		return base
	}
	return base + "-" + identifier
}

func d1NormalizeMigrationSegment(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return ""
	}
	re := regexp.MustCompile(`[^a-z0-9._-]+`)
	normalized := re.ReplaceAllString(trimmed, "-")
	return strings.Trim(normalized, "-._")
}

func d1CarryLegacyDevMetadataOnUpdate(nextOptions, existingOptions map[string]any) map[string]any {
	merged := copyDatasourceOptions(nextOptions)
	if previousDatabaseID := strings.TrimSpace(optionAnyString(existingOptions, "databaseId")); previousDatabaseID != "" {
		merged["previousDatabaseId"] = previousDatabaseID
	}
	if previousBinding := strings.TrimSpace(optionAnyString(existingOptions, "binding")); previousBinding != "" {
		merged["previousBinding"] = previousBinding
	}
	if !d1IsLegacyDevDatasource(existingOptions) {
		return merged
	}
	if strings.TrimSpace(optionAnyString(nextOptions, "wranglerConfigPath")) != "" {
		return merged
	}
	if strings.ToLower(strings.TrimSpace(optionAnyString(nextOptions, "mode"))) == "local" {
		return merged
	}
	if _, hasSupportDevOption := nextOptions["supportDev"]; hasSupportDevOption {
		// Respect explicit supportDev updates instead of restoring legacy dev metadata.
		return merged
	}
	legacyWrangler := strings.TrimSpace(optionAnyString(existingOptions, "wranglerConfigPath"))
	if legacyWrangler == "" {
		return merged
	}
	merged["wranglerConfigPath"] = legacyWrangler
	if legacyMigrationsDir := strings.TrimSpace(optionAnyString(existingOptions, "migrationsDir")); legacyMigrationsDir != "" {
		merged["migrationsDir"] = legacyMigrationsDir
	}
	return merged
}

func d1IsLegacyDevDatasource(options map[string]any) bool {
	if strings.TrimSpace(optionAnyString(options, "wranglerConfigPath")) == "" {
		return false
	}
	if optionAnyBool(options, "supportDev") {
		return false
	}
	if strings.TrimSpace(optionAnyString(options, "devProjectPath")) != "" {
		return false
	}
	return true
}

func newDatasourceID() string {
	now := time.Now().UTC().UnixNano()
	return fmt.Sprintf("ds_%x", now)
}

func (a *App) ensureD1WranglerConfig(projectPath string, entry d1WranglerDatabaseEntry, previousDatabaseID, previousBinding string) (string, error) {
	trimmedPath := strings.TrimSpace(projectPath)
	if trimmedPath == "" {
		return "", errors.New("devProjectPath is required when supportDev is enabled")
	}
	info, err := os.Stat(trimmedPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errD1DevProjectPathMissing
		}
		return "", err
	}
	if !info.IsDir() {
		return "", errD1DevProjectPathNotDir
	}

	configPath := filepath.Join(trimmedPath, "wrangler.toml")
	raw, readErr := os.ReadFile(configPath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return "", readErr
	}
	content := string(raw)
	if next, changed := d1WranglerUpsertDatabaseEntryWithFallback(content, entry, previousDatabaseID, previousBinding); changed {
		content = next
		if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
			return "", err
		}
	}
	return configPath, nil
}

func d1WranglerUpsertDatabaseEntry(content string, entry d1WranglerDatabaseEntry) (string, bool) {
	return d1WranglerUpsertDatabaseEntryWithFallback(content, entry, "", "")
}

func d1WranglerUpsertDatabaseEntryWithFallback(content string, entry d1WranglerDatabaseEntry, previousDatabaseID, previousBinding string) (string, bool) {
	attempts := []struct {
		key   string
		value string
	}{
		{key: "database_id", value: entry.DatabaseID},
		{key: "database_id", value: previousDatabaseID},
		{key: "binding", value: entry.Binding},
		{key: "binding", value: previousBinding},
	}
	seen := make(map[string]struct{}, len(attempts))
	for _, attempt := range attempts {
		trimmedValue := strings.TrimSpace(attempt.value)
		if trimmedValue == "" {
			continue
		}
		signature := attempt.key + ":" + trimmedValue
		if _, ok := seen[signature]; ok {
			continue
		}
		seen[signature] = struct{}{}
		if replaced, ok := d1WranglerReplaceDatabaseEntryByKey(content, entry, attempt.key, trimmedValue); ok {
			return replaced, replaced != content
		}
	}
	next := d1WranglerAppendDatabaseEntry(content, entry)
	return next, next != content
}

func d1WranglerReplaceDatabaseEntryByKey(content string, entry d1WranglerDatabaseEntry, key, value string) (string, bool) {
	trimmedValue := strings.TrimSpace(value)
	if trimmedValue == "" {
		return "", false
	}
	return d1WranglerReplaceDatabaseEntry(content, entry, func(block string) bool {
		return d1WranglerBlockHasTomlString(block, key, trimmedValue)
	})
}

func d1WranglerReplaceDatabaseEntry(content string, entry d1WranglerDatabaseEntry, shouldReplace func(block string) bool) (string, bool) {
	if shouldReplace == nil {
		return "", false
	}
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return "", false
	}
	out := make([]string, 0, len(lines))
	replaced := false
	for i := 0; i < len(lines); {
		if strings.TrimSpace(lines[i]) != "[[d1_databases]]" {
			out = append(out, lines[i])
			i++
			continue
		}
		j := i + 1
		for j < len(lines) {
			trimmed := strings.TrimSpace(lines[j])
			if strings.HasPrefix(trimmed, "[") {
				break
			}
			j++
		}
		block := strings.Join(lines[i:j], "\n")
		if !replaced && shouldReplace(block) {
			replacement := strings.TrimRight(d1WranglerToml(entry), "\n")
			out = append(out, strings.Split(replacement, "\n")...)
			replaced = true
		} else {
			out = append(out, lines[i:j]...)
		}
		i = j
	}
	if !replaced {
		return "", false
	}
	updated := strings.TrimRight(strings.Join(out, "\n"), "\n")
	return updated + "\n", true
}

func d1WranglerBlockHasTomlString(block, key, value string) bool {
	trimmedKey := strings.TrimSpace(key)
	trimmedValue := strings.TrimSpace(value)
	if trimmedKey == "" || trimmedValue == "" {
		return false
	}
	pattern := fmt.Sprintf(`(?m)^\s*%s\s*=\s*%s\s*$`, regexp.QuoteMeta(trimmedKey), regexp.QuoteMeta(d1TomlString(trimmedValue)))
	return regexp.MustCompile(pattern).MatchString(block)
}

func d1WranglerAppendDatabaseEntry(content string, entry d1WranglerDatabaseEntry) string {
	block := d1WranglerToml(entry)
	trimmed := strings.TrimRight(content, "\n")
	if strings.TrimSpace(trimmed) == "" {
		return block
	}
	return trimmed + "\n\n" + strings.TrimRight(block, "\n") + "\n"
}

func d1WranglerToml(entry d1WranglerDatabaseEntry) string {
	lines := []string{
		"[[d1_databases]]",
		`binding = ` + d1TomlString(entry.Binding),
		`database_name = ` + d1TomlString(entry.DatabaseName),
		`database_id = ` + d1TomlString(entry.DatabaseID),
	}
	if strings.TrimSpace(entry.MigrationsDir) != "" {
		lines = append(lines, `migrations_dir = `+d1TomlString(entry.MigrationsDir))
	}
	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

func d1NormalizeProjectPath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	if strings.HasPrefix(trimmed, "~/") || trimmed == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if trimmed == "~" {
			trimmed = home
		} else {
			trimmed = filepath.Join(home, strings.TrimPrefix(trimmed, "~/"))
		}
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func d1BindingFromDatabaseName(databaseName string) string {
	trimmed := strings.TrimSpace(strings.ToLower(databaseName))
	re := regexp.MustCompile(`[^a-z0-9_]+`)
	binding := re.ReplaceAllString(trimmed, "_")
	binding = strings.Trim(binding, "_")
	if binding == "" {
		return "db"
	}
	if binding[0] >= '0' && binding[0] <= '9' {
		return "db_" + binding
	}
	return binding
}

func optionAnyBool(options map[string]any, key string) bool {
	if options == nil {
		return false
	}
	raw, ok := options[key]
	if !ok || raw == nil {
		return false
	}
	switch typed := raw.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true
		default:
			return false
		}
	case int:
		return typed != 0
	case int8:
		return typed != 0
	case int16:
		return typed != 0
	case int32:
		return typed != 0
	case int64:
		return typed != 0
	case uint:
		return typed != 0
	case uint8:
		return typed != 0
	case uint16:
		return typed != 0
	case uint32:
		return typed != 0
	case uint64:
		return typed != 0
	case float32:
		return typed != 0
	case float64:
		return typed != 0
	default:
		return false
	}
}

func d1DatasourceSupportsDev(options map[string]any) bool {
	if strings.ToLower(strings.TrimSpace(optionAnyString(options, "mode"))) == "local" {
		return true
	}
	if strings.TrimSpace(optionAnyString(options, "wranglerConfigPath")) != "" {
		return true
	}
	if !optionAnyBool(options, "supportDev") {
		return false
	}
	if strings.TrimSpace(optionAnyString(options, "devProjectPath")) == "" {
		return false
	}
	return true
}

func d1TomlString(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

func (a *App) DynamoDBSSOListProfiles(configPath string) ([]DynamoDBSSOProfile, error) {
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

func (a *App) DynamoDBSSOLogin(profile string) (DynamoDBSSOLoginResult, error) {
	configPath, err := awsConfigPath("")
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	token, err := a.dynamoDBSSOEnsureAccessToken(ctx, profileConfig, ssoRegion)
	if err != nil {
		return DynamoDBSSOLoginResult{}, err
	}

	return DynamoDBSSOLoginResult{
		AccessToken: token.AccessToken,
		ExpiresAt:   token.ExpiresAt,
	}, nil
}

func (a *App) DynamoDBSSOOAuthAuthorize(profile, region, configPath string) (DynamoDBSSOOAuthResult, error) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	token, err := a.dynamoDBSSOEnsureAccessToken(ctx, profileConfig, ssoRegion)
	if err != nil {
		return DynamoDBSSOOAuthResult{}, err
	}

	accountID := strings.TrimSpace(profileConfig.AccountID)
	if accountID == "" {
		accounts, err := a.DynamoDBSSOListAccounts(token.AccessToken, ssoRegion)
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
		roles, err := a.DynamoDBSSOListAccountRoles(accountID, token.AccessToken, ssoRegion)
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

	credentials, err := a.DynamoDBSSOGetRoleCredentials(accountID, roleName, token.AccessToken, ssoRegion)
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

func (a *App) DynamoDBSSOListAccounts(accessToken, region string) ([]DynamoDBSSOAccount, error) {
	accessToken = strings.TrimSpace(accessToken)
	region = strings.TrimSpace(region)
	if accessToken == "" {
		return nil, errors.New("accessToken is required")
	}
	if region == "" {
		return nil, errors.New("region is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	client := newDynamoDBSSOClient(region, a.resolvedHTTPClient())
	out := make([]DynamoDBSSOAccount, 0, 8)
	nextToken := ""
	for {
		resp, err := client.ListAccounts(ctx, dynamoDBSSOListAccountsInput{
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

func (a *App) DynamoDBSSOListAccountRoles(accountID, accessToken, region string) ([]DynamoDBSSORole, error) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	client := newDynamoDBSSOClient(region, a.resolvedHTTPClient())
	out := make([]DynamoDBSSORole, 0, 8)
	nextToken := ""
	for {
		resp, err := client.ListAccountRoles(ctx, dynamoDBSSOListAccountRolesInput{
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
			out = append(out, DynamoDBSSORole{
				RoleName:  roleName,
				AccountID: roleAccountID,
			})
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

func (a *App) DynamoDBSSOGetRoleCredentials(accountID, roleName, accessToken, region string) (DynamoDBSSORoleCredentials, error) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	client := newDynamoDBSSOClient(region, a.resolvedHTTPClient())
	resp, err := client.GetRoleCredentials(ctx, dynamoDBSSOGetRoleCredentialsInput{
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

func (a *App) resolvedHTTPClient() *http.Client {
	if a != nil && a.httpClient != nil {
		return a.httpClient
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (a *App) dynamoDBSSOEnsureAccessToken(ctx context.Context, profileConfig awsProfileConfig, oidcRegion string) (awsSSOCacheToken, error) {
	if cacheDir, err := awsSSOCacheDir(); err == nil {
		if token, tokenErr := awsResolveSSOCacheToken(cacheDir, profileConfig.StartURL); tokenErr == nil {
			return token, nil
		}
	}
	return a.dynamoDBSSOAuthorizeWithDeviceCode(ctx, profileConfig, oidcRegion)
}

func (a *App) dynamoDBSSOAuthorizeWithDeviceCode(ctx context.Context, profileConfig awsProfileConfig, oidcRegion string) (awsSSOCacheToken, error) {
	startURL := strings.TrimSpace(profileConfig.StartURL)
	if startURL == "" {
		return awsSSOCacheToken{}, errors.New("sso_start_url is required for AWS SSO profile")
	}
	oidcRegion = strings.TrimSpace(oidcRegion)
	if oidcRegion == "" {
		return awsSSOCacheToken{}, errors.New("sso_region or region is required for AWS SSO profile")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	clientName := strings.TrimSpace(profileConfig.Name)
	if clientName == "" {
		clientName = "futrixdata-dynamodb-sso"
	}

	oidcClient := newDynamoDBSSOOIDCClient(oidcRegion, a.resolvedHTTPClient())
	registerResp, err := oidcClient.RegisterClient(ctx, dynamoDBSSOOIDCRegisterClientInput{
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

	authorizeResp, err := oidcClient.StartDeviceAuthorization(ctx, dynamoDBSSOOIDCStartDeviceAuthorizationInput{
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
	if err := openDynamoDBSSOVerificationURL(verifyURL); err != nil {
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
	pollCtx, cancel := context.WithTimeout(ctx, remaining)
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
			return awsSSOCacheToken{
				AccessToken: accessToken,
				ExpiresAt:   expiresAt.Format(time.RFC3339),
				ExpiresTime: expiresAt,
			}, nil
		}

		var apiErr *dynamoDBSSOAPIError
		if errors.As(tokenErr, &apiErr) && (apiErr.hasCode("AuthorizationPendingException") || apiErr.hasCode("authorization_pending")) {
			if waitErr := waitDynamoDBSSOPollInterval(pollCtx, interval); waitErr != nil {
				return awsSSOCacheToken{}, fmt.Errorf("aws sso authorization timed out: %w", waitErr)
			}
			continue
		}

		if errors.As(tokenErr, &apiErr) && (apiErr.hasCode("SlowDownException") || apiErr.hasCode("slow_down")) {
			interval += time.Second
			if waitErr := waitDynamoDBSSOPollInterval(pollCtx, interval); waitErr != nil {
				return awsSSOCacheToken{}, fmt.Errorf("aws sso authorization timed out: %w", waitErr)
			}
			continue
		}

		if errors.As(tokenErr, &apiErr) && (apiErr.hasCode("AccessDeniedException") || apiErr.hasCode("access_denied")) {
			return awsSSOCacheToken{}, errors.New("aws sso authorization denied by user")
		}

		if errors.As(tokenErr, &apiErr) && (apiErr.hasCode("ExpiredTokenException") || apiErr.hasCode("expired_token")) {
			return awsSSOCacheToken{}, errors.New("aws sso device authorization expired; retry OAuth authorization")
		}

		if errors.Is(tokenErr, context.DeadlineExceeded) || errors.Is(tokenErr, context.Canceled) {
			return awsSSOCacheToken{}, fmt.Errorf("aws sso authorization timed out: %w", tokenErr)
		}
		return awsSSOCacheToken{}, fmt.Errorf("aws sso create-token failed: %w", tokenErr)
	}
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
		name      string
		region    string
		ssoRegion string
		start     string
		accountID string
		roleName  string
		session   string
	}
	type sessionEntry struct {
		name      string
		ssoRegion string
		start     string
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
				currentProfile = "default"
				currentSession = ""
			case strings.HasPrefix(section, "profile "):
				currentProfile = strings.TrimSpace(strings.TrimPrefix(section, "profile "))
				currentSession = ""
			case strings.HasPrefix(section, "sso-session "):
				currentProfile = ""
				currentSession = strings.TrimSpace(strings.TrimPrefix(section, "sso-session "))
			default:
				currentProfile = ""
				currentSession = ""
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
		value := strings.TrimSpace(line[sep+1:])
		value = strings.Trim(value, `"`)
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
		if entry == nil {
			continue
		}
		if strings.TrimSpace(entry.name) == "" {
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
		left := strings.ToLower(strings.TrimSpace(out[i].Name))
		right := strings.ToLower(strings.TrimSpace(out[j].Name))
		return left < right
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
		path := filepath.Join(cacheDir, entry.Name())
		raw, err := os.ReadFile(path)
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
		if err != nil {
			continue
		}
		if expiresAt.Before(now) {
			continue
		}
		if !matched || expiresAt.After(best.ExpiresTime) {
			matched = true
			best = awsSSOCacheToken{
				AccessToken: token,
				ExpiresAt:   expiresAtRaw,
				ExpiresTime: expiresAt,
			}
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
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z0700",
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05UTC",
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, trimmed)
		if err == nil {
			return parsed.UTC(), nil
		}
	}
	if strings.HasSuffix(trimmed, "UTC") {
		normalized := strings.TrimSuffix(trimmed, "UTC") + "Z"
		parsed, err := time.Parse(time.RFC3339, normalized)
		if err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid expiresAt: %s", trimmed)
}

func (a *App) D1OAuthLogin() (D1OAuthSession, error) {
	return a.d1OAuthLogin(false)
}

func (a *App) D1OAuthReLogin() (D1OAuthSession, error) {
	return a.d1OAuthLogin(true)
}

func (a *App) D1IsWranglerInstalled() bool {
	return d1WranglerInstalled()
}

func (a *App) d1OAuthLogin(forceBrowserLogin bool) (D1OAuthSession, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	base := []string{"npx", "wrangler"}

	if !forceBrowserLogin {
		// Reuse the existing session first so valid tokens do not trigger browser login again.
		session, err := a.d1ResolveOAuthSession(ctx, base)
		if err == nil {
			return session, nil
		}
	}

	loginCommand := append([]string{}, base...)
	loginCommand = append(loginCommand, "login")
	if _, loginErr := a.runAppCommand(ctx, loginCommand); loginErr != nil {
		lower := strings.ToLower(loginErr.Error())
		if !strings.Contains(lower, "already logged in") {
			return D1OAuthSession{}, loginErr
		}
	}

	return a.d1ResolveOAuthSession(ctx, base)
}

func (a *App) d1ResolveOAuthSession(ctx context.Context, baseCommand []string) (D1OAuthSession, error) {
	token, err := a.d1ResolveWranglerToken(ctx, baseCommand)
	if err != nil {
		return D1OAuthSession{}, err
	}
	accounts, accountID, err := a.d1ResolveWranglerAccounts(ctx, baseCommand)
	if err != nil {
		return D1OAuthSession{}, err
	}
	return D1OAuthSession{
		Accounts:  accounts,
		AccountID: accountID,
		Token:     token,
	}, nil
}

func (a *App) D1ListCloudDatabases(accountID, token string) ([]D1CloudDatabase, error) {
	accountID = strings.TrimSpace(accountID)
	token = strings.TrimSpace(token)
	if accountID == "" {
		return nil, errors.New("accountId is required")
	}
	if token == "" {
		return nil, errors.New("token is required")
	}
	return a.d1ListCloudDatabases(context.Background(), accountID, token)
}

// D1ListCloudDatabasesForDatasource lists databases for an already-stored D1 Cloud
// datasource using its server-side token. The list/get payloads redact the API
// token to "[REDACTED]", so the edit form cannot pass it back; this resolves the
// real token (inline or secret-ref backed) by id without ever exposing it to the
// client.
func (a *App) D1ListCloudDatabasesForDatasource(id, accountID string) ([]D1CloudDatabase, error) {
	id = strings.TrimSpace(id)
	accountID = strings.TrimSpace(accountID)
	if id == "" {
		return nil, errors.New("datasource id is required")
	}
	if accountID == "" {
		return nil, errors.New("accountId is required")
	}
	item, ok := a.store.Get(id)
	if !ok {
		return nil, errors.New("datasource not found")
	}
	// Only resolve and use the token for an actual D1 datasource. Other integrations
	// may also store an options.apiToken, and this helper must never forward an
	// unrelated (possibly SecretRef-backed) token to the Cloudflare D1 endpoint.
	if item.Type != datasource.TypeD1 {
		return nil, errors.New("datasource is not a Cloudflare D1 datasource")
	}
	// The server-side token is scoped to the datasource's configured Cloudflare
	// account, and this binding exists precisely to use it without exposing it to
	// the renderer. The account must therefore come from the stored datasource — a
	// renderer caller must not redirect the stored secret to an arbitrary account.
	// The account ID is a plaintext identifier (never a secret ref — see
	// SupportedSecretFieldPath), so enforce the match locally BEFORE any
	// secret-provider read; a mismatched request then fails fast and cheaply rather
	// than triggering a Vault read that only fails on availability afterward.
	storedAccount := strings.TrimSpace(optionAnyString(item.Options, "accountId"))
	if storedAccount == "" {
		return nil, errors.New("datasource has no configured account")
	}
	if accountID != storedAccount {
		return nil, errors.New("accountId does not match the datasource account")
	}
	ctx := context.Background()
	resolved, err := a.manager.ResolveDatasource(ctx, item)
	if err != nil {
		return nil, err
	}
	token := optionAnyString(resolved.Options, "apiToken")
	if token == "" {
		return nil, errors.New("token is required")
	}
	return a.d1ListCloudDatabases(ctx, storedAccount, token)
}

func (a *App) D1CreateCloudDatabase(accountID, token, name string) (D1CloudDatabase, error) {
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

	endpoint := fmt.Sprintf(
		"%s/accounts/%s/d1/database",
		d1CloudflareBaseURL(),
		url.PathEscape(accountID),
	)
	body, err := json.Marshal(map[string]any{
		"name": name,
	})
	if err != nil {
		return D1CloudDatabase{}, err
	}
	raw, err := a.d1CloudflareRequest(context.Background(), http.MethodPost, endpoint, token, body)
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
	return D1CloudDatabase{
		ID:   id,
		Name: strings.TrimSpace(envelope.Result.Name),
	}, nil
}

func (a *App) d1ListCloudDatabases(ctx context.Context, accountID, token string) ([]D1CloudDatabase, error) {
	endpoint := fmt.Sprintf(
		"%s/accounts/%s/d1/database",
		d1CloudflareBaseURL(),
		url.PathEscape(accountID),
	)
	raw, err := a.d1CloudflareRequest(ctx, http.MethodGet, endpoint, token, nil)
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
		out = append(out, D1CloudDatabase{
			ID:   id,
			Name: strings.TrimSpace(item.Name),
		})
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

func (a *App) d1CloudflareRequest(ctx context.Context, method, endpoint, token string, body []byte) ([]byte, error) {
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

	client := a.httpClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
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

func (a *App) d1ResolveWranglerToken(ctx context.Context, baseCommand []string) (string, error) {
	command := append([]string{}, baseCommand...)
	command = append(command, "auth", "token", "--json")
	raw, err := a.runAppCommand(ctx, command)
	if err != nil {
		return "", fmt.Errorf("wrangler auth token failed: %w", err)
	}
	var payload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", fmt.Errorf("parse wrangler auth token: %w", err)
	}
	token := strings.TrimSpace(payload.Token)
	if token == "" {
		return "", errors.New("wrangler auth token is empty")
	}
	return token, nil
}

func (a *App) d1ResolveWranglerAccounts(ctx context.Context, baseCommand []string) ([]D1OAuthAccount, string, error) {
	command := append([]string{}, baseCommand...)
	command = append(command, "whoami", "--json")
	raw, err := a.runAppCommand(ctx, command)
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
		accounts = append(accounts, D1OAuthAccount{
			ID:   accountID,
			Name: accountName,
		})
	}

	if selectedAccountID != "" {
		if _, exists := seen[selectedAccountID]; !exists {
			accounts = append(accounts, D1OAuthAccount{
				ID:   selectedAccountID,
				Name: selectedAccountID,
			})
			seen[selectedAccountID] = struct{}{}
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

func (a *App) runAppCommand(ctx context.Context, command []string) ([]byte, error) {
	if a != nil && a.runCommand != nil {
		return a.runCommand(ctx, command)
	}
	return appRunCommand(ctx, command)
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

func appRunCommand(ctx context.Context, command []string) ([]byte, error) {
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
