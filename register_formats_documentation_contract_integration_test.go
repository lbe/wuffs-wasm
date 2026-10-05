package wuffs

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestIntegrationRegisterFormatsDocumentationContract pins the published
// registration contract: exported Go documentation, API.md, and README.md
// must describe RegisterFormats as explicit, idempotent, concurrency-safe
// opt-in for exactly the thirteen verified formats, including the
// exact-header TGA and literal-prefix WBMP registry limits and their
// intentional prefix-recognition false-positive boundaries; state that the
// global image registry is first-match-wins and earlier registrations remain
// authoritative; and show image.Decode and image.DecodeConfig usage.
func TestIntegrationRegisterFormatsDocumentationContract(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) failed; cannot locate the repository root")
	}
	root := filepath.Dir(thisFile)

	read := func(name string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return string(b)
	}

	registration := read("registration.go")
	api := read("API.md")
	readme := read("README.md")
	roadmap := read("plans/api-roadmap.md")

	// window returns the documentation window around the first RegisterFormats
	// mention so whole-file incidental matches cannot satisfy the contract.
	// The input is folded once so the match index and the returned section
	// agree.
	window := func(text string) string {
		folded := strings.ToLower(text)
		idx := strings.Index(folded, "registerformats")
		if idx < 0 {
			return ""
		}
		return folded[max(idx-500, 0):min(idx+3000, len(folded))]
	}

	// godoc extracts the RegisterFormats comment block, folded for
	// case-insensitive term matching.
	godoc := func(src string) string {
		const anchor = "// RegisterFormats"
		idx := strings.Index(src, anchor)
		if idx < 0 {
			return ""
		}
		block := src[idx:]
		if end := strings.Index(block, "func RegisterFormats()"); end >= 0 {
			block = block[:end]
		}
		return strings.ToLower(block)
	}(registration)

	apiWindow := window(api)
	readmeWindow := window(readme)

	// contractTerms are the registration-contract phrases each of the three
	// documentation surfaces must state in its RegisterFormats section.
	contractTerms := []string{
		"explicit",
		"opt-in",
		"once",
		"concurrent",
		"thirteen",
		"first-match",
		"earlier",
		"image.decode",
		"image.decodeconfig",
		"tga",
		"wbmp",
		"false-positive",
	}

	requireTerms := func(what, section string) {
		t.Helper()
		if section == "" {
			t.Errorf("%s must document RegisterFormats; no RegisterFormats section found", what)
			return
		}
		var missing []string
		for _, term := range contractTerms {
			if !strings.Contains(section, term) {
				missing = append(missing, term)
			}
		}
		if len(missing) > 0 {
			t.Errorf("%s RegisterFormats section must state %v; missing %v", what, contractTerms, missing)
		}
	}

	requireTerms("registration.go GoDoc", godoc)
	requireTerms("API.md", apiWindow)
	requireTerms("README.md", readmeWindow)

	if strings.Contains(strings.ToLower(api), "not currently implemented") {
		t.Errorf("API.md must accept RegisterFormats as implemented; found stale \"not currently implemented\" claim")
	}

	// retainedScopes are the format-specific limitations that publication
	// must not drop from either user-facing document.
	retainedScopes := []string{
		"binary pgm p5 and ppm p6",
		"maxval exactly 255 or 65535",
		"indexed types 1 and 9",
		"true-color types 2 and 10",
		"grayscale types 3 and 11",
		"type 0",
		"canonical shortest-form",
		"handsum",
		"frame zero",
		"cooked",
	}
	foldedAPI := strings.ToLower(api)
	foldedReadme := strings.ToLower(readme)
	for _, scope := range retainedScopes {
		if !strings.Contains(foldedAPI, scope) {
			t.Errorf("API.md must retain scope limitation %q after publishing the registration contract", scope)
		}
		if !strings.Contains(foldedReadme, scope) {
			t.Errorf("README.md must retain scope limitation %q after publishing the registration contract", scope)
		}
	}

	// Roadmap: REG-01 must record this plan path and be Complete.
	foundReg := ""
	for line := range strings.Lines(roadmap) {
		if strings.Contains(line, "REG-01") {
			foundReg = line
			break
		}
	}
	if foundReg == "" {
		t.Errorf("plans/api-roadmap.md must keep a REG-01 entry")
	} else {
		for _, want := range []string{".pi/tdd-plans/reg-01-register-formats.yaml", "complete"} {
			if !strings.Contains(strings.ToLower(foundReg), want) {
				t.Errorf("REG-01 entry must mention %q; entry is: %s", want, strings.TrimSpace(foundReg))
			}
		}
	}
}
