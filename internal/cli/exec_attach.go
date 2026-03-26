package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/breezewish/run9-cli/internal/api"
	"github.com/gorilla/websocket"
	"golang.org/x/term"
)

const (
	execAttachInputTypeStdin      = "stdin"
	execAttachInputTypeCloseStdin = "close_stdin"
	execAttachInputTypeResize     = "resize"
)

type execReadResult struct {
	event api.ExecStreamEvent
	err   error
}

func (a *app) runInteractiveExec(
	ctx context.Context,
	client *api.Client,
	creds api.Credentials,
	boxID string,
	req api.ExecBoxRequest,
) error {
	if req.TTY {
		size, err := a.currentTTYSize()
		if err != nil {
			return commandErrorf("%v", err)
		}
		req.TTYSize = size
	}

	view, err := client.Exec(ctx, creds, boxID, req)
	if err != nil {
		return commandErrorf("%v", err)
	}

	socket, err := client.ExecAttach(ctx, creds, view.ExecID)
	if err != nil {
		return commandErrorf("%v", err)
	}
	defer socket.Close()

	restoreTTY, err := a.enterInteractiveTTY(req)
	if err != nil {
		return commandErrorf("%v", err)
	}
	defer restoreTTY()

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	readCh := make(chan execReadResult, 1)
	go func() {
		for {
			event, err := socket.ReadEvent()
			readCh <- execReadResult{event: event, err: err}
			if err != nil {
				return
			}
		}
	}()

	stdinErrCh := a.startExecStdinPump(streamCtx, socket, req.StdinEnabled)
	resizeErrCh := a.startExecResizePump(streamCtx, socket, req.TTY)

	for {
		select {
		case <-streamCtx.Done():
			return commandErrorf("exec stream ended unexpectedly")
		case result := <-readCh:
			if result.err != nil {
				cancel()
				if isExpectedExecAttachClose(result.err) {
					return commandErrorf("exec stream ended unexpectedly")
				}
				return commandErrorf("%v", result.err)
			}
			switch result.event.Type {
			case "keepalive", "started":
				continue
			case "stdout":
				if _, err := a.stdout.Write(result.event.Data); err != nil {
					return commandErrorf("%v", err)
				}
			case "stderr":
				if _, err := a.stderr.Write(result.event.Data); err != nil {
					return commandErrorf("%v", err)
				}
			case "exit":
				cancel()
				return exitCodeError(normalizeExitCode(int(result.event.ExitCode)))
			case "cancelled":
				cancel()
				reason := result.event.CancelReason
				if reason == "" {
					reason = "exec cancelled"
				}
				return commandErrorf("%s", reason)
			case "error":
				cancel()
				reason := result.event.FailureReason
				if reason == "" {
					reason = "exec failed"
				}
				return commandErrorf("%s", reason)
			}
		case err := <-stdinErrCh:
			if err == nil {
				stdinErrCh = nil
				continue
			}
			cancel()
			return commandErrorf("%v", err)
		case err := <-resizeErrCh:
			if err == nil {
				resizeErrCh = nil
				continue
			}
			cancel()
			return commandErrorf("%v", err)
		}
	}
}

func (a *app) currentTTYSize() (*api.TTYSize, error) {
	if a.stdinFile == nil || a.stdoutFile == nil {
		return nil, errors.New("-t requires a terminal on stdin and stdout")
	}
	if !term.IsTerminal(int(a.stdinFile.Fd())) || !term.IsTerminal(int(a.stdoutFile.Fd())) {
		return nil, errors.New("-t requires a terminal on stdin and stdout")
	}
	cols, rows, err := term.GetSize(int(a.stdoutFile.Fd()))
	if err != nil {
		return nil, fmt.Errorf("read terminal size: %w", err)
	}
	if rows <= 0 || cols <= 0 {
		return nil, errors.New("terminal size must be non-zero")
	}
	return &api.TTYSize{Rows: uint32(rows), Cols: uint32(cols)}, nil
}

func (a *app) enterInteractiveTTY(req api.ExecBoxRequest) (func(), error) {
	if !req.TTY || !req.StdinEnabled {
		return func() {}, nil
	}
	if a.stdinFile == nil {
		return nil, errors.New("stdin is not available")
	}
	state, err := term.MakeRaw(int(a.stdinFile.Fd()))
	if err != nil {
		return nil, fmt.Errorf("set terminal raw mode: %w", err)
	}
	return func() {
		_ = term.Restore(int(a.stdinFile.Fd()), state)
	}, nil
}

func (a *app) startExecStdinPump(ctx context.Context, socket *api.ExecAttachSocket, enabled bool) <-chan error {
	if !enabled {
		return nil
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- pumpExecStdin(ctx, socket, a.stdin)
	}()
	return errCh
}

func pumpExecStdin(ctx context.Context, socket *api.ExecAttachSocket, source io.Reader) error {
	if source == nil {
		return errors.New("stdin is not available")
	}
	buf := make([]byte, 64*1024)
	for {
		n, err := source.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			if err := socket.WriteInput(api.ExecAttachInput{Type: execAttachInputTypeStdin, Data: data}); err != nil {
				return err
			}
		}
		if err == io.EOF {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			return socket.WriteInput(api.ExecAttachInput{Type: execAttachInputTypeCloseStdin})
		}
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		default:
		}
	}
}

func (a *app) startExecResizePump(ctx context.Context, socket *api.ExecAttachSocket, enabled bool) <-chan error {
	if !enabled || a.stdoutFile == nil {
		return nil
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- pumpExecResize(ctx, socket, a.stdoutFile)
	}()
	return errCh
}

func pumpExecResize(ctx context.Context, socket *api.ExecAttachSocket, tty *os.File) error {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGWINCH)
	defer signal.Stop(signals)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-signals:
			cols, rows, err := term.GetSize(int(tty.Fd()))
			if err != nil {
				return fmt.Errorf("read terminal size: %w", err)
			}
			if rows == 0 || cols == 0 {
				continue
			}
			if err := socket.WriteInput(api.ExecAttachInput{
				Type: execAttachInputTypeResize,
				Rows: uint32(rows),
				Cols: uint32(cols),
			}); err != nil {
				return err
			}
		}
	}
}

func isExpectedExecAttachClose(err error) bool {
	return websocket.IsCloseError(
		err,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseNoStatusReceived,
	)
}
