// WASM host API for Wuffs image decoding (first frame, BGRA premultiplied).
#include "wuffs_config.h"

#include <stdint.h>
#include <string.h>

#include "../../wuffs-mirror-release-c/release/c/wuffs-v0.4.c"

// ── Bump allocator ──────────────────────────────────────────────────────
// Lives above the wasm stack [0, 0x800000) at 0x900000. Rewound at the
// start of every wuffs_decode_image call so that repeated decodes reuse
// the same region.
static uint8_t* bump_ptr  = (uint8_t*)(uintptr_t)0x900000;
static uint8_t* bump_base = (uint8_t*)(uintptr_t)0x900000;

enum { BUMP_LIMIT = 4194304 };  // 4 MiB max per allocation

static void bump_rewind(void) { bump_ptr = bump_base; }

static uint8_t* bump_alloc(uint32_t size) {
  // 8-byte align
  size = (size + 7u) & ~7u;
  uint8_t* p = bump_ptr;
  bump_ptr += size;
  return p;
}

typedef struct wuffs_wasm_decode_meta {
  int32_t err;
  uint32_t width;
  uint32_t height;
  uint32_t stride;
  uint32_t bytes_written;
  uint32_t format;
} wuffs_wasm_decode_meta;

enum {
  WUFFS_WASM_OK = 0,
  WUFFS_WASM_ERR_UNKNOWN_FORMAT = -1,
  WUFFS_WASM_ERR_DST_TOO_SMALL = -2,
  WUFFS_WASM_ERR_DECODE = -3,
  WUFFS_WASM_ERR_BAD_ARG = -4,
};

static uint8_t* mem_ptr(uint32_t off) { return (uint8_t*)(uintptr_t)off; }

// TGA has no reliable magic bytes, so the fallback is a conservative parse of
// the complete 18-byte header, applied only after all signature-bearing
// formats have been checked. Only the supported subset is recognized here
// (broader headers the Wuffs targa decoder might accept stay outside the API
// boundary): image types 1 and 9 (indexed color) with color-map type 1,
// first-entry index 0, length 1..256, entry depth 15/24/32, and 8-bit index
// depth; image types 2 and 10 (true color) with color-map type 0, zeroed
// color-map fields, and pixel depth 15/16/24/32; and image types 3 and 11
// (grayscale) with color-map type 0, zeroed color-map fields, and pixel depth
// 8. Width and height must be nonzero. The descriptor must have interleave
// bits 6-7 and the right-to-left bit 4 clear (vertical-origin bit 5 is free),
// and attribute bits 0-3 must be 0 for every class except 32-bit true color,
// which requires 8. Recognition depends only on these structural header
// fields; image ID, color-map, and pixel payload availability is validated by
// the Wuffs targa decoder, not by this sniff.
static int sniff_tga(const uint8_t* src, uint32_t len) {
  if (len < 18) {
    return 0;
  }

  uint32_t width = (uint32_t)src[12] | ((uint32_t)src[13] << 8);
  uint32_t height = (uint32_t)src[14] | ((uint32_t)src[15] << 8);
  if (width == 0 || height == 0) {
    return 0;
  }

  uint8_t cmap_type = src[1];
  uint8_t img_type = src[2];
  uint16_t cmap_len = (uint16_t)src[5] | ((uint16_t)src[6] << 8);
  uint8_t depth = src[16];
  uint8_t desc = src[17];

  switch (img_type) {
    case 1:
    case 9:
      // Indexed color: color-map type 1, first-entry index 0, length 1..256,
      // entry depth 15/24/32, index depth 8. Wuffs validates that the color
      // map itself is present in the payload.
      if (cmap_type != 1 || src[3] != 0 || src[4] != 0) {
        return 0;
      }
      if (cmap_len < 1 || cmap_len > 256) {
        return 0;
      }
      if (src[7] != 15 && src[7] != 24 && src[7] != 32) {
        return 0;
      }
      if (depth != 8) {
        return 0;
      }
      break;
    case 2:
    case 3:
    case 10:
    case 11:
      // True color (2/10) and grayscale (3/11): color-map type 0 with all
      // color-map fields zeroed. Only the pixel-depth rule differs below.
      if (cmap_type != 0 || src[3] != 0 || src[4] != 0 || src[5] != 0 ||
          src[6] != 0 || src[7] != 0) {
        return 0;
      }
      break;
    default:
      return 0;
  }

  // Pixel depth: true color (2/10) is 15/16/24/32; grayscale (3/11) is 8.
  // Indexed classes (1/9) checked depth 8 inside the switch above.
  if ((img_type == 2 || img_type == 10) &&
      depth != 15 && depth != 16 && depth != 24 && depth != 32) {
    return 0;
  }
  if ((img_type == 3 || img_type == 11) && depth != 8) {
    return 0;
  }

  // Descriptor: right-to-left origin (bit 4) and interleave (bits 6-7) must
  // be clear; vertical-origin bit 5 is unrestricted. Attribute bits 0-3 must
  // be 0 for every class except 32-bit true color, which requires 8.
  if (desc & 0xD0) {
    return 0;
  }
  uint8_t want_attr = 0;
  if ((img_type == 2 || img_type == 10) && depth == 32) {
    want_attr = 8;
  }
  if ((desc & 0x0F) != want_attr) {
    return 0;
  }

  return 1;
}

// WBMP Type 0 has no literal magic bytes: valid files begin with TypeField
// and FixHeaderField zero bytes, followed by canonical width and height
// multi-byte integers, each an unsigned base-128 varint with the
// most-significant 7-bit group first, the continuation bit set on every
// non-final byte and clear on the final byte, and the shortest encoding only
// (the leading group must be nonzero). Dimension zero is rejected, as are
// values above 0xFF_FFFF (16,777,215, whose minimal encoding is
// {87,FF,FF,7F}). Because the header itself is the signature, it is checked
// only after all literal-signature formats and the conservative TGA
// fallback have been given a chance.
static int sniff_wbmp(const uint8_t* src, uint32_t len) {
  if (len < 4 || src[0] != 0 || src[1] != 0) {
    return 0;
  }

  uint32_t i = 2;
  for (uint32_t dim = 0; dim < 2; dim++) {
    uint32_t val = 0;
    uint32_t groups = 0;
    for (;;) {
      if (i >= len) {
        return 0;  // truncated multi-byte integer
      }
      uint8_t c = src[i++];
      groups++;
      // The largest allowed dimension needs exactly four groups; a fifth
      // group can never be part of a shortest, in-range encoding.
      if (groups > 4) {
        return 0;
      }
      // Shortest encoding: the leading group's 7-bit value must be nonzero.
      // Only 0x80 (continuation bit set, value bits clear) violates that; a
      // leading 0x00 terminates immediately and is rejected below.
      if (groups == 1 && c == 0x80) {
        return 0;
      }
      val = (val << 7) | (uint32_t)(c & 0x7F);
      if (c & 0x80) {
        continue;
      }
      if (val == 0 || val > 0x00FFFFFF) {
        return 0;
      }
      break;
    }
  }
  return 1;
}

// HNSM (Handsum) recognition parses the conservative three-byte structural
// header, mirroring do_decode_image_config in decode_handsum.wuffs: the top
// 15 bits of the 24-bit big-endian value must equal 0x7F6B, color class 1
// and the reserved geometry code 0x1F are rejected, and every accepted code
// resolves to one dimension of 16 and the other from 1 through 16.
static int sniff_hnsm(const uint8_t* src, uint32_t len) {
  if (len < 3) {
    return 0;
  }

  uint32_t c32 = ((uint32_t)src[0] << 16) | ((uint32_t)src[1] << 8) |
                 (uint32_t)src[2];
  if ((c32 >> 9) != 0x7F6B) {
    return 0;
  }

  // Color class 1 is reserved; bits 6-5 (quality) are always 0..3.
  if (((c32 >> 7) & 3) == 1) {
    return 0;
  }

  // Geometry code 0x1F is reserved for future expansion.
  if ((c32 & 0x1F) == 0x1F) {
    return 0;
  }

  return 1;
}

static uint32_t sniff_fourcc(const uint8_t* src, uint32_t len) {
  if (len >= 2 && src[0] == 'B' && src[1] == 'M') {
    return WUFFS_BASE__FOURCC__BMP;
  }
  if (len >= 8 && src[0] == 0x89 && src[1] == 'P' && src[2] == 'N' &&
      src[3] == 'G' && src[4] == '\r' && src[5] == '\n' && src[6] == 0x1A &&
      src[7] == '\n') {
    return WUFFS_BASE__FOURCC__PNG;
  }
  if (len >= 3 && src[0] == 0xFF && src[1] == 0xD8 && src[2] == 0xFF) {
    return WUFFS_BASE__FOURCC__JPEG;
  }
  if (len >= 6 && src[0] == 'G' && src[1] == 'I' && src[2] == 'F' &&
      src[3] == '8' && (src[4] == '7' || src[4] == '9') && src[5] == 'a') {
    return WUFFS_BASE__FOURCC__GIF;
  }
  if (len >= 12 && src[0] == 'R' && src[1] == 'I' && src[2] == 'F' &&
      src[3] == 'F' && src[8] == 'W' && src[9] == 'E' && src[10] == 'B' &&
      src[11] == 'P') {
    return WUFFS_BASE__FOURCC__WEBP;
  }
  if (len >= 4 && src[0] == 'q' && src[1] == 'o' && src[2] == 'i' &&
      src[3] == 'f') {
    return WUFFS_BASE__FOURCC__QOI;
  }
  if (len >= 4 && src[0] == 0x6E && src[1] == 0xC3 && src[2] == 0xAF &&
      (src[3] == 0x45 || src[3] == 0x41)) {  // "nïE" still or "nïA" container
    return WUFFS_BASE__FOURCC__NIE;
  }
  if (len >= 4 && src[0] == 'P' &&
      (src[1] == '1' || src[1] == '2' || src[1] == '3' || src[1] == '4' ||
       src[1] == '5' || src[1] == '6') &&
      (src[2] == ' ' || src[2] == '\n' || src[2] == '\r' || src[2] == '\t')) {
    return WUFFS_BASE__FOURCC__NPBM;
  }
  if (len >= 2 && src[0] == 'P' && src[1] == '6') {
    return WUFFS_BASE__FOURCC__NPBM;
  }
  // ETC2 (PKM) has a literal four-byte magic, "PKM ": the complete header
  // (version, format fields, and dimensions) and the pixel payload are
  // validated by the Wuffs etc2 decoder, not by this sniff.
  if (len >= 4 && src[0] == 'P' && src[1] == 'K' && src[2] == 'M' &&
      src[3] == ' ') {
    return WUFFS_BASE__FOURCC__ETC2;
  }
  // ThumbHash (cooked) has a literal three-byte magic, "\xC3\xBE\xFE":
  // "\xC3\xBE" is the UTF-8 encoding of 'þ' (U+00FE LATIN SMALL LETTER
  // THORN) and "\xFE" is the ISO-8859-1 encoding of 'þ'. Wuffs'
  // decode_thumbhash.wuffs requires this prefix unless
  // QUIRK_JUST_RAW_THUMBHASH is enabled; the wasm guest leaves that quirk
  // disabled, so only cooked files beginning {0xC3, 0xBE, 0xFE} are
  // ThumbHash. The payload header and coefficients are validated by the
  // Wuffs thumbhash decoder, not by this sniff.
  if (len >= 3 && src[0] == 0xC3 && src[1] == 0xBE && src[2] == 0xFE) {
    return WUFFS_BASE__FOURCC__TH;
  }
  if (sniff_tga(src, len)) {
    return WUFFS_BASE__FOURCC__TGA;
  }
  if (sniff_wbmp(src, len)) {
    return WUFFS_BASE__FOURCC__WBMP;
  }
  // Handsum (HNSM) has no literal magic bytes: the format is a three-byte
  // structural header followed by an encoded coefficient payload. Because
  // the header itself is the signature, it is checked only after all
  // literal-signature formats and the conservative TGA and WBMP fallbacks
  // have been given a chance. The 24-bit big-endian header's top 15 bits
  // must equal 0x7F6B; of the remaining 9 bits, color (bits 8-7) must be
  // 0, 2, or 3 (1 is reserved), quality (bits 6-5) is always 0..3, and
  // geometry (bits 4-0) must not be the reserved code 0x1F: bit 4 clear
  // decodes to 16 x (g & 15 + 1) and bit 4 set to (g & 15 + 1) x 16, so
  // every accepted code resolves to one dimension of 16 and the other from
  // 1 through 16. The encoded coefficient payload length and its values
  // are validated by the Wuffs handsum decoder, not by this sniff.
  if (sniff_hnsm(src, len)) {
    return WUFFS_BASE__FOURCC__HNSM;
  }
  return 0;
}

typedef struct wuffs_wasm_decoder_slot {
  uint32_t fourcc;
  size_t obj_size;
  wuffs_base__status (*init)(void* self, size_t self_size, uint64_t wuffs_version,
                             uint32_t initialize_flags);
  wuffs_base__image_decoder* (*upcast)(void* self);
} wuffs_wasm_decoder_slot;

#define WUFFS_WASM_DECODER_SLOT(NAME, TYPE)                         \
  {                                                                 \
      WUFFS_BASE__FOURCC__##NAME, sizeof(TYPE),                     \
          (wuffs_base__status(*)(void*, size_t, uint64_t, uint32_t)) \
              TYPE##__initialize,                                   \
          (wuffs_base__image_decoder * (*)(void*))                   \
              TYPE##__upcast_as__wuffs_base__image_decoder,         \
  }

static const wuffs_wasm_decoder_slot k_decoders[] = {
    WUFFS_WASM_DECODER_SLOT(BMP, wuffs_bmp__decoder),
    WUFFS_WASM_DECODER_SLOT(ETC2, wuffs_etc2__decoder),
    WUFFS_WASM_DECODER_SLOT(GIF, wuffs_gif__decoder),
    WUFFS_WASM_DECODER_SLOT(HNSM, wuffs_handsum__decoder),
    WUFFS_WASM_DECODER_SLOT(JPEG, wuffs_jpeg__decoder),
    WUFFS_WASM_DECODER_SLOT(NIE, wuffs_nie__decoder),
    WUFFS_WASM_DECODER_SLOT(NPBM, wuffs_netpbm__decoder),
    WUFFS_WASM_DECODER_SLOT(PNG, wuffs_png__decoder),
    WUFFS_WASM_DECODER_SLOT(QOI, wuffs_qoi__decoder),
    WUFFS_WASM_DECODER_SLOT(TGA, wuffs_targa__decoder),
    WUFFS_WASM_DECODER_SLOT(TH, wuffs_thumbhash__decoder),
    WUFFS_WASM_DECODER_SLOT(WBMP, wuffs_wbmp__decoder),
    WUFFS_WASM_DECODER_SLOT(WEBP, wuffs_webp__decoder),
};

static const wuffs_wasm_decoder_slot* find_decoder(uint32_t fourcc) {
  for (size_t i = 0; i < sizeof(k_decoders) / sizeof(k_decoders[0]); i++) {
    if (k_decoders[i].fourcc == fourcc) {
      return &k_decoders[i];
    }
  }
  return NULL;
}

static int32_t decode_image(uint32_t src_off, uint32_t src_len, uint32_t dst_off,
                            uint32_t dst_cap, uint32_t meta_off) {
  if (src_len == 0 || meta_off == 0) {
    return WUFFS_WASM_ERR_BAD_ARG;
  }

  // Rewind bump allocator for this decode call.
  bump_rewind();

  uint8_t* src_ptr = mem_ptr(src_off);
  uint8_t* dst_ptr = mem_ptr(dst_off);
  wuffs_wasm_decode_meta* meta = (wuffs_wasm_decode_meta*)mem_ptr(meta_off);
  memset(meta, 0, sizeof(*meta));

  uint32_t fourcc = sniff_fourcc(src_ptr, src_len);
  const wuffs_wasm_decoder_slot* slot = find_decoder(fourcc);
  if (slot == NULL) {
    meta->err = WUFFS_WASM_ERR_UNKNOWN_FORMAT;
    return WUFFS_WASM_ERR_UNKNOWN_FORMAT;
  }

  // Guard: reject decoders whose object exceeds bump limit.
  if (slot->obj_size > BUMP_LIMIT) {
    meta->err = WUFFS_WASM_ERR_DECODE;
    return WUFFS_WASM_ERR_DECODE;
  }

  // Allocate decoder from bump region.
  void* dec = bump_alloc((uint32_t)slot->obj_size);
  memset(dec, 0, slot->obj_size);

  wuffs_base__status status =
      slot->init(dec, slot->obj_size, WUFFS_VERSION, 0);
  if (status.repr != NULL) {
    meta->err = WUFFS_WASM_ERR_DECODE;
    return WUFFS_WASM_ERR_DECODE;
  }

  wuffs_base__image_decoder* decoder = slot->upcast(dec);
  if (fourcc == WUFFS_BASE__FOURCC__PNG) {
    wuffs_base__image_decoder__set_quirk(
        decoder, WUFFS_BASE__QUIRK_IGNORE_CHECKSUM, 1);
  }

  wuffs_base__io_buffer src = {
      .data = {.ptr = src_ptr, .len = src_len},
      .meta = {.wi = src_len, .ri = 0, .pos = 0, .closed = 1},
  };

  wuffs_base__image_config ic = {0};
  status = wuffs_base__image_decoder__decode_image_config(decoder, &ic, &src);
  if (status.repr != NULL) {
    meta->err = WUFFS_WASM_ERR_DECODE;
    return WUFFS_WASM_ERR_DECODE;
  }

  wuffs_base__pixel_format pixfmt =
      wuffs_base__make_pixel_format(WUFFS_BASE__PIXEL_FORMAT__BGRA_PREMUL);
  wuffs_base__pixel_config__set(
      &ic.pixcfg, pixfmt.repr, WUFFS_BASE__PIXEL_SUBSAMPLING__NONE,
      wuffs_base__pixel_config__width(&ic.pixcfg),
      wuffs_base__pixel_config__height(&ic.pixcfg));

  uint32_t width = wuffs_base__pixel_config__width(&ic.pixcfg);
  uint32_t height = wuffs_base__pixel_config__height(&ic.pixcfg);
  uint64_t need = (uint64_t)width * (uint64_t)height * 4;
  if (need == 0 || need > dst_cap) {
    meta->width = width;
    meta->height = height;
    meta->stride = width * 4;
    meta->err = WUFFS_WASM_ERR_DST_TOO_SMALL;
    return WUFFS_WASM_ERR_DST_TOO_SMALL;
  }

  // Allocate workbuf sized by the decoder (not hardcoded).
  // Wuffs reports workbuf length as range [L, L]; use max_incl as the byte length.
  wuffs_base__range_ii_u64 wb_range = wuffs_base__image_decoder__workbuf_len(decoder);
  if (wb_range.min_incl > wb_range.max_incl) {
    meta->err = WUFFS_WASM_ERR_DECODE;
    return WUFFS_WASM_ERR_DECODE;
  }
  uint32_t wb_len = (uint32_t)wb_range.max_incl;
  if (wb_len == 0) {
    wb_len = 64 * 1024;  // minimum 64 KiB fallback
  }
  uint8_t* workbuf = bump_alloc(wb_len);
  wuffs_base__slice_u8 work_slice = {
      .ptr = workbuf,
      .len = wb_len,
  };

  wuffs_base__pixel_buffer pb = {0};
  wuffs_base__slice_u8 pix_slice = {.ptr = dst_ptr, .len = dst_cap};
  status = wuffs_base__pixel_buffer__set_from_slice(&pb, &ic.pixcfg, pix_slice);
  if (status.repr != NULL) {
    meta->err = WUFFS_WASM_ERR_DECODE;
    return WUFFS_WASM_ERR_DECODE;
  }

  wuffs_base__frame_config fc = {0};
  status = wuffs_base__image_decoder__decode_frame_config(decoder, &fc, &src);
  if (status.repr != NULL && status.repr != wuffs_base__note__end_of_data) {
    meta->err = WUFFS_WASM_ERR_DECODE;
    return WUFFS_WASM_ERR_DECODE;
  }

  status = wuffs_base__image_decoder__decode_frame(
      decoder, &pb, &src, WUFFS_BASE__PIXEL_BLEND__SRC, work_slice, NULL);
  if (status.repr != NULL) {
    meta->err = WUFFS_WASM_ERR_DECODE;
    return WUFFS_WASM_ERR_DECODE;
  }

  meta->err = WUFFS_WASM_OK;
  meta->width = width;
  meta->height = height;
  meta->stride = width * 4;
  meta->bytes_written = (uint32_t)need;
  meta->format = fourcc;
  return WUFFS_WASM_OK;
}

// probe_image reports image dimensions, stride, and format from src without
// decoding pixels. It mirrors decode_image up to decode_image_config, then
// overrides the pixel config to the BGRA_PREMUL dest layout (4 bytes/pixel,
// stride = width * 4) used by decode_image. It allocates no frame, workbuf,
// or dst, and writes no destination pixels; bytes_written stays 0.
static int32_t probe_image(uint32_t src_off, uint32_t src_len,
                           uint32_t meta_off) {
  if (src_len == 0 || meta_off == 0) {
    return WUFFS_WASM_ERR_BAD_ARG;
  }

  // Rewind bump allocator for this probe call.
  bump_rewind();

  uint8_t* src_ptr = mem_ptr(src_off);
  wuffs_wasm_decode_meta* meta = (wuffs_wasm_decode_meta*)mem_ptr(meta_off);
  memset(meta, 0, sizeof(*meta));

  uint32_t fourcc = sniff_fourcc(src_ptr, src_len);
  const wuffs_wasm_decoder_slot* slot = find_decoder(fourcc);
  if (slot == NULL) {
    meta->err = WUFFS_WASM_ERR_UNKNOWN_FORMAT;
    return WUFFS_WASM_ERR_UNKNOWN_FORMAT;
  }

  // Guard: reject decoders whose object exceeds bump limit.
  if (slot->obj_size > BUMP_LIMIT) {
    meta->err = WUFFS_WASM_ERR_DECODE;
    return WUFFS_WASM_ERR_DECODE;
  }

  // Allocate decoder from bump region.
  void* dec = bump_alloc((uint32_t)slot->obj_size);
  memset(dec, 0, slot->obj_size);

  wuffs_base__status status =
      slot->init(dec, slot->obj_size, WUFFS_VERSION, 0);
  if (status.repr != NULL) {
    meta->err = WUFFS_WASM_ERR_DECODE;
    return WUFFS_WASM_ERR_DECODE;
  }

  wuffs_base__image_decoder* decoder = slot->upcast(dec);
  if (fourcc == WUFFS_BASE__FOURCC__PNG) {
    wuffs_base__image_decoder__set_quirk(
        decoder, WUFFS_BASE__QUIRK_IGNORE_CHECKSUM, 1);
  }

  wuffs_base__io_buffer src = {
      .data = {.ptr = src_ptr, .len = src_len},
      .meta = {.wi = src_len, .ri = 0, .pos = 0, .closed = 1},
  };

  wuffs_base__image_config ic = {0};
  status = wuffs_base__image_decoder__decode_image_config(decoder, &ic, &src);
  if (status.repr != NULL) {
    meta->err = WUFFS_WASM_ERR_DECODE;
    return WUFFS_WASM_ERR_DECODE;
  }

  // Override the pixel config to the BGRA_PREMUL dest layout that
  // decode_image uses (DecodeRGBA dest is 4 bpp). Stride = width * 4.
  wuffs_base__pixel_format pixfmt =
      wuffs_base__make_pixel_format(WUFFS_BASE__PIXEL_FORMAT__BGRA_PREMUL);
  wuffs_base__pixel_config__set(
      &ic.pixcfg, pixfmt.repr, WUFFS_BASE__PIXEL_SUBSAMPLING__NONE,
      wuffs_base__pixel_config__width(&ic.pixcfg),
      wuffs_base__pixel_config__height(&ic.pixcfg));

  uint32_t width = wuffs_base__pixel_config__width(&ic.pixcfg);
  uint32_t height = wuffs_base__pixel_config__height(&ic.pixcfg);

  meta->err = WUFFS_WASM_OK;
  meta->width = width;
  meta->height = height;
  meta->stride = width * 4;
  meta->bytes_written = 0;
  meta->format = fourcc;
  return WUFFS_WASM_OK;
}

__attribute__((export_name("wuffs_probe_image"))) int32_t wuffs_wasm_probe_image(
    uint32_t src_off, uint32_t src_len, uint32_t meta_off) {
  return probe_image(src_off, src_len, meta_off);
}

__attribute__((export_name("wuffs_version"))) uint32_t wuffs_wasm_version(void) {
  return (uint32_t)WUFFS_VERSION;
}

__attribute__((export_name("wuffs_decode_image"))) int32_t wuffs_wasm_decode_image(
    uint32_t src_off, uint32_t src_len, uint32_t dst_off, uint32_t dst_cap,
    uint32_t meta_off) {
  return decode_image(src_off, src_len, dst_off, dst_cap, meta_off);
}
