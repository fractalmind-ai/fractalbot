package memory

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"sort"

	_ "modernc.org/sqlite"
)

// IndexedChunk is a chunk with embedding metadata.
type IndexedChunk struct {
	Path       string
	StartLine  int
	EndLine    int
	Content    string
	Embedding  []float32
	TokenCount int
}

// SearchResult is a scored result for a query.
type SearchResult struct {
	Path      string
	StartLine int
	EndLine   int
	Content   string
	Score     float32
}

// Store persists memory embeddings in SQLite.
type Store struct {
	db *sql.DB
}

// OpenStore opens or creates a SQLite store at the given path.
func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite: %w", err)
	}
	if err := initSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Reset deletes all indexed chunks.
func (s *Store) Reset(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is nil")
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM chunks")
	if err != nil {
		return fmt.Errorf("failed to reset store: %w", err)
	}
	return nil
}

// InsertChunks inserts new chunks into the store.
func (s *Store) InsertChunks(ctx context.Context, chunks []IndexedChunk) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is nil")
	}
	if len(chunks) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin tx: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, "INSERT INTO chunks(path,start_line,end_line,content,token_count,embedding) VALUES(?,?,?,?,?,?)")
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("failed to prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, chunk := range chunks {
		data, err := encodeVector(chunk.Embedding)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if _, err := stmt.ExecContext(ctx, chunk.Path, chunk.StartLine, chunk.EndLine, chunk.Content, chunk.TokenCount, data); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed to insert chunk: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}
	return nil
}

// Search returns the top-K most similar chunks for the query embedding.
func (s *Store) Search(ctx context.Context, query []float32, topK int) ([]SearchResult, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("store is nil")
	}
	if len(query) == 0 {
		return nil, fmt.Errorf("query embedding is empty")
	}
	if topK <= 0 {
		topK = 5
	}

	rows, err := s.db.QueryContext(ctx, "SELECT path,start_line,end_line,content,embedding FROM chunks")
	if err != nil {
		return nil, fmt.Errorf("failed to query chunks: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var path, content string
		var startLine, endLine int
		var blob []byte
		if err := rows.Scan(&path, &startLine, &endLine, &content, &blob); err != nil {
			return nil, fmt.Errorf("failed to scan chunk: %w", err)
		}
		embedding, err := decodeVector(blob)
		if err != nil {
			return nil, err
		}
		score := cosineSimilarity(query, embedding)
		results = append(results, SearchResult{
			Path:      path,
			StartLine: startLine,
			EndLine:   endLine,
			Content:   content,
			Score:     score,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read rows: %w", err)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

func initSchema(db *sql.DB) error {
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS chunks (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	path TEXT NOT NULL,
	start_line INTEGER NOT NULL,
	end_line INTEGER NOT NULL,
	content TEXT NOT NULL,
	token_count INTEGER NOT NULL,
	embedding BLOB NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_chunks_path ON chunks(path);
`); err != nil {
		return fmt.Errorf("failed to init schema: %w", err)
	}
	return nil
}

func encodeVector(vec []float32) ([]byte, error) {
	if len(vec) == 0 {
		return nil, fmt.Errorf("embedding is empty")
	}
	data := make([]byte, len(vec)*4)
	for i, v := range vec {
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(v))
	}
	return data, nil
}

func decodeVector(data []byte) ([]float32, error) {
	if len(data)%4 != 0 {
		return nil, fmt.Errorf("invalid embedding blob")
	}
	vec := make([]float32, len(data)/4)
	for i := 0; i < len(vec); i++ {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return vec, nil
}

func cosineSimilarity(a, b []float32) float32 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}
