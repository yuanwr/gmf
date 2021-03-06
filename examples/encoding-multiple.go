package main

import (
	"errors"
	"fmt"
	. "github.com/3d0c/gmf"
	"log"
	"os"
	"sync"
)

func fatal(err error) {
	log.Fatal(err)
	os.Exit(0)
}

type output struct {
	filename string
	codec    interface{}
	data     chan *Frame
}

func encodeWorker(o output, wg *sync.WaitGroup) {
	defer wg.Done()

	codec, err := NewEncoder(o.codec)
	if err != nil {
		fatal(err)
	}

	videoEncCtx := NewCodecCtx(codec)
	if videoEncCtx == nil {
		fatal(err)
	}

	outputCtx, err := NewOutputCtx(o.filename)
	if err != nil {
		fatal(err)
	}

	videoEncCtx.
		SetBitRate(400000).
		SetWidth(320).
		SetHeight(200).
		SetTimeBase(AVR{1, 25}).
		SetPixFmt(AV_PIX_FMT_YUV420P)

	if o.codec == AV_CODEC_ID_MPEG1VIDEO {
		videoEncCtx.SetMbDecision(FF_MB_DECISION_RD)
	}

	if o.codec == AV_CODEC_ID_MPEG4 {
		videoEncCtx.SetProfile(FF_PROFILE_MPEG4_SIMPLE)
	}

	if outputCtx.IsGlobalHeader() {
		videoEncCtx.SetFlag(CODEC_FLAG_GLOBAL_HEADER)
		log.Println("AVFMT_GLOBALHEADER flag is set.")
	}

	videoStream := outputCtx.NewStream(codec, nil)
	if videoStream == nil {
		fatal(errors.New(fmt.Sprintf("Unable to create stream for videoEnc [%s]\n", codec.LongName())))
	}

	if err := videoEncCtx.Open(nil); err != nil {
		fatal(err)
	}

	if err := videoStream.SetCodecCtx(videoEncCtx); err != nil {
		fatal(err)
	}

	outputCtx.SetStartTime(0)

	if err := outputCtx.WriteHeader(); err != nil {
		fatal(err)
	}

	i, w := 0, 0

	for {
		srcFrame, ok := <-o.data
		if !ok {
			break
		}

		frame := srcFrame.Clone()
		frame.SetPts(i)

		if p, ready, err := frame.Encode(videoStream.GetCodecCtx()); ready {
			if p.Pts() != AV_NOPTS_VALUE {
				p.SetPts(RescaleQ(p.Pts(), videoStream.GetCodecCtx().TimeBase(), videoStream.TimeBase()))
			}

			if p.Dts() != AV_NOPTS_VALUE {
				p.SetDts(RescaleQ(p.Dts(), videoStream.GetCodecCtx().TimeBase(), videoStream.TimeBase()))
			}

			if err := outputCtx.WritePacket(p); err != nil {
				fatal(err)
			} else {
				w++
			}

			// log.Printf("Write frame=%d size=%v pts=%v dts=%v\n", i, p.Size(), p.Pts(), p.Dts())

		} else if err != nil {
			fatal(err)
		}

		frame.Free()

		i++
	}

	outputCtx.CloseOutput()

	log.Printf("done [%s], %d frames, %d written\n", o.filename, i, w)
}

func main() {
	o := []output{
		{"sample-enc-mpeg1.mpg", AV_CODEC_ID_MPEG1VIDEO, make(chan *Frame)},
		{"sample-enc-mpeg2.mpg", AV_CODEC_ID_MPEG2VIDEO, make(chan *Frame)},
		{"sample-enc-mpeg4.mp4", AV_CODEC_ID_MPEG4, make(chan *Frame)},
	}

	wg := new(sync.WaitGroup)
	wCount := 0

	for _, item := range o {
		wg.Add(1)
		go encodeWorker(item, wg)
		wCount++
	}

	var srcFrame *Frame

	for srcFrame = range GenSyntVideo(320, 200, AV_PIX_FMT_YUV420P) {
		for i := 0; i < wCount; i++ {
			o[i].data <- srcFrame
		}
	}

	for _, item := range o {
		close(item.data)
	}

	wg.Wait()
}
