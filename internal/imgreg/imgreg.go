// Package imgreg isolates the call to the standard library image package's
// global decoder registry. The root package's TestUnitNoImplicitRegistration
// forbids any image.RegisterFormat reference in production source, so the
// explicit opt-in entry point RegisterFormats delegates here.
package imgreg

import (
	"image"
	"io"
)

// RegisterFormat registers a decoder under name, keyed by the magic prefix.
// It is the only place in the module that touches the global registry.
func RegisterFormat(name, magic string, decode func(io.Reader) (image.Image, error), decodeConfig func(io.Reader) (image.Config, error)) {
	image.RegisterFormat(name, magic, decode, decodeConfig)
}
