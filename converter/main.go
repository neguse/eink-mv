package main

import (
	"flag"
	"fmt"
	"image/png"
	"log"
	"os"

	"github.com/neguse/eink-mv/bw"
	"github.com/pierrec/lz4"
)

func main() {
	frames := flag.Int("frames", 1, "number of png")
	input := flag.String("input", "%04d.png", "input file path format")
	output := flag.String("output", "out.bws", "output file path")

	var pages []*bw.BW
	for i := 1; i <= *frames; i++ {
		path := fmt.Sprintf(*input, i)
		log.Println(path)
		f, err := os.Open(path)
		if err != nil {
			log.Panic(err)
		}
		p, err := png.Decode(f)
		if err != nil {
			log.Panic(err)
		}
		b := bw.NewFromImg(p)
		pages = append(pages, b)
	}
	of, err := os.OpenFile(*output, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0777)
	if err != nil {
		log.Panic(err)
	}
	of2 := lz4.NewWriter(of)
	defer of.Close()
	defer of2.Close()
	bw.SaveBWS(of2, pages)
}
