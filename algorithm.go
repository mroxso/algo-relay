package main

import (
	"context"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
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

func GetUserFeed(ctx context.Context, userID string, limit int) ([]nostr.Event, error) {
	authorFeed, err := repository.GetUserFeedByAuthors(ctx, userID, limit/2)
	if err != nil {
		return nil, err
	}

	viralFeed, err := repository.GetViralPosts(ctx, limit/2)
	if err != nil {
		return nil, err
	}

	combinedFeed := append(authorFeed, viralFeed...)

	// strip out the user's own posts
	filteredFeed := make([]FeedPost, 0, len(combinedFeed))
	for _, feedPost := range combinedFeed {
		if feedPost.Event.PubKey != userID {
			filteredFeed = append(filteredFeed, feedPost)
		}
	}

	sort.Slice(filteredFeed, func(i, j int) bool {
		return filteredFeed[i].Score > filteredFeed[j].Score
	})

	var result []nostr.Event
	for i, feedPost := range filteredFeed {
		if i >= limit {
			break
		}
		result = append(result, feedPost.Event)
	}

	return result, nil
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
