package main

import (
	"context"
	"fmt"
	"log"
	"math"
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

	if cached, ok := getCachedUserFeeds(userID, kind); ok && now.Sub(cached.Timestamp) < feedCacheDuration {
		log.Println("Returning cached feed for user:", userID, "kind:", kind)
		return serveSequentialFeedResult(userID, kind, cached, limit), nil
	}

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

	pending := make(chan struct{})
	pendingRequests[cacheKey] = pending
	pendingRequestsMutex.Unlock()

	defer func() {
		pendingRequestsMutex.Lock()
		close(pending)
		delete(pendingRequests, cacheKey)
		pendingRequestsMutex.Unlock()
	}()

	log.Println("No cache or pending request found, generating feed variants for user:", userID, "kind:", kind)
	authorFeed, err := repository.GetUserFeedByAuthors(ctx, userID, variantFeedSize*numFeedVariants, kind)
	if err != nil {
		return nil, err
	}

	viralNoteCacheMutex.Lock()
	viralFeed := viralNoteCache.notes
	viralNoteCacheMutex.Unlock()

	kinds := []int{kind}
	feedVariants := generateFeedVariants(authorFeed, viralFeed, variantFeedSize, kinds)

	cachedFeeds := CachedFeeds{
		Feeds:           feedVariants,
		Timestamp:       now,
		LastServedIndex: -1,
	}
	storeCachedUserFeeds(userID, kind, cachedFeeds)

	return serveSequentialFeedResult(userID, kind, cachedFeeds, limit), nil
}

func getCachedUserFeeds(userID string, kind int) (CachedFeeds, bool) {
	cacheKey := getCacheKey(userID, kind)
	if cached, ok := userFeedCache.Load(cacheKey); ok {
		return cached.(CachedFeeds), true
	}
	return CachedFeeds{}, false
}

func storeCachedUserFeeds(userID string, kind int, cachedFeeds CachedFeeds) {
	cacheKey := getCacheKey(userID, kind)
	userFeedCache.Store(cacheKey, cachedFeeds)
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

	// Update LastServedIndex in the cache for the given kind
	cachedFeeds.LastServedIndex = nextIndex
	storeCachedUserFeeds(userID, kind, cachedFeeds)

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

func generateFeedVariants(authorFeed, viralFeed []FeedNote, variantSize int, kinds []int) map[int][][]FeedNote {
	// Create a map to store feed variants for each kind
	feedVariantsByKind := make(map[int][][]FeedNote)

	for _, kind := range kinds {
		// Filter author feed and viral feed by kind
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

		// Generate multiple feed variants with one note per author in each
		var feedVariants [][]FeedNote
		for i := 0; i < numFeedVariants; i++ {
			var feed []FeedNote
			usedAuthors := make(map[string]bool)

			// Add author notes for the variant
			for _, notes := range authorNotes {
				if len(notes) > i {
					// Use a different note from this author in each variant, if available
					feed = append(feed, notes[i])
				} else {
					// If not enough unique notes, wrap around to use existing ones
					feed = append(feed, notes[i%len(notes)])
				}
				usedAuthors[notes[0].Event.PubKey] = true
			}

			// Add viral notes, ensuring no duplicate authors
			for _, viralNote := range filteredViralFeed {
				if len(feed) >= variantSize {
					break
				}
				authorID := viralNote.Event.PubKey
				if !usedAuthors[authorID] {
					feed = append(feed, viralNote)
					usedAuthors[authorID] = true
				}
			}

			// Sort each feed by score in descending order and truncate to variant size
			sort.Slice(feed, func(i, j int) bool {
				return feed[i].Score > feed[j].Score
			})
			if len(feed) > variantSize {
				feed = feed[:variantSize]
			}
			feedVariants = append(feedVariants, feed)
		}

		// Store feed variants for the current kind
		feedVariantsByKind[kind] = feedVariants
	}

	log.Printf("Generated feed variants for kinds %v, each with %d notes per variant", kinds, variantSize)
	return feedVariantsByKind
}
func (r *NostrRepository) GetUserFeedByAuthors(ctx context.Context, userID string, limit, kind int) ([]FeedNote, error) {
	authorInteractions, err := r.fetchTopInteractedAuthors(userID)
	if err != nil {
		return nil, err
	}

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
