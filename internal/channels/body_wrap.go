package channels

import (
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

const (
	// BodyModeInline means the message body is carried inline in the data map.
	BodyModeInline = "inline"
	// BodyModeFilePointer means the body was written to a file; body_file holds the path.
	BodyModeFilePointer = "file_pointer"

	// bodyLineThreshold triggers file-backing when the body has this many lines or more.
	bodyLineThreshold = 12
	// bodyCharThreshold triggers file-backing when the body has this many characters or more.
	bodyCharThreshold = 1800
)

// IsLongBody returns true when text exceeds the long-body thresholds.
func IsLongBody(text string) bool {
	if len(text) >= bodyCharThreshold {
		return true
	}
	return strings.Count(text, "\n")+1 >= bodyLineThreshold
}

// WrapMessageBody evaluates the "text" field in msg.Data and annotates the
// message with body_mode metadata. If the body exceeds the long-body threshold
// and bodyDir is non-empty, the body is written to a file and body_file is set.
//
// The original "text" key is preserved for backward compatibility. Callers that
// understand body_mode should prefer body_file (when file_pointer) or body_text
// (when inline) over the raw text field.
func WrapMessageBody(msg *protocol.Message, bodyDir string) error {
	if msg == nil {
		return nil
	}
	data, ok := msg.Data.(map[string]interface{})
	if !ok {
		return nil
	}

	text, _ := data["text"].(string)
	if strings.TrimSpace(text) == "" {
		return nil
	}

	if !IsLongBody(text) {
		data["body_mode"] = BodyModeInline
		data["body_text"] = text
		return nil
	}

	// Long body — attempt to write to file.
	if strings.TrimSpace(bodyDir) == "" {
		bodyDir = filepath.Join(os.TempDir(), "fractalbot", "bodies")
	}
	if err := os.MkdirAll(bodyDir, 0755); err != nil {
		log.Printf("body_wrap: cannot create dir %s: %v; keeping inline", bodyDir, err)
		data["body_mode"] = BodyModeInline
		data["body_text"] = text
		return nil
	}

	filename := fmt.Sprintf("body-%d.md", time.Now().UnixNano())
	filePath := filepath.Join(bodyDir, filename)
	if err := os.WriteFile(filePath, []byte(text), 0644); err != nil {
		log.Printf("body_wrap: write failed %s: %v; keeping inline", filePath, err)
		data["body_mode"] = BodyModeInline
		data["body_text"] = text
		return nil
	}

	hash := sha256.Sum256([]byte(text))
	data["body_mode"] = BodyModeFilePointer
	data["body_file"] = filePath
	data["body_sha256"] = fmt.Sprintf("%x", hash)

	return nil
}
