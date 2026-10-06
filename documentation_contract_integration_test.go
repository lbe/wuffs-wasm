package wuffs

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// TestIntegrationVerifiedFormatDocumentationInventory pins the thirteen-format
// documentation contract: adapter.go DecodeReader GoDoc, API.md, README.md,
// testdata/README, and plans/api-roadmap.md must consistently advertise
// exactly FormatPNG, FormatWEBP, FormatBMP, FormatGIF, FormatJPEG,
// FormatNPBM, FormatQOI, FormatTGA, FormatWBMP, FormatETC2, FormatHNSM,
// FormatNIE, and FormatTH as the verified public format set; the NPBM binary
// P5/P6 exact-Maxval subset, the TGA type/depth/palette/origin/attribute
// subset, and the WBMP Type 0 canonical dimension subset described below in
// README.md and API.md; HNSM identified as Handsum; NIE limited to still
// images and frame zero; TH supporting only the cooked form while raw
// ThumbHash is unsupported; FORMAT-03 Complete; and REG-01 Complete with
// its plan file recorded.
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

	// The complete verified set: every Wuffs image decoder the guest ships is
	// verified. Canonical reader names are lowercase; roadmap identifiers
	// match the format constant names. FORMAT-03 verified the remaining batch
	// (ETC2, HNSM, NIE, TH), so nothing is deferred anymore.
	acceptedNames := []string{"png", "webp", "bmp", "gif", "jpeg", "npbm", "qoi", "tga", "wbmp", "etc2", "hnsm", "nie", "th"}
	acceptedIDs := []string{"FormatPNG", "FormatWEBP", "FormatBMP", "FormatGIF", "FormatJPEG", "FormatNPBM", "FormatQOI", "FormatTGA", "FormatWBMP", "FormatETC2", "FormatHNSM", "FormatNIE", "FormatTH"}

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

	// acceptedWords precompiles a whole-word matcher per canonical name, so
	// short names (notably "th" and "nie") never match inside unrelated
	// words. Compiled once here, the per-text checks below stay free of
	// regexp work.
	acceptedWords := make(map[string]*regexp.Regexp, len(acceptedNames))
	for _, name := range acceptedNames {
		acceptedWords[name] = regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
	}

	// missingNames lists the canonical names absent from text as whole words.
	missingNames := func(text string) []string {
		var missing []string
		for _, name := range acceptedNames {
			if !acceptedWords[name].MatchString(text) {
				missing = append(missing, name)
			}
		}
		return missing
	}

	// sameLine reports whether any single line of text carries every sub.
	sameLine := func(text string, subs []string) bool {
		for _, line := range strings.Split(text, "\n") {
			lower := strings.ToLower(line)
			ok := true
			for _, s := range subs {
				if !strings.Contains(lower, strings.ToLower(s)) {
					ok = false
					break
				}
			}
			if ok {
				return true
			}
		}
		return false
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
				t.Errorf("%s must not %s %s outside the verified thirteen-format set", what, verb, name)
			}
		}
	}

	// adapter.go: the DecodeReader GoDoc must advertise exactly the thirteen
	// canonical reader names and keep registration out of scope.
	docStart := strings.Index(adapter, "// DecodeReader reads r to EOF")
	funcStart := strings.Index(adapter, "func DecodeReader(")
	if docStart < 0 || funcStart < 0 || funcStart <= docStart {
		t.Fatal("adapter.go does not contain the expected DecodeReader GoDoc")
	}
	decoderGoDoc := strings.ToLower(adapter[docStart:funcStart])
	if got := missingNames(decoderGoDoc); len(got) > 0 {
		t.Errorf("DecodeReader GoDoc must advertise every verified format; missing %v", got)
	}
	if !strings.Contains(decoderGoDoc, "does not register") {
		t.Error("DecodeReader GoDoc must keep format registration out of scope")
	}

	// API.md: the DecodeReader section must advertise all thirteen canonical
	// reader names.
	secStart := strings.Index(api, "func DecodeReader(r io.Reader) (image.Image, string, error)")
	secEnd := strings.Index(api, "func DecodeConfigReader(r io.Reader) (image.Config, error)")
	if secStart < 0 || secEnd < 0 || secEnd <= secStart {
		t.Fatal("API.md does not contain the expected DecodeReader section")
	}
	apiReaderSection := strings.ToLower(api[secStart:secEnd])
	if got := missingNames(apiReaderSection); len(got) > 0 {
		t.Errorf("API.md reader section must advertise every verified format; missing %v", got)
	}

	// API.md: the verified format subsets section must state the full
	// thirteen-format set.
	subsetsStart := strings.Index(api, "### Verified format subsets")
	if subsetsStart < 0 {
		t.Fatal("API.md does not contain a Verified format subsets section")
	}
	if got := missingNames(strings.ToLower(api[subsetsStart:])); len(got) > 0 {
		t.Errorf("API.md verified format subsets section must state the full thirteen-format set; missing %v", got)
	}

	// API.md: the image format FourCC constants block must declare exactly the
	// thirteen verified format constants and no others.
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

	// API.md: RegisterFormats is implemented REG-01 work.
	if !strings.Contains(api, "RegisterFormats") {
		t.Errorf("API.md must document RegisterFormats as implemented REG-01 work")
	}
	if strings.Contains(strings.ToLower(api), "not currently implemented") {
		t.Errorf("API.md must accept RegisterFormats as implemented; found stale \"not currently implemented\" claim")
	}

	// README.md and API.md must state the NPBM, TGA, and WBMP support
	// subsets verbatim and must limit the FORMAT-03 formats: HNSM identified
	// as Handsum, NIE as still images and frame zero only, and TH as only the
	// cooked form with raw ThumbHash unsupported.
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
		if !sameLine(text, []string{"hnsm", "handsum"}) {
			t.Errorf("%s must identify HNSM as Handsum on one line", file)
		}
		if !sameLine(text, []string{"nie", "still", "frame zero"}) {
			t.Errorf("%s must limit NIE to still images and frame zero on one line", file)
		}
		if !sameLine(text, []string{"thumbhash", "cooked", "raw", "unsupported"}) {
			t.Errorf("%s must state that TH supports only the cooked form while raw ThumbHash is unsupported, on one line", file)
		}
	}

	// README.md: the supported table must list exactly the thirteen verified
	// formats, the not-yet-verified table must be empty (nothing is deferred),
	// and the API table must name all thirteen format constants.
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
	if len(notVerified) > 0 {
		cells := make([]string, 0, len(notVerified))
		for cell := range notVerified {
			cells = append(cells, cell)
		}
		slices.Sort(cells)
		t.Errorf("README not-yet-verified table must be empty now that FORMAT-03 verified every remaining format; found %v", cells)
	}

	if got := requireAll(readme, acceptedIDs); len(got) > 0 {
		t.Errorf("README API table must name the format constants %v", got)
	}

	// testdata/README: one inventory line must state the thirteen-format
	// verified set.
	verifiedLine := false
	for _, line := range strings.Split(strings.ToLower(testdataReadme), "\n") {
		seen := map[string]bool{}
		for _, field := range strings.Fields(line) {
			token := strings.Trim(field, ",.;:()[]|*\"'`")
			if slices.Contains(acceptedNames, token) {
				seen[token] = true
			}
		}
		if len(seen) == len(acceptedNames) {
			verifiedLine = true
			break
		}
	}
	if !verifiedLine {
		t.Error("testdata/README must state the thirteen-format verified set (PNG, WebP, BMP, GIF, JPEG, NPBM, QOI, TGA, WBMP, ETC2, HNSM, NIE, TH) on one line")
	}

	// plans/api-roadmap.md: a distinct FORMAT-03 entry points to this plan
	// with dependency CORE-01, guest changes Yes, and status Complete; the
	// generic FORMAT-* entry is gone (no deferred formats remain); REG-01
	// (RegisterFormats) is Complete and points to its plan file.
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

	if line := findLine(regBody, "FORMAT-03"); line == "" {
		t.Error("api-roadmap.md registry must add a distinct FORMAT-03 remaining-images entry")
	} else {
		for _, want := range []string{"format-03-remaining-images.yaml", "CORE-01", "Yes", "Complete", "ETC2", "HNSM", "NIE", "TH"} {
			if !strings.Contains(line, want) {
				t.Errorf("FORMAT-03 entry must mention %s; entry is: %s", want, line)
			}
		}
	}

	if line := findLine(regBody, "FORMAT-*"); line != "" {
		t.Errorf("api-roadmap.md registry must replace the generic FORMAT-* entry with FORMAT-03; found: %s", line)
	}

	if line := findLine(regBody, "REG-01"); line == "" {
		t.Error("api-roadmap.md registry must keep REG-01")
	} else {
		for _, want := range []string{".pi/tdd-plans/reg-01-register-formats.yaml", "Complete"} {
			if !strings.Contains(line, want) {
				t.Errorf("REG-01 entry must mention %s; entry is: %s", want, line)
			}
		}
	}

	// ANIM-01: animation documentation inventory. README.md and API.md must
	// advertise FrameCount, LoopCount, and DecodeFrame; NIE scope moves from
	// frame-zero-only to nïA multi-frame verified; GIF animation must
	// document DecodeFrame instead of standing as "first frame only"; PNG
	// remains first-frame-only; WebP remains still-only with no animation;
	// ANIM-01 is In progress or Review.
	animationAPIs := []string{"FrameCount", "LoopCount", "DecodeFrame"}
	for file, text := range map[string]string{"README.md": readme, "API.md": api} {
		if got := requireAll(text, animationAPIs); len(got) > 0 {
			t.Errorf("%s must document the animation APIs (%v)", file, got)
		}
		if !sameLine(text, []string{"nie", "still", "frame zero"}) {
			t.Errorf("%s must limit NIE stills to still images and frame zero on one line", file)
		}
		if !sameLine(text, []string{"nïa", "multi-frame"}) {
			t.Errorf("%s must verify NIE nïA multi-frame animation on one line", file)
		}
		if !sameLine(text, []string{"gif", "DecodeFrame"}) {
			t.Errorf("%s must document GIF animation via DecodeFrame on one line", file)
		}
		for _, line := range strings.Split(text, "\n") {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "gif") && strings.Contains(lower, "first frame only") {
				t.Errorf("%s must not list GIF animation as first frame only without DecodeFrame: %q", file, line)
			}
		}
		if !sameLine(text, []string{"png", "first-frame"}) {
			t.Errorf("%s must keep PNG as first-frame-only on one line", file)
		}
		if !sameLine(text, []string{"webp", "no animation"}) {
			t.Errorf("%s must keep WebP as still-only with no animation on one line", file)
		}
	}

	if line := findLine(regBody, "ANIM-01"); line == "" {
		t.Error("api-roadmap.md registry must keep ANIM-01")
	} else if !strings.Contains(line, "In progress") && !strings.Contains(line, "Review") && !strings.Contains(line, "Complete") {
		t.Errorf("ANIM-01 entry must be In progress, Review, or Complete; entry is: %s", line)
	}
}
