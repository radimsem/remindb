CREATE TABLE IF NOT EXISTS relations (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    source_node_id  CHAR(11) NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    target_node_id  CHAR(11) NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    weight          REAL NOT NULL DEFAULT 1.0,
    origin          VARCHAR(8) NOT NULL,
    created_at      INTEGER DEFAULT (unixepoch()),
    UNIQUE(source_node_id, target_node_id, origin)
);

CREATE INDEX IF NOT EXISTS idx_relations_source ON relations(source_node_id);
CREATE INDEX IF NOT EXISTS idx_relations_target ON relations(target_node_id);

CREATE TABLE IF NOT EXISTS pending_relations (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    source_node_id  CHAR(11) NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    target_label    TEXT,
    target_source   TEXT,
    target_id_hint  CHAR(11),
    weight          REAL NOT NULL DEFAULT 1.0,
    origin          VARCHAR(8) NOT NULL,
    created_at      INTEGER DEFAULT (unixepoch())
);

CREATE INDEX IF NOT EXISTS idx_pending_source ON pending_relations(source_node_id);
CREATE INDEX IF NOT EXISTS idx_pending_label  ON pending_relations(target_label);

CREATE INDEX IF NOT EXISTS idx_nodes_label ON nodes(label) WHERE node_type = 'heading';

CREATE TRIGGER IF NOT EXISTS relations_repend_on_node_delete
BEFORE DELETE ON nodes
FOR EACH ROW
BEGIN
    INSERT INTO pending_relations (source_node_id, target_label, target_source, weight, origin)
    SELECT r.source_node_id, OLD.label, OLD.source_file, r.weight, r.origin
    FROM relations r
    WHERE r.target_node_id = OLD.id;
END;
