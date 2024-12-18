CREATE TABLE IF NOT EXISTS posts (
    id TEXT PRIMARY KEY,
    author_id TEXT,
    content TEXT,
    raw_json JSONB, 
    created_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS reactions (
    id TEXT PRIMARY KEY,
    post_id TEXT REFERENCES posts(id),
    reactor_id TEXT,
    created_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS zaps (
    id TEXT PRIMARY KEY,
    post_id TEXT REFERENCES posts(id),
    zapper_id TEXT,
    amount BIGINT,
    created_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS comments (
    id TEXT PRIMARY KEY,
    post_id TEXT REFERENCES posts(id),
    commenter_id TEXT,
    content TEXT,
    created_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS follows (
    pubkey TEXT,
    follow_id TEXT
);

CREATE INDEX IF NOT EXISTS idx_posts_author_id ON posts(author_id);
CREATE INDEX IF NOT EXISTS idx_posts_created_at ON posts(created_at);

CREATE INDEX IF NOT EXISTS idx_reactions_post_id ON reactions(post_id);
CREATE INDEX IF NOT EXISTS idx_reactions_reactor_id ON reactions(reactor_id);
CREATE INDEX IF NOT EXISTS idx_reactions_created_at ON reactions(created_at);

CREATE INDEX IF NOT EXISTS idx_zaps_post_id ON zaps(post_id);
CREATE INDEX IF NOT EXISTS idx_zaps_zapper_id ON zaps(zapper_id);
CREATE INDEX IF NOT EXISTS idx_zaps_amount ON zaps(amount);
CREATE INDEX IF NOT EXISTS idx_zaps_created_at ON zaps(created_at);

CREATE INDEX IF NOT EXISTS idx_comments_post_id ON comments(post_id);
CREATE INDEX IF NOT EXISTS idx_comments_commenter_id ON comments(commenter_id);
CREATE INDEX IF NOT EXISTS idx_comments_created_at ON comments(created_at);

CREATE INDEX IF NOT EXISTS idx_follows_pubkey ON follows(pubkey);
CREATE INDEX IF NOT EXISTS idx_follows_follow_id ON follows(follow_id);
