package bw

import (
	"bytes"
	"encoding/binary"
	"image"
	"io"
	"log"
)

func SaveBWS(w io.Writer, pages []*BW) error {
	num := uint32(len(pages))
	if err := binary.Write(w, binary.LittleEndian, &num); err != nil {
		return err
	}
	for _, page := range pages {
		if err := page.Save(w); err != nil {
			return err
		}
	}
	return nil
}

type BWSLoader struct {
	r   io.Reader
	num uint32
}

func LoadBWS(r io.Reader) (*BWSLoader, error) {
	ld := &BWSLoader{
		r: r,
	}
	if err := binary.Read(r, binary.LittleEndian, &ld.num); err != nil {
		return nil, err
	}
	return ld, nil
}

func (ld *BWSLoader) Load() (*BW, error) {
	if ld.num == 0 {
		return nil, io.EOF
	}
	return NewFromReader(ld.r)
}

type BW struct {
	Width  uint32
	Height uint32
	Raw    []byte
}

func ToBinary(r, g, b, a uint32) byte {
	if (r+g+b)/3 > 0x80 {
		return 1
	}
	return 0
}

func NewFromImg(img image.Image) *BW {
	b := &BW{
		Width:  uint32(img.Bounds().Dx()),
		Height: uint32(img.Bounds().Dy()),
	}

	if b.Height%8 != 0 {
		log.Panic("height must be 8 align")
	}
	for x := 0; x < int(b.Width); x++ {
		for dy := 0; dy < int(b.Height/8); dy++ {
			y := dy * 8
			var r byte
			r |= ToBinary(img.At(x, y+0).RGBA()) << 0
			r |= ToBinary(img.At(x, y+1).RGBA()) << 1
			r |= ToBinary(img.At(x, y+2).RGBA()) << 2
			r |= ToBinary(img.At(x, y+3).RGBA()) << 3
			r |= ToBinary(img.At(x, y+4).RGBA()) << 4
			r |= ToBinary(img.At(x, y+5).RGBA()) << 5
			r |= ToBinary(img.At(x, y+6).RGBA()) << 6
			r |= ToBinary(img.At(x, y+7).RGBA()) << 7
			b.Raw = append(b.Raw, r)
		}
	}
	return b
}

func NewFromReader(r io.Reader) (*BW, error) {
	bw := &BW{}
	if err := binary.Read(r, binary.LittleEndian, &bw.Width); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &bw.Height); err != nil {
		return nil, err
	}
	bw.Raw = make([]byte, bw.Width*bw.Height/8)
	if _, err := io.ReadFull(r, bw.Raw); err != nil {
		return nil, err
	}
	return bw, nil
}

func (b *BW) Save(w io.Writer) error {
	if err := binary.Write(w, binary.LittleEndian, &b.Width); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, &b.Height); err != nil {
		return err
	}
	if _, err := io.Copy(w, bytes.NewReader(b.Raw)); err != nil {
		return err
	}
	return nil
}
