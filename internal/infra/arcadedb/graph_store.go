// Package arcadedb implements repository.GraphRepository on ArcadeDB via the
// Neo4j Bolt protocol (neo4j-go-driver/v5, Apache-2.0). ArcadeDB is a native
// property-graph database — Chunk nodes + RELATED_TO edges — deployed as an
// independent docker service. (SPEC-070)
package arcadedb

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// schemaDDL is the idempotent startup seed: uniqueness constraint on chunk_id
// plus a lookup index on creator_id (used by graph queries' permission
// scoping). ArcadeDB is openCypher-compatible; creation errors caused by the
// object already existing are tolerated so repeated startups are safe.
var schemaDDL = []string{
	"CREATE CONSTRAINT chunk_id_unique IF NOT EXISTS FOR (c:Chunk) REQUIRE c.chunk_id IS UNIQUE",
	"CREATE INDEX chunk_creator_id IF NOT EXISTS FOR (c:Chunk) ON (c.creator_id)",
}

// GraphStore is the ArcadeDB-backed GraphRepository.
type GraphStore struct {
	driver   neo4j.DriverWithContext
	database string
	mu       sync.Mutex // serializes writes; Bolt sessions are cheap, ordering matters for edges
}

// NewGraphStore wraps an existing driver (created once at wire time) and the
// target database name.
func NewGraphStore(driver neo4j.DriverWithContext, database string) *GraphStore {
	if database == "" {
		database = "kbgraph"
	}
	return &GraphStore{driver: driver, database: database}
}

// NewDriver creates the global Bolt driver singleton for ArcadeDB.
func NewDriver(ctx context.Context, uri, username, password string) (neo4j.DriverWithContext, error) {
	return neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(username, password, ""))
}

func (g *GraphStore) session(ctx context.Context) neo4j.SessionWithContext {
	return g.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: g.database})
}

// run executes a write (auto-commit) statement.
func (g *GraphStore) run(ctx context.Context, cypher string, params map[string]any) error {
	s := g.session(ctx)
	defer s.Close(ctx)
	_, err := s.Run(ctx, cypher, params)
	return err
}

// read executes a read statement, collecting records into fn.
func (g *GraphStore) read(ctx context.Context, cypher string, params map[string]any, fn func(record *neo4j.Record) error) error {
	s := g.session(ctx)
	defer s.Close(ctx)
	result, err := s.Run(ctx, cypher, params)
	if err != nil {
		return err
	}
	for result.Next(ctx) {
		rec := result.Record()
		if err := fn(rec); err != nil {
			return err
		}
	}
	return result.Err()
}

// EnsureSchema idempotently creates the constraint/index DDL.
func (g *GraphStore) EnsureSchema(ctx context.Context) error {
	for _, ddl := range schemaDDL {
		if err := g.run(ctx, ddl, nil); err != nil {
			if isAlreadyExistsErr(err) {
				continue
			}
			return fmt.Errorf("ensure graph schema: %w", err)
		}
	}
	return nil
}

func isAlreadyExistsErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"already exists", "already existing",
		"constraint already", "index already",
		"constraintalreadyexists", "indexalreadyexists",
		"equivalentschemarulealreadyexists",
		"schema rule already",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

// UpsertChunk merges a chunk node by chunk_id and refreshes its properties.
func (g *GraphStore) UpsertChunk(ctx context.Context, c repository.GraphChunk) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	cypher := `MERGE (c:Chunk {chunk_id: $chunkID})
SET c.doc_id = $docID, c.chunk_idx = $chunkIdx, c.creator_id = $creatorID,
    c.is_public = $isPublic, c.char_count = $charCount`
	return g.run(ctx, cypher, map[string]any{
		"chunkID":   c.ChunkID,
		"docID":     c.DocID,
		"chunkIdx":  c.ChunkIdx,
		"creatorID": c.CreatorID,
		"isPublic":  c.IsPublic,
		"charCount": c.CharCount,
	})
}

// LinkRelated creates RELATED_TO edges (score on the edge) from chunkID to
// each ref, idempotently (MERGE).
func (g *GraphStore) LinkRelated(ctx context.Context, chunkID string, refs []repository.RelatedRef) error {
	if len(refs) == 0 {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	params := make([]map[string]any, 0, len(refs))
	for _, r := range refs {
		if r.ChunkID == "" || r.ChunkID == chunkID {
			continue
		}
		params = append(params, map[string]any{"chunk_id": r.ChunkID, "score": r.Score})
	}
	if len(params) == 0 {
		return nil
	}
	cypher := `MATCH (a:Chunk {chunk_id: $chunkID})
UNWIND $refs AS ref
MATCH (b:Chunk {chunk_id: ref.chunk_id})
MERGE (a)-[r:RELATED_TO]->(b)
SET r.score = ref.score`
	return g.run(ctx, cypher, map[string]any{"chunkID": chunkID, "refs": params})
}

// DeleteChunk removes a chunk node and its edges (idempotent).
func (g *GraphStore) DeleteChunk(ctx context.Context, chunkID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.run(ctx, `MATCH (c:Chunk {chunk_id: $chunkID}) DETACH DELETE c`,
		map[string]any{"chunkID": chunkID})
}

// DeleteByDocID removes all chunk nodes of a document and their edges
// (idempotent DETACH DELETE).
func (g *GraphStore) DeleteByDocID(ctx context.Context, docID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.run(ctx, `MATCH (c:Chunk {doc_id: $docID}) DETACH DELETE c`,
		map[string]any{"docID": docID})
}

// QueryTopN returns up to topN related chunks of the anchor, sorted by score
// desc, filtered by visibility (system_admin sees all; regular users see
// their own or public chunks).
func (g *GraphStore) QueryTopN(ctx context.Context, anchorID string, topN int, filter repository.GraphFilter) ([]repository.GraphNode, error) {
	if topN <= 0 {
		topN = 5
	}
	cypher := `MATCH (a:Chunk {chunk_id: $anchorID})-[r:RELATED_TO]->(n:Chunk)
WHERE $isAdmin OR n.creator_id = $creatorID OR n.is_public = true
RETURN n.chunk_id AS chunk_id, n.doc_id AS doc_id, r.score AS score
ORDER BY score DESC LIMIT $topN`
	nodes := make([]repository.GraphNode, 0, topN)
	err := g.read(ctx, cypher, map[string]any{
		"anchorID":  anchorID,
		"isAdmin":   filter.IsSystemAdmin,
		"creatorID": filter.CreatorID,
		"topN":      topN,
	}, func(rec *neo4j.Record) error {
		chunkID, _ := rec.Get("chunk_id")
		docID, _ := rec.Get("doc_id")
		score, _ := rec.Get("score")
		node := repository.GraphNode{}
		if s, ok := chunkID.(string); ok {
			node.ChunkID = s
		}
		if s, ok := docID.(string); ok {
			node.DocID = s
		}
		switch v := score.(type) {
		case float64:
			node.Score = v
		case int64:
			node.Score = float64(v)
		}
		nodes = append(nodes, node)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("query graph topN: %w", err)
	}
	return nodes, nil
}

var _ repository.GraphRepository = (*GraphStore)(nil)
