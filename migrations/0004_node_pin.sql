ALTER TABLE nodes ADD COLUMN pinned BOOLEAN NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_nodes_pinned ON nodes(pinned) WHERE pinned = 1;
