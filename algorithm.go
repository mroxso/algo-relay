package main

import (
	"context"
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
	viralPostDampening           float64
	decayRate                    float64
)

type CachedFeed struct {
	Feed      []FeedPost
	Timestamp time.Time
}

var userFeedCache sync.Map

const feedCacheDuration = 5 * time.Minute

func GetUserFeed(ctx context.Context, userID string, limit int) ([]nostr.Event, error) {
	now := time.Now()

	if cached, ok := getCachedUserFeed(userID); ok && now.Sub(cached.Timestamp) < feedCacheDuration {
		log.Println("Returning cached feed for user:", userID)
		return createFeedResult(cached.Feed, limit), nil
	}

	authorFeed, err := repository.GetUserFeedByAuthors(ctx, userID, limit/2)
	if err != nil {
		return nil, err
	}

	log.Println("found", len(authorFeed), "posts from authors")

	viralFeed, err := repository.GetViralPosts(ctx, limit/2)
	if err != nil {
		return nil, err
	}

	combinedFeed := append(authorFeed, viralFeed...)

	filteredFeed := make([]FeedPost, 0, len(combinedFeed))
	for _, feedPost := range combinedFeed {
		if feedPost.Event.PubKey != userID {
			filteredFeed = append(filteredFeed, feedPost)
		}
	}

	sort.Slice(filteredFeed, func(i, j int) bool {
		return filteredFeed[i].Score > filteredFeed[j].Score
	})

	userFeedCache.Store(userID, CachedFeed{
		Feed:      filteredFeed,
		Timestamp: now,
	})

	return createFeedResult(filteredFeed, limit), nil
}

func getCachedUserFeed(userID string) (CachedFeed, bool) {
	if cached, ok := userFeedCache.Load(userID); ok {
		return cached.(CachedFeed), true
	}
	return CachedFeed{}, false
}

func createFeedResult(filteredFeed []FeedPost, limit int) []nostr.Event {
	var result []nostr.Event
	for i, feedPost := range filteredFeed {
		if i >= limit {
			break
		}
		result = append(result, feedPost.Event)
	}
	return result
}

func (r *NostrRepository) GetUserFeedByAuthors(ctx context.Context, userID string, limit int) ([]FeedPost, error) {
	authors, err := r.fetchTopInteractedAuthors(userID, limit)
	if err != nil {
		return nil, err
	}

	posts, err := r.fetchPostsFromAuthors(authors)
	if err != nil {
		return nil, err
	}

	var feedPosts []FeedPost
	for _, post := range posts {
		score := r.calculateAuthorPostScore(post)
		feedPosts = append(feedPosts, FeedPost{Event: post.Event, Score: score})
	}

	sort.Slice(feedPosts, func(i, j int) bool {
		return feedPosts[i].Score > feedPosts[j].Score
	})

	return feedPosts, nil
}

func (r *NostrRepository) calculateAuthorPostScore(event EventWithMeta) float64 {
	recencyFactor := calculateRecencyFactor(event.CreatedAt)

	score := float64(event.GlobalCommentsCount)*weightCommentsGlobal +
		float64(event.GlobalReactionsCount)*weightReactionsGlobal +
		float64(event.GlobalZapsCount)*weightZapsGlobal +
		recencyFactor*weightRecency +
		float64(weightInteractionsWithAuthor)*weightInteractionsWithAuthor

	return score
}

func calculateRecencyFactor(createdAt time.Time) float64 {
	hoursSinceCreation := time.Since(createdAt).Hours()
	return math.Exp(-decayRate * hoursSinceCreation)
}

func getWeightFloat64(envKey string) float64 {
	weight := os.Getenv(envKey)

	// Log the environment key and value for debugging purposes
	log.Printf("Fetching environment variable for %s: %s", envKey, weight)

	// Trim any extra spaces from the environment variable
	weight = strings.TrimSpace(weight)

	if weight == "" {
		log.Printf("Environment variable %s not set, defaulting to 1", envKey)
		return 1
	}

	// Parse the float value
	w, err := strconv.ParseFloat(weight, 64)
	if err != nil {
		// Log the error and return default
		log.Printf("Error parsing float for %s: %v, defaulting to 1", envKey, err)
		return 1
	}

	return w
}
