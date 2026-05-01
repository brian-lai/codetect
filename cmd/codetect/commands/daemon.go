package commands

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"codetect/internal/daemon"
	"codetect/internal/logging"
	"codetect/internal/registry"
)

// RunDaemon dispatches to daemon subcommands: start, stop, status.
// Moved from cmd/codetect-daemon/main.go.
// NOTE: `logs` subcommand is intentionally absent — it does not exist in the
// current daemon and is scoped to the §8 daemon redesign plan.
func RunDaemon(args []string) ExitCode {
	logger := logging.Default("codetect")

	if len(args) == 0 {
		printDaemonUsage(os.Stderr)
		return ExitError
	}

	switch args[0] {
	case "start":
		return daemonStart(logger, args[1:])
	case "stop":
		return daemonStop(logger)
	case "status":
		return daemonStatus(logger)
	case "help", "--help", "-h":
		printDaemonUsage(os.Stdout)
		return ExitOK
	default:
		fmt.Fprintf(os.Stderr, "codetect: unknown daemon subcommand %q\n", args[0])
		printDaemonUsage(os.Stderr)
		return ExitUnknownArg
	}
}

func printDaemonUsage(w io.Writer) {
	fmt.Fprintf(w, "Usage: codetect daemon <command>\n\n")
	fmt.Fprintf(w, "Commands:\n")
	fmt.Fprintf(w, "  start     Start the daemon\n")
	fmt.Fprintf(w, "  stop      Stop the daemon\n")
	fmt.Fprintf(w, "  status    Show daemon status\n")
	fmt.Fprintf(w, "  help      Show this help\n")
}

func daemonStart(logger *slog.Logger, args []string) ExitCode {
	fs := flag.NewFlagSet("daemon start", flag.ExitOnError)
	_ = fs.Bool("foreground", false, "Run in foreground (don't daemonize)")
	fs.Parse(args)

	client := daemon.NewIPCClient(daemon.DefaultSocketPath())
	if client.IsRunning() {
		logger.Error("daemon is already running")
		return ExitError
	}

	reg, err := registry.NewRegistry()
	if err != nil {
		logger.Error("failed to load registry", "error", err)
		return ExitError
	}

	cfg := daemon.DefaultConfig()
	d, err := daemon.New(reg, cfg)
	if err != nil {
		logger.Error("failed to create daemon", "error", err)
		return ExitError
	}

	logger.Info("daemon started", "pid", os.Getpid())
	logger.Info("note: Run with --foreground or use '&' to background")
	if err := d.Run(cfg); err != nil {
		logger.Error("daemon error", "error", err)
		return ExitError
	}
	return ExitOK
}

func daemonStop(logger *slog.Logger) ExitCode {
	client := daemon.NewIPCClient(daemon.DefaultSocketPath())
	if !client.IsRunning() {
		logger.Error("daemon is not running")
		return ExitError
	}
	if err := client.Stop(); err != nil {
		logger.Error("failed to stop daemon", "error", err)
		return ExitError
	}
	logger.Info("daemon stopped")
	return ExitOK
}

func daemonStatus(logger *slog.Logger) ExitCode {
	client := daemon.NewIPCClient(daemon.DefaultSocketPath())
	status, err := client.Status()
	if err != nil {
		logger.Info("daemon is not running")
		return ExitError
	}
	data, _ := json.MarshalIndent(status, "", "  ")
	fmt.Println(string(data))
	return ExitOK
}
