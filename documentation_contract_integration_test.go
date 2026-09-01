package wuffs

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestIntegrationCommonImageDocumentationInventory pins the common-image
// documentation contract: adapter.go DecodeReader GoDoc, API.md, README.md,
// testdata/README, and plans/api-roadmap.md must consistently advertise
// exactly FormatPNG, FormatWEBP, FormatBMP, FormatGIF, and FormatJPEG as the
// verified public format set, keep ETC2, HNSM, NIE, NPBM, QOI, TGA, TH, and
// WBMP deferred, leave RegisterFormats as future REG-01 work, and hold
// FORMAT-01 in Complete.
func TestIntegrationCommonImageDocumentationInventory(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed; cannot locate the repository root")
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

	adapter := read("adapter.go")
	api := read("API.md")
	readme := read("README.md")
	testdataReadme := read("testdata/README")
	roadmap := read("plans/api-roadmap.md")

	// The exact verified public set and the deferred formats from the common
	// image batch. Canonical reader names are lowercase; roadmap identifiers
	// match the format constant names.
	acceptedNames := []string{"png", "webp", "bmp", "gif", "jpeg"}
	deferredNames := []string{"tga", "qoi", "wbmp", "nie", "etc2", "hnsm", "netpbm", "thumbhash"}
	deferredIDs := []string{"ETC2", "HNSM", "NIE", "NPBM", "QOI", "TGA", "TH", "WBMP"}
	acceptedIDs := []string{"FormatPNG", "FormatWEBP", "FormatBMP", "FormatGIF", "FormatJPEG"}

	requireAll := func(text string, subs []string) []string {
		var notFound []string
		for _, s := range subs {
			if !strings.Contains(text, s) {
				notFound = append(notFound, s)
			}
		}
		return notFound
	}
	requireNone := func(text string, subs []string) []string {
		var found []string
		for _, s := range subs {
			if strings.Contains(text, s) {
				found = append(found, s)
			}
		}
		return found
	}

	// adapter.go: the DecodeReader GoDoc must advertise exactly the five
	// canonical names, name no deferred format, and keep registration out of
	// scope.
	docStart := strings.Index(adapter, "// DecodeReader reads r to EOF")
	funcStart := strings.Index(adapter, "func DecodeReader(")
	if docStart < 0 || funcStart < 0 || funcStart <= docStart {
		t.Fatal("adapter.go does not contain the expected DecodeReader GoDoc")
	}
	decoderGoDoc := strings.ToLower(adapter[docStart:funcStart])
	if got := requireAll(decoderGoDoc, acceptedNames); len(got) > 0 {
		t.Errorf("DecodeReader GoDoc must advertise the verified formats; missing %v", got)
	}
	if got := requireNone(decoderGoDoc, deferredNames); len(got) > 0 {
		t.Errorf("DecodeReader GoDoc must not name deferred formats; found %v", got)
	}
	if !strings.Contains(decoderGoDoc, "does not register") {
		t.Error("DecodeReader GoDoc must keep format registration out of scope")
	}

	// API.md: the DecodeReader section must advertise exactly the five
	// canonical names and must not present a deferred format as implemented.
	secStart := strings.Index(api, "func DecodeReader(r io.Reader) (image.Image, string, error)")
	secEnd := strings.Index(api, "func DecodeConfigReader(r io.Reader) (image.Config, error)")
	if secStart < 0 || secEnd < 0 || secEnd <= secStart {
		t.Fatal("API.md does not contain the expected DecodeReader section")
	}
	apiReaderSection := strings.ToLower(api[secStart:secEnd])
	if got := requireAll(apiReaderSection, acceptedNames); len(got) > 0 {
		t.Errorf("API.md reader section must advertise the verified formats; missing %v", got)
	}
	if got := requireNone(apiReaderSection, deferredNames); len(got) > 0 {
		t.Errorf("API.md reader section must not present deferred formats as implemented; found %v", got)
	}

	// API.md: the image format FourCC constants block must declare exactly the
	// five verified format constants and no others.
	formatsIdx := strings.Index(api, "Image format FourCCs")
	codeIdx := strings.Index(api[formatsIdx:], "```go")
	if formatsIdx < 0 || codeIdx < 0 {
		t.Fatal("API.md does not contain the image format constants block")
	}
	codeStart := formatsIdx + codeIdx + len("```go")
	closeIdx := strings.Index(api[codeStart:], "```")
	if closeIdx < 0 {
		t.Fatal("API.md image format constants block is not fenced")
	}
	constBlock := api[codeStart : codeStart+closeIdx]
	declared := map[string]bool{}
	for _, line := range strings.Split(constBlock, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "Format") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) > 0 {
			declared[fields[0]] = true
		}
	}
	for _, name := range acceptedIDs {
		if !declared[name] {
			t.Errorf("API.md constants block must declare %s as a verified format", name)
		}
	}
	for name := range declared {
		found := false
		for _, accepted := range acceptedIDs {
			if name == accepted {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("API.md constants block must not declare %s; only the five verified formats are public", name)
		}
	}

	// API.md: RegisterFormats remains future work, not part of the API today.
	if got := requireAll(api, []string{"RegisterFormats", "future", "not currently implemented"}); len(got) > 0 {
		t.Errorf("API.md must keep RegisterFormats as future REG-01 work; missing %v", got)
	}

	// README.md: the supported table must list exactly the five verified
	// formats, and the newly verified batch must leave the not-yet-verified
	// table.
	// firstColumn returns the first table cell of every data row in the named
	// subsection, skipping the header row and its separator until the first
	// separator row appears.
	firstColumn := func(text string, sectionHeader string) map[string]bool {
		set := map[string]bool{}
		inSection := false
		seenSeparator := false
		for _, line := range strings.Split(text, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "### ") {
				inSection = strings.Contains(trimmed, sectionHeader)
				seenSeparator = false
				continue
			}
			if !inSection || !strings.HasPrefix(trimmed, "|") {
				continue
			}
			rest := strings.TrimPrefix(trimmed, "|")
			end := strings.Index(rest, "|")
			if end < 0 {
				continue
			}
			cell := strings.TrimSpace(rest[:end])
			if strings.HasPrefix(cell, "-") {
				seenSeparator = true
				continue
			}
			if seenSeparator && cell != "" {
				set[strings.ToLower(cell)] = true
			}
		}
		return set
	}

	supported := firstColumn(readme, "Supported (tested)")
	for _, name := range acceptedNames {
		if !supported[name] {
			t.Errorf("README supported table must list %s as verified", name)
		}
	}
	for name := range supported {
		found := false
		for _, accepted := range acceptedNames {
			if name == accepted {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("README supported table must not list %s; only the five verified formats belong there", name)
		}
	}

	notVerified := firstColumn(readme, "Not yet verified")
	for _, name := range []string{"bmp", "gif", "jpeg"} {
		if notVerified[name] {
			t.Errorf("README not-yet-verified table must not list %s; the batch is verified", name)
		}
	}

	// testdata/README: one inventory line must state the verified set.
	verifiedLine := false
	for _, line := range strings.Split(strings.ToLower(testdataReadme), "\n") {
		if len(requireAll(line, acceptedNames)) == 0 {
			verifiedLine = true
			break
		}
	}
	if !verifiedLine {
		t.Error("testdata/README must state the verified format set (PNG, WebP, BMP, GIF, JPEG) on one line")
	}

	// plans/api-roadmap.md: FORMAT-01 is Complete for the BMP/GIF/JPEG batch,
	// the generic FORMAT-* entry remains Not planned and defers the eight
	// deferred formats, and REG-01 (RegisterFormats) remains Not planned.
	regStart := strings.Index(roadmap, "## Work registry")
	if regStart < 0 {
		t.Fatal("api-roadmap.md does not contain a Work registry section")
	}
	regBody := roadmap[regStart+len("## Work registry"):]
	if next := strings.Index(regBody, "\n## "); next >= 0 {
		regBody = regBody[:next]
	}

	findLine := func(text string, sub string) string {
		for _, line := range strings.Split(text, "\n") {
			if strings.Contains(line, sub) {
				return line
			}
		}
		return ""
	}

	if line := findLine(regBody, "FORMAT-01"); line == "" {
		t.Error("api-roadmap.md registry must add a distinct FORMAT-01 common-images entry")
	} else {
		if !strings.Contains(line, "Complete") {
			t.Errorf("FORMAT-01 must be Complete; entry is: %s", line)
		}
		for _, name := range []string{"BMP", "GIF", "JPEG"} {
			if !strings.Contains(line, name) {
				t.Errorf("FORMAT-01 entry must cover %s; entry is: %s", name, line)
			}
		}
	}

	if line := findLine(regBody, "FORMAT-*"); line == "" {
		t.Error("api-roadmap.md registry must keep the generic FORMAT-* entry")
	} else if !strings.Contains(line, "Not planned") {
		t.Errorf("generic FORMAT-* entry must remain Not planned; entry is: %s", line)
	}

	if line := findLine(regBody, "REG-01"); line == "" {
		t.Error("api-roadmap.md registry must keep REG-01")
	} else if !strings.Contains(line, "Not planned") {
		t.Errorf("REG-01 (RegisterFormats) must remain Not planned; entry is: %s", line)
	}

	if got := requireAll(regBody, deferredIDs); len(got) > 0 {
		t.Errorf("FORMAT-* entry must explicitly defer %v", got)
	}
}
