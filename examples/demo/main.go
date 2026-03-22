// Command demo demonstrates the methane matchmaking library by creating two
// local service instances, creating a lobby, and exchanging messages.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/opd-ai/methane"
	"github.com/opd-ai/toxcore"
)

func mustNewTox() *toxcore.Tox {
	opts := toxcore.NewOptions()
	tox, err := toxcore.New(opts)
	if err != nil {
		log.Fatalf("toxcore.New: %v", err)
	}
	return tox
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create two Tox nodes.
	toxA := mustNewTox()
	toxB := mustNewTox()
	defer toxA.Kill()
	defer toxB.Kill()

	fmt.Println("Node A:", toxA.SelfGetAddress())
	fmt.Println("Node B:", toxB.SelfGetAddress())

	// Create matchmaking services.
	svcA := methane.NewMatchmakingService(toxA)
	svcB := methane.NewMatchmakingService(toxB)

	if err := svcA.Start(ctx); err != nil {
		log.Fatalf("svcA.Start: %v", err)
	}
	if err := svcB.Start(ctx); err != nil {
		log.Fatalf("svcB.Start: %v", err)
	}
	defer svcA.Stop()
	defer svcB.Stop()

	// Register lobby-found callback on B.
	svcB.OnLobbyFound(func(ad *methane.LobbyAdvertisement) {
		fmt.Printf("[B] Discovered lobby: game=%s id=%d players=%d/%d\n",
			ad.GameName, ad.LobbyID, ad.CurPlayers, ad.MaxPlayers)
	})

	// Register match-found callback on both.
	svcA.OnMatchFound(func(ev *methane.MatchFoundEvent) {
		fmt.Printf("[A] Match found: session=%s players=%d\n", ev.SessionID, len(ev.Players))
	})
	svcB.OnMatchFound(func(ev *methane.MatchFoundEvent) {
		fmt.Printf("[B] Match found: session=%s players=%d\n", ev.SessionID, len(ev.Players))
	})

	// A creates a lobby.
	lobby, err := svcA.CreateLobby(methane.LobbyConfig{
		GameName:   "DemoGame",
		MapName:    "dust2",
		GameMode:   methane.ModeFFA,
		MaxPlayers: 4,
		Region:     methane.RegionAny,
	})
	if err != nil {
		log.Fatalf("CreateLobby: %v", err)
	}
	fmt.Printf("[A] Created lobby id=%d\n", lobby.ID)

	// Queue both players for a match so we can demonstrate matchmaking.
	tp := methane.NewMockTimeProvider(time.Now())
	svcA.SetTimeProvider(tp)
	svcB.SetTimeProvider(tp)

	playerA := svcA.GetSelfPlayer()
	playerB := svcB.GetSelfPlayer()

	// Create a shared queue for the demo.
	q := methane.NewMatchmakingQueue(methane.DefaultQueueConfig())
	matched := make(chan []*methane.Player, 1)
	q.OnMatchFound(func(players []*methane.Player) {
		matched <- players
	})
	_ = q.Enqueue(playerA, methane.ModeFFA, methane.RegionAny)
	_ = q.Enqueue(playerB, methane.ModeFFA, methane.RegionAny)
	q.RunOnce()

	select {
	case players := <-matched:
		fmt.Printf("Demo: matched %d players!\n", len(players))
		for _, p := range players {
			fmt.Printf("  - %s (rating=%.0f)\n", p.PublicKeyHex(), p.GetRating())
		}
	case <-time.After(5 * time.Second):
		fmt.Println("Demo: no match formed (queue may need more players)")
	}

	fmt.Println("Demo complete.")
}
