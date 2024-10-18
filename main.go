package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"net/http"

	"github.com/fiatjaf/khatru"
	"github.com/joho/godotenv"
	"github.com/nbd-wtf/go-nostr"
)

var ctx = context.Background()
var pool = nostr.NewSimplePool(ctx)
var repository *NostrRepository
var relays = []string{
	"wss://relay.lexingtonbitcoin.org",
	"wss://nostr.600.wtf",
	"wss://nostr.hexhex.online",
	"wss://wot.utxo.one",
	"wss://nostrelites.org",
	"wss://wot.nostr.party",
	"wss://wot.puhcho.me",
	"wss://wot.girino.org",
	"wss://relay.beeola.me",
	"wss://zap.watch",
	"wss://wot.yeghro.site",
	"wss://wot.innovativecerebrum.ai",
	"wss://wot.swarmstr.com",
	"wss://wot.azzamo.net",
	"wss://satsage.xyz",
	"wss://wot.sandwich.farm",
	"wss://wons.calva.dev",
	"wss://wot.shaving.kiwi",
	"wss://wot.tealeaf.dev",
	"wss://wot.dtonon.com",
	"wss://wot.relay.vanderwarker.family",
	"wss://wot.zacoos.com",
}
var db *sql.DB

func main() {
	importFlag := flag.Bool("import", false, "Run the importNotes function after initializing relays")
	flag.Parse()
	conn, err := getDBConnection()

	if err != nil {
		log.Fatalf("Error getting db connection: %v", err)
	}
	defer conn.Close()
	db = conn
	repository = NewNostrRepository(db)

	if *importFlag {
		log.Println("📦 importing notes")
		importNotes(nostr.KindTextNote)
		importNotes(nostr.KindReaction)
		importNotes(nostr.KindZap)
		return
	}

	go subscribeAll()

	relay := khatru.NewRelay()
	relay.OnConnect = append(relay.OnConnect, func(ctx context.Context) {
		khatru.RequestAuth(ctx)
	})
	relay.RejectFilter = append(relay.RejectFilter, func(ctx context.Context, filter nostr.Filter) (bool, string) {
		authenticatedUser := khatru.GetAuthed(ctx)
		if authenticatedUser == "" {
			return true, "auth-required: this query requires you to be authenticated"
		}
		return false, ""
	})
	relay.RejectEvent = append(relay.RejectEvent, func(ctx context.Context, event *nostr.Event) (bool, string) {
		return true, "you cannot publish to this relay"
	})

	relay.QueryEvents = append(relay.QueryEvents, func(ctx context.Context, filter nostr.Filter) (chan *nostr.Event, error) {
		ch := make(chan *nostr.Event)
		copyFilter := filter
		authenticatedUser := khatru.GetAuthed(ctx)

		go func() {
			defer close(ch)

			limit := copyFilter.Limit
			if limit == 0 {
				limit = 50
			}

			log.Println("Fetching user feed for", authenticatedUser)

			events, err := repository.GetUserFeed(ctx, authenticatedUser, limit)
			if err != nil {
				log.Println("Error fetching most reacted posts:", err)
				return
			}

			for _, event := range events {
				ch <- &event
			}
		}()

		return ch, nil
	})

	err = http.ListenAndServe(":3334", relay)
	if err != nil {
		log.Fatal(err)
	}

}

func subscribeAll() {
	repository := NewNostrRepository(db)
	filters := nostr.Filters{{
		Kinds: []int{
			nostr.KindTextNote,
			nostr.KindReaction,
			nostr.KindZap,
		},
	}}

	for ev := range pool.SubMany(ctx, relays, filters) {
		err := repository.SaveNostrEvent(ev.Event)
		if err != nil {
			continue
		}
	}
}

func loadEnv() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}
}
