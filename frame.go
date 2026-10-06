package wuffs

import (
	"image"
	"image/color"
	"time"
)

// Frame describes one animation frame within the overall canvas.
type Frame struct {
	// Index is the 0-based frame number; the first frame is 0.
	Index int
	// Bounds is the frame rectangle inside the overall canvas (half-open).
	Bounds image.Rectangle
	// Duration is how long to display the frame. Zero means still or display
	// forever. Durations convert from Wuffs flicks (1 flick = 1/705_600_000 s).
	Duration time.Duration
	// Disposal says what to do with the frame rectangle after display.
	Disposal Disposal
	// Opaque is a conservative flag: all pixels in Bounds are opaque.
	Opaque bool
	// Overwrite is true when the frame replaces canvas pixels; false means it
	// blends over them.
	Overwrite bool
	// Background is the canvas background, straight RGBA.
	Background color.RGBA
	// IOPosition is the source offset of this frame config (Wuffs io_position).
	IOPosition uint64
}

// Disposal is the post-display disposal method for an animation frame.
type Disposal uint8

const (
	// DisposalNone leaves the frame; the next frame draws on top.
	DisposalNone Disposal = 0
	// DisposalRestoreBackground restores the frame rectangle to the background.
	DisposalRestoreBackground Disposal = 1
	// DisposalRestorePrevious restores the canvas to before this frame.
	DisposalRestorePrevious Disposal = 2
)
