package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
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
	viralPostDampening           float64
	decayRate                    float64
)

type CachedFeeds struct {
	Feeds     [][]FeedPost // Multiple feed variants
	Timestamp time.Time
}

var userFeedCache sync.Map

const feedCacheDuration = 5 * time.Minute
const numFeedVariants = 5   // Number of different feed variants to generate
const variantFeedSize = 100 // Each variant feed size (fixed to 100 posts)

var pendingRequests = make(map[string]chan struct{})
var pendingRequestsMutex sync.Mutex

func GetUserFeed(ctx context.Context, userID string, limit int) ([]nostr.Event, error) {
	now := time.Now()

	// Check if the feed variants are already cached
	if cached, ok := getCachedUserFeeds(userID); ok && now.Sub(cached.Timestamp) < feedCacheDuration {
		log.Println("Returning cached feed for user:", userID)
		return createRandomFeedResult(cached.Feeds, limit), nil
	}

	// Check if feed generation is already pending
	pendingRequestsMutex.Lock()
	if pending, exists := pendingRequests[userID]; exists {
		log.Println("Waiting for existing feed generation for user:", userID)
		pendingRequestsMutex.Unlock()
		<-pending
		if cached, ok := getCachedUserFeeds(userID); ok && now.Sub(cached.Timestamp) < feedCacheDuration {
			return createRandomFeedResult(cached.Feeds, limit), nil
		}
		return nil, fmt.Errorf("feed generation failed after waiting for cache")
	}

	// Set up a new pending request for this user
	pending := make(chan struct{})
	pendingRequests[userID] = pending
	pendingRequestsMutex.Unlock()

	defer func() {
		pendingRequestsMutex.Lock()
		close(pending)
		delete(pendingRequests, userID)
		pendingRequestsMutex.Unlock()
	}()

	// Generate the feed variants
	log.Println("No cache or pending request found, generating feed variants for user:", userID)
	authorFeed, err := repository.GetUserFeedByAuthors(ctx, userID, variantFeedSize*numFeedVariants)
	if err != nil {
		return nil, err
	}

	// Generate multiple feed variants with a fixed size of 100 posts each
	feedVariants := generateFeedVariants(authorFeed, variantFeedSize)

	// Cache the generated feed variants
	userFeedCache.Store(userID, CachedFeeds{
		Feeds:     feedVariants,
		Timestamp: now,
	})

	return createRandomFeedResult(feedVariants, limit), nil
}

func getCachedUserFeeds(userID string) (CachedFeeds, bool) {
	if cached, ok := userFeedCache.Load(userID); ok {
		return cached.(CachedFeeds), true
	}
	return CachedFeeds{}, false
}

func createRandomFeedResult(feedVariants [][]FeedPost, limit int) []nostr.Event {
	// Select a random feed variant to serve
	randomIndex := rand.Intn(len(feedVariants))
	selectedFeed := feedVariants[randomIndex]

	// Convert the selected feed to nostr.Event results, applying the limit
	var result []nostr.Event
	for i, feedPost := range selectedFeed {
		if i >= limit {
			break
		}
		result = append(result, feedPost.Event)
	}
	log.Printf("Serving feed variant %d with %d posts (limit %d)", randomIndex, len(result), limit)
	return result
}

func generateFeedVariants(authorFeed []FeedPost, variantSize int) [][]FeedPost {
	// Group posts by author
	authorPosts := make(map[string][]FeedPost)
	for _, post := range authorFeed {
		authorID := post.Event.PubKey
		authorPosts[authorID] = append(authorPosts[authorID], post)
	}

	// Generate multiple feed variants with one post per author in each
	var feedVariants [][]FeedPost
	for i := 0; i < numFeedVariants; i++ {
		var feed []FeedPost
		for _, posts := range authorPosts {
			if len(posts) > i {
				// Use a different post from this author in each variant, if available
				feed = append(feed, posts[i])
			} else {
				// If not enough unique posts, wrap around to use existing ones
				feed = append(feed, posts[i%len(posts)])
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
	log.Printf("Generated %d feed variants for user, each with %d posts", numFeedVariants, variantSize)
	return feedVariants
}

func (r *NostrRepository) GetUserFeedByAuthors(ctx context.Context, userID string, limit int) ([]FeedPost, error) {
	authorInteractions, err := r.fetchTopInteractedAuthors(userID)
	if err != nil {
		return nil, err
	}

	posts, err := r.fetchPostsFromAuthors(authorInteractions)
	if err != nil {
		return nil, err
	}

	var feedPosts []FeedPost
	for _, post := range posts {
		interactionCount := getInteractionCountForAuthor(post.Event.PubKey, authorInteractions)
		score := r.calculateAuthorPostScore(post, interactionCount)
		feedPosts = append(feedPosts, FeedPost{Event: post.Event, Score: score})
	}

	// Sort all posts by score in descending order initially
	sort.Slice(feedPosts, func(i, j int) bool {
		return feedPosts[i].Score > feedPosts[j].Score
	})

	return feedPosts, nil
}

func getInteractionCountForAuthor(authorID string, interactions []AuthorInteraction) int {
	for _, interaction := range interactions {
		if interaction.AuthorID == authorID {
			return interaction.InteractionCount
		}
	}
	return 0
}

func (r *NostrRepository) calculateAuthorPostScore(event EventWithMeta, interactionCount int) float64 {
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
