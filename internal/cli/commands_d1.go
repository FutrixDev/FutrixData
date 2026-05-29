package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"futrixdata/platform/internal/datasourceops"
	"futrixdata/platform/internal/toolreg"
)

func (r *Runner) runD1(ctx context.Context, service Service, opts Options, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing d1 subcommand.\n\n%s", d1Usage())
	}
	fs := flag.NewFlagSet("d1 "+args[0], flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var accountID, token, name, datasourceID string
	var approve bool
	fs.StringVar(&accountID, "account-id", "", "account id")
	fs.StringVar(&token, "token", "", "api token")
	fs.StringVar(&name, "name", "", "database name")
	fs.StringVar(&datasourceID, "datasource", "", "datasource id")
	fs.BoolVar(&approve, "approve", false, "approve D1 mutation")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if approve {
		approveToolName, approveParams := d1ApproveAuditInfo(args[0], accountID, name, datasourceID)
		if err := rejectAgentApproveFlag(opts, service, approveToolName, approveParams); err != nil {
			return err
		}
	}
	switch args[0] {
	case "oauth-login":
		item, err := auditedCall(ctx, opts, service, "d1_oauth_login", nil, func(c context.Context) (datasourceops.D1OAuthSession, error) {
			return service.D1OAuthLogin(c)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, datasourceops.RedactValue(item), func() string { return "D1 login complete.\n" })
	case "oauth-relogin":
		item, err := auditedCall(ctx, opts, service, "d1_oauth_relogin", nil, func(c context.Context) (datasourceops.D1OAuthSession, error) {
			return service.D1OAuthReLogin(c)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, datasourceops.RedactValue(item), func() string { return "D1 relogin complete.\n" })
	case "wrangler-installed":
		installed, err := auditedCall(ctx, opts, service, "d1_is_wrangler_installed", nil, func(c context.Context) (bool, error) {
			return service.D1IsWranglerInstalled(c)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, map[string]any{"installed": installed}, func() string { return fmt.Sprintf("%v\n", installed) })
	case "list-cloud-databases":
		// Token is intentionally omitted from the audit param map — RedactValue
		// would zero it out downstream, but excluding it up front keeps the
		// on-disk row from carrying credential length information.
		items, err := auditedCall(ctx, opts, service, "d1_list_cloud_databases", map[string]any{"accountId": accountID}, func(c context.Context) ([]datasourceops.D1CloudDatabase, error) {
			return service.D1ListCloudDatabases(c, accountID, token)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, items, func() string { return fmt.Sprintf("Databases: %d\n", len(items)) })
	case "create-cloud-database":
		if !approve {
			// Token is intentionally omitted from the audit-row params; the
			// approvalRequired prompt envelope handles secret redaction via
			// datasourceops.RedactValue, but the audit row goes through
			// BuildSummary/PrimaryTarget which reads params verbatim.
			auditParams := map[string]any{"accountId": accountID, "name": name}
			if err := validateAgentAccessForApproval(opts, service, "d1_create_cloud_database", auditParams, nil); err != nil {
				return err
			}
			return r.approvalRequired(opts, "d1_create_cloud_database", toolreg.ApprovalSummary("d1_create_cloud_database", map[string]any{"name": name}), map[string]any{
				"accountId": accountID,
				"token":     token,
				"name":      name,
			})
		}
		item, err := auditedCall(ctx, opts, service, "d1_create_cloud_database", map[string]any{"accountId": accountID, "name": name}, func(c context.Context) (datasourceops.D1CloudDatabase, error) {
			return service.D1CreateCloudDatabase(c, accountID, token, name)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, item, func() string { return fmt.Sprintf("Created %s (%s).\n", item.Name, item.ID) })
	case "deploy-migrations":
		if !approve {
			deployParams := map[string]any{"datasourceId": datasourceID}
			if err := validateAgentAccessForApproval(opts, service, "d1_deploy_migrations", deployParams, nil); err != nil {
				return err
			}
			return r.approvalRequired(opts, "d1_deploy_migrations", toolreg.ApprovalSummary("d1_deploy_migrations", deployParams), deployParams)
		}
		ok, err := auditedCall(ctx, opts, service, "d1_deploy_migrations", map[string]any{"datasourceId": datasourceID}, func(c context.Context) (bool, error) {
			return service.D1DeployMigrations(c, datasourceID)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, map[string]any{"ok": ok}, func() string { return "Migrations deployed.\n" })
	default:
		return fmt.Errorf("unknown d1 subcommand: %s\n\n%s", args[0], d1Usage())
	}
}

func d1ApproveAuditInfo(subcommand, accountID, name, datasourceID string) (string, map[string]any) {
	switch subcommand {
	case "oauth-login":
		return "d1_oauth_login", nil
	case "oauth-relogin":
		return "d1_oauth_relogin", nil
	case "wrangler-installed":
		return "d1_is_wrangler_installed", nil
	case "list-cloud-databases":
		return "d1_list_cloud_databases", map[string]any{"accountId": accountID}
	case "create-cloud-database":
		return "d1_create_cloud_database", map[string]any{"accountId": accountID, "name": name}
	case "deploy-migrations":
		return "d1_deploy_migrations", map[string]any{"datasourceId": datasourceID}
	default:
		return "d1_" + strings.ReplaceAll(subcommand, "-", "_"), nil
	}
}

func d1Usage() string {
	return `Usage: futrixdata-cli d1 <subcommand> [flags]

Subcommands:
  oauth-login            Authenticate with Cloudflare via OAuth
  oauth-relogin          Re-authenticate with Cloudflare via OAuth
  wrangler-installed     Check if Wrangler CLI is available
  list-cloud-databases   List D1 cloud databases
  create-cloud-database  Create a new D1 cloud database
  deploy-migrations      Deploy pending D1 migrations

Flags:
  --account-id <id>    Cloudflare account ID
  --token <token>      Cloudflare API token
  --name <name>        Database name (for create)
  --datasource <id>    Datasource ID (for deploy-migrations)
  --approve            Approve mutation (required for create/deploy)`
}
