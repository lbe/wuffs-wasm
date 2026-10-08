package wuffs

import (
	"go/types"
	"testing"
)

// TestUnitMetadataTypesAndConstants verifies that the package source declares
// the exported metadata Metadata and Chromaticities types with the exact
// API.md shape, plus the seven Meta* FourCC constants. It inspects only
// non-test .go files via go/parser and go/types (never regular expressions
// and never the wasm guest), and it looks each name up by string so that this
// test compiles even before the declarations exist.
func TestUnitMetadataTypesAndConstants(t *testing.T) {
	chrmObj, err := lookupDecl("Chromaticities")
	if err != nil {
		t.Fatalf("inspect package source: %v", err)
	}
	if chrmObj == nil {
		t.Fatalf("expected exported type Chromaticities with eight float64 fields WhiteX, WhiteY, RedX, RedY, GreenX, GreenY, BlueX, BlueY in the package source, but no such declaration exists")
	}
	chrmName, ok := chrmObj.(*types.TypeName)
	if !ok {
		t.Fatalf("Chromaticities is declared as %T, want type", chrmObj)
	}
	if !chrmObj.Exported() {
		t.Errorf("Chromaticities is not exported")
	}
	chrmStruct, ok := chrmName.Type().Underlying().(*types.Struct)
	if !ok {
		t.Fatalf("Chromaticities underlying type = %v, want struct", chrmName.Type().Underlying())
	}
	wantChrmFields := []string{"WhiteX", "WhiteY", "RedX", "RedY", "GreenX", "GreenY", "BlueX", "BlueY"}
	if got := chrmStruct.NumFields(); got != len(wantChrmFields) {
		t.Errorf("Chromaticities has %d fields, want %d", got, len(wantChrmFields))
	}
	for _, name := range wantChrmFields {
		field, found := fieldByName(chrmStruct, name)
		if !found {
			t.Errorf("Chromaticities is missing exported field %s", name)
			continue
		}
		if !field.Exported() {
			t.Errorf("Chromaticities.%s is not exported", name)
			continue
		}
		if got := field.Type().String(); got != "float64" {
			t.Errorf("Chromaticities.%s type = %s, want float64", name, got)
		}
	}

	metaObj, err := lookupDecl("Metadata")
	if err != nil {
		t.Fatalf("inspect package source: %v", err)
	}
	if metaObj == nil {
		t.Fatalf("expected exported type Metadata with fields Format, EXIF, ICC, XMP, HasGamma, Gamma, HasChromaticities, Chromaticities, HasSRGB, SRGB, HasModTime, ModTime in the package source, but no such declaration exists")
	}
	metaName, ok := metaObj.(*types.TypeName)
	if !ok {
		t.Fatalf("Metadata is declared as %T, want type", metaObj)
	}
	if !metaObj.Exported() {
		t.Errorf("Metadata is not exported")
	}
	metaStruct, ok := metaName.Type().Underlying().(*types.Struct)
	if !ok {
		t.Fatalf("Metadata underlying type = %v, want struct", metaName.Type().Underlying())
	}
	wantMetaFields := []struct {
		name string
		want string
	}{
		{"Format", "uint32"},
		{"EXIF", "[]byte"},
		{"ICC", "[]byte"},
		{"XMP", "[]byte"},
		{"HasGamma", "bool"},
		{"Gamma", "float64"},
		{"HasChromaticities", "bool"},
		{"Chromaticities", "wuffs.Chromaticities"},
		{"HasSRGB", "bool"},
		{"SRGB", "uint32"},
		{"HasModTime", "bool"},
		{"ModTime", "time.Time"},
	}
	if got := metaStruct.NumFields(); got != len(wantMetaFields) {
		t.Errorf("Metadata has %d fields, want %d", got, len(wantMetaFields))
	}
	for _, want := range wantMetaFields {
		field, found := fieldByName(metaStruct, want.name)
		if !found {
			t.Errorf("Metadata is missing exported field %s", want.name)
			continue
		}
		if !field.Exported() {
			t.Errorf("Metadata.%s is not exported", want.name)
			continue
		}
		if got := field.Type().String(); got != want.want {
			t.Errorf("Metadata.%s type = %s, want %s", want.name, got, want.want)
		}
	}

	assertFormatFourCC(t, "MetaEXIF", 0x45584946)
	assertFormatFourCC(t, "MetaICCP", 0x49434350)
	assertFormatFourCC(t, "MetaXMP", 0x584D5020)
	assertFormatFourCC(t, "MetaGAMA", 0x47414D41)
	assertFormatFourCC(t, "MetaCHRM", 0x4348524D)
	assertFormatFourCC(t, "MetaSRGB", 0x53524742)
	assertFormatFourCC(t, "MetaMTIM", 0x4D54494D)
}
