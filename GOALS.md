# Goals

These are goals **you stated**. Wording is yours. Dates are from Cursor
transcripts. Nothing here is an agent invention.

There was **no GOALS.md or design doc in this repo** after the early sessions.
The grilling (17 Aug 2026) produced a TDD YAML under `.pi/tdd-plans/` (gitignored
via `.pi/`). That is why it is hard to find.

---

## 17 Aug 2026 — product constraints

- CGO is not an option. Image codecs only.
- Route: compile Wuffs to WASM; Go module via wasm2go + wasm2go-wasi-host.
  Consumer API in the root package. Generated bindings in
  `internal/wuffswasm/wuffs.go` only. WASI host in the root.
- “In order for wuffs to be advantageous as a go module, its go module needs to
  be zero-allocation or very, very close.”
- Concurrent decode: multiple `Decoder` instances, each used from one goroutine.
  One instance is not concurrent.
- No hard 9MP cap. Do not pre-assign 50MP (or larger) on every `New()`.
- “No, this is Go. image.Image formats rule. We need to be able to generate at a
  minimum image.RGBA. Unlike the stdlib image libraries. We **MUST** be able to
  pass it a pre-allocated place in which to write.”

You then **Agreed** (same session) to the grilling restatement of that last
goal:

- `DecodeRGBA(dst *image.RGBA, src []byte)`
- Caller **must** pre-allocate `dst.Pix` and set `dst.Rect` and `dst.Stride`
  before calling.
- Library **never** allocates `Pix`.
- Guest decodes BGRA premul in wasm; host converts into **the caller’s**
  `dst.Pix` as straight RGBA. Zero heap allocs on the success path when `dst` is
  already sized (`AllocsPerRun` after `Reserve`).
- Guest memory grows via `Reserve`, not a fixed megapixel cap.

---

## 22–23 Aug 2026 — package scope

- Cover Wuffs image formats incrementally. Show what is and is not verified.
- Product boundary: **image decode only** (no compression/hash/JSON as a public
  API).
- “I want to be able to decode to a pre-allocated variable.”
- Spell out the shape of remaining methods/types; definitions can wait.
- “I think we should include Probe in the plan. It is logically the first thing
  to call.”

---

## 25 Aug 2026 — restated (same product, after the aliasing failure)

- “The whole fucking goal of creating wuffs-wasm is to populate an existing
  image.RGBA and not have to reallocate it every fucking decode.”

That is the 17 Aug `Pix` contract in one sentence. The repo still does not do
it. See `HANDOVER.md`.
