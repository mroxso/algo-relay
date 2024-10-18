package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/lib/pq"
	"github.com/nbd-wtf/go-nostr"
)

type NostrRepository struct {
	db *sql.DB
}

type FeedPost struct {
	Event nostr.Event
	Score float64
}

type EventWithMeta struct {
	Event                nostr.Event
	GlobalCommentsCount  int
	GlobalReactionsCount int
	GlobalZapsCount      int
	CreatedAt            time.Time
}

func NewNostrRepository(db *sql.DB) *NostrRepository {
	return &NostrRepository{db: db}
}

func (r *NostrRepository) SaveNostrEvent(event *nostr.Event) error {
	switch event.Kind {
	case 1:
		return r.savePostOrComment(event)
	case 7:
		return r.saveReaction(event)
	case 9735:
		return r.saveZap(event)
	default:
		return fmt.Errorf("unhandled event kind: %d", event.Kind)
	}
}

func (r *NostrRepository) savePostOrComment(event *nostr.Event) error {
	rootID := getRootNoteID(event)
	if rootID == "" {
		return r.savePost(event)
	}
	return r.saveComment(event, rootID)
}

func (r *NostrRepository) savePost(event *nostr.Event) error {
	query := `
        INSERT INTO posts (id, author_id, content, raw_json, created_at)
        VALUES ($1, $2, $3, $4, to_timestamp($5))
        ON CONFLICT (id) DO NOTHING;
    `
	_, err := r.db.ExecContext(context.Background(), query,
		event.ID, event.PubKey, event.Content, event.String(), event.CreatedAt)
	return err
}

func (r *NostrRepository) saveComment(event *nostr.Event, rootID string) error {
	query := `
        INSERT INTO comments (id, post_id, commenter_id, created_at)
        VALUES ($1, $2, $3, to_timestamp($4))
        ON CONFLICT (id) DO NOTHING;
    `
	_, err := r.db.ExecContext(context.Background(), query,
		event.ID, rootID, event.PubKey, event.CreatedAt)
	return err
}

func getRootNoteID(event *nostr.Event) string {
	var rootID string
	for _, tag := range event.Tags {
		if len(tag) > 0 && tag[0] == "e" {
			if len(tag) >= 3 && (tag[2] == "root" || tag[2] == "") {
				rootID = tag[1]
				break
			}
		}
	}
	return rootID
}

func (r *NostrRepository) saveReaction(event *nostr.Event) error {
	postID, err := getTaggedPostID(event)
	if err != nil {
		return err
	}
	query := `
        INSERT INTO reactions (id, post_id, reactor_id, created_at)
        VALUES ($1, $2, $3, to_timestamp($4))
        ON CONFLICT (id) DO NOTHING;
    `
	_, err = r.db.ExecContext(context.Background(), query,
		event.ID, postID, event.PubKey, event.CreatedAt)
	return err
}

func (r *NostrRepository) saveZap(event *nostr.Event) error {
	postID, err := getTaggedPostID(event)
	if err != nil {
		return err
	}
	amount, err := getZapAmount(event)
	if err != nil {
		return err
	}
	zapperID, err := getZapperID(event)
	if err != nil {
		return err
	}
	query := `
        INSERT INTO zaps (id, post_id, zapper_id, amount, created_at)
        VALUES ($1, $2, $3, $4, to_timestamp($5))
        ON CONFLICT (id) DO NOTHING;
    `
	_, err = r.db.ExecContext(context.Background(), query,
		event.ID, postID, zapperID, amount, event.CreatedAt)
	return err
}

func getZapperID(event *nostr.Event) (string, error) {
	for _, tag := range event.Tags {
		if len(tag) > 0 && tag[0] == "description" {
			// Parse the description JSON
			var descriptionData struct {
				PubKey string `json:"pubkey"`
			}
			err := json.Unmarshal([]byte(tag[1]), &descriptionData)
			if err != nil {
				return "", fmt.Errorf("error parsing description tag: %v", err)
			}
			return descriptionData.PubKey, nil
		}
	}
	return "", fmt.Errorf("no zapper pubkey found in description tag")
}

func getTaggedPostID(event *nostr.Event) (string, error) {
	for _, tag := range event.Tags {
		if len(tag) > 0 && tag[0] == "e" {
			return tag[1], nil
		}
	}
	return "", fmt.Errorf("no post ID found in event tags")
}

func getZapAmount(event *nostr.Event) (int64, error) {
	for _, tag := range event.Tags {
		if len(tag) > 0 && tag[0] == "bolt11" {
			return decodeBolt11Invoice(tag[1])
		}
	}
	return 0, fmt.Errorf("no zap amount found in event tags")
}

func decodeBolt11Invoice(bolt11 string) (int64, error) {
	millisat, err := hrpToMillisat(bolt11)
	if err != nil {
		return 0, err
	}
	satsInt64 := millisat.Int64() / 1000
	return satsInt64, nil
}

func (r *NostrRepository) fetchTopInteractedAuthors(userID string, limit int) ([]string, error) {
	query := `
		SELECT author_id, COUNT(*) AS interaction_count
		FROM posts p
		LEFT JOIN zaps z ON p.id = z.post_id
		LEFT JOIN reactions r ON p.id = r.post_id
		LEFT JOIN comments c ON p.id = c.post_id
		WHERE z.zapper_id = $1 OR r.reactor_id = $1 OR c.commenter_id = $1
		GROUP BY author_id
		ORDER BY interaction_count DESC
		LIMIT $2
	`
	rows, err := r.db.QueryContext(context.Background(), query, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var authors []string
	for rows.Next() {
		var authorID string
		var interactionCount int
		if err := rows.Scan(&authorID, &interactionCount); err != nil {
			return nil, err
		}
		authors = append(authors, authorID)
	}
	return authors, nil
}

func (r *NostrRepository) GetViralPosts(ctx context.Context, limit int) ([]FeedPost, error) {
	query := `
    SELECT p.raw_json, COUNT(c.id) AS comment_count, COUNT(r.id) AS reaction_count, COUNT(z.id) AS zap_count
    FROM posts p
    LEFT JOIN comments c ON p.id = c.post_id
    LEFT JOIN reactions r ON p.id = r.post_id
    LEFT JOIN zaps z ON p.id = z.post_id
    GROUP BY p.id
    HAVING COUNT(c.id) + COUNT(r.id) + COUNT(z.id) >= $1
    ORDER BY COUNT(c.id) + COUNT(r.id) + COUNT(z.id) DESC
    LIMIT $2;
`

	rows, err := r.db.QueryContext(context.Background(), query, viralThreshold, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var viralPosts []FeedPost
	for rows.Next() {
		var rawJSON string
		var commentCount, reactionCount, zapCount int

		if err := rows.Scan(&rawJSON, &commentCount, &reactionCount, &zapCount); err != nil {
			return nil, err
		}

		var event nostr.Event
		if err := json.Unmarshal([]byte(rawJSON), &event); err != nil {
			log.Printf("Failed to unmarshal raw JSON: %v", err)
			continue
		}

		recencyFactor := calculateRecencyFactor(event.CreatedAt.Time())
		score := (float64(commentCount)*weightCommentsGlobal +
			float64(reactionCount)*weightReactionsGlobal +
			float64(zapCount)*weightZapsGlobal +
			recencyFactor*weightRecency) * viralPostDampening

		viralPosts = append(viralPosts, FeedPost{
			Event: event,
			Score: score,
		})
	}

	return viralPosts, nil
}

func (r *NostrRepository) fetchPostsFromAuthors(authors []string) ([]EventWithMeta, error) {
	query := `
		SELECT p.raw_json, COUNT(c.id) AS comment_count, COUNT(r.id) AS reaction_count, COUNT(z.id) AS zap_count
		FROM posts p
		LEFT JOIN comments c ON p.id = c.post_id
		LEFT JOIN reactions r ON p.id = r.post_id
		LEFT JOIN zaps z ON p.id = z.post_id
		WHERE p.author_id = ANY($1)
		GROUP BY p.id
		ORDER BY p.created_at DESC
	`

	rows, err := r.db.QueryContext(context.Background(), query, pq.Array(authors))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []EventWithMeta
	for rows.Next() {
		var rawJSON string
		var commentCount, reactionCount, zapCount int

		if err := rows.Scan(&rawJSON, &commentCount, &reactionCount, &zapCount); err != nil {
			return nil, err
		}

		var event nostr.Event
		if err := json.Unmarshal([]byte(rawJSON), &event); err != nil {
			log.Printf("Failed to unmarshal raw JSON: %v", err)
			continue
		}

		posts = append(posts, EventWithMeta{
			Event:                event,
			GlobalCommentsCount:  commentCount,
			GlobalReactionsCount: reactionCount,
			GlobalZapsCount:      zapCount,
			CreatedAt:            event.CreatedAt.Time(),
		})
	}

	return posts, nil
}
