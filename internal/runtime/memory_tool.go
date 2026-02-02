package runtime

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fractalmind-ai/fractalbot/internal/config"
	"github.com/fractalmind-ai/fractalbot/internal/memory"
)

const (
	defaultMemoryToolName       = "memory.search"
	defaultMemoryQueryPrefix    = "query:"
	defaultMemoryPassagePrefix  = "passage:"
	defaultMemoryChunkTokens    = memory.DefaultChunkTokens
	defaultMemoryChunkOverlap   = memory.DefaultChunkOverlap
	defaultMemoryTopK           = memory.DefaultTopK
	defaultMemoryIndexBatchSize = memory.DefaultBatchSize
	defaultMemoryMaxTokens      = 512
)

// MemorySearchTool exposes semantic memory search for the runtime.
type MemorySearchTool struct {
	cfg     *config.MemoryConfig
	once    sync.Once
	search  *memory.Searcher
	indexer *memory.Indexer
	store   *memory.Store
	initErr error
}

// NewMemorySearchTool creates a new memory search tool.
func NewMemorySearchTool(cfg *config.MemoryConfig) (Tool, error) {
	if cfg == nil {
		return nil, fmt.Errorf("memory config is required")
	}
	return &MemorySearchTool{cfg: cfg}, nil
}

// Name returns the tool name.
func (t *MemorySearchTool) Name() string {
	return defaultMemoryToolName
}

// Execute searches semantic memory for the query text.
func (t *MemorySearchTool) Execute(ctx context.Context, req ToolRequest) (string, error) {
	query := strings.TrimSpace(req.Args)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	if err := t.ensureReady(ctx); err != nil {
		return "", err
	}
	results, err := t.search.Search(ctx, query)
	if err != nil {
		return "", err
	}
	return formatMemoryResults(results), nil
}

func (t *MemorySearchTool) ensureReady(ctx context.Context) error {
	t.once.Do(func() {
		cacheDir, err := memory.ResolveCacheDir(t.cfg.CacheDir)
		if err != nil {
			t.initErr = err
			return
		}
		spec := resolveModelSpec(t.cfg)
		assets, err := memory.EnsureModelAssets(ctx, cacheDir, spec)
		if err != nil {
			t.initErr = err
			return
		}

		tokenizer, err := memory.NewHFTokenizer(assets.TokenizerPath)
		if err != nil {
			t.initErr = err
			return
		}
		chunkTokens := t.cfg.ChunkTokens
		if chunkTokens <= 0 {
			chunkTokens = defaultMemoryChunkTokens
		}
		chunkOverlap := t.cfg.ChunkOverlap
		if chunkOverlap <= 0 {
			chunkOverlap = defaultMemoryChunkOverlap
		}

		chunker := memory.Chunker{
			MaxTokens:     chunkTokens,
			OverlapTokens: chunkOverlap,
			Counter:       memory.TokenizerCounter{Tokenizer: tokenizer},
		}

		indexPath := memory.IndexPath(cacheDir, spec.ID)
		if err := ensureParentDir(indexPath); err != nil {
			t.initErr = err
			return
		}
		store, err := memory.OpenStore(indexPath)
		if err != nil {
			t.initErr = err
			return
		}

		embedder, err := memory.NewOnnxEmbedder(memory.OnnxConfig{
			ModelPath:     assets.ModelPath,
			TokenizerPath: assets.TokenizerPath,
			Tokenizer:     tokenizer,
			MaxTokens:     resolveMaxTokens(t.cfg),
			CacheDir:      cacheDir,
		})
		if err != nil {
			t.initErr = err
			_ = store.Close()
			return
		}

		sourceRoot := strings.TrimSpace(t.cfg.SourceRoot)
		if sourceRoot == "" {
			sourceRoot = "."
		}

		indexer := &memory.Indexer{
			Embedder:  memory.PrefixedEmbedder{Prefix: defaultMemoryPassagePrefix, Base: embedder},
			Chunker:   chunker,
			BatchSize: defaultMemoryIndexBatchSize,
		}
		if _, err := indexer.Index(ctx, sourceRoot, store); err != nil {
			t.initErr = err
			_ = store.Close()
			return
		}

		t.search = &memory.Searcher{
			Embedder: memory.PrefixedEmbedder{Prefix: defaultMemoryQueryPrefix, Base: embedder},
			Store:    store,
			TopK:     resolveTopK(t.cfg),
		}
		t.indexer = indexer
		t.store = store
		log.Printf("memory: indexed %s (model=%s)", sourceRoot, spec.ID)
	})
	return t.initErr
}

func resolveModelSpec(cfg *config.MemoryConfig) memory.ModelSpec {
	spec := memory.DefaultModelSpec()
	if cfg == nil {
		return spec
	}
	if strings.TrimSpace(cfg.ModelID) != "" {
		spec.ID = strings.TrimSpace(cfg.ModelID)
	}
	if strings.TrimSpace(cfg.ModelURL) != "" {
		spec.ModelURL = strings.TrimSpace(cfg.ModelURL)
	}
	if strings.TrimSpace(cfg.ModelSHA256) != "" {
		spec.ModelSHA256 = strings.TrimSpace(cfg.ModelSHA256)
	}
	if strings.TrimSpace(cfg.TokenizerURL) != "" {
		spec.TokenizerURL = strings.TrimSpace(cfg.TokenizerURL)
	}
	if strings.TrimSpace(cfg.TokenizerSHA256) != "" {
		spec.TokenizerSHA256 = strings.TrimSpace(cfg.TokenizerSHA256)
	}
	return spec
}

func resolveTopK(cfg *config.MemoryConfig) int {
	if cfg == nil || cfg.TopK <= 0 {
		return defaultMemoryTopK
	}
	return cfg.TopK
}

func resolveMaxTokens(cfg *config.MemoryConfig) int {
	if cfg == nil || cfg.MaxTokens <= 0 {
		return defaultMemoryMaxTokens
	}
	return cfg.MaxTokens
}

func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0755)
}

func formatMemoryResults(results []memory.SearchResult) string {
	if len(results) == 0 {
		return "no memory results"
	}
	var builder strings.Builder
	for _, result := range results {
		builder.WriteString(fmt.Sprintf("- %s:%d-%d\n", result.Path, result.StartLine, result.EndLine))
		text := strings.TrimSpace(result.Content)
		if text != "" {
			builder.WriteString(text)
			builder.WriteString("\n")
		}
	}
	return strings.TrimSpace(builder.String())
}
