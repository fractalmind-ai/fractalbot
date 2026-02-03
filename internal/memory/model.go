package memory

// ModelSpec describes embedding model artifacts.
type ModelSpec struct {
	ID              string
	ModelURL        string
	ModelSHA256     string
	TokenizerURL    string
	TokenizerSHA256 string
}

const (
	DefaultModelID           = "multilingual-e5-small"
	DefaultModelURL          = "https://huggingface.co/intfloat/multilingual-e5-small/resolve/main/onnx/model.onnx"
	DefaultModelSHA256       = "ca456c06b3a9505ddfd9131408916dd79290368331e7d76bb621f1cba6bc8665"
	DefaultTokenizerURL      = "https://huggingface.co/intfloat/multilingual-e5-small/resolve/main/tokenizer.json"
	DefaultTokenizerSHA256   = "0b44a9d7b51c3c62626640cda0e2c2f70fdacdc25bbbd68038369d14ebdf4c39"
	DefaultTokenizerFileName = "tokenizer.json"
	DefaultModelFileName     = "model.onnx"
)

// DefaultModelSpec returns the default multilingual E5 model configuration.
func DefaultModelSpec() ModelSpec {
	return ModelSpec{
		ID:              DefaultModelID,
		ModelURL:        DefaultModelURL,
		ModelSHA256:     DefaultModelSHA256,
		TokenizerURL:    DefaultTokenizerURL,
		TokenizerSHA256: DefaultTokenizerSHA256,
	}
}
