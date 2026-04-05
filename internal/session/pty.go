package session

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"helm-wails/internal/agent"

	"github.com/creack/pty"
)

type PTYStarter struct {
	FlushInterval time.Duration
	FlushBytes    int
}

func NewPTYStarter() *PTYStarter {
	return &PTYStarter{
		FlushInterval: 24 * time.Millisecond,
		FlushBytes:    4096,
	}
}

func (s *PTYStarter) Start(spec agent.LaunchSpec, meta StartMeta, sink EventSink, onExit ExitHandler) (Handle, error) {
	cmd := exec.Command(spec.Command, spec.Args...)
	cmd.Dir = spec.CWD
	cmd.Env = ensureTerminalEnv(spec.Env)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: 32,
		Cols: 120,
	})
	if err != nil {
		return nil, fmt.Errorf("start terminal session: %w", err)
	}

	handle := &ptyHandle{
		cmd:           cmd,
		ptmx:          ptmx,
		meta:          meta,
		sink:          sink,
		flushInterval: s.FlushInterval,
		flushBytes:    s.FlushBytes,
		outputCh:      make(chan []byte, 128),
		readDone:      make(chan struct{}),
		outputDone:    make(chan struct{}),
	}

	go handle.readLoop()
	go handle.outputLoop()
	go handle.waitLoop(onExit)

	return handle, nil
}

type ptyHandle struct {
	cmd  *exec.Cmd
	ptmx *os.File
	meta StartMeta
	sink EventSink

	flushInterval time.Duration
	flushBytes    int

	outputCh   chan []byte
	readDone   chan struct{}
	outputDone chan struct{}
	closeOnce  sync.Once
}

func (h *ptyHandle) Write(data string) error {
	if h.ptmx == nil {
		return errors.New("terminal is closed")
	}
	_, err := io.WriteString(h.ptmx, data)
	return err
}

func (h *ptyHandle) Resize(cols, rows int) error {
	if h.ptmx == nil || cols <= 0 || rows <= 0 {
		return nil
	}
	return pty.Setsize(h.ptmx, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
}

func (h *ptyHandle) Close() error {
	var closeErr error
	h.closeOnce.Do(func() {
		if h.cmd != nil && h.cmd.Process != nil {
			if pid := h.cmd.Process.Pid; pid > 0 {
				_ = syscall.Kill(-pid, syscall.SIGTERM)
			}
			_ = h.cmd.Process.Signal(syscall.SIGTERM)
		}
		if h.ptmx != nil {
			closeErr = h.ptmx.Close()
		}
	})
	return closeErr
}

func (h *ptyHandle) readLoop() {
	defer close(h.readDone)
	defer close(h.outputCh)

	buf := make([]byte, 8192)
	for {
		n, err := h.ptmx.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			h.outputCh <- chunk
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			if errors.Is(err, os.ErrClosed) {
				return
			}
			return
		}
	}
}

func (h *ptyHandle) outputLoop() {
	defer close(h.outputDone)

	ticker := time.NewTicker(h.flushInterval)
	defer ticker.Stop()

	var pending bytes.Buffer
	flush := func() {
		if pending.Len() == 0 {
			return
		}
		h.sink.Emit(EventSessionOutput, SessionOutputEvent{
			SessionID: h.meta.SessionID,
			Data:      pending.String(),
		})
		pending.Reset()
	}

	for {
		select {
		case chunk, ok := <-h.outputCh:
			if !ok {
				flush()
				return
			}
			_, _ = pending.Write(chunk)
			if pending.Len() >= h.flushBytes {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (h *ptyHandle) waitLoop(onExit ExitHandler) {
	err := h.cmd.Wait()
	<-h.readDone
	<-h.outputDone

	if onExit != nil {
		onExit(ExitInfo{
			SessionID:  h.meta.SessionID,
			WorktreeID: h.meta.WorktreeID,
			ExitCode:   exitCode(err),
			Err:        normalizeExitError(err),
		})
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			if status.Signaled() {
				return 128 + int(status.Signal())
			}
			return status.ExitStatus()
		}
	}
	return -1
}

func normalizeExitError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, os.ErrClosed) {
		return nil
	}
	return err
}

func ensureTerminalEnv(env []string) []string {
	out := append([]string(nil), env...)
	out = upsertEnv(out, "TERM", "xterm-256color")
	out = upsertEnv(out, "COLORTERM", "truecolor")
	return out
}

func upsertEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, item := range env {
		if strings.HasPrefix(item, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}
