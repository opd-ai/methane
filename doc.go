// Package methane provides a decentralized game matchmaking service built on
// top of the Tox protocol via github.com/opd-ai/toxcore.
//
// Methane enables peer-to-peer game lobby discovery, matchmaking queues, and
// session lifecycle management without any central server. All communication
// uses the Tox friend-message and conference APIs.
//
// # Quick Start
//
//	opts := toxcore.NewOptions()
//	tox, err := toxcore.New(opts)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer tox.Kill()
//
//	svc := methane.NewMatchmakingService(tox)
//	if err := svc.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//	defer svc.Stop()
//
//	lobby, err := svc.CreateLobby(methane.LobbyConfig{
//	    GameName: "MyGame", MaxPlayers: 4, GameMode: methane.ModeFFA,
//	})
package methane
