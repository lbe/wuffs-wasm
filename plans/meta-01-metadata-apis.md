# META-01 — Metadata APIs (`Metadata`, `Chromaticities`, `(*Decoder).Metadata`)

**Execution protocol:** `plans/pi-runbook.md`

**Execution branch:** `meta-01-metadata-apis`

**Progress file:** `tmp/pi_progress.md`

**Commit authorized:** Yes

**Purpose:** Implement the metadata section of `API.md`: exported `Metadata`,
`Chromaticities`, metadata FourCC constants (`MetaEXIF` … `MetaMTIM`), and
`(*Decoder).Metadata`; extend the wasm guest with a metadata-only export that
walks Wuffs `decode_image_config` with `set_report_metadata` enabled; verify on
upstream Wuffs fixtures for PNG and GIF; update API-boundary inventory,
documentation, and roadmap completion evidence.

This plan does **not** add package-level `Metadata(src)`, `RegisterFormats`
changes, pixel decode changes, animation APIs, or new image format verification.

---

## Before Pi starts (owner / orchestrator, not worker)

1. Ensure this file exists at `plans/meta-01-metadata-apis.md` on `main`
   before creating branch `meta-01-metadata-apis`.
2. In `plans/api-roadmap.md`, update **only** the `META-01` row:
   - `Plan type` → `Runbook`
   - `Plan file` → `plans/meta-01-metadata-apis.md`
   - `Status` → `Ready`
3. Confirm `ANIM-01` and all format workstreams remain `Complete`.
4. Confirm sibling checkout exists: `../wuffs/test/data/` (upstream Wuffs test
   fixtures). If missing, **do not start Pi** — worker STOP gate 6 applies.

Workers must not edit the roadmap from `Not planned` to `Ready`. That is owner
prep above.

---

## Sources of truth

Read before acting:

1. `AGENTS.md`
2. The user's latest instruction
3. `plans/pi-runbook.md`
4. This plan
5. `GOALS.md`
6. `API.md` (metadata types, constants, `(*Decoder).Metadata`)
7. `HANDOVER.md` (caller-owned `Pix`, explicit `Reserve`, zero-allocation hot path)
8. `plans/api-roadmap.md`, especially `META-01`

If sources conflict, stop and report the conflict. Preflight verifies `CORE-02`
`Complete` and `META-01` `Ready` in `plans/api-roadmap.md` (STOP on failure).

**Wuffs reference (read-only, not vendored into this repo):**

- `../wuffs/test/c/std/png.c` — metadata oracle for PNG (`test_wuffs_png_decode_metadata_*`)
- `../wuffs/test/c/std/gif.c` — metadata oracle for GIF (`do_test_wuffs_gif_decode_metadata`)
- `../wuffs/release/c/wuffs-v0.4.c` — `set_report_metadata`, `decode_image_config`,
  `wuffs_base__note__metadata_reported`, `wuffs_base__image_decoder__tell_me_more`,
  `wuffs_base__more_information__metadata_*` helpers

---

## Verified metadata scope (fixed fixtures)

Copy these from `../wuffs/test/data/` into `testdata/` during **Preflight
only**. Record one-line provenance in `testdata/README`.

| Fixture | Role |
| ------- | ---- |
| `bricks-color.png` | PNG with **no** ancillary metadata (all absent) |
| `bricks-dither.png` | PNG with **cHRM**, **gAMA**, and **sRGB** (parsed) |
| `artificial-png/exif.png` | PNG with **EXIF** (raw blob) |
| `red-blue-gradient.dcip3d65-no-chrm-no-gama.png` | PNG with **iCCP** |
| `DCI-P3-D65.icc` | Oracle bytes for ICC profile (not an image input) |
| `artificial-gif/metadata-full.gif` | GIF with **ICCP** and **XMP** application extensions |
| `artificial-gif/metadata-empty.gif` | GIF with **no** metadata extensions |

Do not add metadata verification for every verified image format in this plan.
PNG + GIF evidence is sufficient for META-01.

**Modification time (`MetaMTIM` / `HasModTime`):** Embedded Wuffs v0.4 defines
`WUFFS_BASE__FOURCC__MTIM` but has **no** `metadata_parsed__mtim` helper and no
upstream C test exercising MTIM delivery. This plan requires:

- Guest calls `set_report_metadata(MTIM, true)` with the other kinds.
- Guest sets `has_modtime` in the pack header only when Wuffs delivers MTIM
  through `tell_me_more` during the metadata walk; host maps `modtime_sec` /
  `modtime_nsec` to `time.Unix(sec, nsec)` in UTC.
- On every fixture in **Verified metadata scope**, `HasModTime` is false and
  `ModTime` is zero until a later plan adds an MTIM oracle fixture.
- Task 7 adds `TestIntegrationMetadataModTimeAbsent` on **both**
  `testdata/bricks-color.png` and `testdata/bricks-dither.png`.
- Do **not** add MTIM fixtures, vendored files, or tests beyond
  `TestIntegrationMetadataModTimeAbsent` in this plan.

---

## Product invariants (must hold after every task)

- No CGO. Root package owns the consumer API.
- `Metadata` does **not** decode pixels and does **not** read or write caller
  `image.RGBA` / `image.NRGBA` / `image.Gray` memory.
- `Metadata` uses the same `src` capacity rules as `Probe` (`checkSrcCapacity`,
  `ErrSrcTooLarge`, empty `src` → `ErrDecode`, unknown header → `ErrUnknownFormat`).
- Only `Reserve` grows guest scratch; `Metadata` never calls `Reserve` or
  `memory.Grow`.
- `Metadata` uses the **guest destination scratch slot** as a byte pack buffer
  (not caller `Pix`). It does not require a correctly sized canvas
  `image.RGBA` in Go memory.
- Returned `[]byte` fields in `Metadata` must be **copies** in Go heap memory,
  never wasm sub-slices.
- Reusable `DecodeRGBA` / `Probe` zero-allocation contracts remain unchanged.
  `Metadata` allocates Go heap memory for returned blob slices; that does not
  change the zero-allocation decode contract.
- Generated `internal/wuffswasm/wuffs.go` is never hand-edited; guest changes
  run `make generate` and commit `wasm/shim.c`, `wasm/wuffs.wasm`, and
  `internal/wuffswasm/wuffs.go` together.
- Do not add `MetaKVP` or other Wuffs metadata kinds not listed in `API.md`.

---

## Wuffs metadata algorithm (guest — fixed)

Implement a static function `read_image_metadata` in `wasm/shim.c` used only by
the new export. Pattern matches upstream `png.c` / `gif.c` tests and Wuffs
`DecodeImage` metadata handling:

1. `bump_rewind()` at entry.
2. `sniff_fourcc` + `find_decoder`; unknown → `WUFFS_WASM_ERR_UNKNOWN_FORMAT`.
3. Allocate decoder from bump; `init`; `upcast` to `wuffs_base__image_decoder*`.
4. If PNG fourcc, `set_quirk(IGNORE_CHECKSUM, 1)` (same as `probe_image`).
5. For each of these FourCCs, call `wuffs_base__image_decoder__set_report_metadata(decoder, fourcc, true)`:
   - `WUFFS_BASE__FOURCC__EXIF`, `ICCP`, `XMP`, `GAMA`, `CHRM`, `SRGB`, `MTIM`
6. Loop `wuffs_base__image_decoder__decode_image_config(&ic, &src)`:
   - **OK (`repr == NULL`):** break out (image config decoded; no pixels).
   - **`wuffs_base__note__metadata_reported`:** handle one metadata item (step 7), then continue loop.
   - **Any other status:** `WUFFS_WASM_ERR_DECODE`.
7. On `metadata_reported`, call `wuffs_base__image_decoder__tell_me_more` in a
   loop until the metadata item is fully consumed. For EXIF, repeat
   `tell_me_more` until the passthrough range is empty (two-pass pattern in
   `png.c` `test_wuffs_png_decode_metadata_exif`):
   - **Parsed flavor** (`WUFFS_BASE__MORE_INFORMATION__FLAVOR__METADATA_PARSED`):
     read fourcc via `wuffs_base__more_information__metadata__fourcc`; merge into
     pack header (`CHRM`, `GAMA`, `SRGB`, `MTIM` when Wuffs reports it).
   - **Raw passthrough flavor:** read fourcc and
     `wuffs_base__more_information__metadata_raw_passthrough__range`; copy bytes
     from `src` at `[min_incl, max_excl)` into the pack blob region (append in
     stable order: EXIF, then ICC, then XMP — first-seen append per kind; at most
     one blob per kind).
   - **GIF ICC/XMP:** follow `gif.c` `do_test_wuffs_gif_decode_metadata` (raw
     chunk bytes via `tell_me_more` into a temp buffer).
8. Write the pack (header + blobs) into guest memory at `pack_off` with total
   size ≤ `pack_cap`. If the pack does not fit, set `meta->err` /
   return `WUFFS_WASM_ERR_DECODE` (do **not** add a new public Go error).
   Oversize-pack failure is guest-only (`ErrDecode` on the host); META-01 does
   **not** add a dedicated integration test for it.
9. Do **not** call `decode_frame` or write pixels to a pixel buffer.

**Numeric conversions (host, fixed):**

| Wuffs input | Go `Metadata` field |
| ----------- | ------------------- |
| `metadata_parsed__chrm(i)` int32 scaled ×100000 | `Chromaticities` field `i/100000.0` (order: WhiteX, WhiteY, RedX, RedY, GreenX, GreenY, BlueX, BlueY for `i` 0..7) |
| `metadata_parsed__gama()` uint32 ≈ 100000/γ | `HasGamma=true`, `Gamma = 100000.0 / float64(gama)` |
| `metadata_parsed__srgb()` uint32 intent 0..3 | `HasSRGB=true`, `SRGB` = that uint32 (per `API.md` “as reported”) |
| `has_modtime` in pack + `modtime_sec` / `modtime_nsec` | `HasModTime=true`, `ModTime=time.Unix(sec, nsec).UTC()` |

---

## Guest export contract (fixed)

Add in `wasm/shim.c`:

```c
// Packed metadata written to [pack_off, pack_off + written_len).
// Header is little-endian. Blob order: EXIF, ICC, XMP concatenated.
typedef struct wuffs_wasm_metadata_pack_header {
  int32_t err;              // WUFFS_WASM_OK on success
  uint32_t format;          // image FourCC
  uint32_t exif_len;
  uint32_t icc_len;
  uint32_t xmp_len;
  uint8_t has_gamma;
  uint8_t has_chrm;
  uint8_t has_srgb;
  uint8_t has_modtime;
  uint8_t _pad0[3];
  uint32_t gama_scaled;     // Wuffs gama() raw; valid if has_gamma
  int32_t chrm[8];          // Wuffs chrm components; valid if has_chrm
  uint32_t srgb_intent;     // valid if has_srgb
  int64_t modtime_sec;      // Unix seconds; valid if has_modtime
  int32_t modtime_nsec;     // 0..999999999; valid if has_modtime
} wuffs_wasm_metadata_pack_header;  // size MUST be 88 bytes (static_assert in shim)

// Returns WUFFS_WASM_OK or negative WUFFS_WASM_ERR_* .
// pack_cap is the guest destination scratch byte capacity (host passes dstLen).
__attribute__((export_name("wuffs_read_image_metadata")))
int32_t wuffs_wasm_read_image_metadata(
    uint32_t src_off, uint32_t src_len,
    uint32_t pack_off, uint32_t pack_cap);
```

Rules:

- `pack_off == 0` or `src_len == 0` → `WUFFS_WASM_ERR_BAD_ARG`.
- On success, guest sets `header.err = WUFFS_WASM_OK` and returns `WUFFS_WASM_OK`.
- `static_assert(sizeof(wuffs_wasm_metadata_pack_header) == 88)` in `wasm/shim.c`.
- Existing exports unchanged: `wuffs_probe_image`, `wuffs_decode_image`,
  `wuffs_frame_count`, `wuffs_animation_loops`, `wuffs_decode_frame`, `wuffs_version`.

---

## Host `Metadata` call pattern (fixed)

Mirror `FrameCount` / `Probe`:

1. `checkSrcCapacity(src)`.
2. `copySrcToSlot(src)`.
3. Ensure `d.currentLayout.dstLen >= 65536` (64 KiB). If smaller, return
   `ErrDecode` (metadata pack needs guest scratch; default `New()` dst slot is
   128 KiB so normal decoders pass). **Do not** auto-`Reserve`.
4. Zero the first `88` bytes at `pack_off = lay.dstOff` (reuse dst scratch).
5. Call `Xwuffs_read_image_metadata(srcOff, srcLen, packOff, dstLen)`.
6. Map guest errors like `Probe` (`ErrUnknownFormat`, `ErrDecode`, etc.).
7. Parse header + blobs from wasm memory; **copy** EXIF/ICC/XMP into new Go
   `[]byte` slices; fill `d.lastMetadata`; return `&d.lastMetadata`.
8. Do not mutate `lastMeta`, frame scratch, or caller image buffers.

Integration tests that call `assertGuestMemoryPreserved` after `Metadata` assert
wasm backing pointer, byte length, and slot layout only—not dst scratch byte
contents (same contract as `FrameCount` tests).

Add `lastMetadata Metadata` on `Decoder` (same reuse pattern as `lastMeta` /
`lastFrame`; slices replaced each successful call).

---

## Execution control

- Use only Pi subagents `worker` and `reviewer` per `plans/pi-runbook.md`.
- Orchestrator does not edit product code or run tests.
- Before execution, `plans/api-roadmap.md` must list this plan path and `META-01` status
  `Ready`. If not, stop.
- Branch `meta-01-metadata-apis`: create with
  `git switch -c meta-01-metadata-apis` only when on `main`, branch absent,
  working tree clean.
- Complete and review each task before the next.
- Each committing task: worker implements → Verify → `tmp/commit_message.txt` →
  reviewer → `git commit -F tmp/commit_message.txt` → append hash to
  `tmp/pi_progress.md`.
- Three-attempt limit per task.

---

## Executor STOP gates (mandatory)

**Global rules:** `plans/pi-runbook.md` standing constraint 13; **Plan executor STOP
(zero deviation)** in the same runbook.

Executors **must not** edit this plan file. On any trigger below (or any global rule):
**STOP**, message the user (task id, evidence), **no commit**, **no next task** until
written user direction.

| # | Trigger | Executor action |
| - | ------- | --------------- |
| 1 | Preflight check fails | STOP — notify user |
| 2 | Plan step contradicts runtime, libs, or tests | STOP — notify user |
| 3 | Verify fails for reason not explained by current RED/GREEN step | STOP — notify user |
| 4 | Required file not in task whitelist | STOP — notify user |
| 5 | `make generate` changes files outside `wasm/shim.c`, `wasm/wuffs.wasm`, `internal/wuffswasm/wuffs.go` | STOP — notify user |
| 6 | Upstream fixture missing at `../wuffs/test/data/<name>` | STOP — notify user |
| 7 | Wuffs v0.4 cannot implement a required `API.md` metadata field with the algorithm in this plan | STOP — notify user with shim + Wuffs cite |

---

## Preflight — baseline and roadmap gate

### Worker instructions

1. Working tree and index **clean** before preflight; else STOP.
2. Run the preflight commands below; log to `tmp/meta-01-metadata-apis/preflight.log`.
   If the roadmap gate greps or upstream fixture `test -f` commands fail, **STOP**
   (executor STOP gate 1)—do not copy fixtures or change roadmap status.
3. Copy `plans/api-roadmap.md` →
   `tmp/meta-01-metadata-apis/api-roadmap.pre-execution.md`, `chmod a-w`.
4. Copy all fixtures in **Verified metadata scope** into `testdata/` (preserve
   `artificial-png/` and `artificial-gif/` subdirs).
5. Append provenance lines to `testdata/README`.
6. Record `BASELINE_HEAD`, `BASELINE_PI_SHA256`, `BASELINE_ROADMAP_SHA256`, and
   `BASELINE_ROADMAP_SNAPSHOT` in `tmp/pi_progress.md` (create the file).
7. Change **only** `META-01` in `plans/api-roadmap.md`: status `Ready` →
   `In progress`; plan file `plans/meta-01-metadata-apis.md`.
8. Commit authorized: **no** (uncommitted fixtures + roadmap until Task 1).
   Expect **uncommitted** changes after preflight until Task 1 commits.

### Allowed files

- `testdata/**` (listed fixtures only)
- `testdata/README`
- `plans/api-roadmap.md` (`META-01` `Ready` → `In progress` only)
- `tmp/pi_progress.md`, `tmp/meta-01-metadata-apis/` (gitignored — do not stage)

### Preflight commands

```bash
grep '`CORE-02`' plans/api-roadmap.md | grep -q 'Complete'
grep '`ANIM-01`' plans/api-roadmap.md | grep -q 'Complete'
grep '`META-01`' plans/api-roadmap.md | grep -q 'Ready'
git branch --show-current
git rev-parse HEAD
git status --short
git diff --cached --name-only
git diff --name-only
test -f ../wuffs/test/data/bricks-color.png
test -f ../wuffs/test/data/bricks-dither.png
test -f ../wuffs/test/data/artificial-png/exif.png
test -f ../wuffs/test/data/red-blue-gradient.dcip3d65-no-chrm-no-gama.png
test -f ../wuffs/test/data/DCI-P3-D65.icc
test -f ../wuffs/test/data/artificial-gif/metadata-full.gif
test -f ../wuffs/test/data/artificial-gif/metadata-empty.gif
tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner -cf - .pi | sha256sum
sha256sum tmp/meta-01-metadata-apis/api-roadmap.pre-execution.md
```

### Preflight acceptance

- `CORE-02` and `ANIM-01` are `Complete`; `META-01` is `Ready` (preflight grep).
- All upstream fixtures exist; all listed files are present under `testdata/`
  after preflight step 4.
- `META-01` is `In progress` with plan path `plans/meta-01-metadata-apis.md`.
- `tmp/pi_progress.md` contains baseline hashes.
- Working tree and index were clean before preflight started.
- After preflight, uncommitted product changes remain until Task 1 commits (not a STOP).

---

## Task 1 — Export `Metadata`, `Chromaticities`, and Meta constants (host-only)

### Allowed files

- `metadata.go` (new)
- `metadata_declaration_test.go` (new)
- `api_boundary_test.go`
- `testdata/**` (from preflight)
- `testdata/README`
- `plans/api-roadmap.md` (no status change)

### RED

`TestUnitMetadataTypesAndConstants` (parser style like `frame_declaration_test.go`):

- Type `Metadata` with exported fields exactly per `API.md`:
  `Format`, `EXIF`, `ICC`, `XMP`, `HasGamma`, `Gamma`, `HasChromaticities`,
  `Chromaticities`, `HasSRGB`, `SRGB`, `HasModTime`, `ModTime`.
- Type `Chromaticities` with eight exported `float64` fields (names per `API.md`).
- Constants `MetaEXIF`, `MetaICCP`, `MetaXMP`, `MetaGAMA`, `MetaCHRM`, `MetaSRGB`,
  `MetaMTIM` with exact hex values from `API.md`.
- Extend `TestUnitPublicAPIBoundary` inventory (types + constants only; method in Task 7).

### GREEN

Implement `metadata.go` with godoc matching `API.md`.

### Verify

```bash
go test -run '^TestUnitMetadataTypesAndConstants$' -count=1 -v .
go test -run '^TestUnitPublicAPIBoundary$' -count=1 -v .
make format-check
go build ./...
```

FORBIDDEN before commit: `make test`, `make test-race`, `make cover`, `go test ./...`, or any test command not listed above.

### Acceptance

- Declaration tests pass; no wasm changes.
- Task 1 commit stages `testdata/**` (preflight fixtures), `testdata/README`, and
  `plans/api-roadmap.md` with the preflight `META-01` `In progress` row unchanged
  (Task 1 does not change roadmap status again).

---

## Task 2 — Guest `wuffs_read_image_metadata` + empty PNG metadata

### Allowed files

- `wasm/shim.c`
- `wasm/wuffs.wasm`
- `internal/wuffswasm/wuffs.go`
- `metadata.go`
- `decoder.go` (add `lastMetadata` field only)
- `metadata_pack_test.go` (new) — `TestUnitMetadataPackHeaderSize88` asserts the
  Go mirror struct size is 88 bytes (field layout matches
  `wuffs_wasm_metadata_pack_header` in `wasm/shim.c`)
- `metadata_integration_test.go` (new)
- `export_test.go` (do **not** extend `GuestMemoryState` in this task; metadata uses
  dst scratch only; `assertGuestMemoryPreserved` stays unchanged)

### RED

`TestIntegrationMetadataAbsentBricksColor`:

- `Reserve(0, len(src))` then `Metadata(bricks-color.png)` succeeds.
- `Format == FormatPNG`; `EXIF`, `ICC`, `XMP` nil; `HasGamma`, `HasChromaticities`,
  `HasSRGB`, `HasModTime` false.
- Guest memory layout preserved (`assertGuestMemoryPreserved`).

`TestIntegrationMetadataPackFlagsBricksDither` on `testdata/bricks-dither.png`
(same `Reserve` + `Metadata` pattern):

- After `Metadata` succeeds: `HasChromaticities`, `HasGamma`, and `HasSRGB` are
  all true (guest pack header flags populated; host float/int parsing and oracle
  tolerances remain Task 3).

### GREEN

1. Implement pack header + the full **Wuffs metadata algorithm** (steps 1–9) in
   `read_image_metadata` + export in `wasm/shim.c` (`static_assert` 88-byte
   header). Task 2 integration proof is the no-metadata path only; later tasks
   add format-specific assertions.
2. `make generate`; commit wasm trio.
3. Add `lastMetadata` on `Decoder` in `decoder.go`.
4. Implement `(*Decoder).Metadata` in `metadata.go` only (do not add a second
   `Metadata` method in `decoder.go`).

### Verify

```bash
go test -run '^TestUnitMetadataPackHeaderSize88$' -count=1 -v .
go test -run '^TestIntegrationMetadataAbsentBricksColor$|^TestIntegrationMetadataPackFlagsBricksDither$' -count=1 -v .
make format-check
go build ./...
git diff --exit-code -- wasm/wuffs_config.h
strings wasm/wuffs.wasm | grep -q wuffs_read_image_metadata
```

FORBIDDEN before commit: `make test`, `make test-race`, `make cover`, `go test ./...`, or any test command not listed above.

### Acceptance

- Export present; absent-metadata PNG passes; full guest metadata walk implemented.

---

## Task 3 — PNG parsed cHRM / gAMA / sRGB (`bricks-dither.png`)

### Allowed files

- `metadata.go` (host parsing only)
- `metadata_integration_test.go`

Do not edit `wasm/shim.c` or the wasm trio in this task. Guest failure → STOP
gate 3.

### RED

`TestIntegrationMetadataPNGParsedChunks` — oracle values from
`../wuffs/test/c/std/png.c` `test_wuffs_png_decode_metadata_chrm_gama_srgb` for
`bricks-dither.png`:

- `HasChromaticities` true; `WhiteX` ≈ `0.31270`, `WhiteY` ≈ `0.32900`,
  `RedX` ≈ `0.64`, `RedY` ≈ `0.33`, `GreenX` ≈ `0.30`, `GreenY` ≈ `0.60`,
  `BlueX` ≈ `0.15`, `BlueY` ≈ `0.06` (use `tolerance` 1e-5).
- `HasGamma` true; `Gamma` ≈ `2.2` (from gama `45455` → `100000/45455`).
- `HasSRGB` true; `SRGB == 0` (perceptual).
- `HasModTime` false on this fixture.

### GREEN

Complete host parsing of the guest metadata pack (chromaticity, gamma, sRGB
conversions per **Numeric conversions**). Do not edit `wasm/shim.c` or the wasm
trio; guest defect → STOP gate 3.

### Verify

```bash
go test -run '^TestIntegrationMetadataPNGParsedChunks$' -count=1 -v .
make format-check
go build ./...
```

FORBIDDEN before commit: `make test`, `make test-race`, `make cover`, `go test ./...`, or any test command not listed above.

### Acceptance

- Parsed PNG ancillary metadata matches Wuffs oracle tolerances.

---

## Task 4 — PNG EXIF raw blob (`artificial-png/exif.png`)

### Allowed files

- `metadata.go`
- `metadata_integration_test.go`
- `wasm/shim.c`
- `wasm/wuffs.wasm`
- `internal/wuffswasm/wuffs.go`

### RED

`TestIntegrationMetadataPNGEXIF`:

- `len(EXIF) > 0`; bytes at offset matching Wuffs range oracle: substring
  `"LoremIpsum"` present (see `png.c` `test_wuffs_png_decode_metadata_exif`).

### GREEN

1. Fix guest EXIF passthrough in `wasm/shim.c` (full `tell_me_more` loop per **Wuffs
   metadata algorithm** step 7).
2. Run `make generate`; commit wasm trio when `wasm/shim.c` changes.
3. In `metadata.go`, copy EXIF bytes from the guest pack into Go heap memory only when
   the pack header already reports `exif_len` and blob bytes are present.

### Verify

```bash
go test -run '^TestIntegrationMetadataPNGEXIF$' -count=1 -v .
make format-check
go build ./...
```

FORBIDDEN before commit: `make test`, `make test-race`, `make cover`, `go test ./...`, or any test command not listed above.

### Acceptance

- EXIF blob matches Wuffs range oracle (`LoremIpsum`).

---

## Task 5 — PNG ICC (`red-blue-gradient…` + `DCI-P3-D65.icc` oracle)

### Allowed files

- `metadata.go`
- `metadata_integration_test.go`
- `wasm/shim.c`
- `wasm/wuffs.wasm`
- `internal/wuffswasm/wuffs.go`

### RED

`TestIntegrationMetadataPNGICCP`:

- `bytes.Equal` metadata `ICC` to file `testdata/DCI-P3-D65.icc` (604 bytes).

### GREEN

1. Fix guest ICC passthrough in `wasm/shim.c` (raw passthrough range append per **Wuffs
   metadata algorithm** step 7).
2. Run `make generate`; commit wasm trio when `wasm/shim.c` changes.
3. In `metadata.go`, copy ICC bytes from the guest pack into Go heap memory only when
   the pack header already reports `icc_len` and blob bytes are present.

### Verify

```bash
go test -run '^TestIntegrationMetadataPNGICCP$' -count=1 -v .
make format-check
go build ./...
```

FORBIDDEN before commit: `make test`, `make test-race`, `make cover`, `go test ./...`, or any test command not listed above.

### Acceptance

- ICC bytes equal `testdata/DCI-P3-D65.icc`.

---

## Task 6 — GIF ICC + XMP (`metadata-full.gif`)

### Allowed files

- `scripts/gen_metadata_golden.go` (new)
- `testdata/metadata-full.gif.metadata.golden` (new, committed binary golden)
- `testdata/README` (provenance for golden + script)
- `metadata_integration_test.go`
- `metadata.go`
- `wasm/shim.c`
- `wasm/wuffs.wasm`
- `internal/wuffswasm/wuffs.go`

### RED

`TestIntegrationMetadataGIFChunks`:

- On `testdata/artificial-gif/metadata-full.gif`: `bytes.Equal` on `ICC` and `XMP`
  to the payloads recorded in `testdata/metadata-full.gif.metadata.golden`
  (golden format: 4-byte ICC len, 4-byte XMP len, ICC bytes, XMP bytes,
  little-endian uint32 lengths).
- On `testdata/artificial-gif/metadata-empty.gif`: `ICC` and `XMP` nil.

### GREEN

1. Fix guest GIF ICC/XMP passthrough in `wasm/shim.c` (raw passthrough range
   append per **Wuffs metadata algorithm** step 7 and `gif.c`
   `do_test_wuffs_gif_decode_metadata`).
2. Run `make generate`; commit wasm trio when `wasm/shim.c` changes.
3. In `metadata.go`, copy ICC and XMP bytes from the guest pack into Go heap
   memory only when the pack header already reports `icc_len` / `xmp_len` and
   blob bytes are present.
4. Implement `scripts/gen_metadata_golden.go` with `//go:build ignore` at file
   top (same pattern as `scripts/gen_animation_golden.go`). The script calls
   `wuffs.New()`, `Reserve(65536, len(src))`, `(*Decoder).Metadata(src)` on
   `testdata/artificial-gif/metadata-full.gif`, and writes
   `testdata/metadata-full.gif.metadata.golden`.
5. Run `go run ./scripts/gen_metadata_golden.go` from the repository root; commit
   the golden file.
6. Wire the integration test to the golden file.

### Verify

```bash
go run ./scripts/gen_metadata_golden.go
go test -run '^TestIntegrationMetadataGIFChunks$' -count=1 -v .
make format-check
go build ./...
```

FORBIDDEN before commit: `make test`, `make test-race`, `make cover`, `go test ./...`, or any test command not listed above.

### Acceptance

- GIF metadata-full ICC/XMP bytes match golden; metadata-empty returns nil blobs.

---

## Task 7 — `Metadata` errors, detachment, and decode interaction

### Allowed files

- `metadata.go`
- `metadata_errors_integration_test.go` (new, `package wuffs` — same as
  `decoder_typed_matrix_test.go`, not `package wuffs_test`)
- `api_boundary_test.go` (add `Metadata` to `Decoder` methods inventory)

### RED

`metadata_errors_integration_test.go` is `package wuffs` so tests access
`d.module.Xmemory().Slice()` and mutate guest dst scratch after `Metadata`, matching
`decoder_typed_matrix_test.go` (meta-slot mutation pattern; use `lay.dstOff` and
blob region offsets from the metadata pack layout instead of `lay.metaOff`).

Tests:

- `TestIntegrationMetadataErrors`: empty `src` → `ErrDecode`; oversized `src` →
  `ErrSrcTooLarge`; garbage header → `ErrUnknownFormat`.
- `TestIntegrationMetadataCorruptEXIF`: build corrupt `src` in test only by copying
  `testdata/artificial-png/exif.png` and flipping the final byte; `Metadata` returns
  `ErrDecode` (malformed ancillary metadata inside a valid PNG container).
- `TestIntegrationMetadataDoesNotAliasWasm`: after successful `Metadata` on a fixture
  with non-nil `EXIF`, `ICC`, or `XMP`, capture returned blob slices; mutate bytes in
  wasm memory at `lay.dstOff` through the pack blob region via
  `memBytes := *d.module.Xmemory().Slice()`; confirm returned slices unchanged;
  second `Metadata` call replaces slice contents (new heap copies).
- `TestIntegrationMetadataPreservesDecodeState`: `Probe` then `Metadata` on same
  decoder; `DecodeRGBA` golden unchanged on `bricks-color.png`.
- `TestIntegrationMetadataProbeMetaUnchangedAfterMetadata`: `Probe` on
  `testdata/bricks-color.png`; capture a copy of the returned `*Meta` fields;
  `Metadata` on the same `src`; assert `Probe` meta fields and `d.lastMeta`
  unchanged.
- `TestIntegrationMetadataModTimeAbsent` on `bricks-color.png` and
  `bricks-dither.png`: `HasModTime` false, `ModTime` zero.
- `TestIntegrationMetadataDstSlotTooSmall`: with `SetInitialDstSlotBytes` so
  `dstLen < 65536` (same hook pattern as `decoder_test.go`), `Metadata` on a valid
  PNG returns `ErrDecode` without auto-`Reserve`.
- `TestUnitPublicAPIBoundary` includes `Decoder.Metadata` method.

### GREEN

Finalize `(*Decoder).Metadata` godoc per `API.md`.

### Verify

```bash
go test -run '^TestIntegrationMetadataErrors$|^TestIntegrationMetadataCorruptEXIF$|^TestIntegrationMetadataDoesNotAliasWasm$|^TestIntegrationMetadataPreservesDecodeState$|^TestIntegrationMetadataProbeMetaUnchangedAfterMetadata$|^TestIntegrationMetadataModTimeAbsent$|^TestIntegrationMetadataDstSlotTooSmall$' -count=1 -v .
go test -run '^TestUnitPublicAPIBoundary$' -count=1 -v .
make format-check
go build ./...
```

FORBIDDEN before commit: `make test`, `make test-race`, `make cover`, `go test ./...`, or any test command not listed above.

### Acceptance

- Error contracts, wasm detachment, decode interaction, and API boundary inventory pass.

---

## Task 8 — Documentation and documentation contract

### Allowed files

- `README.md`
- `docs/DEVELOPMENT.md`
- `testdata/README`
- `documentation_contract_integration_test.go`

Do not edit `API.md` in this task.

### RED

Extend `TestIntegrationVerifiedFormatDocumentationInventory` with metadata inventory
assertions. The test must call `requireAll` (closure in
`documentation_contract_integration_test.go`) for these substrings:

**README.md**

- `(*Decoder).Metadata`
- `Metadata` (type name in API surface prose)
- `Chromaticities`
- `MetaEXIF`, `MetaICCP`, `MetaXMP`, `MetaGAMA`, `MetaCHRM`, `MetaSRGB`, `MetaMTIM`
- Absence of `No metadata API`

**documentation_contract_integration_test.go** (new checks in RED, wired in GREEN)

- Same README substrings as above.
- `docs/DEVELOPMENT.md` contains `wuffs_read_image_metadata` and the phrase
  `metadata pack` (host/guest boundary).
- `docs/DEVELOPMENT.md` consumer API bullet lists `Decoder.Metadata` alongside
  `Decoder.Probe`, `Decoder.DecodeRGBA`, `Decoder.FrameCount`, `Decoder.LoopCount`,
  and `Decoder.DecodeFrame`.

### GREEN

Implement the required documentation content below; update
`TestIntegrationVerifiedFormatDocumentationInventory` expectations to match the
documented wording exactly.

### Required documentation content

1. **README.md** — Remove the limitation line `No metadata API`; document
   `(*Decoder).Metadata`, type `Metadata`, type `Chromaticities`, and constants
   `MetaEXIF`, `MetaICCP`, `MetaXMP`, `MetaGAMA`, `MetaCHRM`, `MetaSRGB`, `MetaMTIM`
   at a high level (field names per `API.md`).
2. **docs/DEVELOPMENT.md** — Extend the **Module layout** consumer API bullet to
   include `Decoder.Metadata` with `Decoder.Probe`, `Decoder.DecodeRGBA`,
   `Decoder.FrameCount`, `Decoder.LoopCount`, and `Decoder.DecodeFrame`; add
   `wuffs_read_image_metadata` to the wasm export list; add one bullet describing
   the metadata pack written to guest dst scratch and parsed on the host. Do **not**
   edit the existing mermaid diagram block.
3. **testdata/README** — Metadata fixture provenance remains accurate after prior tasks.

### Verify

```bash
go test -run '^TestIntegrationVerifiedFormatDocumentationInventory$' -count=1 -v .
make format-check
make lint
go build ./...
```

FORBIDDEN before commit: `make test`, `make test-race`, `make cover`, `go test ./...`, or any test command not listed above.

### Acceptance

- Documentation contract test passes; README and DEVELOPMENT.md updated per instructions.

---

## Task 9 — Roadmap `Review`

### Allowed files

- `plans/api-roadmap.md` (`META-01` `In progress` → `Review` only)

### Worker instructions

1. Change only `META-01` status `In progress` → `Review` in `plans/api-roadmap.md`.
2. Commit authorized: **yes** (roadmap row only).

### Verify

```bash
grep '`META-01`' plans/api-roadmap.md | grep -q 'Review'
```

FORBIDDEN before commit: `make test`, `make test-race`, `make cover`, `go test ./...`, or any test command not listed above.

### Acceptance

- `META-01` is `Review` with plan path `plans/meta-01-metadata-apis.md`.

---

## Task 10 — Final gates and `Complete`

### Allowed files

- `plans/api-roadmap.md` (`Review` → `Complete` only, staged alone)
- `tmp/meta-01-metadata-apis/final-gates.log` (gitignored — write but **do not stage**)

### Worker instructions

1. Run every command in **Verify** below; save combined log to
   `tmp/meta-01-metadata-apis/final-gates.log`.
2. Change only `META-01` status `Review` → `Complete` in `plans/api-roadmap.md`.
3. Commit authorized: **yes** (stage and commit `plans/api-roadmap.md` only; record
   log path in `tmp/pi_progress.md`).

### Verify

```bash
make format-check
make lint
make test
make test-race
go build ./...
go doc -all .
go test -run 'Metadata|PublicAPIBoundary|VerifiedFormatDocumentation' -count=1 -v .
git status --porcelain --untracked-files=all
git diff --exit-code main...HEAD -- internal/wuffswasm/wuffs.go wasm/shim.c wasm/wuffs.wasm
```

FORBIDDEN before commit: any file other than `plans/api-roadmap.md` in the Task 10 commit.

### Final acceptance

- All Verify commands pass.
- `META-01` is `Complete` with plan path `plans/meta-01-metadata-apis.md`.
- `tmp/pi_progress.md` lists every task commit hash.
- Working tree clean except ignored `tmp/` artifacts.

---

## Completion report (orchestrator → user)

1. Branch `meta-01-metadata-apis` and final HEAD.
2. Guest export `wuffs_read_image_metadata` + wasm trio SHA-256.
3. Fixture list.
4. Metadata integration test names + pass evidence.
5. Docs updated paths.
6. Roadmap `META-01` `Complete`.
7. `tmp/meta-01-metadata-apis/final-gates.log` path.

Stop after reporting. Do not push unless instructed.

---

## Execution prompt (paste to Pi orchestrator)

```text
Execute plans/meta-01-metadata-apis.md Preflight through Task 10 in order.
Follow plans/pi-runbook.md (worker test budget, commits, format).
Obey Executor STOP gates in the plan and pi-runbook standing constraint 13.
Progress file: tmp/pi_progress.md.
Worker model: fast; do not refactor unrelated code.
Do not push unless I instruct.
Begin with Preflight only after META-01 is Ready in plans/api-roadmap.md.
```
