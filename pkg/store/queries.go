package store

// Shared column lists for SELECT scans.
const (
	nodeColumns = `id, parent_id, source_file, node_type, depth,
	label, content, format, token_count, content_hash,
	temperature, access_count, last_accessed_at,
	created_at, updated_at`

	nodeColumnsAliased = `n.id, n.parent_id, n.source_file, n.node_type, n.depth,
	n.label, n.content, n.format, n.token_count, n.content_hash,
	n.temperature, n.access_count, n.last_accessed_at,
	n.created_at, n.updated_at`

	diffColumns = `snapshot_id, node_id, op, old_hash, new_hash, old_content, new_content`

	snapshotColumns = `id, cursor_hash, parent_id, message, compile_root, created_at`
)

// nodes
const (
	qSelectNodeByID = `SELECT ` + nodeColumns + ` FROM nodes WHERE id = ?`

	qSelectNodesByFile = `SELECT ` + nodeColumns + ` FROM nodes WHERE source_file = ?`

	// IN clause is closed by the caller after appending placeholders.
	qSelectNodesByFilesPrefix = `SELECT ` + nodeColumns + ` FROM nodes WHERE source_file IN (`

	qSelectChildren = `SELECT ` + nodeColumns + ` FROM nodes WHERE parent_id = ?`

	qSelectAncestors = `
		WITH RECURSIVE anc AS (
			SELECT * FROM nodes WHERE id = ?
			UNION ALL
			SELECT n.* FROM nodes n
			JOIN anc a ON n.id = a.parent_id
			WHERE a.parent_id IS NOT NULL
		)
		SELECT * FROM anc`

	qSelectDescendants = `
		WITH RECURSIVE desc_cte(nid, lvl) AS (
			SELECT id, 1 FROM nodes WHERE parent_id = ?
			UNION ALL
			SELECT n.id, d.lvl + 1 FROM nodes n
			JOIN desc_cte d ON n.parent_id = d.nid
			WHERE d.lvl < ?
		)
		SELECT ` + nodeColumns + ` FROM nodes WHERE id IN (SELECT nid FROM desc_cte)`

	qSelectSiblings = `
		SELECT ` + nodeColumns + ` FROM nodes
		WHERE parent_id = (SELECT parent_id FROM nodes WHERE id = ?)
		AND id != ?`

	qSelectRootNodes = `SELECT ` + nodeColumns + ` FROM nodes WHERE parent_id IS NULL ORDER BY source_file, depth`

	qSelectAllNodes = `SELECT ` + nodeColumns + ` FROM nodes ORDER BY source_file, depth`

	qUpsertNode = `
		INSERT INTO nodes (id, parent_id, source_file, node_type, depth,
				label, content, format, token_count, content_hash, temperature)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, COALESCE(?11, 0.5))
		ON CONFLICT(id) DO UPDATE SET
			parent_id = excluded.parent_id,
			source_file = excluded.source_file,
			node_type = excluded.node_type,
			depth = excluded.depth,
			label = excluded.label,
			content = excluded.content,
			format = excluded.format,
			token_count = excluded.token_count,
			content_hash = excluded.content_hash,
			temperature = CASE WHEN ?11 IS NOT NULL THEN excluded.temperature ELSE temperature END,
			updated_at = unixepoch()`

	qDeleteNode = `DELETE FROM nodes WHERE id = ?`

	qRewriteSourcePaths = `UPDATE nodes SET source_file = ? || substr(source_file, length(?) + 1)
		WHERE source_file LIKE ? || '%'`
)

// snapshots & diffs
const (
	qSelectHeadCursorSnapID = `SELECT snapshot_id FROM cursors WHERE id = 'HEAD'`

	qInsertSnapshot = `INSERT INTO snapshots (cursor_hash, parent_id, message, compile_root) VALUES (?, ?, ?, ?)`

	qSelectLatestCompileRoot = `SELECT compile_root FROM snapshots
		WHERE compile_root != ''
		ORDER BY id DESC LIMIT 1`

	qInsertDiff = `INSERT INTO diffs (snapshot_id, node_id, op, old_hash, new_hash, old_content, new_content)
		VALUES (?, ?, ?, ?, ?, ?, ?)`

	qUpsertHeadCursor = `INSERT INTO cursors (id, snapshot_id) VALUES ('HEAD', ?)
		ON CONFLICT(id) DO UPDATE SET snapshot_id = excluded.snapshot_id, updated_at = unixepoch()`

	qSelectHeadCursorHash = `SELECT s.cursor_hash FROM cursors c
		JOIN snapshots s ON s.id = c.snapshot_id
		WHERE c.id = 'HEAD'`

	qSelectSnapshotByID = `SELECT ` + snapshotColumns + ` FROM snapshots WHERE id = ?`

	qListSnapshots = `SELECT ` + snapshotColumns + ` FROM snapshots ORDER BY id DESC LIMIT ?`

	qSelectDiffsBySnapshot = `SELECT ` + diffColumns + ` FROM diffs WHERE snapshot_id = ? ORDER BY id`

	qSelectDiffsSince = `SELECT ` + diffColumns + ` FROM diffs WHERE snapshot_id > ? ORDER BY snapshot_id, id`

	qSelectDiffsForNode = `SELECT ` + diffColumns + ` FROM diffs WHERE node_id = ? ORDER BY snapshot_id`
)

// search
const (
	qSearchFTS = `
		SELECT ` + nodeColumns + ` FROM nodes
		WHERE rowid IN (
			SELECT rowid FROM nodes_fts WHERE nodes_fts MATCH ?
			ORDER BY rank
			LIMIT ?
		)`

	qSearchRanked = `
		SELECT ` + nodeColumnsAliased + `, nodes_fts.rank
		FROM nodes_fts
		JOIN nodes n ON n.rowid = nodes_fts.rowid
		WHERE nodes_fts MATCH ?
		ORDER BY nodes_fts.rank
		LIMIT ?`
)

// stats
const qSelectStats = `
	SELECT count(*), coalesce(avg(temperature), 0),
		coalesce(sum(temperature >= ?), 0),
		coalesce(sum(temperature < ?), 0),
		(SELECT count(*) FROM snapshots)
	FROM nodes`

// temperature
const (
	qUpdateTemperature = `UPDATE nodes SET temperature = ?, updated_at = unixepoch() WHERE id = ?`

	qIncrementAccess = `UPDATE nodes SET access_count = access_count + 1, last_accessed_at = ?, updated_at = unixepoch()
		WHERE id = ?`

	qBoostTemperature = `UPDATE nodes SET temperature = min(1.0, temperature + ?),
		access_count = access_count + 1, last_accessed_at = ?, updated_at = unixepoch()
		WHERE id = ?`

	// IN clause is closed by the caller after appending placeholders.
	qBoostTemperatureBatchPrefix = `UPDATE nodes SET temperature = min(1.0, temperature + ?),
		access_count = access_count + 1, last_accessed_at = ?, updated_at = unixepoch()
		WHERE id IN (`

	qDecayTemperatures = `UPDATE nodes SET temperature = temperature * ?, updated_at = unixepoch()
		WHERE temperature > 0`

	qSelectColdNodes = `SELECT ` + nodeColumns + ` FROM nodes WHERE temperature < ? ORDER BY temperature ASC`
)
