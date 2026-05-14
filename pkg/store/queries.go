package store

// Shared column lists for SELECT scans.
const (
	nodeColumns = `id, parent_id, source_file, node_type, depth,
	label, content, format, token_count, content_hash,
	temperature, access_count, last_accessed_at,
	created_at, updated_at, pinned`

	nodeColumnsAliased = `n.id, n.parent_id, n.source_file, n.node_type, n.depth,
	n.label, n.content, n.format, n.token_count, n.content_hash,
	n.temperature, n.access_count, n.last_accessed_at,
	n.created_at, n.updated_at, n.pinned`

	diffColumns = `snapshot_id, node_id, op, old_hash, new_hash, old_content, new_content`

	snapshotColumns = `id, cursor_hash, parent_id, message, compile_root, created_at`
)

// nodes
const (
	qSelectNodeByID = `SELECT ` + nodeColumns + ` FROM nodes WHERE id = ?`

	qSelectNodesByFile = `SELECT ` + nodeColumns + ` FROM nodes WHERE source_file = ?`

	// IN clause is closed by the caller after appending placeholders.
	qSelectNodesByFilesPrefix = `SELECT ` + nodeColumns + ` FROM nodes WHERE source_file IN (`

	// IN clause is closed by the caller after appending placeholders.
	qSelectNodesByIDsPrefix = `SELECT ` + nodeColumns + ` FROM nodes WHERE id IN (`

	// Nodes whose most recent diffs entry was written under the given compile_root.
	qSelectNodesByCompileRoot = `
		WITH latest_per_node AS (
			SELECT node_id, MAX(snapshot_id) AS sid
			FROM diffs
			GROUP BY node_id
		)
		SELECT ` + nodeColumnsAliased + ` FROM nodes n
		JOIN latest_per_node lpn ON lpn.node_id = n.id
		JOIN snapshots s ON s.id = lpn.sid
		WHERE s.compile_root = ?`

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

	// compile_root comes from the most recent snapshot that touched any of the file's nodes.
	qSelectFileSummaries = `
		WITH file_latest AS (
			SELECT n.source_file, MAX(d.snapshot_id) AS sid
			FROM nodes n
			JOIN diffs d ON d.node_id = n.id
			GROUP BY n.source_file
		)
		SELECT
			n.source_file,
			count(*),
			coalesce(sum(n.token_count), 0),
			coalesce(s.compile_root, '')
		FROM nodes n
		LEFT JOIN file_latest fl ON fl.source_file = n.source_file
		LEFT JOIN snapshots s ON s.id = fl.sid
		GROUP BY n.source_file, s.compile_root
		ORDER BY s.compile_root, n.source_file`

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

	// IN clause is closed by the caller after appending placeholders.
	qDeleteNodesByFilesPrefix = `DELETE FROM nodes WHERE source_file IN (`

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

// relations & pending_relations
const (
	pendingColumns = `id, source_node_id, target_label, target_source, target_id_hint,
	weight, origin, created_at`

	qUpsertRelation = `
		INSERT INTO relations (source_node_id, target_node_id, weight, origin)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(source_node_id, target_node_id, origin)
		DO UPDATE SET weight = excluded.weight`

	qInsertPendingRelation = `
		INSERT INTO pending_relations (source_node_id, target_label, target_source, target_id_hint, weight, origin)
		VALUES (?, ?, ?, ?, ?, ?)`

	qDeletePendingByID = `DELETE FROM pending_relations WHERE id = ?`

	qDeleteParsedPendingForSource = `
		DELETE FROM pending_relations
		WHERE source_node_id = ? AND origin = 'parsed'`

	qSelectAllPendingRelations = `SELECT ` + pendingColumns + ` FROM pending_relations ORDER BY id`

	qSelectPendingBySource = `SELECT ` + pendingColumns + ` FROM pending_relations WHERE source_node_id = ?`

	qRelatedOut = `
		WITH RECURSIVE walk(target, hop, path_weight) AS (
			SELECT target_node_id, 1, weight FROM relations
			WHERE source_node_id = ? AND weight >= ?
			UNION ALL
			SELECT r.target_node_id, w.hop + 1, w.path_weight + r.weight
			FROM walk w
			JOIN relations r ON r.source_node_id = w.target
			WHERE w.hop < ? AND r.weight >= ?
		)
		SELECT ` + nodeColumnsAliased + `, MIN(w.hop), MAX(w.path_weight)
		FROM walk w
		JOIN nodes n ON n.id = w.target
		WHERE w.target != ?
		GROUP BY n.id
		ORDER BY MAX(w.path_weight) DESC, n.temperature DESC
		LIMIT ?`

	qRelatedIn = `
		WITH RECURSIVE walk(src, hop, path_weight) AS (
			SELECT source_node_id, 1, weight FROM relations
			WHERE target_node_id = ? AND weight >= ?
			UNION ALL
			SELECT r.source_node_id, w.hop + 1, w.path_weight + r.weight
			FROM walk w
			JOIN relations r ON r.target_node_id = w.src
			WHERE w.hop < ? AND r.weight >= ?
		)
		SELECT ` + nodeColumnsAliased + `, MIN(w.hop), MAX(w.path_weight)
		FROM walk w
		JOIN nodes n ON n.id = w.src
		WHERE w.src != ?
		GROUP BY n.id
		ORDER BY MAX(w.path_weight) DESC, n.temperature DESC
		LIMIT ?`

	qFindHeadingByLabel = `
		SELECT id FROM nodes
		WHERE node_type = 'heading' AND LOWER(TRIM(label)) = LOWER(TRIM(?))
		ORDER BY source_file ASC, depth ASC, id ASC
		LIMIT 1`

	qFindHeadingByLabelInFile = `
		SELECT id FROM nodes
		WHERE node_type = 'heading'
		  AND (source_file = ? OR source_file LIKE '%/' || ?)
		  AND LOWER(TRIM(label)) = LOWER(TRIM(?))
		ORDER BY depth ASC, id ASC
		LIMIT 1`

	qRelatedBoth = `
		WITH RECURSIVE
		out_walk(nid, hop, path_weight) AS (
			SELECT target_node_id, 1, weight FROM relations
			WHERE source_node_id = ? AND weight >= ?
			UNION ALL
			SELECT r.target_node_id, w.hop + 1, w.path_weight + r.weight
			FROM out_walk w
			JOIN relations r ON r.source_node_id = w.nid
			WHERE w.hop < ? AND r.weight >= ?
		),
		in_walk(nid, hop, path_weight) AS (
			SELECT source_node_id, 1, weight FROM relations
			WHERE target_node_id = ? AND weight >= ?
			UNION ALL
			SELECT r.source_node_id, w.hop + 1, w.path_weight + r.weight
			FROM in_walk w
			JOIN relations r ON r.target_node_id = w.nid
			WHERE w.hop < ? AND r.weight >= ?
		)
		SELECT ` + nodeColumnsAliased + `, MIN(c.hop), MAX(c.path_weight)
		FROM (SELECT nid, hop, path_weight FROM out_walk
			  UNION ALL
			  SELECT nid, hop, path_weight FROM in_walk) c
		JOIN nodes n ON n.id = c.nid
		WHERE c.nid != ?
		GROUP BY n.id
		ORDER BY MAX(c.path_weight) DESC, n.temperature DESC
		LIMIT ?`
)

// temperature
const (
	qUpdateTemperature = `UPDATE nodes SET temperature = ?, updated_at = unixepoch() WHERE id = ?`

	qIncrementAccess = `UPDATE nodes SET access_count = access_count + 1, last_accessed_at = ?, updated_at = unixepoch()
		WHERE id = ?`

	qBoostTemperature = `UPDATE nodes SET temperature = max(0.0, min(1.0, temperature + ?)),
		access_count = access_count + 1, last_accessed_at = ?, updated_at = unixepoch()
		WHERE id = ?`

	// IN clause is closed by the caller after appending placeholders.
	qBoostTemperatureBatchPrefix = `UPDATE nodes SET temperature = max(0.0, min(1.0, temperature + ?)),
		access_count = access_count + 1, last_accessed_at = ?, updated_at = unixepoch()
		WHERE id IN (`

	qDecayTemperatures = `UPDATE nodes SET temperature = max(0.0, min(1.0, temperature * ?)), updated_at = unixepoch()
		WHERE temperature > 0 AND pinned = 0`

	qSelectColdNodes = `SELECT ` + nodeColumns + ` FROM nodes
		WHERE temperature < ? AND pinned = 0
		ORDER BY temperature ASC LIMIT ?`

	qSetPinned = `UPDATE nodes SET pinned = ?, updated_at = unixepoch() WHERE id = ?`

	// IN clause is closed by the caller after appending placeholders.
	qResetTemperaturesByFilesPrefix = `UPDATE nodes SET temperature = ?, updated_at = unixepoch()
		WHERE source_file IN (`
)
