package startuprecovery

import (
	"errors"
	"strings"

	"futrixdata/platform/internal/securefile"
)

type Reason string

const (
	ReasonAppTooOld           Reason = "app_too_old"
	ReasonKeychainUnavailable Reason = "keychain_unavailable"
	ReasonKeyMismatch         Reason = "key_mismatch"
	ReasonCorruptFile         Reason = "corrupt_file"
	ReasonMigrationFailed     Reason = "migration_failed"
	ReasonUnknown             Reason = "unknown"
)

type Action string

const (
	ActionRetry               Action = "retry"
	ActionUpdateApp           Action = "update_app"
	ActionOpenLogs            Action = "open_logs"
	ActionMoveAsideAndRestart Action = "move_aside_and_restart"
)

type Info struct {
	Reason              Reason   `json:"reason"`
	Message             string   `json:"message"`
	DataPath            string   `json:"dataPath,omitempty"`
	DataDir             string   `json:"dataDir,omitempty"`
	RetentionDir        string   `json:"retentionDir,omitempty"`
	FormatVersion       int      `json:"formatVersion,omitempty"`
	WriterAppVersion    string   `json:"writerAppVersion,omitempty"`
	MinReaderAppVersion string   `json:"minReaderAppVersion,omitempty"`
	Actions             []Action `json:"actions,omitempty"`
	Details             string   `json:"details,omitempty"`
}

type Error struct {
	Info Info
	Err  error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Info.Message) != "" {
		return e.Info.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "startup recovery required"
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func Wrap(err error, info Info) error {
	info = normalize(info, err)
	return &Error{Info: info, Err: err}
}

func FromError(err error) (Info, bool) {
	var recoveryErr *Error
	if errors.As(err, &recoveryErr) && recoveryErr != nil {
		return normalize(recoveryErr.Info, recoveryErr.Err), true
	}
	return Info{}, false
}

func Classify(err error, dataPath string) Info {
	info := Info{DataPath: strings.TrimSpace(dataPath)}
	if existing, ok := FromError(err); ok {
		if existing.DataPath == "" {
			existing.DataPath = info.DataPath
		}
		return normalize(existing, err)
	}

	var versionErr *securefile.EnvelopeVersionError
	switch {
	case errors.As(err, &versionErr):
		info.Reason = ReasonAppTooOld
		info.FormatVersion = versionErr.Metadata.FormatVersion
		info.WriterAppVersion = versionErr.Metadata.WriterAppVersion
		info.MinReaderAppVersion = versionErr.Metadata.MinReaderAppVersion
	case errors.Is(err, securefile.ErrKeyUnavailable):
		info.Reason = ReasonKeychainUnavailable
	case errors.Is(err, securefile.ErrDecryptFailed):
		info.Reason = ReasonKeyMismatch
	case errors.Is(err, securefile.ErrDataCorrupt):
		info.Reason = ReasonCorruptFile
	default:
		lower := strings.ToLower(errString(err))
		switch {
		case strings.Contains(lower, "keychain") || strings.Contains(lower, "local root encryption key unavailable"):
			info.Reason = ReasonKeychainUnavailable
		case strings.Contains(lower, "message authentication failed"):
			info.Reason = ReasonKeyMismatch
		case strings.Contains(lower, "migrate"):
			info.Reason = ReasonMigrationFailed
		default:
			info.Reason = ReasonUnknown
		}
	}
	info.Details = errString(err)
	return normalize(info, err)
}

func normalize(info Info, err error) Info {
	if info.Reason == "" {
		info.Reason = ReasonUnknown
	}
	if strings.TrimSpace(info.Message) == "" {
		info.Message = defaultMessage(info.Reason)
	}
	if strings.TrimSpace(info.Details) == "" {
		info.Details = errString(err)
	}
	if len(info.Actions) == 0 {
		info.Actions = defaultActions(info.Reason)
	}
	return info
}

func defaultMessage(reason Reason) string {
	switch reason {
	case ReasonAppTooOld:
		return "This version of FutrixData is too old to read the local encrypted data."
	case ReasonKeychainUnavailable:
		return "local root encryption key unavailable: FutrixData could not access the OS keychain needed to open local encrypted data."
	case ReasonKeyMismatch:
		return "The local encrypted data could not be opened with this device key."
	case ReasonCorruptFile:
		return "The local encrypted data appears damaged or incomplete."
	case ReasonMigrationFailed:
		return "FutrixData could not safely migrate local encrypted data."
	default:
		return "FutrixData could not open local data during startup."
	}
}

func defaultActions(reason Reason) []Action {
	switch reason {
	case ReasonAppTooOld:
		return []Action{ActionUpdateApp, ActionOpenLogs}
	case ReasonKeychainUnavailable:
		return []Action{ActionRetry, ActionOpenLogs}
	case ReasonKeyMismatch, ReasonCorruptFile:
		return []Action{ActionRetry, ActionOpenLogs, ActionMoveAsideAndRestart}
	default:
		return []Action{ActionRetry, ActionOpenLogs}
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}
