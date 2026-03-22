package methane

import (
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/sirupsen/logrus"
)

// LaunchConfig holds configuration for the game launcher.
type LaunchConfig struct {
	// ExecPath is the path to the game executable.
	ExecPath string
	// ExtraArgs is a list of additional command-line arguments.
	ExtraArgs []string
	// ExtraEnv is a map of additional environment variables.
	ExtraEnv map[string]string
}

// GameLauncher manages launching and monitoring a game process.
//
//export GameLauncher
type GameLauncher struct {
	config  LaunchConfig
	cmd     *exec.Cmd
	process *os.Process
	mu      sync.Mutex
	logger  *logrus.Logger
}

// NewGameLauncher creates a new GameLauncher with the given config.
//
//export NewGameLauncher
func NewGameLauncher(config LaunchConfig) *GameLauncher {
	logger := logrus.New()
	return &GameLauncher{
		config: config,
		logger: logger,
	}
}

// Launch starts the game process using the provided GameLaunchInfo.
// Returns an error if the game is already running.
func (l *GameLauncher) Launch(info *GameLaunchInfo) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.process != nil {
		return fmt.Errorf("game is already running (PID %d)", l.process.Pid)
	}

	args := l.buildArgs(info)
	env := l.buildEnv(info)

	cmd := exec.Command(l.config.ExecPath, args...) //nolint:gosec
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch game %q: %w", l.config.ExecPath, err)
	}
	l.cmd = cmd
	l.process = cmd.Process

	l.logger.WithFields(logrus.Fields{
		"pid":        l.process.Pid,
		"exec":       l.config.ExecPath,
		"session_id": info.SessionID,
	}).Info("game launched")

	return nil
}

// IsRunning returns true if the game process is currently running.
func (l *GameLauncher) IsRunning() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.process != nil
}

// Wait blocks until the game process exits and cleans up internal state.
func (l *GameLauncher) Wait() error {
	l.mu.Lock()
	cmd := l.cmd
	l.mu.Unlock()

	if cmd == nil {
		return fmt.Errorf("no game process running")
	}

	err := cmd.Wait()

	l.mu.Lock()
	l.process = nil
	l.cmd = nil
	l.mu.Unlock()

	if err != nil {
		return fmt.Errorf("game process exited with error: %w", err)
	}
	return nil
}

// Stop sends a kill signal to the running game process.
func (l *GameLauncher) Stop() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.process == nil {
		return fmt.Errorf("no game process running")
	}
	if err := l.process.Kill(); err != nil {
		return fmt.Errorf("kill game process: %w", err)
	}
	l.logger.WithField("pid", l.process.Pid).Info("game process stopped")
	l.process = nil
	l.cmd = nil
	return nil
}

// buildArgs constructs the argument list for the game process.
func (l *GameLauncher) buildArgs(info *GameLaunchInfo) []string {
	args := make([]string, 0, len(l.config.ExtraArgs)+len(info.Args))
	args = append(args, l.config.ExtraArgs...)
	args = append(args, info.Args...)
	return args
}

// buildEnv constructs the environment for the game process.
func (l *GameLauncher) buildEnv(info *GameLaunchInfo) []string {
	base := os.Environ()
	env := make([]string, 0, len(base)+len(l.config.ExtraEnv)+len(info.Env))
	env = append(env, base...)
	for k, v := range l.config.ExtraEnv {
		env = append(env, k+"="+v)
	}
	for k, v := range info.Env {
		env = append(env, k+"="+v)
	}
	return env
}
