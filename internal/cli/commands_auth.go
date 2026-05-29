package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"futrixdata/platform/internal/auth"
)

func (r *Runner) runAuth(ctx context.Context, service Service, opts Options, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing auth subcommand.\n\n%s", authUsage())
	}
	switch args[0] {
	case "status":
		state, err := service.CurrentAuth(ctx)
		if err != nil {
			return err
		}
		return r.printResult(opts, state, func() string {
			if state.Session == nil {
				return "Not logged in.\n"
			}
			return fmt.Sprintf("Logged in as %s.\n", state.Session.User.Email)
		})
	case "login":
		fs := flag.NewFlagSet("auth login", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		var noBrowser bool
		var code string
		fs.BoolVar(&noBrowser, "no-browser", false, "do not open a browser automatically")
		fs.StringVar(&code, "code", "", "manual login code")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if code != "" {
			state, err := service.CompleteAuthLogin(ctx, code)
			if err != nil {
				return err
			}
			return r.printResult(opts, state, func() string { return "Login complete.\n" })
		}
		started, err := service.StartAuthLogin(ctx, auth.StartLoginInput{NoBrowser: noBrowser})
		if err != nil {
			return err
		}
		if opts.JSON {
			return r.printJSON(started)
		}
		r.writeString(started.LoginURL + "\n")
		return r.waitForAuthLogin(ctx, service)
	case "logout":
		state, err := service.LogoutAuth(ctx)
		if err != nil {
			return err
		}
		return r.printResult(opts, state, func() string { return "Logged out.\n" })
	case "devices":
		if len(args) < 2 {
			return fmt.Errorf("missing auth devices subcommand.\n\n%s", authDevicesUsage())
		}
		switch args[1] {
		case "list":
			result, err := service.ListAuthDevices(ctx)
			if err != nil {
				return err
			}
			return r.printResult(opts, result, func() string { return fmt.Sprintf("Devices: %d\n", len(result.Devices)) })
		case "remove":
			fs := flag.NewFlagSet("auth devices remove", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			var deviceID string
			fs.StringVar(&deviceID, "device-id", "", "device id")
			if err := fs.Parse(args[2:]); err != nil {
				return err
			}
			result, err := service.RemoveAuthDevice(ctx, deviceID)
			if err != nil {
				return err
			}
			return r.printResult(opts, result, func() string { return fmt.Sprintf("Devices: %d\n", len(result.Devices)) })
		default:
			return fmt.Errorf("unknown auth devices subcommand: %s\n\n%s", args[1], authDevicesUsage())
		}
	default:
		return fmt.Errorf("unknown auth subcommand: %s\n\n%s", args[0], authUsage())
	}
}

func authUsage() string {
	return `Usage: futrixdata-cli auth <subcommand> [flags]

Subcommands:
  status    Show current authentication status
  login     Log in to FutrixData (opens browser by default)
  logout    Log out of FutrixData
  devices   Manage authorized devices (list, remove)

Flags for login:
  --no-browser   Do not open browser automatically
  --code <code>  Complete login with a manual code`
}

func authDevicesUsage() string {
	return `Usage: futrixdata-cli auth devices <subcommand> [flags]

Subcommands:
  list    List all authorized devices
  remove  Remove a device by ID

Flags for remove:
  --device-id <id>  Device ID to remove`
}

func (r *Runner) waitForAuthLogin(ctx context.Context, service Service) error {
	attempts := r.authPollAttempts
	if attempts <= 0 {
		attempts = 150
	}
	interval := r.authPollInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	for attempt := 0; attempt < attempts; attempt++ {
		poll, err := service.PollAuthLogin(ctx)
		if err != nil {
			return err
		}
		switch poll.Status {
		case "":
			return nil
		case "completed":
			state, err := service.CompleteAuthLogin(ctx, poll.Code)
			if err != nil {
				return err
			}
			if state.Session != nil && strings.TrimSpace(state.Session.User.Email) != "" {
				r.writeString(fmt.Sprintf("Logged in as %s.\n", state.Session.User.Email))
				return nil
			}
			r.writeString("Login complete.\n")
			return nil
		case "expired":
			return fmt.Errorf("login expired")
		}
		if attempt < attempts-1 {
			time.Sleep(interval)
		}
	}
	return nil
}
