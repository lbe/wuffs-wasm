// WASM host API for Wuffs image decoding (first frame, BGRA premultiplied).
#include "wuffs_config.h"

#include <stdint.h>
#include <string.h>

#include "../../wuffs-mirror-release-c/release/c/wuffs-v0.4.c"

#define WUFFS_WASM_WORKBUF_SIZE (1024 * 1024)

typedef struct wuffs_wasm_decode_meta {
  int32_t err;
  uint32_t width;
  uint32_t height;
  uint32_t stride;
  uint32_t bytes_written;
} wuffs_wasm_decode_meta;

enum {
  WUFFS_WASM_OK = 0,
  WUFFS_WASM_ERR_UNKNOWN_FORMAT = -1,
  WUFFS_WASM_ERR_DST_TOO_SMALL = -2,
  WUFFS_WASM_ERR_DECODE = -3,
  WUFFS_WASM_ERR_BAD_ARG = -4,
};

static uint8_t* mem_ptr(uint32_t off) { return (uint8_t*)(uintptr_t)off; }

static uint32_t read_u32le(const uint8_t* p) {
  return ((uint32_t)p[0]) | ((uint32_t)p[1] << 8) | ((uint32_t)p[2] << 16) |
         ((uint32_t)p[3] << 24);
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
  if (len >= 4 && src[0] == 'w' && src[1] == 'B' && src[2] == 'M' &&
      src[3] == 'P') {
    return WUFFS_BASE__FOURCC__WBMP;
  }
  if (len >= 4 && read_u32le(src) == 0x6E696541) {  // "nïA" little-endian
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
  if (len >= 18 && src[0] == 0 && src[1] == 0 && src[16] == 0x20 &&
      src[17] == 0) {
    return WUFFS_BASE__FOURCC__TGA;
  }
  if (len >= 4 && src[0] == 0x13 && src[1] == 0xAB && src[2] == 0xA1 &&
      src[3] == 0x5C) {
    return WUFFS_BASE__FOURCC__ETC2;
  }
  if (len >= 4 && src[0] == 'H' && src[1] == 'N' && src[2] == 'S' &&
      src[3] == 'M') {
    return WUFFS_BASE__FOURCC__HNSM;
  }
  if (len >= 5 && src[0] == 0xFF && src[1] == 0xFF && src[2] == 0xFF &&
      src[3] == 0xFF && src[4] == 0xFF) {
    return WUFFS_BASE__FOURCC__TH;
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

  uint8_t workbuf[WUFFS_WASM_WORKBUF_SIZE];
  wuffs_base__slice_u8 work_slice = {
      .ptr = workbuf,
      .len = WUFFS_WASM_WORKBUF_SIZE,
  };

  uint8_t dec_storage[4096];
  if (slot->obj_size > sizeof(dec_storage)) {
    meta->err = WUFFS_WASM_ERR_DECODE;
    return WUFFS_WASM_ERR_DECODE;
  }
  void* dec = dec_storage;
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
  return WUFFS_WASM_OK;
}

__attribute__((export_name("wuffs_version"))) uint32_t wuffs_wasm_version(void) {
  return (uint32_t)WUFFS_VERSION;
}

__attribute__((export_name("wuffs_decode_image"))) int32_t wuffs_wasm_decode_image(
    uint32_t src_off, uint32_t src_len, uint32_t dst_off, uint32_t dst_cap,
    uint32_t meta_off) {
  return decode_image(src_off, src_len, dst_off, dst_cap, meta_off);
}
