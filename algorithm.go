package main

import (
	"context"
	"math"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

var (
	weightInteractionsWithAuthor = getWeightFloat64("WEIGHT_INTERACTIONS_WITH_AUTHOR")
	weightCommentsGlobal         = getWeightFloat64("WEIGHT_COMMENTS_GLOBAL")
	weightReactionsGlobal        = getWeightFloat64("WEIGHT_REACTIONS_GLOBAL")
	weightZapsGlobal             = getWeightFloat64("WEIGHT_ZAPS_GLOBAL")
	weightRecency                = getWeightFloat64("WEIGHT_RECENCY")
	viralThreshold               = getWeightFloat64("VIRAL_THRESHOLD")
	viralPostDampening           = getWeightFloat64("VIRAL_POST_DAMPENING")
	decayRate                    = getWeightFloat64("DECAY_RATE")
)

func (r *NostrRepository) GetUserFeed(ctx context.Context, userID string, limit int) ([]nostr.Event, error) {
	authorFeed, err := r.GetUserFeedByAuthors(ctx, userID, limit/2)
	if err != nil {
		return nil, err
	}

	viralFeed, err := r.GetViralPosts(ctx, limit/2)
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
	if weight == "" {
		return 1
	}

	w, err := strconv.ParseFloat(weight, 64)
	if err != nil {
		return 1
	}

	return w
}
