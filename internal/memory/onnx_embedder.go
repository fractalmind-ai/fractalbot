//go:build cgo

package memory

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/fractalmind-ai/fractalbot/internal/memory/ort"
	ortapi "github.com/yalue/onnxruntime_go"
)

var (
	ortInitOnce sync.Once
	ortInitErr  error
	ortLibPath  string
)

// NewOnnxEmbedder constructs an ONNX runtime embedder.
func NewOnnxEmbedder(cfg OnnxConfig) (Embedder, error) {
	if strings.TrimSpace(cfg.ModelPath) == "" {
		return nil, fmt.Errorf("model path is required")
	}
	if cfg.Tokenizer == nil && strings.TrimSpace(cfg.TokenizerPath) == "" {
		return nil, fmt.Errorf("tokenizer path is required")
	}

	libPath := strings.TrimSpace(cfg.LibraryPath)
	if libPath == "" {
		cacheDir := strings.TrimSpace(cfg.CacheDir)
		if cacheDir == "" {
			resolved, err := ResolveCacheDir("")
			if err != nil {
				return nil, err
			}
			cacheDir = resolved
		}
		extracted, err := ort.EnsureLibrary(cacheDir)
		if err != nil {
			return nil, err
		}
		libPath = extracted
	}
	if err := ensureOrt(libPath); err != nil {
		return nil, err
	}

	inputInfo, outputInfo, err := ortapi.GetInputOutputInfo(cfg.ModelPath)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect model IO: %w", err)
	}
	inputNames := extractNames(inputInfo)
	outputNames := extractNames(outputInfo)
	if len(inputNames) == 0 || len(outputNames) == 0 {
		return nil, fmt.Errorf("model IO names not found")
	}
	outputIndex := selectOutputIndex(outputNames)

	session, err := ortapi.NewDynamicAdvancedSession(cfg.ModelPath, inputNames, outputNames, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	tokenizer := cfg.Tokenizer
	if tokenizer == nil {
		tokenizer, err = NewHFTokenizer(cfg.TokenizerPath)
		if err != nil {
			_ = session.Destroy()
			return nil, err
		}
	}

	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	return &OnnxEmbedder{
		session:     session,
		tokenizer:   tokenizer,
		maxTokens:   maxTokens,
		inputNames:  inputNames,
		outputNames: outputNames,
		outputIndex: outputIndex,
	}, nil
}

// OnnxEmbedder runs the ONNX model to produce embeddings.
type OnnxEmbedder struct {
	session     *ortapi.DynamicAdvancedSession
	tokenizer   Tokenizer
	maxTokens   int
	inputNames  []string
	outputNames []string
	outputIndex int
	mu          sync.Mutex
}

// Embed embeds the input texts.
func (e *OnnxEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if e == nil || e.session == nil || e.tokenizer == nil {
		return nil, ErrOnnxUnavailable
	}
	if len(texts) == 0 {
		return nil, nil
	}

	results := make([][]float32, 0, len(texts))
	for _, text := range texts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		embedding, err := e.embedOne(ctx, text)
		if err != nil {
			return nil, err
		}
		results = append(results, embedding)
	}
	return results, nil
}

func (e *OnnxEmbedder) embedOne(ctx context.Context, text string) ([]float32, error) {
	_ = ctx
	ids, err := e.tokenizer.Encode(text)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("tokenizer produced no tokens")
	}
	if len(ids) > e.maxTokens {
		ids = ids[:e.maxTokens]
	}

	attention := make([]int64, len(ids))
	for i := range attention {
		attention[i] = 1
	}
	tokenTypes := make([]int64, len(ids))

	inputs, destroyInputs, err := e.buildInputs(ids, attention, tokenTypes)
	if err != nil {
		return nil, err
	}
	defer destroyValues(destroyInputs)

	outputs := make([]ortapi.Value, len(e.outputNames))

	e.mu.Lock()
	err = e.session.Run(inputs, outputs)
	e.mu.Unlock()
	if err != nil {
		destroyOutputs(outputs)
		return nil, err
	}
	defer destroyOutputs(outputs)

	value := outputs[e.outputIndex]
	tensor, ok := value.(*ortapi.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("unexpected output type")
	}

	shape := tensor.GetShape()
	data := tensor.GetData()
	embedding, err := extractEmbedding(data, shape, attention)
	if err != nil {
		return nil, err
	}
	normalizeInPlace(embedding)
	return embedding, nil
}

func (e *OnnxEmbedder) buildInputs(ids, attention, tokenTypes []int64) ([]ortapi.Value, []func() error, error) {
	var values []ortapi.Value
	var destroy []func() error
	seqLen := len(ids)
	shape := ortapi.NewShape(1, int64(seqLen))

	for _, name := range e.inputNames {
		var data []int64
		switch name {
		case "input_ids":
			data = ids
		case "attention_mask":
			data = attention
		case "token_type_ids":
			data = tokenTypes
		default:
			return nil, nil, fmt.Errorf("unsupported input name %q", name)
		}

		if len(data) != seqLen {
			return nil, nil, fmt.Errorf("input %s length mismatch", name)
		}
		tensor, err := ortapi.NewTensor(shape, data)
		if err != nil {
			return nil, nil, err
		}
		values = append(values, tensor)
		destroy = append(destroy, tensor.Destroy)
	}
	return values, destroy, nil
}

func ensureOrt(path string) error {
	ortInitOnce.Do(func() {
		ortapi.SetSharedLibraryPath(path)
		ortInitErr = ortapi.InitializeEnvironment()
		if ortInitErr == nil {
			ortLibPath = path
		}
	})
	if ortInitErr != nil {
		return ortInitErr
	}
	if ortLibPath != path {
		return fmt.Errorf("onnxruntime already initialized with %s", ortLibPath)
	}
	return nil
}

func extractNames(info []ortapi.InputOutputInfo) []string {
	names := make([]string, 0, len(info))
	for _, entry := range info {
		if entry.Name == "" {
			continue
		}
		names = append(names, entry.Name)
	}
	return names
}

func selectOutputIndex(names []string) int {
	preferred := []string{"sentence_embedding", "pooler_output", "last_hidden_state"}
	for _, name := range preferred {
		for i, candidate := range names {
			if candidate == name {
				return i
			}
		}
	}
	return 0
}

func extractEmbedding(data []float32, shape ortapi.Shape, attention []int64) ([]float32, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty output tensor")
	}
	if len(shape) == 2 {
		hidden := int(shape[1])
		if hidden <= 0 {
			hidden = len(data)
		}
		if len(data) < hidden {
			return nil, fmt.Errorf("output tensor shape mismatch")
		}
		embedding := make([]float32, hidden)
		copy(embedding, data[:hidden])
		return embedding, nil
	}
	if len(shape) == 3 {
		batch := int(shape[0])
		seq := int(shape[1])
		hidden := int(shape[2])
		if batch <= 0 {
			batch = 1
		}
		if seq <= 0 {
			seq = len(attention)
		}
		if hidden <= 0 {
			if seq > 0 {
				hidden = len(data) / (batch * seq)
			}
		}
		if hidden <= 0 {
			return nil, fmt.Errorf("invalid output tensor shape")
		}
		return meanPool(data, batch, seq, hidden, attention), nil
	}
	return nil, fmt.Errorf("unsupported output tensor shape")
}

func meanPool(data []float32, batch, seq, hidden int, attention []int64) []float32 {
	if batch <= 0 {
		batch = 1
	}
	if seq <= 0 {
		seq = len(attention)
	}
	if len(attention) == 0 {
		attention = make([]int64, seq)
		for i := range attention {
			attention[i] = 1
		}
	}
	embedding := make([]float32, hidden)
	var denom float64
	for token := 0; token < seq; token++ {
		mask := float64(1)
		if token < len(attention) {
			mask = float64(attention[token])
		}
		if mask == 0 {
			continue
		}
		denom += mask
		base := token * hidden
		for h := 0; h < hidden; h++ {
			embedding[h] += float32(mask) * data[base+h]
		}
	}
	if denom == 0 {
		return embedding
	}
	for i := range embedding {
		embedding[i] /= float32(denom)
	}
	return embedding
}

func normalizeInPlace(vec []float32) {
	var sum float64
	for _, v := range vec {
		sum += float64(v * v)
	}
	if sum == 0 {
		return
	}
	norm := float32(1.0 / math.Sqrt(sum))
	for i := range vec {
		vec[i] *= norm
	}
}

func destroyOutputs(outputs []ortapi.Value) {
	for _, value := range outputs {
		if value == nil {
			continue
		}
		_ = value.Destroy()
	}
}

func destroyValues(destroyers []func() error) {
	for _, destroy := range destroyers {
		_ = destroy()
	}
}
