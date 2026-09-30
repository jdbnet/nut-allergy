package alert

import (
	"strings"
	"testing"
)

func TestSamplePayloadAndEmailHTML(t *testing.T) {
	p := SamplePayload("test")
	plain, html := EmailMIME(p)
	if !strings.Contains(plain, p.UPSName) {
		t.Fatal("plain missing ups name")
	}
	if !strings.Contains(html, p.UPSName) {
		t.Fatal("html missing ups name")
	}
}
