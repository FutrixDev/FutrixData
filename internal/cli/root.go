package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"futrixdata/platform/internal/auth"
	"gopkg.in/yaml.v3"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/bootstrap"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasourceops"
	"futrixdata/platform/internal/keyring"
	"futrixdata/platform/internal/localcrypto"
	"futrixdata/platform/internal/riskengine"
	"futrixdata/platform/internal/sensitivity"
	"futrixdata/platform/internal/skill"
	"futrixdata/platform/internal/startuprecovery"
	"futrixdata/platform/internal/toolexec"
	"futrixdata/platform/internal/toolreg"
)

type Options struct {
	ConfigPath       string
	DataPath         string
	AuthBaseURL      string
	AgentAccessKey   string
	JSON             bool
	Quiet            bool
	ExplicitDataPath bool // true when --data-path was explicitly provided by the user
}

// Service extends toolreg.Service with auth management methods needed by
// the CLI's auth subcommands (login, logout, device management).
type Service interface {
	toolreg.AuthService
	StartAuthLogin(context.Context, auth.StartLoginInput) (auth.LoginStart, error)
	PollAuthLogin(context.Context) (auth.LoginPoll, error)
	CompleteAuthLogin(context.Context, string) (auth.State, error)
	LogoutAuth(context.Context) (auth.State, error)
	ListAuthDevices(context.Context) (auth.DeviceList, error)
	RemoveAuthDevice(context.Context, string) (auth.DeviceList, error)
}

type Runner struct {
	stdout              io.Writer
	stderr              io.Writer
	stdin               io.Reader
	serviceFactory      func(Options) (Service, error)
	authPollInterval    time.Duration
	authPollAttempts    int
	desktopAppValidator func() error
}

type jsonEnvelopeError struct {
	payload any
}

func (e *jsonEnvelopeError) Error() string {
	return "json envelope error"
}

var initSecurefileKey = func(dataPath string) error {
	_, err := localcrypto.InitWithOptions(dataPath, localcrypto.InitOptions{
		AuxiliaryLoadMode: bootstrap.AuxiliaryLoadBestEffort,
	})
	return err
}

func NewRunner(stdout, stderr io.Writer) *Runner {
	return &Runner{
		stdout:              stdout,
		stderr:              stderr,
		stdin:               os.Stdin,
		serviceFactory:      defaultServiceFactory,
		authPollInterval:    2 * time.Second,
		authPollAttempts:    150,
		desktopAppValidator: skill.ValidateDesktopAppForCLI,
	}
}

// directSubcommand wraps the four direct-subcommand cases (datasource/console/
// d1/dynamodb-sso) with the standard pipeline: agent-key preflight → service
// factory → handler. The pipeline has two failure modes worth
// distinguishing:
//
//  1. Bad agent key + serviceFactory succeeds. Handler runs and its own
//     preflightAgentAccess / auditedCall writes the canonical row using the
//     specific tool name (e.g., "execute_statement"). We do nothing extra.
//
//  2. Bad agent key + serviceFactory fails (e.g., corrupted datastore, missing
//     keyring). Handler is unreachable, so we surface the access-key error
//     instead of the factory error and write a coarse revocation row tagged
//     with the subcommand verb. Without this, codex pass-8 [P2] noted the
//     access rejection would be masked by the factory error and no audit row
//     would be written at all — both contracting the "validate first, audit
//     always" guarantee that toolexec.Dispatch upholds.
//
// The CheckAccess on the happy path is the small cost we pay for that
// guarantee; ensureAuthenticatedForDirect's redundant CheckAccess catches
// mid-flight revocation between this call and the handler.
func (r *Runner) directSubcommand(ctx context.Context, opts Options, verb string, run func(context.Context, Service) error) error {
	accessKey := strings.TrimSpace(opts.AgentAccessKey)
	var preflightErr error
	if accessKey != "" {
		_, preflightErr = agentaudit.CheckAccess(opts.DataPath, accessKey)
	}
	service, factoryErr := r.serviceFactory(opts)
	if factoryErr != nil {
		if preflightErr != nil {
			if errors.Is(preflightErr, agentaudit.ErrAccessRevoked) {
				agentaudit.LogRevokedAccess(opts.DataPath, nil, string(toolexec.SourceCLI), accessKey, verb, nil, preflightErr.Error())
			}
			return preflightErr
		}
		return factoryErr
	}
	return run(ctx, service)
}

func (r *Runner) Run(args []string) int {
	opts, remaining, help, err := parseGlobalOptions(args)
	if err != nil {
		if rawWantsJSON(args) {
			if jsonErr := r.printJSON(runFailurePayload(nil, err)); jsonErr != nil {
				r.printError(jsonErr)
				return 1
			}
			return 1
		}
		r.printError(err)
		return 1
	}
	if help || len(remaining) == 0 {
		r.writeString(rootUsage())
		return 0
	}
	if r.desktopAppValidator != nil && needsDesktopAppValidation(remaining) {
		if err := r.desktopAppValidator(); err != nil {
			if opts.JSON {
				if jsonErr := r.printJSON(runFailurePayload(remaining, err)); jsonErr != nil {
					r.printError(jsonErr)
					return 1
				}
				return 1
			}
			r.printError(err)
			return 1
		}
	}

	ctx := context.Background()
	var commandErr error
	if commandNeedsLocalCrypto(remaining) {
		if err := initSecurefileKey(opts.DataPath); err != nil {
			commandErr = err
		}
	}
	if commandErr == nil {
		switch remaining[0] {
		case "auth":
			service, err := r.serviceFactory(opts)
			if err != nil {
				commandErr = err
				break
			}
			commandErr = r.runAuth(ctx, service, opts, remaining[1:])
		case "audit":
			commandErr = r.runAudit(ctx, opts, remaining[1:])
		case "datasource":
			commandErr = r.directSubcommand(ctx, opts, "datasource", func(c context.Context, svc Service) error {
				return r.runDatasource(c, svc, opts, remaining[1:])
			})
		case "console":
			commandErr = r.directSubcommand(ctx, opts, "console", func(c context.Context, svc Service) error {
				return r.runConsole(c, svc, opts, remaining[1:])
			})
		case "d1":
			commandErr = r.directSubcommand(ctx, opts, "d1", func(c context.Context, svc Service) error {
				return r.runD1(c, svc, opts, remaining[1:])
			})
		case "dynamodb-sso":
			commandErr = r.directSubcommand(ctx, opts, "dynamodb-sso", func(c context.Context, svc Service) error {
				return r.runDynamoDBSSO(c, svc, opts, remaining[1:])
			})
		case "skill":
			commandErr = r.runSkill(ctx, opts, remaining[1:])
		case "codex":
			commandErr = r.runCodex(ctx, opts, remaining[1:])
		case "tool":
			commandErr = r.runTool(ctx, opts, remaining[1:])
		case "mcp":
			commandErr = r.runMCP(ctx, opts, remaining[1:])
		default:
			commandErr = fmt.Errorf("unknown command: %s\n\n%s", remaining[0], rootUsage())
		}
	}

	if commandErr != nil {
		var structured *jsonEnvelopeError
		if errors.As(commandErr, &structured) {
			if jsonErr := r.printJSON(structured.payload); jsonErr != nil {
				r.printError(jsonErr)
				return 1
			}
			return 1
		}
		if opts.JSON {
			if jsonErr := r.printJSON(runFailurePayload(remaining, commandErr)); jsonErr != nil {
				r.printError(jsonErr)
				return 1
			}
			return 1
		}
		r.printError(commandErr)
		return 1
	}
	return 0
}

func commandNeedsLocalCrypto(remaining []string) bool {
	if len(remaining) == 0 {
		return false
	}
	switch remaining[0] {
	case "audit", "datasource", "console", "d1", "dynamodb-sso":
		return true
	case "skill":
		return len(remaining) > 1 && remaining[1] == "install"
	default:
		return false
	}
}

func needsDesktopAppValidation(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch strings.TrimSpace(args[0]) {
	case "audit", "codex":
		return false
	default:
		return true
	}
}

func parseGlobalOptions(args []string) (Options, []string, bool, error) {
	var opts Options
	fs := flag.NewFlagSet("futrixdata-cli", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var help bool
	fs.StringVar(&opts.ConfigPath, "config", "", "path to config file")
	fs.StringVar(&opts.DataPath, "data-path", "", "path to datasources.json")
	fs.StringVar(&opts.AgentAccessKey, "agent-access-key", "", "agent audit access key")
	fs.BoolVar(&opts.JSON, "json", false, "print JSON output")
	fs.BoolVar(&opts.Quiet, "quiet", false, "suppress human-readable output")
	fs.BoolVar(&help, "help", false, "show help")
	fs.BoolVar(&help, "h", false, "show help")
	globalArgs, remaining := splitGlobalArgs(args)
	agentAccessKeyFlagProvided := hasGlobalFlag(globalArgs, "--agent-access-key")
	if err := fs.Parse(globalArgs); err != nil {
		return Options{}, nil, false, err
	}
	opts.AgentAccessKey = strings.TrimSpace(opts.AgentAccessKey)
	if opts.AgentAccessKey == "" && !agentAccessKeyFlagProvided {
		opts.AgentAccessKey = agentAccessKeyFromEnv()
	}
	opts.ExplicitDataPath = strings.TrimSpace(opts.DataPath) != ""
	if err := resolveOptions(&opts); err != nil {
		return Options{}, nil, false, err
	}
	return opts, remaining, help, nil
}

func agentAccessKeyFromEnv() string {
	if value := strings.TrimSpace(os.Getenv("FUTRIXDATA_AGENT_ACCESS_KEY")); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv("FUTRIXDATA_AGENT_KEY"))
}

func hasGlobalFlag(args []string, flagName string) bool {
	prefix := flagName + "="
	for _, arg := range args {
		if arg == flagName || strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}

func resolveOptions(opts *Options) error {
	if opts == nil {
		return nil
	}
	opts.AuthBaseURL = auth.DefaultBaseURL
	if value := strings.TrimSpace(os.Getenv("FUTRIX_AUTH_BASE_URL")); value != "" {
		opts.AuthBaseURL = value
	}
	if strings.TrimSpace(opts.DataPath) != "" {
		opts.DataPath = bootstrap.ResolveDataPath(strings.TrimSpace(opts.DataPath))
		return nil
	}
	if strings.TrimSpace(opts.ConfigPath) == "" {
		opts.DataPath = bootstrap.ResolveDataPath("")
		return nil
	}
	content, err := os.ReadFile(opts.ConfigPath)
	if err != nil {
		return err
	}
	var cfg struct {
		DataPath    string `json:"data_path" yaml:"data_path"`
		AuthBaseURL string `json:"auth_base_url" yaml:"auth_base_url"`
	}
	switch ext := filepath.Ext(opts.ConfigPath); ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(content, &cfg); err != nil {
			return err
		}
	default:
		if err := json.Unmarshal(content, &cfg); err != nil {
			return err
		}
	}
	dataPath := strings.TrimSpace(cfg.DataPath)
	if authBaseURL := strings.TrimSpace(cfg.AuthBaseURL); authBaseURL != "" {
		opts.AuthBaseURL = authBaseURL
	}
	if dataPath == "" {
		opts.DataPath = bootstrap.ResolveDataPath("")
		return nil
	}
	if !filepath.IsAbs(dataPath) {
		dataPath = filepath.Join(filepath.Dir(opts.ConfigPath), dataPath)
	}
	opts.DataPath = bootstrap.ResolveDataPath(dataPath)
	return nil
}

func defaultServiceFactory(opts Options) (Service, error) {
	if err := initSecurefileKey(opts.DataPath); err != nil {
		return nil, err
	}
	runtime, err := bootstrap.NewRuntime(bootstrap.Config{
		DataPath:          opts.DataPath,
		AuxiliaryLoadMode: bootstrap.AuxiliaryLoadBestEffort,
	})
	if err != nil {
		return nil, err
	}
	authStore := auth.NewStore(auth.PathForDataPath(runtime.DataPath))
	if err := authStore.Load(); err != nil {
		return nil, err
	}
	var sensitivityStore *sensitivity.Store
	ss := sensitivity.NewStore(bootstrap.SensitivityStorePath(runtime.DataPath))
	if err := ss.Load(); err == nil {
		sensitivityStore = ss
	}
	maskingSecret, _ := keyring.EnsureMaskingSecret()
	return datasourceops.NewService(datasourceops.Config{
		Store:               runtime.Store,
		Manager:             runtime.Manager,
		RedisDocs:           runtime.RedisDocs,
		AuthStore:           authStore,
		AuthBaseURL:         strings.TrimSpace(opts.AuthBaseURL),
		SchemaKnowledgeRoot: bootstrap.SchemaKnowledgeRoot(runtime.DataPath),
		SensitivityStore:    sensitivityStore,
		MaskingSecret:       maskingSecret,
		RiskEngine:          runtime.RiskEngine,
		RiskStore:           runtime.RiskStore,
		RiskGuard:           runtime.RiskGuard,
		RedisProtoStore:     runtime.RedisProtoStore,
		DatasourceSecrets:   runtime.DatasourceSecrets,
	}), nil
}

func (r *Runner) writeString(value string) {
	if r.stdout != nil {
		_, _ = io.WriteString(r.stdout, value)
	}
}

func (r *Runner) printJSON(value any) error {
	enc := json.NewEncoder(r.stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func (r *Runner) printResult(opts Options, value any, render func() string) error {
	if opts.JSON {
		return r.printJSON(value)
	}
	if opts.Quiet {
		return nil
	}
	r.writeString(render())
	return nil
}

func (r *Runner) printError(err error) {
	if err == nil || r.stderr == nil {
		return
	}
	_, _ = fmt.Fprintf(r.stderr, "error: %v\n", err)
}

func (r *Runner) approvalRequired(opts Options, kind, summary string, payload any, attribution ...*agentaudit.RiskAttribution) error {
	return r.approvalRequiredWithDetail(opts, kind, summary, payload, nil, attribution...)
}

func (r *Runner) approvalRequiredWithDetail(opts Options, kind, summary string, payload any, detailExtras map[string]any, attribution ...*agentaudit.RiskAttribution) error {
	detail := map[string]any{
		"kind":            strings.TrimSpace(kind),
		"summary":         strings.TrimSpace(summary),
		"arguments":       datasourceops.RedactValue(payload),
		"riskAttribution": approvalResponseAttribution(attribution...),
	}
	for key, value := range detailExtras {
		if strings.TrimSpace(key) != "" && value != nil {
			detail[key] = value
		}
	}
	envelope := map[string]any{
		"ok":               false,
		"approvalRequired": detail,
	}
	if opts.JSON {
		return r.printJSON(envelope)
	}
	if strings.TrimSpace(opts.AgentAccessKey) == "" {
		if strings.TrimSpace(summary) == "" {
			return fmt.Errorf("%s requires --approve", kind)
		}
		if impact := approvalImpactSummary(detailExtras); impact != "" {
			return fmt.Errorf("%s requires --approve (%s; %s)", kind, summary, impact)
		}
		return fmt.Errorf("%s requires --approve (%s)", kind, summary)
	}
	if impact := approvalImpactSummary(detailExtras); impact != "" {
		if strings.TrimSpace(summary) == "" {
			return fmt.Errorf("%s is waiting for user approval in FutrixData (%s)", kind, impact)
		}
		return fmt.Errorf("%s is waiting for user approval in FutrixData (%s; %s)", kind, summary, impact)
	}
	if strings.TrimSpace(summary) == "" {
		return fmt.Errorf("%s is waiting for user approval in FutrixData", kind)
	}
	return fmt.Errorf("%s is waiting for user approval in FutrixData (%s)", kind, summary)
}

func approvalImpactSummary(detailExtras map[string]any) string {
	if detailExtras == nil {
		return ""
	}
	raw := detailExtras["writePreview"]
	if raw == nil {
		return ""
	}
	var preview console.WritePreview
	switch typed := raw.(type) {
	case console.WritePreview:
		preview = typed
	case *console.WritePreview:
		if typed == nil {
			return ""
		}
		preview = *typed
	default:
		return ""
	}
	if preview.EstimatedAffectedRows < 0 {
		return ""
	}
	impact := fmt.Sprintf("estimated affected rows: %d", preview.EstimatedAffectedRows)
	if preview.RequiresElevatedApproval && preview.ThresholdRows > 0 {
		impact += fmt.Sprintf("; elevated approval required above %d", preview.ThresholdRows)
	}
	return impact
}

func approvalResponseAttribution(attribution ...*agentaudit.RiskAttribution) *agentaudit.RiskAttribution {
	for _, attr := range attribution {
		if attr != nil {
			return attr
		}
	}
	return agentaudit.PolicyAttribution(string(riskengine.ActionRequireApproval))
}

func (r *Runner) commandFailure(opts Options, payload any, err error) error {
	if !opts.JSON {
		return err
	}
	return &jsonEnvelopeError{payload: payload}
}

func runFailurePayload(args []string, err error) map[string]any {
	errorBody := map[string]any{
		"message": err.Error(),
	}
	if attr := riskAttributionFromError(err); attr != nil {
		errorBody["riskAttribution"] = attr
	}
	if info, ok := startuprecovery.FromError(err); ok {
		errorBody["startupRecovery"] = info
	}
	payload := map[string]any{
		"ok":    false,
		"error": errorBody,
	}
	if command := commandName(args); command != "" {
		payload["command"] = command
	}
	return payload
}

func commandName(args []string) string {
	parts := make([]string, 0, 2)
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			break
		}
		parts = append(parts, strings.TrimSpace(arg))
		if len(parts) == 2 {
			break
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func rawWantsJSON(args []string) bool {
	for _, arg := range args {
		if arg == "--json" {
			return true
		}
	}
	return false
}

func requiredArgs(args []string, want int, usage string) error {
	if len(args) >= want {
		return nil
	}
	if usage == "" {
		return errors.New("missing required arguments")
	}
	return errors.New(usage)
}

func rootUsage() string {
	return strings.TrimSpace(`
Usage:
  futrixdata-cli [--config FILE] [--data-path FILE] [--agent-access-key KEY] [--json] <command> [args]

Commands:
  auth          shared desktop and CLI login commands
  audit         verify local agent audit history
  datasource    datasource CRUD and connectivity
  console       query and inspection commands
  d1            Cloudflare D1 special commands
  dynamodb-sso  AWS DynamoDB SSO helpers
  skill         manage AI agent skill installation
  codex         inspect Codex plugin/MCP readiness
  tool          agent-friendly tool list/call surface
  mcp           MCP server for AI agent integration

Agent key:
  --agent-access-key overrides FUTRIXDATA_AGENT_ACCESS_KEY.
  FUTRIXDATA_AGENT_KEY is accepted as a short compatibility alias.
`) + "\n"
}

func splitGlobalArgs(args []string) ([]string, []string) {
	globalArgs := make([]string, 0, len(args))
	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json" || arg == "--quiet" || arg == "--help" || arg == "-h":
			globalArgs = append(globalArgs, arg)
		case arg == "--config" || arg == "--data-path" || arg == "--agent-access-key":
			globalArgs = append(globalArgs, arg)
			if i+1 < len(args) {
				globalArgs = append(globalArgs, args[i+1])
				i++
			}
		case strings.HasPrefix(arg, "--config=") || strings.HasPrefix(arg, "--data-path=") || strings.HasPrefix(arg, "--agent-access-key="):
			globalArgs = append(globalArgs, arg)
		default:
			remaining = append(remaining, arg)
		}
	}
	return globalArgs, remaining
}
