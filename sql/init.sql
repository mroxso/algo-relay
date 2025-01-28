CREATE TABLE IF NOT EXISTS notes (
    id TEXT PRIMARY KEY,
    author_id TEXT,
    kind INTEGER,
    content TEXT,
    raw_json JSONB, 
    created_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS reactions (
    id TEXT PRIMARY KEY,
    note_id TEXT REFERENCES notes(id),
    reactor_id TEXT,
    created_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS zaps (
    id TEXT PRIMARY KEY,
    note_id TEXT REFERENCES notes(id),
    zapper_id TEXT,
    amount BIGINT,
    created_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS comments (
    id TEXT PRIMARY KEY,
    note_id TEXT REFERENCES notes(id),
    commenter_id TEXT,
    content TEXT,
    created_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS follows (
    pubkey TEXT,
    follow_id TEXT
);

CREATE INDEX IF NOT EXISTS idx_notes_author_id ON notes(author_id);
CREATE INDEX IF NOT EXISTS idx_notes_kind ON notes(kind);
CREATE INDEX IF NOT EXISTS idx_notes_created_at ON notes(created_at);

CREATE INDEX IF NOT EXISTS idx_reactions_note_id ON reactions(note_id);
CREATE INDEX IF NOT EXISTS idx_reactions_reactor_id ON reactions(reactor_id);
CREATE INDEX IF NOT EXISTS idx_reactions_created_at ON reactions(created_at);

CREATE INDEX IF NOT EXISTS idx_zaps_note_id ON zaps(note_id);
CREATE INDEX IF NOT EXISTS idx_zaps_zapper_id ON zaps(zapper_id);
CREATE INDEX IF NOT EXISTS idx_zaps_amount ON zaps(amount);
CREATE INDEX IF NOT EXISTS idx_zaps_created_at ON zaps(created_at);

CREATE INDEX IF NOT EXISTS idx_comments_note_id ON comments(note_id);
CREATE INDEX IF NOT EXISTS idx_comments_commenter_id ON comments(commenter_id);
CREATE INDEX IF NOT EXISTS idx_comments_created_at ON comments(created_at);

CREATE INDEX IF NOT EXISTS idx_follows_pubkey ON follows(pubkey);
CREATE INDEX IF NOT EXISTS idx_follows_follow_id ON follows(follow_id);
