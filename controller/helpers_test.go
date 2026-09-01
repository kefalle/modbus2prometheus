package controller

import "testing"

func TestParseOperationRejectsUnsupportedValue(t *testing.T) {
	_, err := ParseOperation("unsupported")
	if err == nil {
		t.Fatal("ParseOperation must return an error for an unsupported operation")
	}
}

func TestParseOperationPreservesConfiguredFlags(t *testing.T) {
	got, err := ParseOperation("read_uint|write_uint")
	if err != nil {
		t.Fatalf("ParseOperation returned an unexpected error: %v", err)
	}
	want := uint8(READ_UINT | WRITE_UINT)
	if got != want {
		t.Fatalf("ParseOperation flags = %d, want %d", got, want)
	}
}
