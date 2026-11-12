package vdf

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// builder writes raw binary-VDF bytes independently of WriteBinary, so the
// round-trip test compares against a known-good encoding.
type builder struct{ bytes.Buffer }

func (b *builder) cstr(s string)     { b.WriteString(s); b.WriteByte(0) }
func (b *builder) mapStart(k string) { b.WriteByte(0x00); b.cstr(k) }
func (b *builder) str(k, v string)   { b.WriteByte(0x01); b.cstr(k); b.cstr(v) }
func (b *builder) i32(k string, v int32) {
	b.WriteByte(0x02)
	b.cstr(k)
	_ = binary.Write(&b.Buffer, binary.LittleEndian, v)
}
func (b *builder) f32(k string, bits uint32) {
	b.WriteByte(0x03)
	b.cstr(k)
	_ = binary.Write(&b.Buffer, binary.LittleEndian, bits)
}
func (b *builder) u64(k string, v uint64) {
	b.WriteByte(0x07)
	b.cstr(k)
	_ = binary.Write(&b.Buffer, binary.LittleEndian, v)
}
func (b *builder) end() { b.WriteByte(0x08) }

func sampleShortcuts() []byte {
	var b builder
	b.mapStart("shortcuts")
	b.mapStart("0")
	b.i32("appid", -305070)
	b.str("AppName", "Among Us Modded")
	b.str("Exe", `"C:\Games\Ä\CInstaller.exe"`)
	b.f32("weird", 0x3f800000)
	b.u64("LastPlayTime64", 1<<40)
	b.mapStart("tags")
	b.str("0", "favorite")
	b.end()
	b.end()
	b.end()
	b.end()
	return b.Bytes()
}

func TestBinaryRoundTrip(t *testing.T) {
	in := sampleShortcuts()
	root, err := ReadBinary(bytes.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := WriteBinary(&out, root); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(in, out.Bytes()) {
		t.Fatalf("round trip changed bytes\nin  %x\nout %x", in, out.Bytes())
	}
	e := root.Child("shortcuts").Child("0")
	if e.Child("appid").Int != -305070 || e.Child("AppName").Str != "Among Us Modded" {
		t.Fatalf("decoded wrong: %+v", e)
	}
	if e.Child("lastplaytime64").U64 != 1<<40 {
		t.Fatal("u64 wrong")
	}
}

func TestBinaryPreservesOrder(t *testing.T) {
	var b builder
	b.str("b", "1")
	b.str("a", "2")
	b.str("c", "3")
	b.end()
	root, err := ReadBinary(bytes.NewReader(b.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, c := range root.Children {
		keys = append(keys, c.Key)
	}
	if got := keys; len(got) != 3 || got[0] != "b" || got[1] != "a" || got[2] != "c" {
		t.Fatalf("order %v", got)
	}
}

func TestBinaryUnknownType(t *testing.T) {
	in := []byte{0x05, 'k', 0, 0, 0, 0x08}
	if _, err := ReadBinary(bytes.NewReader(in)); err == nil {
		t.Fatal("expected error")
	}
}

func TestBinaryTruncated(t *testing.T) {
	in := sampleShortcuts()
	for _, n := range []int{0, 3, 20, len(in) - 1} {
		if _, err := ReadBinary(bytes.NewReader(in[:n])); err == nil {
			t.Errorf("no error at length %d", n)
		}
	}
}
