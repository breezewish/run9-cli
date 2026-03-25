package cli

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/breezewish/run9-cli/internal/api"
	archiveutil "github.com/breezewish/run9-cli/internal/archive"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	defaultExecDeadline = 15 * time.Minute
	defaultBoxShape     = "1c2g"
	defaultBoxImageRef  = "public.ecr.aws/docker/library/alpine:3.20"
)

type execOptions struct {
	deadline time.Duration
	user     string
	workdir  string
	envVars  stringList
}

func (a *app) newBoxCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "box",
		Short:   "Manage boxes",
		Long:    "Create boxes, inspect them, copy files, run commands, and manage their lifecycle through portal-api.",
		Example: "  run9 box create my-box --image public.ecr.aws/docker/library/alpine:3.20\n  run9 box ls --state ready\n  run9 box exec my-box /bin/sh -lc 'echo hello'\n  run9 box cp ./local.txt my-box:/work/local.txt",
		RunE: func(cmd *cobra.Command, args []string) error {
			return usageError(cmd, "missing box subcommand (expected: create|ls|inspect|exec|cp|stop|commit|rm)")
		},
	}

	cmd.AddCommand(
		a.newBoxCreateCommand(),
		a.newBoxListCommand(),
		a.newBoxInspectCommand(),
		a.newBoxExecCommand(),
		a.newBoxCopyCommand(),
		a.newBoxStopCommand(),
		a.newBoxCommitCommand(),
		a.newBoxRemoveCommand(),
	)
	return cmd
}

func (a *app) newBoxCreateCommand() *cobra.Command {
	var shape string
	var description string
	var sourceSnapID string
	var sourceImage string
	var sourceImageRef string
	var labels stringList

	cmd := &cobra.Command{
		Use:     "create [box-id]",
		Short:   "Create one box",
		Long:    "Create one box from exactly one source: a snap or an image. When box-id is omitted, portal-api allocates one org-unique random identifier.",
		Example: "  run9 box create\n  run9 box create my-box --image public.ecr.aws/docker/library/alpine:3.20\n  run9 box create my-box --snap snap-1 --description \"My workspace\" --label team=portal",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return usageError(cmd, "unexpected args: %v", args)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			boxID := ""
			if len(args) == 1 {
				boxID = strings.TrimSpace(args[0])
			}

			labelMap, err := parseKeyValueMap(labels)
			if err != nil {
				return usageError(cmd, "%v", err)
			}

			imageRef := strings.TrimSpace(sourceImage)
			if strings.TrimSpace(sourceImageRef) != "" {
				if imageRef != "" && imageRef != strings.TrimSpace(sourceImageRef) {
					return usageError(cmd, "--image and --image-ref must match when both are set")
				}
				imageRef = strings.TrimSpace(sourceImageRef)
			}

			hasSnap := strings.TrimSpace(sourceSnapID) != ""
			if !hasSnap && imageRef == "" {
				imageRef = defaultBoxImageRef
			}
			hasImage := imageRef != ""
			if hasSnap == hasImage {
				return usageError(cmd, "exactly one of --snap or --image is required")
			}

			_, client, creds, err := loadAuthenticatedClient(a.configPath)
			if err != nil {
				return commandErrorf("%v", err)
			}

			view, err := client.CreateBox(cmd.Context(), creds, api.CreateBoxRequest{
				BoxID:          boxID,
				DesiredShape:   strings.TrimSpace(shape),
				Description:    strings.TrimSpace(description),
				Labels:         labelMap,
				SourceSnapID:   strings.TrimSpace(sourceSnapID),
				SourceImageRef: imageRef,
			})
			if err != nil {
				return commandErrorf("%v", err)
			}
			if err := writeJSON(a.stdout, view); err != nil {
				return commandErrorf("%v", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&shape, "shape", defaultBoxShape, "desired box shape")
	cmd.Flags().StringVar(&description, "description", "", "human-readable box description")
	cmd.Flags().StringVar(&sourceSnapID, "snap", "", "source snap ID")
	cmd.Flags().StringVar(&sourceImage, "image", "", "source image reference")
	cmd.Flags().StringVar(&sourceImageRef, "image-ref", "", "source image reference alias")
	cmd.Flags().Var(&labels, "label", "box label in key=value form (repeatable)")
	cmd.Flags().SortFlags = false
	return cmd
}

func (a *app) newBoxListCommand() *cobra.Command {
	var creator string
	var label string
	var state string

	cmd := &cobra.Command{
		Use:     "ls",
		Short:   "List boxes",
		Long:    "List boxes in the current org. Optional filters narrow the result set before the CLI prints the JSON response.",
		Example: "  run9 box ls\n  run9 box ls --creator alice\n  run9 box ls --label team=portal --state ready",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, client, creds, err := loadAuthenticatedClient(a.configPath)
			if err != nil {
				return commandErrorf("%v", err)
			}
			views, err := client.Boxes(cmd.Context(), creds, creator, label, state)
			if err != nil {
				return commandErrorf("%v", err)
			}
			if err := writeJSON(a.stdout, views); err != nil {
				return commandErrorf("%v", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&creator, "creator", "", "creator filter")
	cmd.Flags().StringVar(&label, "label", "", "label filter (key or key=value)")
	cmd.Flags().StringVar(&state, "state", "", "box state filter")
	cmd.Flags().SortFlags = false
	return cmd
}

func (a *app) newBoxInspectCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "inspect <box-id>",
		Short:   "Inspect one box",
		Long:    "Fetch one box view from portal-api and print it as indented JSON.",
		Example: "  run9 box inspect my-box",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageError(cmd, "usage: %s <box-id>", cmd.CommandPath())
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			_, client, creds, err := loadAuthenticatedClient(a.configPath)
			if err != nil {
				return commandErrorf("%v", err)
			}
			view, err := client.Box(cmd.Context(), creds, args[0])
			if err != nil {
				return commandErrorf("%v", err)
			}
			if err := writeJSON(a.stdout, view); err != nil {
				return commandErrorf("%v", err)
			}
			return nil
		},
	}
}

func (a *app) newBoxExecCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "exec <box-id> [--deadline=15m] [--user=...] [--workdir=...] [-e KEY=VALUE] <command...>",
		Short:              "Run one command inside a box",
		Long:               "Stream one remote exec through portal-api. The CLI accepts local exec flags before the remote command and preserves the remote exit code.",
		Example:            "  run9 box exec my-box /bin/sh -lc 'echo hello'\n  run9 box exec my-box --user root --workdir /workspace /bin/true\n  run9 box exec my-box -- --user remote-flag-value",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && isHelpToken(args[0]) {
				return cmd.Help()
			}

			flagArgs, boxID, command, err := splitBoxExecArgs(args)
			if err != nil {
				return usageError(cmd, "%v", err)
			}

			options, err := parseExecOptions(flagArgs)
			if err != nil {
				return usageError(cmd, "%v", err)
			}
			if strings.TrimSpace(boxID) == "" || len(command) == 0 {
				return usageError(cmd, "usage: %s <box-id> [--deadline=15m] [--user=...] [--workdir=...] [-e KEY=VALUE] <command...>", cmd.CommandPath())
			}

			envMap, err := parseKeyValueMap(options.envVars)
			if err != nil {
				return usageError(cmd, "%v", err)
			}

			_, client, creds, err := loadAuthenticatedClient(a.configPath)
			if err != nil {
				return commandErrorf("%v", err)
			}

			_, body, err := client.ExecStream(cmd.Context(), creds, boxID, api.ExecBoxRequest{
				DeadlineAt:   time.Now().Add(options.deadline),
				Command:      command,
				EnvOverrides: envMap,
				User:         strings.TrimSpace(options.user),
				Workdir:      strings.TrimSpace(options.workdir),
			})
			if err != nil {
				return commandErrorf("%v", err)
			}
			defer body.Close()

			return exitCodeError(streamExec(a.stdout, a.stderr, body))
		},
	}
	return cmd
}

func (a *app) newBoxCopyCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "cp <src> <dst>",
		Short:   "Copy files between the local host and one box",
		Long:    "Copy one file or directory by sending or receiving a tar archive through portal-api. Exactly one side must be a box path written as <box-id>:/absolute/path.",
		Example: "  run9 box cp ./local.txt my-box:/work/local.txt\n  run9 box cp ./project my-box:/work/\n  run9 box cp my-box:/work/result.txt ./result.txt",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return usageError(cmd, "usage: %s <src> <dst>", cmd.CommandPath())
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			srcBoxPath, srcIsBox, err := parseBoxPath(args[0])
			if err != nil {
				return usageError(cmd, "%v", err)
			}
			dstBoxPath, dstIsBox, err := parseBoxPath(args[1])
			if err != nil {
				return usageError(cmd, "%v", err)
			}
			if srcIsBox == dstIsBox {
				return usageError(cmd, "exactly one side of box cp must be <box-id>:/absolute/path")
			}

			_, client, creds, err := loadAuthenticatedClient(a.configPath)
			if err != nil {
				return commandErrorf("%v", err)
			}

			if dstIsBox {
				upload, err := archiveutil.BuildUploadArchive(args[0], dstBoxPath.RawPath)
				if err != nil {
					return commandErrorf("%v", err)
				}
				defer func() {
					_ = upload.File.Close()
					_ = os.Remove(upload.File.Name())
				}()

				if _, err := client.UploadArchive(cmd.Context(), creds, dstBoxPath.BoxID, upload.BoxAbsPath, upload.File); err != nil {
					return commandErrorf("%v", err)
				}
				return nil
			}

			body, err := client.DownloadArchive(cmd.Context(), creds, srcBoxPath.BoxID, srcBoxPath.AbsPath)
			if err != nil {
				return commandErrorf("%v", err)
			}
			defer body.Close()

			if err := archiveutil.ExtractDownloadArchive(body, args[1]); err != nil {
				return commandErrorf("%v", err)
			}
			return nil
		},
	}
}

func (a *app) newBoxStopCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "stop <box-id>",
		Short:   "Stop one box",
		Long:    "Request portal-api to stop one box and print the resulting box view as JSON.",
		Example: "  run9 box stop my-box",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageError(cmd, "usage: %s <box-id>", cmd.CommandPath())
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			_, client, creds, err := loadAuthenticatedClient(a.configPath)
			if err != nil {
				return commandErrorf("%v", err)
			}
			view, err := client.StopBox(cmd.Context(), creds, args[0])
			if err != nil {
				return commandErrorf("%v", err)
			}
			if err := writeJSON(a.stdout, view); err != nil {
				return commandErrorf("%v", err)
			}
			return nil
		},
	}
}

func (a *app) newBoxCommitCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "commit <box-id>",
		Short:   "Create one snap from a box",
		Long:    "Request portal-api to commit one box into a new snap and print the new snap view as JSON.",
		Example: "  run9 box commit my-box",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageError(cmd, "usage: %s <box-id>", cmd.CommandPath())
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			_, client, creds, err := loadAuthenticatedClient(a.configPath)
			if err != nil {
				return commandErrorf("%v", err)
			}
			view, err := client.CommitBox(cmd.Context(), creds, args[0])
			if err != nil {
				return commandErrorf("%v", err)
			}
			if err := writeJSON(a.stdout, view); err != nil {
				return commandErrorf("%v", err)
			}
			return nil
		},
	}
}

func (a *app) newBoxRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "rm <box-id>",
		Short:   "Remove one box",
		Long:    "Delete one box object through portal-api and print the removed box view as JSON.",
		Example: "  run9 box rm my-box",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageError(cmd, "usage: %s <box-id>", cmd.CommandPath())
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			_, client, creds, err := loadAuthenticatedClient(a.configPath)
			if err != nil {
				return commandErrorf("%v", err)
			}
			view, err := client.RemoveBox(cmd.Context(), creds, args[0])
			if err != nil {
				return commandErrorf("%v", err)
			}
			if err := writeJSON(a.stdout, view); err != nil {
				return commandErrorf("%v", err)
			}
			return nil
		},
	}
}

func parseExecOptions(args []string) (execOptions, error) {
	options := execOptions{deadline: defaultExecDeadline}

	fs := pflag.NewFlagSet("run9 box exec", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.DurationVar(&options.deadline, "deadline", defaultExecDeadline, "exec deadline")
	fs.StringVar(&options.user, "user", "", "exec user")
	fs.StringVar(&options.workdir, "workdir", "", "exec workdir")
	fs.VarP(&options.envVars, "env", "e", "environment override in KEY=VALUE form")
	fs.SortFlags = false
	if err := fs.Parse(args); err != nil {
		return execOptions{}, err
	}
	if len(fs.Args()) != 0 {
		return execOptions{}, commandErrorf("unexpected args: %v", fs.Args())
	}
	return options, nil
}
