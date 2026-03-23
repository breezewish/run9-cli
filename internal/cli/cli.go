package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/breezewish/run9-cli/internal/api"
	archiveutil "github.com/breezewish/run9-cli/internal/archive"
	"github.com/breezewish/run9-cli/internal/config"
)

const defaultExecDeadline = 15 * time.Minute

type commandContext struct {
	configPath string
	stdout     io.Writer
	stderr     io.Writer
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

// Main runs the run9 CLI.
func Main(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	commandCtx, rest, err := parseRoot(args, stdout, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if len(rest) == 0 {
		fmt.Fprintln(stderr, "missing command (expected: auth|box|snap)")
		return 1
	}

	switch rest[0] {
	case "auth":
		return runAuth(ctx, commandCtx, rest[1:])
	case "box":
		return runBox(ctx, commandCtx, rest[1:])
	case "snap":
		return runSnap(ctx, commandCtx, rest[1:])
	default:
		fmt.Fprintf(stderr, "unknown command %q (expected: auth|box|snap)\n", rest[0])
		return 1
	}
}

func parseRoot(args []string, stdout io.Writer, stderr io.Writer) (commandContext, []string, error) {
	fs := flag.NewFlagSet("run9", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", config.DefaultPath(), "path to cli config")
	if err := fs.Parse(args); err != nil {
		return commandContext{}, nil, err
	}
	return commandContext{
		configPath: *configPath,
		stdout:     stdout,
		stderr:     stderr,
	}, fs.Args(), nil
}

func runAuth(ctx context.Context, commandCtx commandContext, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(commandCtx.stderr, "missing auth subcommand (expected: login|logout|whoami)")
		return 1
	}

	switch args[0] {
	case "login":
		return runAuthLogin(ctx, commandCtx, args[1:])
	case "logout":
		return runAuthLogout(commandCtx, args[1:])
	case "whoami":
		return runAuthWhoAmI(ctx, commandCtx, args[1:])
	default:
		fmt.Fprintf(commandCtx.stderr, "unknown auth subcommand %q (expected: login|logout|whoami)\n", args[0])
		return 1
	}
}

func runAuthLogin(ctx context.Context, commandCtx commandContext, args []string) int {
	fs := flag.NewFlagSet("run9 auth login", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	endpoint := fs.String("endpoint", "", "portal endpoint")
	ak := fs.String("ak", "", "api key access key")
	sk := fs.String("sk", "", "api key secret key")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(commandCtx.stderr, "unexpected args: %v\n", fs.Args())
		return 1
	}

	existing, exists, err := config.LoadOptional(commandCtx.configPath)
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	loginEndpoint := strings.TrimSpace(*endpoint)
	if loginEndpoint == "" && exists {
		loginEndpoint = existing.Endpoint
	}
	if loginEndpoint == "" {
		fmt.Fprintln(commandCtx.stderr, "missing --endpoint")
		return 1
	}
	if strings.TrimSpace(*ak) == "" || strings.TrimSpace(*sk) == "" {
		fmt.Fprintln(commandCtx.stderr, "missing --ak or --sk")
		return 1
	}

	client := api.NewClient(loginEndpoint)
	identity, err := client.WhoAmI(ctx, api.Credentials{AK: strings.TrimSpace(*ak), SK: strings.TrimSpace(*sk)})
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}

	cfg := config.File{
		Endpoint: loginEndpoint,
		AK:       strings.TrimSpace(*ak),
		SK:       strings.TrimSpace(*sk),
		Identity: identityToConfig(identity),
	}
	if err := config.Save(commandCtx.configPath, cfg); err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	if err := writeJSON(commandCtx.stdout, identity); err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	return 0
}

func runAuthLogout(commandCtx commandContext, args []string) int {
	if len(args) != 0 {
		fmt.Fprintf(commandCtx.stderr, "unexpected args: %v\n", args)
		return 1
	}
	if err := config.Remove(commandCtx.configPath); err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	return 0
}

func runAuthWhoAmI(ctx context.Context, commandCtx commandContext, args []string) int {
	if len(args) != 0 {
		fmt.Fprintf(commandCtx.stderr, "unexpected args: %v\n", args)
		return 1
	}

	cfg, client, creds, err := loadAuthenticatedClient(commandCtx.configPath)
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}

	identity, err := client.WhoAmI(ctx, creds)
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	cfg.Identity = identityToConfig(identity)
	if err := config.Save(commandCtx.configPath, cfg); err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	if err := writeJSON(commandCtx.stdout, identity); err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	return 0
}

func runBox(ctx context.Context, commandCtx commandContext, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(commandCtx.stderr, "missing box subcommand (expected: create|ls|inspect|exec|cp|stop|commit|rm)")
		return 1
	}

	switch args[0] {
	case "create":
		return runBoxCreate(ctx, commandCtx, args[1:])
	case "ls":
		return runBoxList(ctx, commandCtx, args[1:])
	case "inspect":
		return runBoxInspect(ctx, commandCtx, args[1:])
	case "exec":
		return runBoxExec(ctx, commandCtx, args[1:])
	case "cp":
		return runBoxCopy(ctx, commandCtx, args[1:])
	case "stop":
		return runBoxStop(ctx, commandCtx, args[1:])
	case "commit":
		return runBoxCommit(ctx, commandCtx, args[1:])
	case "rm":
		return runBoxRemove(ctx, commandCtx, args[1:])
	default:
		fmt.Fprintf(commandCtx.stderr, "unknown box subcommand %q (expected: create|ls|inspect|exec|cp|stop|commit|rm)\n", args[0])
		return 1
	}
}

func runBoxCreate(ctx context.Context, commandCtx commandContext, args []string) int {
	fs := flag.NewFlagSet("run9 box create", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	shape := fs.String("shape", "", "desired shape")
	name := fs.String("name", "", "box name")
	sourceSnapID := fs.String("snap", "", "source snap id")
	sourceImage := fs.String("image", "", "source image ref")
	sourceImageRef := fs.String("image-ref", "", "source image ref")
	var labels stringList
	fs.Var(&labels, "label", "box label in key=value form")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(commandCtx.stderr, "unexpected args: %v\n", fs.Args())
		return 1
	}
	if strings.TrimSpace(*shape) == "" {
		fmt.Fprintln(commandCtx.stderr, "missing --shape")
		return 1
	}

	labelMap, err := parseKeyValueMap(labels)
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	imageRef := strings.TrimSpace(*sourceImage)
	if strings.TrimSpace(*sourceImageRef) != "" {
		if imageRef != "" && imageRef != strings.TrimSpace(*sourceImageRef) {
			fmt.Fprintln(commandCtx.stderr, "--image and --image-ref must match when both are set")
			return 1
		}
		imageRef = strings.TrimSpace(*sourceImageRef)
	}

	hasSnap := strings.TrimSpace(*sourceSnapID) != ""
	hasImage := imageRef != ""
	if hasSnap == hasImage {
		fmt.Fprintln(commandCtx.stderr, "exactly one of --snap or --image is required")
		return 1
	}

	_, client, creds, err := loadAuthenticatedClient(commandCtx.configPath)
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	view, err := client.CreateBox(ctx, creds, api.CreateBoxRequest{
		DesiredShape:   strings.TrimSpace(*shape),
		Name:           strings.TrimSpace(*name),
		Labels:         labelMap,
		SourceSnapID:   strings.TrimSpace(*sourceSnapID),
		SourceImageRef: imageRef,
	})
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	if err := writeJSON(commandCtx.stdout, view); err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	return 0
}

func runBoxList(ctx context.Context, commandCtx commandContext, args []string) int {
	fs := flag.NewFlagSet("run9 box ls", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	creator := fs.String("creator", "", "creator filter")
	label := fs.String("label", "", "label filter")
	state := fs.String("state", "", "state filter")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(commandCtx.stderr, "unexpected args: %v\n", fs.Args())
		return 1
	}

	_, client, creds, err := loadAuthenticatedClient(commandCtx.configPath)
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	views, err := client.Boxes(ctx, creds, *creator, *label, *state)
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	if err := writeJSON(commandCtx.stdout, views); err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	return 0
}

func runBoxInspect(ctx context.Context, commandCtx commandContext, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(commandCtx.stderr, "usage: run9 box inspect <box-id>")
		return 1
	}
	_, client, creds, err := loadAuthenticatedClient(commandCtx.configPath)
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	view, err := client.Box(ctx, creds, args[0])
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	if err := writeJSON(commandCtx.stdout, view); err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	return 0
}

func runBoxExec(ctx context.Context, commandCtx commandContext, args []string) int {
	fs := flag.NewFlagSet("run9 box exec", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	deadline := fs.Duration("deadline", defaultExecDeadline, "exec deadline")
	user := fs.String("user", "", "exec user")
	workdir := fs.String("workdir", "", "exec workdir")
	var envVars stringList
	fs.Var(&envVars, "e", "environment override in KEY=VALUE form")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	if fs.NArg() < 2 {
		fmt.Fprintln(commandCtx.stderr, "usage: run9 box exec <box-id> [--deadline=15m] [--user=...] [--workdir=...] [-e KEY=VALUE] -- <command...>")
		return 1
	}

	envMap, err := parseKeyValueMap(envVars)
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}

	boxID := fs.Arg(0)
	command := append([]string(nil), fs.Args()[1:]...)

	_, client, creds, err := loadAuthenticatedClient(commandCtx.configPath)
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	_, body, err := client.ExecStream(ctx, creds, boxID, api.ExecBoxRequest{
		DeadlineAt:   time.Now().Add(*deadline),
		Command:      command,
		EnvOverrides: envMap,
		User:         strings.TrimSpace(*user),
		Workdir:      strings.TrimSpace(*workdir),
	})
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	defer body.Close()

	return streamExec(commandCtx.stdout, commandCtx.stderr, body)
}

func runBoxCopy(ctx context.Context, commandCtx commandContext, args []string) int {
	if len(args) != 2 {
		fmt.Fprintln(commandCtx.stderr, "usage: run9 box cp <src> <dst>")
		return 1
	}

	srcBoxPath, srcIsBox, err := parseBoxPath(args[0])
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	dstBoxPath, dstIsBox, err := parseBoxPath(args[1])
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	if srcIsBox == dstIsBox {
		fmt.Fprintln(commandCtx.stderr, "exactly one side of box cp must be <box-id>:/absolute/path")
		return 1
	}

	_, client, creds, err := loadAuthenticatedClient(commandCtx.configPath)
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}

	if dstIsBox {
		upload, err := archiveutil.BuildUploadArchive(args[0], dstBoxPath.RawPath)
		if err != nil {
			fmt.Fprintln(commandCtx.stderr, err)
			return 1
		}
		defer func() {
			_ = upload.File.Close()
			_ = os.Remove(upload.File.Name())
		}()
		if _, err := client.UploadArchive(ctx, creds, dstBoxPath.BoxID, upload.BoxAbsPath, upload.File); err != nil {
			fmt.Fprintln(commandCtx.stderr, err)
			return 1
		}
		return 0
	}

	body, err := client.DownloadArchive(ctx, creds, srcBoxPath.BoxID, srcBoxPath.AbsPath)
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	defer body.Close()

	if err := archiveutil.ExtractDownloadArchive(body, args[1]); err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	return 0
}

func runBoxStop(ctx context.Context, commandCtx commandContext, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(commandCtx.stderr, "usage: run9 box stop <box-id>")
		return 1
	}
	_, client, creds, err := loadAuthenticatedClient(commandCtx.configPath)
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	view, err := client.StopBox(ctx, creds, args[0])
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	if err := writeJSON(commandCtx.stdout, view); err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	return 0
}

func runBoxCommit(ctx context.Context, commandCtx commandContext, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(commandCtx.stderr, "usage: run9 box commit <box-id>")
		return 1
	}
	_, client, creds, err := loadAuthenticatedClient(commandCtx.configPath)
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	view, err := client.CommitBox(ctx, creds, args[0])
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	if err := writeJSON(commandCtx.stdout, view); err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	return 0
}

func runBoxRemove(ctx context.Context, commandCtx commandContext, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(commandCtx.stderr, "usage: run9 box rm <box-id>")
		return 1
	}
	_, client, creds, err := loadAuthenticatedClient(commandCtx.configPath)
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	view, err := client.RemoveBox(ctx, creds, args[0])
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	if err := writeJSON(commandCtx.stdout, view); err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	return 0
}

func runSnap(ctx context.Context, commandCtx commandContext, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(commandCtx.stderr, "missing snap subcommand (expected: import|ls|inspect|rm)")
		return 1
	}

	switch args[0] {
	case "import":
		return runSnapImport(ctx, commandCtx, args[1:])
	case "ls":
		return runSnapList(ctx, commandCtx, args[1:])
	case "inspect":
		return runSnapInspect(ctx, commandCtx, args[1:])
	case "rm":
		return runSnapRemove(ctx, commandCtx, args[1:])
	default:
		fmt.Fprintf(commandCtx.stderr, "unknown snap subcommand %q (expected: import|ls|inspect|rm)\n", args[0])
		return 1
	}
}

func runSnapImport(ctx context.Context, commandCtx commandContext, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(commandCtx.stderr, "usage: run9 snap import <image-ref>")
		return 1
	}
	_, client, creds, err := loadAuthenticatedClient(commandCtx.configPath)
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	view, err := client.ImportSnap(ctx, creds, args[0])
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	if err := writeJSON(commandCtx.stdout, view); err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	return 0
}

func runSnapList(ctx context.Context, commandCtx commandContext, args []string) int {
	fs := flag.NewFlagSet("run9 snap ls", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	attached := fs.String("attached", "", "attached filter (true|false)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(commandCtx.stderr, "unexpected args: %v\n", fs.Args())
		return 1
	}
	if strings.TrimSpace(*attached) != "" && *attached != "true" && *attached != "false" {
		fmt.Fprintln(commandCtx.stderr, "--attached must be true or false")
		return 1
	}

	_, client, creds, err := loadAuthenticatedClient(commandCtx.configPath)
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	views, err := client.Snaps(ctx, creds, *attached)
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	if err := writeJSON(commandCtx.stdout, views); err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	return 0
}

func runSnapInspect(ctx context.Context, commandCtx commandContext, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(commandCtx.stderr, "usage: run9 snap inspect <snap-id>")
		return 1
	}
	_, client, creds, err := loadAuthenticatedClient(commandCtx.configPath)
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	view, err := client.Snap(ctx, creds, args[0])
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	if err := writeJSON(commandCtx.stdout, view); err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	return 0
}

func runSnapRemove(ctx context.Context, commandCtx commandContext, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(commandCtx.stderr, "usage: run9 snap rm <snap-id>")
		return 1
	}
	_, client, creds, err := loadAuthenticatedClient(commandCtx.configPath)
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	view, err := client.RemoveSnap(ctx, creds, args[0])
	if err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	if err := writeJSON(commandCtx.stdout, view); err != nil {
		fmt.Fprintln(commandCtx.stderr, err)
		return 1
	}
	return 0
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

type boxPath struct {
	BoxID   string
	AbsPath string
	RawPath string
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

func parseFlagsInterspersed(fs *flag.FlagSet, args []string) error {
	return fs.Parse(reorderFlagsFirst(fs, args))
}

func reorderFlagsFirst(fs *flag.FlagSet, args []string) []string {
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}

		flags = append(flags, arg)
		if strings.Contains(arg, "=") {
			continue
		}

		flagName := strings.TrimLeft(arg, "-")
		f := fs.Lookup(flagName)
		if f == nil || isBoolFlag(f) {
			continue
		}
		if i+1 < len(args) {
			flags = append(flags, args[i+1])
			i++
		}
	}
	return append(flags, positionals...)
}

func isBoolFlag(f *flag.Flag) bool {
	type boolFlag interface {
		IsBoolFlag() bool
	}
	bf, ok := f.Value.(boolFlag)
	return ok && bf.IsBoolFlag()
}
