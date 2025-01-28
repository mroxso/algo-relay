package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/fiatjaf/khatru"
	"github.com/fiatjaf/khatru/policies"
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
	"wss://nos.lol",
	"wss://nostr.mom",
	"wss://purplepag.es",
	"wss://purplerelay.com",
	"wss://relay.damus.io",
	"wss://relay.nostr.band",
	"wss://relay.snort.social",
	"wss://relayable.org",
	"wss://relay.primal.net",
	"wss://relay.nostr.bg",
	"wss://no.str.cr",
	"wss://nostr21.com",
	"wss://nostrue.com",
	"wss://relay.siamstr.com",
}

var db *sql.DB
var art = `
 █████╗ ██╗      ██████╗  ██████╗     ██████╗ ███████╗██╗      █████╗ ██╗   ██╗
██╔══██╗██║     ██╔════╝ ██╔═══██╗    ██╔══██╗██╔════╝██║     ██╔══██╗╚██╗ ██╔╝
███████║██║     ██║  ███╗██║   ██║    ██████╔╝█████╗  ██║     ███████║ ╚████╔╝ 
██╔══██║██║     ██║   ██║██║   ██║    ██╔══██╗██╔══╝  ██║     ██╔══██║  ╚██╔╝  
██║  ██║███████╗╚██████╔╝╚██████╔╝    ██║  ██║███████╗███████╗██║  ██║   ██║   
╚═╝  ╚═╝╚══════╝ ╚═════╝  ╚═════╝     ╚═╝  ╚═╝╚══════╝╚══════╝╚═╝  ╚═╝   ╚═╝   
	`

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file")
	}
	nostr.InfoLogger = log.New(io.Discard, "", 0)
	green := "\033[32m"
	reset := "\033[0m"
	fmt.Println(green + art + reset)

	importFlag := flag.Bool("import", false, "Run the importNotes function after initializing relays")
	flag.Parse()
	conn, err := getDBConnection()

	if err != nil {
		log.Fatalf("Error getting db connection: %v", err)
	}
	defer conn.Close()
	db = conn
	repository = NewNostrRepository(db)
	weightInteractionsWithAuthor = getWeightFloat64("WEIGHT_INTERACTIONS_WITH_AUTHOR")
	weightCommentsGlobal = getWeightFloat64("WEIGHT_COMMENTS_GLOBAL")
	weightReactionsGlobal = getWeightFloat64("WEIGHT_REACTIONS_GLOBAL")
	weightZapsGlobal = getWeightFloat64("WEIGHT_ZAPS_GLOBAL")
	weightRecency = getWeightFloat64("WEIGHT_RECENCY")
	viralThreshold = getWeightFloat64("VIRAL_THRESHOLD")
	viralNoteDampening = getWeightFloat64("VIRAL_NOTE_DAMPENING")
	decayRate = getWeightFloat64("DECAY_RATE")

	purgeMonthsStr := os.Getenv("PURGE_MONTHS")
	if purgeMonthsStr == "" {
		log.Fatal("PURGE_MONTHS environment variable is not set")
	}

	purgeMonths, err := strconv.Atoi(purgeMonthsStr)
	if err != nil {
		log.Fatalf("Invalid PURGE_MONTHS value: %v\n", err)
	}

	if *importFlag {
		log.Println("📦 importing notes")
		importNotes(nostr.KindArticle)
		importNotes(20) // KindImage
		importNotes(nostr.KindTextNote)
		importNotes(nostr.KindReaction)
		importNotes(nostr.KindZap)

		log.Println("📦 done importing notes. Please restart relay")
		return
	}

	go subscribeAll()
	go purgeData(purgeMonths)

	go func() {
		refreshViralNotes(ctx)                // Immediate refresh when the application starts
		go refreshViralNotesPeriodically(ctx) // Start the periodic refresh
	}()

	relay := khatru.NewRelay()
	relay.Info.Description = os.Getenv("RELAY_DESCRIPTION")
	relay.Info.Name = os.Getenv("RELAY_NAME")
	relay.Info.PubKey = os.Getenv("RELAY_PUBKEY")
	relay.Info.Software = "https://github.com/bitvora/algo-relay"
	relay.Info.Version = "0.1.1"
	relay.Info.Icon = os.Getenv("RELAY_ICON")

	relay.RejectConnection = append(relay.RejectConnection,
		policies.ConnectionRateLimiter(
			3,
			time.Minute*1,
			3,
		),
	)

	relay.RejectFilter = append(relay.RejectFilter,
		policies.FilterIPRateLimiter(
			3,
			time.Second*10,
			3,
		),
	)

	relay.OnConnect = append(relay.OnConnect, func(ctx context.Context) {
		khatru.RequestAuth(ctx)
	})
	relay.RejectFilter = append(relay.RejectFilter, func(ctx context.Context, filter nostr.Filter) (bool, string) {
		authenticatedUser := khatru.GetAuthed(ctx)
		if authenticatedUser == "" {
			return true, "auth-required: this query requires you to be authenticated"
		}

		if len(filter.Authors) > 0 {
			return true, "this relay is only for algorithmic feeds"
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

			events, err := GetUserFeed(ctx, authenticatedUser, limit, copyFilter.Kinds[0])
			fmt.Println("getting events of kind:", copyFilter.Kinds[0])
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

	log.Println("🚀 Relay started on port 3334")
	err = http.ListenAndServe(":3334", relay)
	if err != nil {
		log.Fatal(err)
	}

}

func subscribeAll() {
	now := nostr.Now()
	filters := nostr.Filters{{
		Kinds: []int{
			nostr.KindTextNote,
			nostr.KindReaction,
			nostr.KindZap,
			nostr.KindFollowList,
			nostr.KindArticle,
			20, // KindImage
		},
		Since: &now,
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

func purgeData(months int) {
	ticker := time.NewTicker(24 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			repository := NewNostrRepository(db)

			log.Println("Starting data purge...")

			if err := repository.PurgeCommentsOlderThan(months); err != nil {
				log.Printf("Error purging comments: %v\n", err)
			}
			if err := repository.PurgeNotesOlderThan(months); err != nil {
				log.Printf("Error purging posts: %v\n", err)
			}
			if err := repository.PurgeReactionsOlderThan(months); err != nil {
				log.Printf("Error purging reactions: %v\n", err)
			}
			if err := repository.PurgeZapsOlderThan(months); err != nil {
				log.Printf("Error purging zaps: %v\n", err)
			}

			log.Println("Data purge completed.")
		}
	}
}
