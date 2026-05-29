package cli

import (
	"context"
	"flag"
	"fmt"
	"io"

	"futrixdata/platform/internal/datasourceops"
)

func (r *Runner) runDynamoDBSSO(ctx context.Context, service Service, opts Options, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing dynamodb-sso subcommand.\n\n%s", dynamoDBSSOUsage())
	}
	fs := flag.NewFlagSet("dynamodb-sso "+args[0], flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var profile, region, configPath, accessToken, accountID, roleName string
	fs.StringVar(&profile, "profile", "", "aws profile")
	fs.StringVar(&region, "region", "", "aws region")
	fs.StringVar(&configPath, "config-path", "", "aws config path")
	fs.StringVar(&accessToken, "access-token", "", "access token")
	fs.StringVar(&accountID, "account-id", "", "account id")
	fs.StringVar(&roleName, "role-name", "", "role name")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	// Access tokens are intentionally omitted from the audit param maps —
	// downstream RedactValue would zero them out, but excluding them up front
	// keeps the on-disk row free of credential-length information.
	switch args[0] {
	case "list-profiles":
		items, err := auditedCall(ctx, opts, service, "dynamodb_sso_list_profiles", map[string]any{"configPath": configPath}, func(c context.Context) ([]datasourceops.DynamoDBSSOProfile, error) {
			return service.DynamoDBSSOListProfiles(c, configPath)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, items, func() string { return fmt.Sprintf("Profiles: %d\n", len(items)) })
	case "login":
		item, err := auditedCall(ctx, opts, service, "dynamodb_sso_login", map[string]any{"profile": profile, "configPath": configPath}, func(c context.Context) (datasourceops.DynamoDBSSOLoginResult, error) {
			return service.DynamoDBSSOLogin(c, profile, configPath)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, datasourceops.RedactValue(item), func() string { return "Login complete.\n" })
	case "authorize":
		item, err := auditedCall(ctx, opts, service, "dynamodb_sso_authorize", map[string]any{"profile": profile, "region": region, "configPath": configPath}, func(c context.Context) (datasourceops.DynamoDBSSOOAuthResult, error) {
			return service.DynamoDBSSOOAuthAuthorize(c, profile, region, configPath)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, datasourceops.RedactValue(item), func() string { return "Authorization complete.\n" })
	case "list-accounts":
		items, err := auditedCall(ctx, opts, service, "dynamodb_sso_list_accounts", map[string]any{"region": region}, func(c context.Context) ([]datasourceops.DynamoDBSSOAccount, error) {
			return service.DynamoDBSSOListAccounts(c, accessToken, region)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, items, func() string { return fmt.Sprintf("Accounts: %d\n", len(items)) })
	case "list-account-roles":
		items, err := auditedCall(ctx, opts, service, "dynamodb_sso_list_account_roles", map[string]any{"accountId": accountID, "region": region}, func(c context.Context) ([]datasourceops.DynamoDBSSORole, error) {
			return service.DynamoDBSSOListAccountRoles(c, accountID, accessToken, region)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, items, func() string { return fmt.Sprintf("Roles: %d\n", len(items)) })
	case "get-role-credentials":
		item, err := auditedCall(ctx, opts, service, "dynamodb_sso_get_role_credentials", map[string]any{"accountId": accountID, "roleName": roleName, "region": region}, func(c context.Context) (datasourceops.DynamoDBSSORoleCredentials, error) {
			return service.DynamoDBSSOGetRoleCredentials(c, accountID, roleName, accessToken, region)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, datasourceops.RedactValue(item), func() string { return "Credentials acquired.\n" })
	default:
		return fmt.Errorf("unknown dynamodb-sso subcommand: %s\n\n%s", args[0], dynamoDBSSOUsage())
	}
}

func dynamoDBSSOUsage() string {
	return `Usage: futrixdata-cli dynamodb-sso <subcommand> [flags]

Subcommands:
  list-profiles         List AWS SSO profiles from config
  login                 Login to an AWS SSO profile
  authorize             Authorize AWS SSO and return credentials
  list-accounts         List AWS SSO accounts
  list-account-roles    List roles for an AWS SSO account
  get-role-credentials  Get temporary credentials for a role

Common flags:
  --profile <name>        AWS SSO profile name
  --region <region>       AWS region
  --config-path <path>    Path to AWS config file
  --access-token <token>  SSO access token
  --account-id <id>       AWS account ID
  --role-name <name>      IAM role name`
}
