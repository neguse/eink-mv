package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/neguse/eink-mv/bw"
	"github.com/pierrec/lz4/v4"
	"github.com/wolfeidau/gioctl"
)

type fbMode struct {
	w, h, vw, vh int
	bits         int
}

func getFbMode(fb string) (fbMode, error) {
	cmd := exec.Command("/usr/sbin/fbset", "-fb", fb)
	o, err := cmd.Output()
	if err != nil {
		return fbMode{}, err
	}
	s := bufio.NewScanner(bytes.NewReader(o))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		ss := strings.Split(line, " ")
		if len(ss) == 6 && ss[0] == "geometry" {
			w, _ := strconv.Atoi(ss[1])
			h, _ := strconv.Atoi(ss[2])
			vw, _ := strconv.Atoi(ss[3])
			vh, _ := strconv.Atoi(ss[4])
			bits, _ := strconv.Atoi(ss[5])
			return fbMode{
				w: w, h: h, vw: vw, vh: vh, bits: bits,
			}, nil
		}
	}
	return fbMode{}, errors.New("geometry not found")
}

type mxcfb_rect struct {
	top    uint32
	left   uint32
	width  uint32
	height uint32
}

type mxcfb_alt_buffer_data struct {
	phys_addr         uint32
	width             uint32
	height            uint32
	alt_update_region mxcfb_rect
}

type mxcfb_update_data struct {
	update_region           mxcfb_rect
	waveform_mode           uint32
	update_mode             uint32
	update_marker           uint32
	hist_bw_waveform_mode   uint32
	hist_gray_waveform_mode uint32
	temp                    int
	flags                   uint32
	alt_buffer_data         mxcfb_alt_buffer_data
}

const WAVEFORM_MODE_INIT = 0x0
const WAVEFORM_MODE_DU = 0x1
const WAVEFORM_MODE_GC16 = 0x2
const WAVEFORM_MODE_GC4 = WAVEFORM_MODE_GC16
const WAVEFORM_MODE_GC16_FAST = 0x3
const WAVEFORM_MODE_A2 = 0x4
const WAVEFORM_MODE_GL16 = 0x5
const WAVEFORM_MODE_GL16_FAST = 0x6
const WAVEFORM_MODE_DU4 = 0x7
const WAVEFORM_MODE_REAGL = 0x8
const WAVEFORM_MODE_REAGLD = 0x9
const WAVEFORM_MODE_GL4 = 0xA
const WAVEFORM_MODE_GL16_INV = 0xB
const WAVEFORM_MODE_AUTO = 257

const UPDATE_MODE_PARTIAL = 0
const UPDATE_MODE_FULL = 1
const TEMP_USE_AMBIENT = 0x1000
const TEMP_USE_AUTO = 0x1001

const EPDC_FLAG_ENABLE_INVERSION = 0x01
const EPDC_FLAG_FORCE_MONOCHROME = 0x02
const EPDC_FLAG_USE_CMAP = 0x04
const EPDC_FLAG_USE_ALT_BUFFER = 0x100
const EPDC_FLAG_TEST_COLLISION = 0x200
const EPDC_FLAG_GROUP_UPDATE = 0x400
const EPDC_FLAG_FORCE_Y2 = 0x800
const EPDC_FLAG_USE_REAGLD = 0x1000
const EPDC_FLAG_USE_DITHERING_Y1 = 0x2000
const EPDC_FLAG_USE_DITHERING_Y2 = 0x4000
const EPDC_FLAG_USE_DITHERING_Y4 = 0x8000

func main() {

	fbDevice := flag.String("fb", "/dev/fb0", "frame buffer file")
	bwsPath := flag.String("bws", "mv.bws", "bws file")
	multiple := flag.Int("multiple", 1, "multiple rate(1,2,3)")
	flag.Parse()

	fbMode, err := getFbMode(*fbDevice)
	if err != nil {
		log.Panic(err)
	}

	fb0, err := os.OpenFile(*fbDevice, os.O_RDWR, 0777)
	if err != nil {
		log.Panic(err)
	}
	defer fb0.Close()

	data, err := syscall.Mmap(int(fb0.Fd()), 0, int(fbMode.vw*fbMode.vh), syscall.PROT_READ|syscall.PROT_WRITE|syscall.PROT_EXEC, syscall.MAP_SHARED)
	if err != nil {
		log.Panic(err)
	}

	f, err := os.Open(*bwsPath)
	if err != nil {
		log.Panic(err)
	}
	defer f.Close()
	f2 := lz4.NewReader(f)

	bws, err := bw.LoadBWS(f2)
	if err != nil {
		log.Panic(err)
	}

	msecFloat := float64(time.Millisecond * 1000)
	wait := time.Duration(msecFloat / 29.97)
	next := time.Now().Add(wait)

	ch := make(chan *bw.BW, 32)
	go func() {
		for {
			bw, err := bws.Load()
			if err != nil {
				log.Panic(err)
			}
			ch <- bw
		}
	}()

	update := func(mode, flags uint32) {
		marker := uint32(1)
		update_data := mxcfb_update_data{
			update_region: mxcfb_rect{
				top:    0,
				left:   0,
				width:  uint32(fbMode.w),
				height: uint32(fbMode.h),
			},
			waveform_mode: WAVEFORM_MODE_A2,
			update_mode:   mode,
			update_marker: marker,
			temp:          TEMP_USE_AUTO,
			flags:         flags,
		}
		MXCFB_SEND_UPDATE := gioctl.IoW(uintptr('F'), uintptr(0x2E), unsafe.Sizeof(update_data))

		err = gioctl.Ioctl(uintptr(fb0.Fd()), MXCFB_SEND_UPDATE, uintptr(unsafe.Pointer(&update_data)))
		if err != nil {
			log.Panic(err)
		}
	}

	p3 := func(x, y uint32, c1, c2 byte) {
		px := x * 3
		py := y * 3

		data[(py+0)+uint32(fbMode.vw)*(px+0)] = c1
		data[(py+1)+uint32(fbMode.vw)*(px+0)] = c1
		data[(py+2)+uint32(fbMode.vw)*(px+0)] = c1
		data[(py+3)+uint32(fbMode.vw)*(px+0)] = c2
		data[(py+4)+uint32(fbMode.vw)*(px+0)] = c2
		data[(py+5)+uint32(fbMode.vw)*(px+0)] = c2

		data[(py+0)+uint32(fbMode.vw)*(px+1)] = c1
		data[(py+1)+uint32(fbMode.vw)*(px+1)] = c1
		data[(py+2)+uint32(fbMode.vw)*(px+1)] = c1
		data[(py+3)+uint32(fbMode.vw)*(px+1)] = c2
		data[(py+4)+uint32(fbMode.vw)*(px+1)] = c2
		data[(py+5)+uint32(fbMode.vw)*(px+1)] = c2

		data[(py+0)+uint32(fbMode.vw)*(px+2)] = c1
		data[(py+1)+uint32(fbMode.vw)*(px+2)] = c1
		data[(py+2)+uint32(fbMode.vw)*(px+2)] = c1
		data[(py+3)+uint32(fbMode.vw)*(px+2)] = c2
		data[(py+4)+uint32(fbMode.vw)*(px+2)] = c2
		data[(py+5)+uint32(fbMode.vw)*(px+2)] = c2
	}
	p2 := func(x, y uint32, c1, c2 byte) {
		px := x * 2
		py := y * 2

		data[(py+0)+uint32(fbMode.vw)*(px+0)] = c1
		data[(py+1)+uint32(fbMode.vw)*(px+0)] = c1
		data[(py+2)+uint32(fbMode.vw)*(px+0)] = c2
		data[(py+3)+uint32(fbMode.vw)*(px+0)] = c2

		data[(py+0)+uint32(fbMode.vw)*(px+1)] = c1
		data[(py+1)+uint32(fbMode.vw)*(px+1)] = c1
		data[(py+2)+uint32(fbMode.vw)*(px+1)] = c2
		data[(py+3)+uint32(fbMode.vw)*(px+1)] = c2
	}
	p1 := func(x, y uint32, c1, c2 byte) {

		px := x
		py := y

		data[(py+0)+uint32(fbMode.vw)*(px+0)] = c1
		data[(py+1)+uint32(fbMode.vw)*(px+0)] = c2

		data[(py+0)+uint32(fbMode.vw)*(px+1)] = c1
		data[(py+1)+uint32(fbMode.vw)*(px+1)] = c2
	}
	p := []func(x, y uint32, c1, c2 byte){
		nil, p1, p2, p3,
	}[*multiple]

	for x := uint32(0); x < uint32(fbMode.w); x++ {
		for y := uint32(0); y < uint32(fbMode.h); y++ {
			data[y*uint32(fbMode.vw)+x] = 0xff
		}
	}
	update(UPDATE_MODE_FULL, 0)

	for x := uint32(0); x < uint32(fbMode.w); x++ {
		for y := uint32(0); y < uint32(fbMode.h); y++ {
			data[y*uint32(fbMode.vw)+x] = 0x00
		}
	}
	update(UPDATE_MODE_FULL, 0)

	for img := range ch {
		for x := uint32(0); x < img.Width; x++ {
			for dy := uint32(0); dy < img.Height/8; dy++ {
				y := dy * 8
				r := img.Raw[x*img.Height/8+dy]
				p(x, y+0, ((r&(1<<0))>>0)*0xff, ((r&(1<<1))>>1)*0xff)
				p(x, y+2, ((r&(1<<2))>>2)*0xff, ((r&(1<<3))>>3)*0xff)
				p(x, y+4, ((r&(1<<4))>>4)*0xff, ((r&(1<<5))>>5)*0xff)
				p(x, y+6, ((r&(1<<6))>>6)*0xff, ((r&(1<<7))>>7)*0xff)
			}
		}
		update(UPDATE_MODE_PARTIAL, 0)

		timeToSleep := next.Sub(time.Now())
		if timeToSleep > 0 {
			time.Sleep(timeToSleep)
		}
		next = next.Add(wait)
	}
}
