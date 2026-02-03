package memory

import "testing"

func TestValidateModelID(t *testing.T) {
	valid := []string{
		"multilingual-e5-small",
		"model_v1.2",
		"ABC-123",
	}
	for _, value := range valid {
		if err := ValidateModelID(value); err != nil {
			t.Fatalf("expected valid model ID %q: %v", value, err)
		}
	}

	invalid := []string{
		"",
		"../model",
		"model/../x",
		"foo/bar",
		"foo\\bar",
		"/absolute",
		"C:\\model",
		"model id",
	}
	for _, value := range invalid {
		if err := ValidateModelID(value); err == nil {
			t.Fatalf("expected invalid model ID %q", value)
		}
	}
}

func TestIndexPathRejectsInvalidModelID(t *testing.T) {
	if _, err := IndexPath("/tmp/cache", "../oops"); err == nil {
		t.Fatal("expected error for invalid model ID")
	}
}
