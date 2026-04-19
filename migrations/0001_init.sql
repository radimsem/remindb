CREATE TABLE IF NOT EXISTS nodes (
    id               CHAR(11) PRIMARY KEY,
    parent_id        CHAR(11) REFERENCES nodes(id) ON DELETE CASCADE,
    source_file      TEXT NOT NULL,
    node_type        VARCHAR(16) NOT NULL,
    depth            INTEGER NOT NULL,
    label            VARCHAR(120) NOT NULL,
    content          TEXT NOT NULL,
    format           VARCHAR(11) NOT NULL DEFAULT 'plain',
    token_count      INTEGER NOT NULL,
    content_hash     CHAR(16) NOT NULL,
    temperature      REAL NOT NULL DEFAULT 0.5,
    access_count     INTEGER NOT NULL DEFAULT 0,
    last_accessed_at INTEGER,
    created_at       INTEGER DEFAULT (unixepoch()),
    updated_at       INTEGER DEFAULT (unixepoch())
);

CREATE TABLE IF NOT EXISTS snapshots (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    cursor_hash CHAR(16) NOT NULL UNIQUE,
    parent_id   INTEGER REFERENCES snapshots(id),
    message     TEXT,
    created_at  INTEGER DEFAULT (unixepoch())
);

CREATE TABLE IF NOT EXISTS diffs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    snapshot_id INTEGER NOT NULL REFERENCES snapshots(id),
    node_id     CHAR(11) NOT NULL,
    op          CHAR(3) NOT NULL,
    old_hash    CHAR(16),
    new_hash    CHAR(16),
    old_content TEXT,
    new_content TEXT
);

CREATE TABLE IF NOT EXISTS cursors (
    id          VARCHAR(11) PRIMARY KEY DEFAULT 'HEAD',
    snapshot_id INTEGER NOT NULL REFERENCES snapshots(id),
    updated_at  INTEGER DEFAULT (unixepoch())
);

-- FTS5 external content table synced via triggers.
CREATE VIRTUAL TABLE IF NOT EXISTS nodes_fts USING fts5(
    label, content, node_type,
    content=nodes,
    content_rowid=rowid,
    tokenize='porter unicode61'
);

CREATE TRIGGER IF NOT EXISTS nodes_fts_insert AFTER INSERT ON nodes BEGIN
    INSERT INTO nodes_fts(rowid, label, content, node_type)
    VALUES (new.rowid, new.label, new.content, new.node_type);
END;

CREATE TRIGGER IF NOT EXISTS nodes_fts_delete AFTER DELETE ON nodes BEGIN
    INSERT INTO nodes_fts(nodes_fts, rowid, label, content, node_type)
    VALUES ('delete', old.rowid, old.label, old.content, old.node_type);
END;

CREATE TRIGGER IF NOT EXISTS nodes_fts_update AFTER UPDATE ON nodes BEGIN
    INSERT INTO nodes_fts(nodes_fts, rowid, label, content, node_type)
    VALUES ('delete', old.rowid, old.label, old.content, old.node_type);
    INSERT INTO nodes_fts(rowid, label, content, node_type)
    VALUES (new.rowid, new.label, new.content, new.node_type);
END;

CREATE INDEX IF NOT EXISTS idx_nodes_source      ON nodes(source_file);
CREATE INDEX IF NOT EXISTS idx_nodes_type        ON nodes(node_type);
CREATE INDEX IF NOT EXISTS idx_nodes_temperature ON nodes(temperature);
CREATE INDEX IF NOT EXISTS idx_nodes_parent      ON nodes(parent_id);
CREATE INDEX IF NOT EXISTS idx_diffs_snapshot    ON diffs(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_diffs_node        ON diffs(node_id);
