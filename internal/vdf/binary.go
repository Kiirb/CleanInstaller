package vdf

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

// Binary VDF type markers.
const (
	TypeMap     byte = 0x00
	TypeString  byte = 0x01
	TypeInt32   byte = 0x02
	TypeFloat32 byte = 0x03
	TypeUint64  byte = 0x07
	typeEnd     byte = 0x08
)

// BNode is a binary-VDF node. Children keep file order, so a read/write round
// trip reproduces the original bytes.
type BNode struct {
	Key      string
	Type     byte
	Str      string
	Int      int32
	U64      uint64
	F32      uint32 // raw IEEE-754 bits
	Children []*BNode
}

// Child returns the first child whose key matches case-insensitively, or nil.
// Safe on a nil receiver.
func (n *BNode) Child(key string) *BNode {
	if n == nil {
		return nil
	}
	for _, c := range n.Children {
		if strings.EqualFold(c.Key, key) {
			return c
		}
	}
	return nil
}

// ReadBinary reads one top-level map. The returned root has an empty Key.
func ReadBinary(r io.Reader) (*BNode, error) {
	br := bufio.NewReader(r)
	root := &BNode{Type: TypeMap}
	if err := readMap(br, root); err != nil {
		return nil, err
	}
	return root, nil
}

func readMap(r *bufio.Reader, parent *BNode) error {
	parent.Children = []*BNode{}
	for {
		t, err := r.ReadByte()
		if err != nil {
			return fmt.Errorf("vdf: truncated binary data: %w", err)
		}
		if t == typeEnd {
			return nil
		}
		key, err := readCString(r)
		if err != nil {
			return err
		}
		n := &BNode{Key: key, Type: t}
		switch t {
		case TypeMap:
			if err := readMap(r, n); err != nil {
				return err
			}
		case TypeString:
			if n.Str, err = readCString(r); err != nil {
				return err
			}
		case TypeInt32:
			err = binary.Read(r, binary.LittleEndian, &n.Int)
		case TypeFloat32:
			err = binary.Read(r, binary.LittleEndian, &n.F32)
		case TypeUint64:
			err = binary.Read(r, binary.LittleEndian, &n.U64)
		default:
			return fmt.Errorf("vdf: unknown binary type 0x%02x for key %q", t, key)
		}
		if err != nil {
			return fmt.Errorf("vdf: truncated value for key %q: %w", key, err)
		}
		parent.Children = append(parent.Children, n)
	}
}

func readCString(r *bufio.Reader) (string, error) {
	s, err := r.ReadString(0)
	if err != nil {
		return "", fmt.Errorf("vdf: truncated string: %w", err)
	}
	return s[:len(s)-1], nil
}

// WriteBinary writes root's children followed by the end marker.
func WriteBinary(w io.Writer, root *BNode) error {
	bw := bufio.NewWriter(w)
	if err := writeMap(bw, root); err != nil {
		return err
	}
	return bw.Flush()
}

func writeMap(w *bufio.Writer, n *BNode) error {
	for _, c := range n.Children {
		if strings.IndexByte(c.Key, 0) >= 0 || strings.IndexByte(c.Str, 0) >= 0 {
			return fmt.Errorf("vdf: NUL byte in key or value %q", c.Key)
		}
		w.WriteByte(c.Type)
		w.WriteString(c.Key)
		w.WriteByte(0)
		var err error
		switch c.Type {
		case TypeMap:
			err = writeMap(w, c)
		case TypeString:
			w.WriteString(c.Str)
			err = w.WriteByte(0)
		case TypeInt32:
			err = binary.Write(w, binary.LittleEndian, c.Int)
		case TypeFloat32:
			err = binary.Write(w, binary.LittleEndian, c.F32)
		case TypeUint64:
			err = binary.Write(w, binary.LittleEndian, c.U64)
		default:
			return fmt.Errorf("vdf: cannot write type 0x%02x for key %q", c.Type, c.Key)
		}
		if err != nil {
			return err
		}
	}
	return w.WriteByte(typeEnd)
}
