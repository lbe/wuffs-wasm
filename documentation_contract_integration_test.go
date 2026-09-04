package wuffs

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// TestIntegrationVerifiedFormatDocumentationInventory pins the nine-format
// documentation contract: adapter.go DecodeReader GoDoc, API.md, README.md,
// testdata/README, and plans/api-roadmap.md must consistently advertise
// exactly FormatPNG, FormatWEBP, FormatBMP, FormatGIF, FormatJPEG,
// FormatNPBM, FormatQOI, FormatTGA, and FormatWBMP as the verified public
// format set; the NPBM binary P5/P6 exact-Maxval subset, the TGA
// type/depth/palette/origin/attribute subset, and the WBMP Type 0 canonical
// dimension subset described below in README.md and API.md; the remaining
// deferred set ETC2, HNSM, NIE, and TH; FORMAT-02 in Review; and REG-01 as
// future work.
func TestIntegrationVerifiedFormatDocumentationInventory(t *testing.T) {
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

	// The exact verified public set and the remaining deferred formats.
	// Canonical reader names are lowercase; roadmap identifiers match the
	// format constant names. FORMAT-02 verified the four portable formats.
	acceptedNames := []string{"png", "webp", "bmp", "gif", "jpeg", "npbm", "qoi", "tga", "wbmp"}
	acceptedIDs := []string{"FormatPNG", "FormatWEBP", "FormatBMP", "FormatGIF", "FormatJPEG", "FormatNPBM", "FormatQOI", "FormatTGA", "FormatWBMP"}
	format02Names := []string{"npbm", "qoi", "tga", "wbmp"}
	format02IDs := []string{"NPBM", "QOI", "TGA", "WBMP"}
	deferredNames := []string{"etc2", "hnsm", "nie", "thumbhash"}
	deferredIDs := []string{"ETC2", "HNSM", "NIE", "TH"}

	// NPBM, TGA, and WBMP support-subset phrases that README.md and API.md
	// must both state verbatim: NPBM is binary PGM P5 and PPM P6 with Maxval
	// exactly 255 or 65535; the TGA subset covers indexed types 1 and 9
	// (8-bit indices, 15-/24-/32-bit palettes starting at entry zero with 1
	// to 256 entries), true-color types 2 and 10 (15, 16, 24, or 32 bits)
	// without a color map, and grayscale types 3 and 11 at 8 bits without a
	// color map; interleaving and right-to-left origins are not permitted but
	// either vertical origin is; attribute bits are 0 for indexed, 15-, 16-,
	// and 24-bit true-color and 8-bit grayscale and 8 for 32-bit true-color;
	// indexed 16-bit palettes and 15- or 16-bit true-color with one attribute
	// bit are unsupported; WBMP is Type 0 with canonical shortest-form
	// dimension encodings and nonzero dimensions no greater than 0xFFFFFF.
	npbmSubset := []string{"binary PGM P5 and PPM P6", "Maxval exactly 255 or 65535", "all other maxima are unsupported"}
	tgaSubset := []string{
		"indexed types 1 and 9",
		"8-bit indices",
		"15-, 24-, or 32-bit",
		"true-color types 2 and 10",
		"15, 16, 24, or 32 bits",
		"grayscale types 3 and 11",
		"at 8 bits",
		"entry zero",
		"1 to 256 entries",
		"no color map",
		"interleaving",
		"right-to-left",
		"vertical origin",
		"not permitted",
		"attribute bits are 0",
		"8 for 32-bit true-color",
		"16-bit palettes",
		"one attribute bit",
		"are unsupported",
	}
	wbmpSubset := []string{"Type 0", "canonical shortest-form", "nonzero dimensions", "no greater than 0xFFFFFF"}

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

	// checkExactSet fails unless present holds exactly the required names:
	// every required name must be present and every present name required.
	checkExactSet := func(what, verb string, present map[string]bool, required []string) {
		t.Helper()
		for _, name := range required {
			if !present[name] {
				t.Errorf("%s must %s %s as a verified format", what, verb, name)
			}
		}
		for name := range present {
			if !slices.Contains(required, name) {
				t.Errorf("%s must not %s %s outside the verified nine-format set", what, verb, name)
			}
		}
	}

	// hasCell reports whether any cell of cells contains sub.
	hasCell := func(cells map[string]bool, sub string) bool {
		for cell := range cells {
			if strings.Contains(cell, sub) {
				return true
			}
		}
		return false
	}

	// adapter.go: the DecodeReader GoDoc must advertise exactly the nine
	// canonical reader names, name none of the remaining deferred formats,
	// and keep registration out of scope.
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

	// API.md: the DecodeReader section must advertise exactly the nine
	// canonical reader names and must not present a deferred format as
	// implemented.
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
	// nine verified format constants and no others.
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
	checkExactSet("API.md constants block", "declare", declared, acceptedIDs)

	// API.md: RegisterFormats remains future REG-01 work, not part of the API.
	if got := requireAll(api, []string{"RegisterFormats", "future", "not currently implemented"}); len(got) > 0 {
		t.Errorf("API.md must keep RegisterFormats as future REG-01 work; missing %v", got)
	}

	// NPBM, TGA, and WBMP support subsets must be stated in both README.md
	// and API.md.
	for file, text := range map[string]string{"README.md": readme, "API.md": api} {
		if got := requireAll(text, npbmSubset); len(got) > 0 {
			t.Errorf("%s must state the NPBM subset (%v) verbatim", file, got)
		}
		if got := requireAll(text, tgaSubset); len(got) > 0 {
			t.Errorf("%s must state the TGA subset (%v) verbatim", file, got)
		}
		if got := requireAll(text, wbmpSubset); len(got) > 0 {
			t.Errorf("%s must state the WBMP subset (%v) verbatim", file, got)
		}
	}

	// README.md: the supported table must list exactly the nine verified
	// formats, the not-yet-verified table must list the four remaining
	// deferred formats and nothing verified by FORMAT-02, and the API table
	// must name all nine format constants.
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
	checkExactSet("README supported table", "list", supported, acceptedNames)

	notVerified := firstColumn(readme, "Not yet verified")
	for _, name := range deferredNames {
		if !hasCell(notVerified, name) {
			t.Errorf("README not-yet-verified table must keep the deferred format %s", name)
		}
	}
	for _, name := range format02Names {
		for cell := range notVerified {
			if strings.Contains(cell, name) {
				t.Errorf("README not-yet-verified table must not list %s; FORMAT-02 verified it (cell %q)", name, cell)
			}
		}
	}

	if got := requireAll(readme, acceptedIDs); len(got) > 0 {
		t.Errorf("README API table must name the format constants %v", got)
	}

	// testdata/README: one inventory line must state the nine-format
	// verified set.
	verifiedLine := false
	for _, line := range strings.Split(strings.ToLower(testdataReadme), "\n") {
		if len(requireAll(line, acceptedNames)) == 0 {
			verifiedLine = true
			break
		}
	}
	if !verifiedLine {
		t.Error("testdata/README must state the verified format set (PNG, WebP, BMP, GIF, JPEG, NPBM, QOI, TGA, WBMP) on one line")
	}

	// plans/api-roadmap.md: a distinct FORMAT-02 entry points to this plan
	// with dependency CORE-01, guest changes Yes, and status Review; the
	// generic FORMAT-* entry remains Not planned and defers exactly ETC2,
	// HNSM, NIE, and TH; REG-01 (RegisterFormats) remains Not planned.
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

	if line := findLine(regBody, "FORMAT-02"); line == "" {
		t.Error("api-roadmap.md registry must add a distinct FORMAT-02 portable-images entry")
	} else {
		for _, want := range append([]string{"format-02-portable-images.yaml", "CORE-01", "Yes", "Review"}, format02IDs...) {
			if !strings.Contains(line, want) {
				t.Errorf("FORMAT-02 entry must mention %s; entry is: %s", want, line)
			}
		}
	}

	if line := findLine(regBody, "FORMAT-*"); line == "" {
		t.Error("api-roadmap.md registry must keep the generic FORMAT-* entry")
	} else {
		if !strings.Contains(line, "Not planned") {
			t.Errorf("generic FORMAT-* entry must remain Not planned; entry is: %s", line)
		}
		if got := requireAll(line, deferredIDs); len(got) > 0 {
			t.Errorf("FORMAT-* entry must defer %v; entry is: %s", got, line)
		}
		if got := requireNone(line, format02IDs); len(got) > 0 {
			t.Errorf("FORMAT-* entry must not defer the FORMAT-02 formats %v; entry is: %s", got, line)
		}
	}

	if line := findLine(regBody, "REG-01"); line == "" {
		t.Error("api-roadmap.md registry must keep REG-01")
	} else if !strings.Contains(line, "Not planned") {
		t.Errorf("REG-01 (RegisterFormats) must remain Not planned; entry is: %s", line)
	}
}
