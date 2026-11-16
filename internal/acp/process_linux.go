//go:build linux

package acp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

func (probe *Probe) RunProcess(parent context.Context, spec ProcessSpec, request Request) (result Result, err error) {
	if err := probe.claim(request); err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(parent, probe.limits.WallTime)
	defer cancel()
	defer func() {
		if parent.Err() != nil {
			err = errors.Join(err, parent.Err())
		} else if deadline, exists := parent.Deadline(); exists && !time.Now().Before(deadline) {
			err = errors.Join(err, context.DeadlineExceeded)
		} else if deadline, _ := ctx.Deadline(); ctx.Err() != nil || !time.Now().Before(deadline) {
			err = errors.Join(err, ErrBudget)
		}
		if err != nil {
			result = Result{}
		}
	}()
	if len(spec.Args) > 32 {
		return Result{}, ErrProcess
	}
	argBytes := 0
	for _, argument := range spec.Args {
		argBytes += len(argument)
		if strings.ContainsRune(argument, 0) || argBytes > 8192 {
			return Result{}, ErrProcess
		}
	}
	executable, err := openPinnedExecutable(ctx, spec)
	if err != nil {
		return Result{}, err
	}
	defer executable.Close()
	root, err := os.MkdirTemp("", "butler-acp-")
	if err != nil {
		return Result{}, ErrProcess
	}
	defer os.RemoveAll(root)
	for _, directory := range []string{"home", "work", "tmp"} {
		if os.Mkdir(filepath.Join(root, directory), 0700) != nil {
			return Result{}, ErrProcess
		}
	}
	command := exec.Command("/proc/self/fd/3", spec.Args...)
	command.Dir = filepath.Join(root, "work")
	command.Env = []string{
		"PATH=/usr/bin:/bin",
		"HOME=" + filepath.Join(root, "home"),
		"XDG_CONFIG_HOME=" + filepath.Join(root, "home", "config"),
		"XDG_CACHE_HOME=" + filepath.Join(root, "home", "cache"),
		"XDG_DATA_HOME=" + filepath.Join(root, "home", "data"),
		"TMPDIR=" + filepath.Join(root, "tmp"),
		"LANG=C", "NO_COLOR=1",
	}
	if spec.profileDir != "" {
		if ValidateAntigravityProfile(spec.profileDir) != nil {
			return Result{}, ErrProcess
		}
		command.Env[1] = "HOME=" + spec.profileDir
	}
	if spec.proxied {
		command.Env = append(command.Env, "HTTPS_PROXY=http://127.0.0.1:3128", "HTTP_PROXY=http://127.0.0.1:3128", "https_proxy=http://127.0.0.1:3128", "http_proxy=http://127.0.0.1:3128")
	}
	command.ExtraFiles = []*os.File{executable}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	stdin, err := command.StdinPipe()
	if err != nil {
		return Result{}, ErrProcess
	}
	defer stdin.Close()
	stdout, err := command.StdoutPipe()
	if err != nil {
		return Result{}, ErrProcess
	}
	defer stdout.Close()
	stderr, err := command.StderrPipe()
	if err != nil {
		return Result{}, ErrProcess
	}
	defer stderr.Close()
	if ctx.Err() != nil {
		return Result{}, ErrBudget
	}
	if command.Start() != nil {
		return Result{}, ErrProcess
	}
	client, bridge := net.Pipe()
	var pumps sync.WaitGroup
	pumps.Add(3)
	go func() {
		defer pumps.Done()
		_, _ = io.Copy(stdin, bridge)
		_ = stdin.Close()
	}()
	go func() {
		defer pumps.Done()
		_, _ = io.Copy(bridge, stdout)
		_ = bridge.Close()
	}()
	stderrFailure := make(chan error, 1)
	go func() {
		defer pumps.Done()
		count, copyErr := io.Copy(io.Discard, io.LimitReader(stderr, 64*1024+1))
		if count > 64*1024 {
			stderrFailure <- ErrBudget
			cancel()
		} else if copyErr != nil && !errors.Is(copyErr, os.ErrClosed) {
			stderrFailure <- ErrProcess
			cancel()
		}
	}()
	defer func() {
		_ = client.Close()
		_ = bridge.Close()
		killErr := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		rootKillErr := command.Process.Kill()
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		waited := make(chan error, 1)
		go func() { waited <- command.Wait() }()
		select {
		case waitErr := <-waited:
			var exitErr *exec.ExitError
			if waitErr != nil && (!errors.As(waitErr, &exitErr) || exitErr.ProcessState.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL) {
				err = errors.Join(err, ErrProcess)
			}
		case <-time.After(250 * time.Millisecond):
			err = errors.Join(err, ErrProcess)
		}
		pumps.Wait()
		if killErr != nil && !errors.Is(killErr, syscall.ESRCH) {
			err = errors.Join(err, ErrProcess)
		}
		if rootKillErr != nil && !errors.Is(rootKillErr, os.ErrProcessDone) {
			err = errors.Join(err, ErrProcess)
		}
		select {
		case failure := <-stderrFailure:
			err = errors.Join(err, failure)
		default:
		}
	}()
	return probe.run(ctx, client, request, command.Dir)
}

func openPinnedExecutable(ctx context.Context, spec ProcessSpec) (*os.File, error) {
	expected, err := hex.DecodeString(spec.SHA256)
	if err != nil || len(expected) != sha256.Size || !filepath.IsAbs(spec.Executable) {
		return nil, ErrProcess
	}
	descriptor, err := syscall.Open(spec.Executable, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, ErrProcess
	}
	executable := os.NewFile(uintptr(descriptor), "acp-qualified-executable")
	accepted := false
	defer func() {
		if !accepted {
			_ = executable.Close()
		}
	}()
	info, err := executable.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0111 == 0 || info.Size() > 512*1024*1024 {
		return nil, ErrProcess
	}
	buffer := make([]byte, 64*1024)
	digest := sha256.New()
	total := 0
	for {
		if ctx.Err() != nil {
			return nil, ErrBudget
		}
		count, readErr := executable.Read(buffer)
		if total == 0 && (count < 4 || !bytes.Equal(buffer[:4], []byte{0x7f, 'E', 'L', 'F'})) {
			return nil, ErrProcess
		}
		total += count
		if total > 512*1024*1024 {
			return nil, ErrProcess
		}
		_, _ = digest.Write(buffer[:count])
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, ErrProcess
		}
	}
	if !bytes.Equal(digest.Sum(nil), expected) {
		return nil, ErrProcess
	}
	if _, err = executable.Seek(0, io.SeekStart); err != nil {
		return nil, ErrProcess
	}
	accepted = true
	return executable, nil
}
