package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand/v2"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

var (
	weightInteractionsWithAuthor float64
	weightCommentsGlobal         float64
	weightReactionsGlobal        float64
	weightZapsGlobal             float64
	weightRecency                float64
	viralThreshold               float64
	viralNoteDampening           float64
	decayRate                    float64
)

type CachedFeeds struct {
	Feeds           map[int][][]FeedNote // Multiple feed variants
	Timestamp       time.Time
	LastServedIndex int // Index of the last served feed variant
}

var userFeedCache sync.Map

const feedCacheDuration = 5 * time.Minute
const numFeedVariants = 5   // Number of different feed variants to generate
const variantFeedSize = 100 // Each variant feed size (fixed to 100 notes)

var pendingRequests = make(map[string]chan struct{})
var pendingRequestsMutex sync.Mutex

func getCacheKey(userID string, kind int) string {
	return fmt.Sprintf("%s_kind_%d", userID, kind)
}

func GetUserFeed(ctx context.Context, userID string, limit, kind int) ([]nostr.Event, error) {
	now := time.Now()

	// Check cache first
	if cached, ok := getCachedUserFeeds(userID, kind); ok && now.Sub(cached.Timestamp) < feedCacheDuration {
		log.Println("Returning cached feed for user:", userID, "kind:", kind)
		return serveSequentialFeedResult(userID, kind, cached, limit), nil
	}

	// Ensure no duplicate feed generation for the same user/kind
	pendingRequestsMutex.Lock()
	cacheKey := getCacheKey(userID, kind)
	if pending, exists := pendingRequests[cacheKey]; exists {
		log.Println("Waiting for existing feed generation for user:", userID, "kind:", kind)
		pendingRequestsMutex.Unlock()
		<-pending
		if cached, ok := getCachedUserFeeds(userID, kind); ok && now.Sub(cached.Timestamp) < feedCacheDuration {
			return serveSequentialFeedResult(userID, kind, cached, limit), nil
		}
		return nil, fmt.Errorf("feed generation failed after waiting for cache")
	}

	// Mark feed generation as in progress
	pending := make(chan struct{})
	pendingRequests[cacheKey] = pending
	pendingRequestsMutex.Unlock()

	defer func() {
		pendingRequestsMutex.Lock()
		close(pending)
		delete(pendingRequests, cacheKey)
		pendingRequestsMutex.Unlock()
	}()

	// Generate the feed
	log.Println("No cache or pending request found, generating feed variants for user:", userID, "kind:", kind)
	authorFeed, err := repository.GetUserFeedByAuthors(ctx, userID, variantFeedSize*numFeedVariants, kind)
	if err != nil {
		return nil, err
	}

	// Fetch viral notes
	viralNoteCacheMutex.Lock()
	viralFeed := viralNoteCache.notes
	viralNoteCacheMutex.Unlock()

	// Generate feed variants
	feedVariants := generateFeedVariants(authorFeed, viralFeed, variantFeedSize, kind)

	// Retrieve existing cached feeds or create a new one
	cachedFeeds, _ := getCachedUserFeeds(userID, kind)
	if cachedFeeds.Feeds == nil {
		cachedFeeds.Feeds = make(map[int][][]FeedNote)
	}

	// Update cached feeds for this kind
	cachedFeeds.Feeds[kind] = feedVariants
	cachedFeeds.Timestamp = now
	cachedFeeds.LastServedIndex = -1

	// Store the updated cached feeds
	storeCachedUserFeeds(userID, kind, feedVariants, &cachedFeeds)

	// Serve the sequential feed result
	return serveSequentialFeedResult(userID, kind, cachedFeeds, limit), nil
}

func getCachedUserFeeds(userID string, kind int) (CachedFeeds, bool) {
	cacheKey := getCacheKey(userID, kind)
	if cached, ok := userFeedCache.Load(cacheKey); ok {
		return cached.(CachedFeeds), true
	}
	return CachedFeeds{}, false
}

func storeCachedUserFeeds(userID string, kind int, feedVariants [][]FeedNote, cachedFeeds *CachedFeeds) {
	if cachedFeeds.Feeds == nil {
		cachedFeeds.Feeds = make(map[int][][]FeedNote)
	}
	cachedFeeds.Feeds[kind] = feedVariants
	cachedFeeds.Timestamp = time.Now()
	log.Printf("Cached feed variants for kind %d for user: %s", kind, userID)
	userFeedCache.Store(userID, cachedFeeds)
}

func serveSequentialFeedResult(userID string, kind int, cachedFeeds CachedFeeds, limit int) []nostr.Event {
	// Check if there are feed variants for the given kind
	feedVariants, ok := cachedFeeds.Feeds[kind]
	if !ok || len(feedVariants) == 0 {
		log.Printf("No feed variants available for user: %s, kind: %d", userID, kind)
		return nil
	}

	// Determine the next feed variant to serve
	nextIndex := (cachedFeeds.LastServedIndex + 1) % len(feedVariants)
	selectedFeed := feedVariants[nextIndex]

	// Update LastServedIndex for the given kind
	cachedFeeds.LastServedIndex = nextIndex
	storeCachedUserFeeds(userID, kind, feedVariants, &cachedFeeds)

	// Convert the selected feed to nostr.Event results, applying the limit
	var result []nostr.Event
	for i, feedNote := range selectedFeed {
		if i >= limit {
			break
		}
		result = append(result, feedNote.Event) // Ensure FeedNote has an Event field
	}

	log.Printf("Serving feed variant %d with %d notes (limit %d, kind %d) for user: %s", nextIndex, len(result), limit, kind, userID)
	return result
}

func shuffleNotes(notes []FeedNote) {
	rand.Shuffle(len(notes), func(i, j int) {
		notes[i], notes[j] = notes[j], notes[i]
	})
}

func generateFeedVariants(authorFeed, viralFeed []FeedNote, variantSize int, kind int) [][]FeedNote {
	// Filter author feed and viral feed by the specified kind
	var filteredAuthorFeed []FeedNote
	var filteredViralFeed []FeedNote

	for _, note := range authorFeed {
		if note.Event.Kind == kind {
			filteredAuthorFeed = append(filteredAuthorFeed, note)
		}
	}

	for _, note := range viralFeed {
		if note.Event.Kind == kind {
			filteredViralFeed = append(filteredViralFeed, note)
		}
	}

	// Group notes by author
	authorNotes := make(map[string][]FeedNote)
	for _, note := range filteredAuthorFeed {
		authorID := note.Event.PubKey
		authorNotes[authorID] = append(authorNotes[authorID], note)
	}

	// Initialize feed variants
	feedVariants := make([][]FeedNote, numFeedVariants)
	for i := range feedVariants {
		feedVariants[i] = []FeedNote{}
	}

	// Distribute one note per author across all variants
	for _, notes := range authorNotes {
		for i := 0; i < len(notes) && i < numFeedVariants; i++ {
			if len(feedVariants[i]) < variantSize {
				feedVariants[i] = append(feedVariants[i], notes[i])
			}
		}
	}

	// Add viral notes to variants while ensuring no duplicate authors
	usedAuthors := make(map[string]bool)
	for i := 0; i < numFeedVariants; i++ {
		for _, viralNote := range filteredViralFeed {
			if len(feedVariants[i]) >= variantSize {
				break
			}
			authorID := viralNote.Event.PubKey
			if !usedAuthors[authorID] {
				feedVariants[i] = append(feedVariants[i], viralNote)
				usedAuthors[authorID] = true
			}
		}
	}

	// Sort each feed by score in descending order and truncate to variant size
	for i := range feedVariants {
		sort.Slice(feedVariants[i], func(a, b int) bool {
			return feedVariants[i][a].Score > feedVariants[i][b].Score
		})
		if len(feedVariants[i]) > variantSize {
			feedVariants[i] = feedVariants[i][:variantSize]
		}
	}

	log.Printf("Generated %d feed variants for kind %d, each with up to %d notes", numFeedVariants, kind, variantSize)
	return feedVariants
}

func (r *NostrRepository) GetUserFeedByAuthors(ctx context.Context, userID string, limit, kind int) ([]FeedNote, error) {
	authorInteractions, err := r.fetchTopInteractedAuthors(userID)
	if err != nil {
		return nil, err
	}

	fmt.Println("Fetched top interacted authors:", len(authorInteractions))

	notes, err := r.fetchNotesFromAuthors(authorInteractions, kind)
	if err != nil {
		return nil, err
	}

	fmt.Println("Fetched notes from authors:", len(notes))
	var FeedNotes []FeedNote
	for _, note := range notes {
		interactionCount := getInteractionCountForAuthor(note.Event.PubKey, authorInteractions)
		score := r.calculateAuthorNoteScore(note, interactionCount)
		FeedNotes = append(FeedNotes, FeedNote{Event: note.Event, Score: score})
	}

	// Sort all posts by score in descending order initially
	sort.Slice(FeedNotes, func(i, j int) bool {
		return FeedNotes[i].Score > FeedNotes[j].Score
	})

	return FeedNotes, nil
}

func getInteractionCountForAuthor(authorID string, interactions []AuthorInteraction) int {
	for _, interaction := range interactions {
		if interaction.AuthorID == authorID {
			return interaction.InteractionCount
		}
	}
	return 0
}

func (r *NostrRepository) calculateAuthorNoteScore(event EventWithMeta, interactionCount int) float64 {
	recencyFactor := calculateRecencyFactor(event.CreatedAt)

	score := float64(event.GlobalCommentsCount)*weightCommentsGlobal +
		float64(event.GlobalReactionsCount)*weightReactionsGlobal +
		float64(event.GlobalZapsCount)*weightZapsGlobal +
		recencyFactor*weightRecency +
		float64(interactionCount)*weightInteractionsWithAuthor

	return score
}

func calculateRecencyFactor(createdAt time.Time) float64 {
	hoursSinceCreation := time.Since(createdAt).Hours()
	scalingFactor := 100.0
	recencyFactor := math.Exp(-decayRate*hoursSinceCreation) * scalingFactor

	if recencyFactor > 1.0 {
		recencyFactor = 1.0
	} else if recencyFactor < 0.001 {
		recencyFactor = 0.001
	}

	return recencyFactor
}

func getWeightFloat64(envKey string) float64 {
	weight := os.Getenv(envKey)
	log.Printf("Fetching environment variable for %s: %s", envKey, weight)
	weight = strings.TrimSpace(weight)

	if weight == "" {
		log.Printf("Environment variable %s not set, defaulting to 1", envKey)
		return 1
	}

	w, err := strconv.ParseFloat(weight, 64)
	if err != nil {
		log.Printf("Error parsing float for %s: %v, defaulting to 1", envKey, err)
		return 1

	}

	return w
}
