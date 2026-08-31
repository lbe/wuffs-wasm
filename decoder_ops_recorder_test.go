package wuffs

import "image"

// reserveCall records one Reserve invocation with its arguments.
type reserveCall struct {
	dstBytes int
	srcBytes int
}

// decoderRecorder is a per-test fake implementing the private
// decoderOperations seam. It records ordered operations and Reserve
// arguments and can inject unique per-test errors at the source Reserve,
// Probe, destination Reserve, and typed decode stages. A fresh instance is
// used per test.
//
// probeMeta, when set, is the metadata returned by a successful Probe and by
// the typed decode methods, letting the shared typed matrix drive dimension
// checks with deterministic geometry. When probeMeta is nil the recorder
// returns a non-nil zero Meta so that callers (probeWithDecoder, decodePrep,
// etc.) can dereference the result without panicking. Tests that need
// specific Probe metadata use a real Decoder instead.
type decoderRecorder struct {
	ops              []string      // ordered operation log, e.g. "Reserve", "Probe", "typed-alloc"
	reservations     []reserveCall // every Reserve invocation, in order
	reserveCallCount int           // tracks which Reserve call we're on (1-based)
	srcReserveErr    error         // returned by the first (source) Reserve only
	dstReserveErr    error         // returned by the second (destination) Reserve only
	probeErr         error         // returned by Probe
	rgbaDecodeErr    error         // returned by DecodeRGBA
	nrgbaDecodeErr   error         // returned by DecodeNRGBA
	grayDecodeErr    error         // returned by DecodeGray
	probeMeta        *Meta         // metadata returned by successful Probe and decodes
}

// Reserve records the invocation and returns the injected source-Reserve
// error for the first call, the injected destination-Reserve error for the
// second call, and nil for any later call.
func (r *decoderRecorder) Reserve(dstBytes, srcBytes int) error {
	r.ops = append(r.ops, "Reserve")
	r.reservations = append(r.reservations, reserveCall{dstBytes, srcBytes})
	r.reserveCallCount++
	switch r.reserveCallCount {
	case 1:
		return r.srcReserveErr
	case 2:
		return r.dstReserveErr
	}
	return nil
}

func (r *decoderRecorder) Probe(src []byte) (*Meta, error) {
	r.ops = append(r.ops, "Probe")
	if r.probeErr != nil {
		return nil, r.probeErr
	}
	return r.successMeta(), nil
}

func (r *decoderRecorder) DecodeRGBA(dst *image.RGBA, src []byte) (*Meta, error) {
	r.ops = append(r.ops, "DecodeRGBA")
	if r.rgbaDecodeErr != nil {
		return nil, r.rgbaDecodeErr
	}
	return r.successMeta(), nil
}

func (r *decoderRecorder) DecodeNRGBA(dst *image.NRGBA, src []byte) (*Meta, error) {
	r.ops = append(r.ops, "DecodeNRGBA")
	if r.nrgbaDecodeErr != nil {
		return nil, r.nrgbaDecodeErr
	}
	return r.successMeta(), nil
}

func (r *decoderRecorder) DecodeGray(dst *image.Gray, src []byte) (*Meta, error) {
	r.ops = append(r.ops, "DecodeGray")
	if r.grayDecodeErr != nil {
		return nil, r.grayDecodeErr
	}
	return r.successMeta(), nil
}

// successMeta returns a non-nil Meta for successful operations. It mirrors
// probeMeta when set so staged callers observe consistent dimensions;
// otherwise it returns an all-zero Meta.
func (r *decoderRecorder) successMeta() *Meta {
	if r.probeMeta != nil {
		m := *r.probeMeta
		return &m
	}
	return &Meta{}
}

// recordAlloc logs one host image allocation made by an injected allocator
// closure, keeping allocation order in the shared operation log.
func (r *decoderRecorder) recordAlloc(op string) {
	r.ops = append(r.ops, op)
}
