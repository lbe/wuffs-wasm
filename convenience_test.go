package wuffs

import (
	"errors"
	"testing"
)

func TestAllocatingGeometryComputesSafeBuffers(t *testing.T) {
	maxInt := int(^uint(0) >> 1)

	tests := []struct {
		name           string
		width          int
		height         int
		bytesPerPixel  int
		wantErr        error
		wantHostStride int
		wantHostLen    int
		wantGuestLen   int
	}{
		{
			name:           "RGBA 100x200",
			width:          100,
			height:         200,
			bytesPerPixel:  4,
			wantHostStride: 400,
			wantHostLen:    80000,
			wantGuestLen:   80000,
		},
		{
			name:           "NRGBA 50x75",
			width:          50,
			height:         75,
			bytesPerPixel:  4,
			wantHostStride: 200,
			wantHostLen:    15000,
			wantGuestLen:   15000,
		},
		{
			name:           "Gray 100x200",
			width:          100,
			height:         200,
			bytesPerPixel:  1,
			wantHostStride: 100,
			wantHostLen:    20000,
			wantGuestLen:   80000,
		},
		{
			name:          "zero width",
			width:         0,
			height:        100,
			bytesPerPixel: 4,
			wantErr:       ErrBadImage,
		},
		{
			name:          "zero height",
			width:         100,
			height:        0,
			bytesPerPixel: 4,
			wantErr:       ErrBadImage,
		},
		{
			name:          "negative width",
			width:         -1,
			height:        100,
			bytesPerPixel: 4,
			wantErr:       ErrBadImage,
		},
		{
			name:          "negative height",
			width:         100,
			height:        -1,
			bytesPerPixel: 4,
			wantErr:       ErrBadImage,
		},
		{
			name:          "zero bytes per pixel",
			width:         100,
			height:        100,
			bytesPerPixel: 0,
			wantErr:       ErrBadImage,
		},
		{
			name:          "unsupported bytes per pixel",
			width:         100,
			height:        100,
			bytesPerPixel: 2,
			wantErr:       ErrBadImage,
		},
		{
			name:          "host stride overflows int",
			width:         maxInt/4 + 1,
			height:        1,
			bytesPerPixel: 4,
			wantErr:       ErrBadImage,
		},
		{
			name:          "guest length overflows uint32",
			width:         65536,
			height:        65536,
			bytesPerPixel: 4,
			wantErr:       ErrBadImage,
		},
		{
			name:          "width exceeds Wuffs max",
			width:         0x1000000,
			height:        1,
			bytesPerPixel: 4,
			wantErr:       ErrBadImage,
		},
		{
			name:          "height exceeds Wuffs max",
			width:         1,
			height:        0x1000000,
			bytesPerPixel: 4,
			wantErr:       ErrBadImage,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hostStride, hostLen, guestLen, err := validateAllocatingGeometry(tc.width, tc.height, tc.bytesPerPixel)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("validateAllocatingGeometry(%d, %d, %d) error = %v, want %v", tc.width, tc.height, tc.bytesPerPixel, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateAllocatingGeometry(%d, %d, %d) unexpected error: %v", tc.width, tc.height, tc.bytesPerPixel, err)
			}
			if hostStride != tc.wantHostStride {
				t.Errorf("hostStride = %d, want %d", hostStride, tc.wantHostStride)
			}
			if hostLen != tc.wantHostLen {
				t.Errorf("hostLen = %d, want %d", hostLen, tc.wantHostLen)
			}
			if guestLen != tc.wantGuestLen {
				t.Errorf("guestLen = %d, want %d", guestLen, tc.wantGuestLen)
			}
		})
	}
}
