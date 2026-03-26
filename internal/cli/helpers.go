package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/breezewish/run9-cli/internal/api"
	"github.com/breezewish/run9-cli/internal/config"
)

type stringList []string

func (s *stringList) String() string {
	return strings.Join(*s, ",")
}

func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func (s *stringList) Type() string {
	return "stringList"
}

type boxPath struct {
	BoxID   string
	AbsPath string
	RawPath string
}

func loadAuthenticatedClient(configPath string) (config.File, *api.Client, api.Credentials, error) {
	cfg, exists, err := config.LoadOptional(configPath)
	if err != nil {
		return config.File{}, nil, api.Credentials{}, err
	}
	if !exists {
		return config.File{}, nil, api.Credentials{}, errors.New("not logged in")
	}
	if strings.TrimSpace(cfg.Endpoint) == "" || strings.TrimSpace(cfg.AK) == "" || strings.TrimSpace(cfg.SK) == "" {
		return config.File{}, nil, api.Credentials{}, errors.New("not logged in")
	}
	return cfg, api.NewClient(cfg.Endpoint), api.Credentials{
		AK: cfg.AK,
		SK: cfg.SK,
	}, nil
}

func identityToConfig(identity api.CurrentOrgIdentityView) config.Identity {
	return config.Identity{
		AuthKind:       identity.AuthKind,
		UserID:         identity.User.UserID,
		PrimaryEmail:   identity.User.PrimaryEmail,
		DisplayName:    identity.User.DisplayName,
		OrgID:          identity.Org.OrgID,
		OrgDisplayName: identity.Org.DisplayName,
		OrgKind:        string(identity.Org.Kind),
		Role:           string(identity.Org.Role),
	}
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func parseKeyValueMap(items []string) (map[string]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	result := make(map[string]string, len(items))
	for _, item := range items {
		key, value, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid KEY=VALUE pair %q", item)
		}
		result[strings.TrimSpace(key)] = value
	}
	return result, nil
}

func streamExec(stdout io.Writer, stderr io.Writer, body io.Reader) int {
	decoder := json.NewDecoder(bufio.NewReader(body))
	for {
		var event api.ExecStreamEvent
		if err := decoder.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				fmt.Fprintln(stderr, "exec stream ended unexpectedly")
				return 1
			}
			fmt.Fprintln(stderr, err)
			return 1
		}

		switch event.Type {
		case "stdout":
			if _, err := stdout.Write(event.Data); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
		case "stderr":
			if _, err := stderr.Write(event.Data); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
		case "exit":
			return normalizeExitCode(int(event.ExitCode))
		case "cancelled":
			reason := strings.TrimSpace(event.CancelReason)
			if reason == "" {
				reason = "exec cancelled"
			}
			fmt.Fprintln(stderr, reason)
			return 1
		case "error":
			reason := strings.TrimSpace(event.FailureReason)
			if reason == "" {
				reason = "exec failed"
			}
			fmt.Fprintln(stderr, reason)
			return 1
		}
	}
}

func normalizeExitCode(code int) int {
	if code < 0 || code > 255 {
		return 1
	}
	return code
}

func parseBoxPath(raw string) (boxPath, bool, error) {
	boxID, absPath, ok := strings.Cut(strings.TrimSpace(raw), ":")
	if !ok {
		return boxPath{}, false, nil
	}
	if strings.TrimSpace(boxID) == "" {
		return boxPath{}, false, errors.New("box path is missing box id")
	}
	cleanPath := path.Clean(strings.TrimSpace(absPath))
	if !path.IsAbs(cleanPath) {
		return boxPath{}, false, errors.New("box path must be <box-id>:/absolute/path")
	}
	return boxPath{
		BoxID:   strings.TrimSpace(boxID),
		AbsPath: cleanPath,
		RawPath: strings.TrimSpace(absPath),
	}, true, nil
}

func splitBoxExecArgs(args []string) ([]string, string, []string, error) {
	flagArgs := make([]string, 0, len(args))
	boxID := ""

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			if boxID == "" {
				return nil, "", nil, errors.New("usage: run9 box exec <box-id> [--deadline=15m] [--user=...] [--workdir=...] [-e KEY=VALUE] [-i] [-t] <command...>")
			}
			return flagArgs, boxID, append([]string(nil), args[i+1:]...), nil
		}

		if boxID == "" {
			if isBoxExecFlagToken(arg) {
				flagArgs = append(flagArgs, arg)
				if boxExecFlagNeedsValue(arg) && i+1 < len(args) {
					flagArgs = append(flagArgs, args[i+1])
					i++
				}
				continue
			}
			if strings.HasPrefix(arg, "-") && arg != "-" {
				return nil, "", nil, fmt.Errorf("unknown flag: %s", arg)
			}

			boxID = strings.TrimSpace(arg)
			continue
		}

		if isBoxExecFlagToken(arg) {
			flagArgs = append(flagArgs, arg)
			if boxExecFlagNeedsValue(arg) && i+1 < len(args) {
				flagArgs = append(flagArgs, args[i+1])
				i++
			}
			continue
		}
		return flagArgs, boxID, append([]string(nil), args[i:]...), nil
	}

	return flagArgs, boxID, nil, nil
}

func isBoxExecFlagToken(arg string) bool {
	switch {
	case arg == "-e":
		return true
	case arg == "-i", arg == "-t", arg == "-it", arg == "-ti":
		return true
	case arg == "--deadline", strings.HasPrefix(arg, "--deadline="):
		return true
	case arg == "--interactive", strings.HasPrefix(arg, "--interactive="):
		return true
	case arg == "--tty", strings.HasPrefix(arg, "--tty="):
		return true
	case arg == "--user", strings.HasPrefix(arg, "--user="):
		return true
	case arg == "--workdir", strings.HasPrefix(arg, "--workdir="):
		return true
	default:
		return false
	}
}

func boxExecFlagNeedsValue(arg string) bool {
	switch arg {
	case "-i", "-t", "-it", "-ti", "--interactive", "--tty":
		return false
	}
	return !strings.Contains(arg, "=")
}

func isHelpToken(arg string) bool {
	return arg == "--help" || arg == "-h"
}
