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

// wuffs_frame_count walks frame configs only (no pixel decompress) and
// writes the number of frames to *count_out (int32). It returns
// WUFFS_WASM_OK on success or a negative WUFFS_WASM_ERR_* code.
static int32_t frame_count(uint32_t src_off, uint32_t src_len,
                           uint32_t count_out_off) {
  if (src_len == 0 || count_out_off == 0) {
    return WUFFS_WASM_ERR_BAD_ARG;
  }

  // Rewind bump allocator for this call.
  bump_rewind();

  uint8_t* src_ptr = mem_ptr(src_off);
  int32_t* count_out = (int32_t*)mem_ptr(count_out_off);
  *count_out = 0;

  uint32_t fourcc = sniff_fourcc(src_ptr, src_len);
  const wuffs_wasm_decoder_slot* slot = find_decoder(fourcc);
  if (slot == NULL) {
    return WUFFS_WASM_ERR_UNKNOWN_FORMAT;
  }

  // Guard: reject decoders whose object exceeds bump limit.
  if (slot->obj_size > BUMP_LIMIT) {
    return WUFFS_WASM_ERR_DECODE;
  }

  // Allocate decoder from bump region.
  void* dec = bump_alloc((uint32_t)slot->obj_size);
  memset(dec, 0, slot->obj_size);

  wuffs_base__status status =
      slot->init(dec, slot->obj_size, WUFFS_VERSION, 0);
  if (status.repr != NULL) {
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
    return WUFFS_WASM_ERR_DECODE;
  }

  int32_t count = 0;
  for (;;) {
    wuffs_base__frame_config fc = {0};
    status = wuffs_base__image_decoder__decode_frame_config(decoder, &fc, &src);
    if (status.repr == wuffs_base__note__end_of_data) {
      break;
    }
    if (status.repr != NULL) {
      return WUFFS_WASM_ERR_DECODE;
    }
    count++;
  }

  *count_out = count;
  return WUFFS_WASM_OK;
}

__attribute__((export_name("wuffs_frame_count"))) int32_t wuffs_wasm_frame_count(
    uint32_t src_off, uint32_t src_len, uint32_t count_out_off) {
  return frame_count(src_off, src_len, count_out_off);
}

// Writes uint32 loop count to *loops_out_off using num_animation_loops .
static int32_t animation_loops(uint32_t src_off, uint32_t src_len,
                               uint32_t loops_out_off) {
  if (src_len == 0 || loops_out_off == 0) {
    return WUFFS_WASM_ERR_BAD_ARG;
  }

  // Rewind bump allocator for this call.
  bump_rewind();

  uint8_t* src_ptr = mem_ptr(src_off);
  uint32_t* loops_out = (uint32_t*)mem_ptr(loops_out_off);
  *loops_out = 0;

  uint32_t fourcc = sniff_fourcc(src_ptr, src_len);
  const wuffs_wasm_decoder_slot* slot = find_decoder(fourcc);
  if (slot == NULL) {
    return WUFFS_WASM_ERR_UNKNOWN_FORMAT;
  }

  // Guard: reject decoders whose object exceeds bump limit.
  if (slot->obj_size > BUMP_LIMIT) {
    return WUFFS_WASM_ERR_DECODE;
  }

  // Allocate decoder from bump region.
  void* dec = bump_alloc((uint32_t)slot->obj_size);
  memset(dec, 0, slot->obj_size);

  wuffs_base__status status =
      slot->init(dec, slot->obj_size, WUFFS_VERSION, 0);
  if (status.repr != NULL) {
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
    return WUFFS_WASM_ERR_DECODE;
  }

  *loops_out = wuffs_base__image_decoder__num_animation_loops(decoder);
  return WUFFS_WASM_OK;
}

__attribute__((export_name("wuffs_animation_loops"))) int32_t
wuffs_wasm_animation_loops(uint32_t src_off, uint32_t src_len,
                           uint32_t loops_out_off) {
  return animation_loops(src_off, src_len, loops_out_off);
}

// wuffs_wasm_frame_meta — written by decode_frame export; zeroed before call.
typedef struct wuffs_wasm_frame_meta {
  int32_t err;
  uint32_t index;
  int32_t bounds_min_x;
  int32_t bounds_min_y;
  int32_t bounds_max_x;
  int32_t bounds_max_y;
  uint64_t duration_flicks;
  uint64_t io_position;
  uint8_t disposal;    // Wuffs animation_disposal 0..2
  uint8_t overwrite;   // 1 = overwrite_instead_of_blend
  uint8_t opaque;      // 1 = opaque_within_bounds
  uint8_t bg_r;
  uint8_t bg_g;
  uint8_t bg_b;
  uint8_t bg_a;
} wuffs_wasm_frame_meta;  // 48 bytes, 8-byte aligned

// Decodes frame `index` (0-based). decode_meta_off is wuffs_wasm_decode_meta;
// frame_meta_off is wuffs_wasm_frame_meta. Decodes into full canvas scratch at
// dst_off with capacity dst_cap (same layout as wuffs_decode_image).
static int32_t decode_frame(uint32_t src_off, uint32_t src_len,
                            uint32_t dst_off, uint32_t dst_cap,
                            uint32_t decode_meta_off, uint32_t frame_meta_off,
                            int32_t index) {
  if (src_len == 0 || decode_meta_off == 0 || frame_meta_off == 0 ||
      index < 0) {
    wuffs_wasm_decode_meta* dm =
        decode_meta_off ? (wuffs_wasm_decode_meta*)mem_ptr(decode_meta_off)
                        : NULL;
    if (dm) {
      memset(dm, 0, sizeof(*dm));
      dm->err = WUFFS_WASM_ERR_DECODE;
    }
    return WUFFS_WASM_ERR_DECODE;
  }

  // Rewind bump allocator for this call.
  bump_rewind();

  uint8_t* src_ptr = mem_ptr(src_off);
  uint8_t* dst_ptr = mem_ptr(dst_off);
  wuffs_wasm_decode_meta* dec_meta =
      (wuffs_wasm_decode_meta*)mem_ptr(decode_meta_off);
  wuffs_wasm_frame_meta* frame_meta =
      (wuffs_wasm_frame_meta*)mem_ptr(frame_meta_off);
  memset(dec_meta, 0, sizeof(*dec_meta));
  memset(frame_meta, 0, sizeof(*frame_meta));

  uint32_t fourcc = sniff_fourcc(src_ptr, src_len);
  const wuffs_wasm_decoder_slot* slot = find_decoder(fourcc);
  if (slot == NULL) {
    dec_meta->err = WUFFS_WASM_ERR_UNKNOWN_FORMAT;
    frame_meta->err = WUFFS_WASM_ERR_UNKNOWN_FORMAT;
    return WUFFS_WASM_ERR_UNKNOWN_FORMAT;
  }

  // Guard: reject decoders whose object exceeds bump limit.
  if (slot->obj_size > BUMP_LIMIT) {
    dec_meta->err = WUFFS_WASM_ERR_DECODE;
    frame_meta->err = WUFFS_WASM_ERR_DECODE;
    return WUFFS_WASM_ERR_DECODE;
  }

  // Allocate decoder from bump region.
  void* dec = bump_alloc((uint32_t)slot->obj_size);
  memset(dec, 0, slot->obj_size);

  wuffs_base__status status =
      slot->init(dec, slot->obj_size, WUFFS_VERSION, 0);
  if (status.repr != NULL) {
    dec_meta->err = WUFFS_WASM_ERR_DECODE;
    frame_meta->err = WUFFS_WASM_ERR_DECODE;
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
    dec_meta->err = WUFFS_WASM_ERR_DECODE;
    frame_meta->err = WUFFS_WASM_ERR_DECODE;
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
    dec_meta->width = width;
    dec_meta->height = height;
    dec_meta->stride = width * 4;
    dec_meta->err = WUFFS_WASM_ERR_DST_TOO_SMALL;
    frame_meta->err = WUFFS_WASM_ERR_DST_TOO_SMALL;
    return WUFFS_WASM_ERR_DST_TOO_SMALL;
  }

  // Allocate workbuf sized by the decoder (not hardcoded).
  wuffs_base__range_ii_u64 wb_range =
      wuffs_base__image_decoder__workbuf_len(decoder);
  if (wb_range.min_incl > wb_range.max_incl) {
    dec_meta->err = WUFFS_WASM_ERR_DECODE;
    frame_meta->err = WUFFS_WASM_ERR_DECODE;
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
    dec_meta->err = WUFFS_WASM_ERR_DECODE;
    frame_meta->err = WUFFS_WASM_ERR_DECODE;
    return WUFFS_WASM_ERR_DECODE;
  }
  memset(dst_ptr, 0, (size_t)need);

  // Advance to frame index via decode_frame_config (which skips prior
  // frames' pixel data), then decode only that frame with SRC (replace)
  // blend so scratch's Bounds region holds the indexed frame's own delta
  // pixels. No prior-frame compositing, no disposal/background replay.
  wuffs_base__frame_config fc = {0};
  for (int32_t i = 0; i <= index; i++) {
    status = wuffs_base__image_decoder__decode_frame_config(decoder, &fc, &src);
    if (status.repr != NULL) {
      // Out-of-range index (end_of_data before reaching index) or corrupt
      // stream: report WUFFS_WASM_ERR_DECODE on both metas.
      dec_meta->err = WUFFS_WASM_ERR_DECODE;
      frame_meta->err = WUFFS_WASM_ERR_DECODE;
      return WUFFS_WASM_ERR_DECODE;
    }
  }
  status = wuffs_base__image_decoder__decode_frame(
      decoder, &pb, &src, WUFFS_BASE__PIXEL_BLEND__SRC, work_slice, NULL);
  if (status.repr != NULL) {
    dec_meta->err = WUFFS_WASM_ERR_DECODE;
    frame_meta->err = WUFFS_WASM_ERR_DECODE;
    return WUFFS_WASM_ERR_DECODE;
  }

  wuffs_base__rect_ie_u32 bounds = wuffs_base__frame_config__bounds(&fc);
  uint32_t bg = wuffs_base__frame_config__background_color(&fc);
  uint32_t bg_nonpremul =
      wuffs_base__color_u32_argb_premul__as__color_u32_argb_nonpremul(bg);

  frame_meta->err = WUFFS_WASM_OK;
  frame_meta->index = (uint32_t)index;
  frame_meta->bounds_min_x = (int32_t)bounds.min_incl_x;
  frame_meta->bounds_min_y = (int32_t)bounds.min_incl_y;
  frame_meta->bounds_max_x = (int32_t)bounds.max_excl_x;
  frame_meta->bounds_max_y = (int32_t)bounds.max_excl_y;
  frame_meta->duration_flicks = (uint64_t)wuffs_base__frame_config__duration(&fc);
  frame_meta->io_position = wuffs_base__frame_config__io_position(&fc);
  frame_meta->disposal = wuffs_base__frame_config__disposal(&fc);
  frame_meta->overwrite =
      wuffs_base__frame_config__overwrite_instead_of_blend(&fc) ? 1 : 0;
  frame_meta->opaque =
      wuffs_base__frame_config__opaque_within_bounds(&fc) ? 1 : 0;
  frame_meta->bg_r = (uint8_t)(bg_nonpremul >> 16);
  frame_meta->bg_g = (uint8_t)(bg_nonpremul >> 8);
  frame_meta->bg_b = (uint8_t)bg_nonpremul;
  frame_meta->bg_a = (uint8_t)(bg_nonpremul >> 24);

  dec_meta->err = WUFFS_WASM_OK;
  dec_meta->width = width;
  dec_meta->height = height;
  dec_meta->stride = width * 4;
  dec_meta->bytes_written = (uint32_t)need;
  dec_meta->format = fourcc;
  return WUFFS_WASM_OK;
}

__attribute__((export_name("wuffs_decode_frame"))) int32_t
wuffs_wasm_decode_frame(uint32_t src_off, uint32_t src_len, uint32_t dst_off,
                         uint32_t dst_cap, uint32_t decode_meta_off,
                         uint32_t frame_meta_off, int32_t index) {
  return decode_frame(src_off, src_len, dst_off, dst_cap, decode_meta_off,
                       frame_meta_off, index);
}

// wuffs_wasm_metadata_pack_header — written by read_image_metadata at
// pack_off; header is little-endian, blobs follow at pack_off+88 in the
// fixed order EXIF, ICC, XMP. Size MUST be 88 bytes (static_assert below).
typedef struct wuffs_wasm_metadata_pack_header {
  int32_t err;
  uint32_t format;
  uint32_t exif_len;
  uint32_t icc_len;
  uint32_t xmp_len;
  uint8_t has_gamma;
  uint8_t has_chrm;
  uint8_t has_srgb;
  uint8_t has_modtime;
  uint8_t _pad0[3];
  uint32_t gama_scaled;
  int32_t chrm[8];
  uint32_t srgb_intent;
  int64_t modtime_sec;
  int32_t modtime_nsec;
} wuffs_wasm_metadata_pack_header;

_Static_assert(sizeof(wuffs_wasm_metadata_pack_header) == 88,
               "metadata pack header must be 88 bytes");

enum { WUFFS_WASM_METADATA_PACK_HEADER_SIZE = 88 };

// Raw-transform destination length (1 MiB) and ICC accumulator capacity
// (1 MiB, so a full decompressed profile fits). EXIF and XMP accumulators
// stay src_len: their bytes are subranges of src.
enum { WUFFS_WASM_METADATA_HAVE_LEN = 1048576 };
enum { WUFFS_WASM_METADATA_ICC_CAP = 1048576 };

// read_image_metadata walks decode_image_config with set_report_metadata
// enabled for EXIF, ICCP, XMP, GAMA, CHRM, SRGB, and MTIM, accumulating raw
// passthrough blobs (EXIF/ICC/XMP) and parsed values (GAMA/CHRM/SRGB) into
// the pack at pack_off. It decodes no pixels and calls no decode_frame.
static int32_t read_image_metadata(uint32_t src_off, uint32_t src_len,
                                   uint32_t pack_off, uint32_t pack_cap) {
  if (pack_off == 0 || src_len == 0) {
    return WUFFS_WASM_ERR_BAD_ARG;
  }

  // Rewind bump allocator for this call.
  bump_rewind();

  uint8_t* src_ptr = mem_ptr(src_off);
  wuffs_wasm_metadata_pack_header* hdr =
      (wuffs_wasm_metadata_pack_header*)mem_ptr(pack_off);

  uint32_t fourcc = sniff_fourcc(src_ptr, src_len);
  const wuffs_wasm_decoder_slot* slot = find_decoder(fourcc);
  if (slot == NULL) {
    memset(hdr, 0, sizeof(*hdr));
    hdr->err = WUFFS_WASM_ERR_UNKNOWN_FORMAT;
    hdr->format = 0;
    return WUFFS_WASM_ERR_UNKNOWN_FORMAT;
  }

  // Guard: the decoder object plus the temp blob buffers must fit the bump
  // region. EXIF and XMP metadata bytes are subranges of src, so one
  // src_len buffer per kind always suffices. The PNG iCCP profile arrives
  // zlib-compressed via METADATA_RAW_TRANSFORM and is decompressed by Wuffs
  // into the caller-supplied destination buffer, so the destination backing
  // store and the ICC accumulator are each 1 MiB.
  uint64_t blob_need = (uint64_t)WUFFS_WASM_METADATA_HAVE_LEN +
                       (uint64_t)WUFFS_WASM_METADATA_ICC_CAP +
                       (uint64_t)2 * (uint64_t)src_len;
  if ((uint64_t)slot->obj_size > (uint64_t)BUMP_LIMIT ||
      blob_need > (uint64_t)BUMP_LIMIT ||
      (uint64_t)slot->obj_size + blob_need > (uint64_t)BUMP_LIMIT) {
    memset(hdr, 0, sizeof(*hdr));
    hdr->err = WUFFS_WASM_ERR_DECODE;
    hdr->format = fourcc;
    return WUFFS_WASM_ERR_DECODE;
  }

  // Allocate decoder object first, then the destination backing store and
  // the three temp blob buffers.
  void* dec = bump_alloc((uint32_t)slot->obj_size);
  memset(dec, 0, slot->obj_size);
  uint8_t* have_ptr = bump_alloc(WUFFS_WASM_METADATA_HAVE_LEN);
  uint8_t* icc_buf = bump_alloc(WUFFS_WASM_METADATA_ICC_CAP);
  uint8_t* exif_buf = bump_alloc(src_len);
  uint8_t* xmp_buf = bump_alloc(src_len);
  uint32_t exif_len = 0;
  uint32_t icc_len = 0;
  uint32_t xmp_len = 0;

  // Destination buffer for METADATA_RAW_TRANSFORM items (PNG iCCP): Wuffs
  // decompresses into have.data and reports bytes via have.meta.wi. PARSED
  // and RAW_PASSTHROUGH flavors ignore a_dst, so passing `have` universally
  // is safe.
  wuffs_base__io_buffer have = {
      .data = {.ptr = have_ptr, .len = WUFFS_WASM_METADATA_HAVE_LEN},
      .meta = {.wi = 0, .ri = 0, .pos = 0, .closed = 0},
  };

  wuffs_base__status status =
      slot->init(dec, slot->obj_size, WUFFS_VERSION, 0);
  if (status.repr != NULL) {
    memset(hdr, 0, sizeof(*hdr));
    hdr->err = WUFFS_WASM_ERR_DECODE;
    hdr->format = fourcc;
    return WUFFS_WASM_ERR_DECODE;
  }

  wuffs_base__image_decoder* decoder = slot->upcast(dec);
  if (fourcc == WUFFS_BASE__FOURCC__PNG) {
    wuffs_base__image_decoder__set_quirk(
        decoder, WUFFS_BASE__QUIRK_IGNORE_CHECKSUM, 1);
  }

  wuffs_base__image_decoder__set_report_metadata(
      decoder, WUFFS_BASE__FOURCC__EXIF, true);
  wuffs_base__image_decoder__set_report_metadata(
      decoder, WUFFS_BASE__FOURCC__ICCP, true);
  wuffs_base__image_decoder__set_report_metadata(
      decoder, WUFFS_BASE__FOURCC__XMP, true);
  wuffs_base__image_decoder__set_report_metadata(
      decoder, WUFFS_BASE__FOURCC__GAMA, true);
  wuffs_base__image_decoder__set_report_metadata(
      decoder, WUFFS_BASE__FOURCC__CHRM, true);
  wuffs_base__image_decoder__set_report_metadata(
      decoder, WUFFS_BASE__FOURCC__SRGB, true);
  wuffs_base__image_decoder__set_report_metadata(
      decoder, WUFFS_BASE__FOURCC__MTIM, true);

  wuffs_base__io_buffer src = {
      .data = {.ptr = src_ptr, .len = src_len},
      .meta = {.wi = src_len, .ri = 0, .pos = 0, .closed = 1},
  };

  uint8_t has_gamma = 0;
  uint8_t has_chrm = 0;
  uint8_t has_srgb = 0;
  uint32_t gama_scaled = 0;
  int32_t chrm[8] = {0};
  uint32_t srgb_intent = 0;

  wuffs_base__image_config ic = {0};
  for (;;) {
    status = wuffs_base__image_decoder__decode_image_config(decoder, &ic, &src);
    if (status.repr == NULL) {
      break;
    }
    if (status.repr != wuffs_base__note__metadata_reported) {
      memset(hdr, 0, sizeof(*hdr));
      hdr->err = WUFFS_WASM_ERR_DECODE;
      hdr->format = fourcc;
      return WUFFS_WASM_ERR_DECODE;
    }

    // Consume one reported metadata item via tell_me_more.
    for (;;) {
      have.meta.wi = 0;
      have.meta.ri = 0;
      wuffs_base__more_information minfo =
          wuffs_base__empty_more_information();
      status =
          wuffs_base__image_decoder__tell_me_more(decoder, &have, &minfo, &src);
      if (wuffs_base__status__is_error(&status)) {
        memset(hdr, 0, sizeof(*hdr));
        hdr->err = WUFFS_WASM_ERR_DECODE;
        hdr->format = fourcc;
        return WUFFS_WASM_ERR_DECODE;
      }
      if (minfo.flavor ==
          WUFFS_BASE__MORE_INFORMATION__FLAVOR__METADATA_PARSED) {
        uint32_t mfourcc =
            wuffs_base__more_information__metadata__fourcc(&minfo);
        if (mfourcc == WUFFS_BASE__FOURCC__CHRM) {
          for (uint32_t i = 0; i < 8; i++) {
            chrm[i] =
                wuffs_base__more_information__metadata_parsed__chrm(&minfo, i);
          }
          has_chrm = 1;
        } else if (mfourcc == WUFFS_BASE__FOURCC__GAMA) {
          gama_scaled =
              wuffs_base__more_information__metadata_parsed__gama(&minfo);
          has_gamma = 1;
        } else if (mfourcc == WUFFS_BASE__FOURCC__SRGB) {
          srgb_intent =
              wuffs_base__more_information__metadata_parsed__srgb(&minfo);
          has_srgb = 1;
        }
        // MTIM has no v0.4 parsed helper and is never delivered; ignore.
        break;
      }
      if (minfo.flavor ==
          WUFFS_BASE__MORE_INFORMATION__FLAVOR__METADATA_RAW_PASSTHROUGH) {
        uint32_t mfourcc =
            wuffs_base__more_information__metadata__fourcc(&minfo);
        wuffs_base__range_ie_u64 r =
            wuffs_base__more_information__metadata_raw_passthrough__range(
                &minfo);
        uint64_t n = wuffs_base__range_ie_u64__length(&r);
        if (n > 0) {
          // Ranges index src bytes; reject ranges outside [0, src_len].
          if (r.min_incl > r.max_excl || r.max_excl > (uint64_t)src_len) {
            memset(hdr, 0, sizeof(*hdr));
            hdr->err = WUFFS_WASM_ERR_DECODE;
            hdr->format = fourcc;
            return WUFFS_WASM_ERR_DECODE;
          }
          if (mfourcc == WUFFS_BASE__FOURCC__EXIF) {
            if ((uint64_t)exif_len + n > (uint64_t)src_len) {
              memset(hdr, 0, sizeof(*hdr));
              hdr->err = WUFFS_WASM_ERR_DECODE;
              hdr->format = fourcc;
              return WUFFS_WASM_ERR_DECODE;
            }
            memcpy(exif_buf + exif_len, src_ptr + r.min_incl, (size_t)n);
            exif_len += (uint32_t)n;
          } else if (mfourcc == WUFFS_BASE__FOURCC__ICCP) {
            if ((uint64_t)icc_len + n > (uint64_t)src_len) {
              memset(hdr, 0, sizeof(*hdr));
              hdr->err = WUFFS_WASM_ERR_DECODE;
              hdr->format = fourcc;
              return WUFFS_WASM_ERR_DECODE;
            }
            memcpy(icc_buf + icc_len, src_ptr + r.min_incl, (size_t)n);
            icc_len += (uint32_t)n;
          } else if (mfourcc == WUFFS_BASE__FOURCC__XMP) {
            if ((uint64_t)xmp_len + n > (uint64_t)src_len) {
              memset(hdr, 0, sizeof(*hdr));
              hdr->err = WUFFS_WASM_ERR_DECODE;
              hdr->format = fourcc;
              return WUFFS_WASM_ERR_DECODE;
            }
            memcpy(xmp_buf + xmp_len, src_ptr + r.min_incl, (size_t)n);
            xmp_len += (uint32_t)n;
          }
          // Other raw kinds are acknowledged by advancing but not stored.
          if (r.max_excl > src.meta.ri) {
            src.meta.ri = (size_t)r.max_excl;
          }
        }
        if (wuffs_base__status__is_ok(&status)) {
          break;
        }
        if (status.repr != wuffs_base__suspension__even_more_information) {
          memset(hdr, 0, sizeof(*hdr));
          hdr->err = WUFFS_WASM_ERR_DECODE;
          hdr->format = fourcc;
          return WUFFS_WASM_ERR_DECODE;
        }
        continue;
      }
      if (minfo.flavor ==
          WUFFS_BASE__MORE_INFORMATION__FLAVOR__METADATA_RAW_TRANSFORM) {
        // PNG iCCP only: Wuffs zlib-decompresses the profile into `have`.
        uint32_t mfourcc =
            wuffs_base__more_information__metadata__fourcc(&minfo);
        if (mfourcc != WUFFS_BASE__FOURCC__ICCP) {
          memset(hdr, 0, sizeof(*hdr));
          hdr->err = WUFFS_WASM_ERR_DECODE;
          hdr->format = fourcc;
          return WUFFS_WASM_ERR_DECODE;
        }
        uint64_t wi = have.meta.wi;
        if (wi > 0) {
          if ((uint64_t)icc_len + wi > (uint64_t)WUFFS_WASM_METADATA_ICC_CAP) {
            memset(hdr, 0, sizeof(*hdr));
            hdr->err = WUFFS_WASM_ERR_DECODE;
            hdr->format = fourcc;
            return WUFFS_WASM_ERR_DECODE;
          }
          memcpy(icc_buf + icc_len, have_ptr, (size_t)wi);
          icc_len += (uint32_t)wi;
        }
        if (wuffs_base__status__is_ok(&status)) {
          break;
        }
        if (status.repr != wuffs_base__suspension__even_more_information) {
          memset(hdr, 0, sizeof(*hdr));
          hdr->err = WUFFS_WASM_ERR_DECODE;
          hdr->format = fourcc;
          return WUFFS_WASM_ERR_DECODE;
        }
        continue;
      }
      memset(hdr, 0, sizeof(*hdr));
      hdr->err = WUFFS_WASM_ERR_DECODE;
      hdr->format = fourcc;
      return WUFFS_WASM_ERR_DECODE;
    }
  }

  uint64_t total = (uint64_t)WUFFS_WASM_METADATA_PACK_HEADER_SIZE +
                   (uint64_t)exif_len + (uint64_t)icc_len + (uint64_t)xmp_len;
  if (total > (uint64_t)pack_cap) {
    memset(hdr, 0, sizeof(*hdr));
    hdr->err = WUFFS_WASM_ERR_DECODE;
    hdr->format = fourcc;
    return WUFFS_WASM_ERR_DECODE;
  }

  memset(hdr, 0, sizeof(*hdr));
  hdr->err = WUFFS_WASM_OK;
  hdr->format = fourcc;
  hdr->exif_len = exif_len;
  hdr->icc_len = icc_len;
  hdr->xmp_len = xmp_len;
  hdr->has_gamma = has_gamma;
  hdr->has_chrm = has_chrm;
  hdr->has_srgb = has_srgb;
  hdr->has_modtime = 0;
  hdr->gama_scaled = gama_scaled;
  for (uint32_t i = 0; i < 8; i++) {
    hdr->chrm[i] = chrm[i];
  }
  hdr->srgb_intent = srgb_intent;
  hdr->modtime_sec = 0;
  hdr->modtime_nsec = 0;

  uint8_t* out = (uint8_t*)hdr + WUFFS_WASM_METADATA_PACK_HEADER_SIZE;
  if (exif_len > 0) {
    memcpy(out, exif_buf, exif_len);
    out += exif_len;
  }
  if (icc_len > 0) {
    memcpy(out, icc_buf, icc_len);
    out += icc_len;
  }
  if (xmp_len > 0) {
    memcpy(out, xmp_buf, xmp_len);
  }
  return WUFFS_WASM_OK;
}

__attribute__((export_name("wuffs_read_image_metadata"))) int32_t
wuffs_wasm_read_image_metadata(uint32_t src_off, uint32_t src_len,
                               uint32_t pack_off, uint32_t pack_cap) {
  return read_image_metadata(src_off, src_len, pack_off, pack_cap);
}

__attribute__((export_name("wuffs_version"))) uint32_t wuffs_wasm_version(void) {
  return (uint32_t)WUFFS_VERSION;
}

__attribute__((export_name("wuffs_decode_image"))) int32_t wuffs_wasm_decode_image(
    uint32_t src_off, uint32_t src_len, uint32_t dst_off, uint32_t dst_cap,
    uint32_t meta_off) {
  return decode_image(src_off, src_len, dst_off, dst_cap, meta_off);
}
