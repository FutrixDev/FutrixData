package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/bootstrap"
)

func (r *Runner) runAudit(_ context.Context, opts Options, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing audit subcommand.\n\n%s", auditUsage())
	}
	switch args[0] {
	case "verify":
		fs := flag.NewFlagSet("audit verify", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return fmt.Errorf("unexpected audit verify argument: %s", fs.Arg(0))
		}
		result, err := agentaudit.VerifyFile(bootstrap.AgentAuditPath(opts.DataPath))
		if err != nil {
			return err
		}
		if opts.JSON {
			if result.Pass {
				return r.printJSON(result)
			}
			return &jsonEnvelopeError{payload: result}
		}
		if result.Pass {
			return r.printResult(opts, result, func() string {
				return fmt.Sprintf("Agent audit verification passed (%d records verified, %d legacy records).\n", result.VerifiedRecords, result.LegacyRecords)
			})
		}
		return fmt.Errorf("agent audit verification failed at record %d: %s", result.FirstBrokenPosition, result.Reason)
	default:
		return fmt.Errorf("unknown audit subcommand: %s\n\n%s", args[0], auditUsage())
	}
}

func auditUsage() string {
	return strings.TrimSpace(`
Usage: futrixdata-cli audit <subcommand> [flags]

Subcommands:
  verify        Verify the local agent audit hash chain

Examples:
  futrixdata-cli audit verify --json
`) + "\n"
}
