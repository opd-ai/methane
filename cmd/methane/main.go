// Command methane is a CLI for the decentralized game matchmaking service.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/opd-ai/methane"
	"github.com/opd-ai/toxcore"
	"github.com/sirupsen/logrus"
)

var log = logrus.New()

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	subcommand := os.Args[1]
	args := os.Args[2:]

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	switch subcommand {
	case "host":
		runHost(args, ctx)
	case "browse":
		runBrowse(args, ctx)
	case "join":
		runJoin(args, ctx)
	case "queue":
		runQueue(args, ctx)
	case "launch":
		runLaunch(args, ctx)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n", subcommand)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage: methane <subcommand> [flags]

Subcommands:
  host     --game <name> --mode <ffa|tdm|ctf|coop> --max-players <n> --region <any|na|eu|asia>
  browse
  join     <lobby-id>
  queue    --mode <ffa|tdm|ctf|coop> [--region <any|na|eu|asia>]
  launch   --exec <path>

Common flags (available in all subcommands):
  --save-file <path>      Tox save file (default: methane.tox)
  --name <name>           Display name (default: methane-player)
  --bootstrap <h:p:key>   Bootstrap node (host:port:pubkey)`)
}

// addCommonFlags registers the shared --save-file / --name / --bootstrap flags
// on fs and returns pointers to their values.
func addCommonFlags(fs *flag.FlagSet) (saveFile, name, bootstrap *string) {
	saveFile = fs.String("save-file", "methane.tox", "path to Tox save file")
	name = fs.String("name", "methane-player", "display name")
	bootstrap = fs.String("bootstrap", "", "bootstrap node as host:port:pubkey")
	return
}

func initTox(saveFile, name, bootstrapStr string) (*toxcore.Tox, error) {
	opts := toxcore.NewOptions()

	// Load save data if it exists.
	if data, err := os.ReadFile(saveFile); err == nil {
		opts.SavedataData = data
		opts.SavedataType = toxcore.SaveDataTypeToxSave
	}

	tox, err := toxcore.New(opts)
	if err != nil {
		return nil, fmt.Errorf("toxcore.New: %w", err)
	}

	if name != "" {
		if err := tox.SelfSetName(name); err != nil {
			log.WithError(err).Warn("failed to set name")
		}
	}

	if bootstrapStr != "" {
		parts := strings.SplitN(bootstrapStr, ":", 3)
		if len(parts) == 3 {
			port, _ := strconv.ParseUint(parts[1], 10, 16)
			if err := tox.Bootstrap(parts[0], uint16(port), parts[2]); err != nil {
				log.WithError(err).Warn("bootstrap failed")
			}
		}
	}

	return tox, nil
}

func newService(saveFile, name, bootstrapStr string, ctx context.Context) (*methane.MatchmakingService, *toxcore.Tox, error) {
	tox, err := initTox(saveFile, name, bootstrapStr)
	if err != nil {
		return nil, nil, err
	}
	svc := methane.NewMatchmakingService(tox)
	if err := svc.Start(ctx); err != nil {
		tox.Kill()
		return nil, nil, fmt.Errorf("start service: %w", err)
	}
	return svc, tox, nil
}

func runHost(args []string, ctx context.Context) {
	fs := flag.NewFlagSet("host", flag.ExitOnError)
	saveFile, name, bootstrap := addCommonFlags(fs)
	game := fs.String("game", "MyGame", "game name")
	mode := fs.String("mode", "ffa", "game mode (ffa, tdm, ctf, coop)")
	maxPlayers := fs.Int("max-players", 4, "maximum players")
	region := fs.String("region", methane.RegionAny, "region")
	_ = fs.Parse(args)

	svc, tox, err := newService(*saveFile, *name, *bootstrap, ctx)
	if err != nil {
		log.WithError(err).Fatal("failed to initialize service")
	}
	defer tox.Kill()
	defer svc.Stop()

	cfg := methane.LobbyConfig{
		GameName:   *game,
		MaxPlayers: *maxPlayers,
		Region:     *region,
		GameMode:   parseModeFlag(*mode),
	}
	lobby, err := svc.CreateLobby(cfg)
	if err != nil {
		log.WithError(err).Fatal("failed to create lobby")
	}
	log.WithField("lobby_id", lobby.ID).Info("lobby created – advertising to friends")

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			svc.AdvertiseLobby(lobby)
		}
	}
}

func runBrowse(args []string, ctx context.Context) {
	fs := flag.NewFlagSet("browse", flag.ExitOnError)
	saveFile, name, bootstrap := addCommonFlags(fs)
	_ = fs.Parse(args)

	svc, tox, err := newService(*saveFile, *name, *bootstrap, ctx)
	if err != nil {
		log.WithError(err).Fatal("failed to initialize service")
	}
	defer tox.Kill()
	defer svc.Stop()

	svc.OnLobbyFound(func(ad *methane.LobbyAdvertisement) {
		fmt.Printf("[LOBBY] id=%d game=%s mode=%d players=%d/%d region=%s host=%s\n",
			ad.LobbyID, ad.GameName, ad.GameMode, ad.CurPlayers, ad.MaxPlayers, ad.Region, ad.HostPK)
	})
	log.Info("browsing lobbies from friends (waiting for advertisements)…")
	<-ctx.Done()
}

func runJoin(args []string, ctx context.Context) {
	fs := flag.NewFlagSet("join", flag.ExitOnError)
	saveFile, name, bootstrap := addCommonFlags(fs)
	_ = fs.Parse(args)

	remaining := fs.Args()
	if len(remaining) < 1 {
		fmt.Fprintln(os.Stderr, "usage: methane join [--save-file <path>] [--name <name>] [--bootstrap <h:p:key>] <lobby-id>")
		os.Exit(1)
	}
	id, err := strconv.ParseUint(remaining[0], 10, 32)
	if err != nil {
		log.WithError(err).Fatal("invalid lobby-id")
	}

	svc, tox, err := newService(*saveFile, *name, *bootstrap, ctx)
	if err != nil {
		log.WithError(err).Fatal("failed to initialize service")
	}
	defer tox.Kill()
	defer svc.Stop()

	// For simplicity, we assume friend 0 is the host.
	if err := svc.JoinLobby(uint32(id), 0); err != nil {
		log.WithError(err).Fatal("failed to join lobby")
	}
	log.WithField("lobby_id", id).Info("join request sent")
}

func runQueue(args []string, ctx context.Context) {
	fs := flag.NewFlagSet("queue", flag.ExitOnError)
	saveFile, name, bootstrap := addCommonFlags(fs)
	mode := fs.String("mode", "ffa", "game mode (ffa, tdm, ctf, coop)")
	region := fs.String("region", methane.RegionAny, "region")
	_ = fs.Parse(args)

	svc, tox, err := newService(*saveFile, *name, *bootstrap, ctx)
	if err != nil {
		log.WithError(err).Fatal("failed to initialize service")
	}
	defer tox.Kill()
	defer svc.Stop()

	svc.OnMatchFound(func(event *methane.MatchFoundEvent) {
		fmt.Printf("[MATCH] session=%s players=%d\n", event.SessionID, len(event.Players))
	})

	if err := svc.EnqueueForMatch(parseModeFlag(*mode), *region); err != nil {
		log.WithError(err).Fatal("failed to enqueue")
	}
	log.Info("queued for match – waiting…")
	<-ctx.Done()
	svc.LeaveQueue()
}

func runLaunch(args []string, ctx context.Context) {
	fs := flag.NewFlagSet("launch", flag.ExitOnError)
	saveFile, name, bootstrap := addCommonFlags(fs)
	execPath := fs.String("exec", "", "path to game executable")
	_ = fs.Parse(args)

	svc, tox, err := newService(*saveFile, *name, *bootstrap, ctx)
	if err != nil {
		log.WithError(err).Fatal("failed to initialize service")
	}
	defer tox.Kill()
	defer svc.Stop()

	if *execPath == "" {
		log.Fatal("--exec is required")
	}
	launcher := methane.NewGameLauncher(methane.LaunchConfig{ExecPath: *execPath})
	info := &methane.GameLaunchInfo{SessionID: "manual"}
	if err := launcher.Launch(info); err != nil {
		log.WithError(err).Fatal("failed to launch game")
	}
	if err := launcher.Wait(); err != nil {
		log.WithError(err).Warn("game exited with error")
	}
}

func parseModeFlag(s string) methane.GameMode {
	switch strings.ToLower(s) {
	case "tdm", "team", "teamdeathmatch":
		return methane.ModeTeamDeathmatch
	case "ctf", "captureflag":
		return methane.ModeCaptureFlag
	case "coop", "cooperative":
		return methane.ModeCooperative
	default:
		return methane.ModeFFA
	}
}
