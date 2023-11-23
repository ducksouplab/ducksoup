package plot

import (
	"image/color"
	"strconv"
	"strings"
	"time"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"

	"github.com/ducksouplab/ducksoup/config"
	"github.com/ducksouplab/ducksoup/types"
)

const (
	bigGlyph   = 3
	smallGlyph = 2
)

type SlicePlot struct {
	controller  types.Terminable
	kind        string
	plotBuffers bool
	id          string
	folder      string
	bitratePlot *plot.Plot
	bufferPlot  *plot.Plot
	// from client/input
	widthLine       plotter.XYs
	framerateLine   plotter.XYs
	framerateLabels []string
	keyframeLine    plotter.XYs
	inputLine       plotter.XYs
	// server-side
	outputLine plotter.XYs
	targetLine plotter.XYs
	// rtpDiffIn              plotter.XYs
	senderCCOptimalLines   map[string]plotter.XYs
	senderLossOptimalLines map[string]plotter.XYs
	currentLevelTimeLines  map[string]plotter.XYs
	// state
	started   bool
	startedAt time.Time
}

// private

// seconds (float64)
func (s *SlicePlot) elapsed() float64 {
	if !s.started {
		// some initial points may be added before loop is started
		return 0
	}
	return float64(time.Since(s.startedAt).Milliseconds()) / 1000
}

func (s *SlicePlot) createFramerateLabels() {
	labels, _ := plotter.NewLabels(plotter.XYLabels{
		s.framerateLine,
		s.framerateLabels,
	})
	s.bitratePlot.Add(labels)
}

func (s *SlicePlot) save() {
	// latest on foreground

	// bitrate plot
	if s.kind == "video" {
		createLinePoints(s.bitratePlot,
			"input width (pixels)",
			s.widthLine,
			1,
			0,
			color.RGBA{R: 34, G: 153, B: 166, A: 255},
			draw.CrossGlyph{},
			smallGlyph,
		)
		createLinePoints(s.bitratePlot,
			"framerate (per second)",
			s.framerateLine,
			0,
			0,
			color.RGBA{R: 0, G: 0, B: 0, A: 255},
			draw.CrossGlyph{},
			smallGlyph,
		)
		s.createFramerateLabels()
	}
	createLinePoints(s.bitratePlot,
		"input",
		s.inputLine,
		1,
		0,
		color.RGBA{R: 0, G: 223, B: 162, A: 255},
		draw.CrossGlyph{},
		smallGlyph,
	)
	for toUserId, line := range s.senderCCOptimalLines {
		createLinePoints(s.bitratePlot,
			"gcc-optimal-output-"+toUserId,
			line,
			1,
			1,
			color.RGBA{R: 160, G: 160, B: 160, A: 255},
			draw.CircleGlyph{},
			smallGlyph,
		)
	}
	for toUserId, line := range s.senderLossOptimalLines {
		createLinePoints(s.bitratePlot,
			"loss-optimal-output-"+toUserId,
			line,
			1,
			5,
			color.RGBA{R: 190, G: 190, B: 190, A: 255},
			draw.CircleGlyph{},
			smallGlyph,
		)
	}
	createLinePoints(s.bitratePlot,
		"target",
		s.targetLine,
		1,
		5,
		color.RGBA{R: 40, G: 40, B: 40, A: 255},
		draw.CircleGlyph{},
		smallGlyph,
	)
	createLinePoints(s.bitratePlot,
		"output",
		s.outputLine,
		1,
		0,
		color.RGBA{R: 242, G: 151, B: 39, A: 255},
		draw.CircleGlyph{},
		smallGlyph,
	)
	// createLinePoints(s.bitratePlot,
	// 	"rtp-diff-in-ms",
	// 	s.rtpDiffIn,
	// 	1,
	// 	0,
	// 	color.RGBA{R: 100, G: 151, B: 200, A: 255},
	// 	draw.PyramidGlyph{},
	// 	smallGlyph,
	// )
	if s.kind == "video" {
		createLinePoints(s.bitratePlot,
			"keyframe (event)",
			s.keyframeLine,
			0,
			0,
			color.RGBA{R: 255, G: 0, B: 0, A: 255},
			draw.TriangleGlyph{},
			bigGlyph,
		)
	}
	// save, may fail silently
	s.bitratePlot.Save(6*vg.Inch, 4*vg.Inch, s.folder+"/"+s.kind+"-"+s.id+"-bitrates.pdf")

	// buffer plot
	if s.kind == "video" && s.plotBuffers {
		counter := 0
		for name, line := range s.currentLevelTimeLines {
			createLinePoints(s.bufferPlot,
				name,
				line,
				1,
				1,
				getColorFromInt(counter),
				draw.PlusGlyph{},
				smallGlyph,
			)
			counter++
		}
		// save, may fail silently
		s.bufferPlot.Save(6*vg.Inch, 4*vg.Inch, s.folder+"/"+s.kind+"-"+s.id+"-buffer.pdf")
	}

}

// API

func NewSlicePlot(controller types.Terminable, kind string, plotBuffers bool, id, folder string) *SlicePlot {
	return &SlicePlot{
		kind:                   kind,
		plotBuffers:            plotBuffers,
		id:                     id,
		controller:             controller,
		folder:                 folder,
		bitratePlot:            newPlot(id+"'s "+kind+" bitrates", "seconds", "kbits/s"),
		bufferPlot:             newPlot(id+"'s "+kind+" buffers", "seconds", "ms"),
		senderCCOptimalLines:   make(map[string]plotter.XYs),
		senderLossOptimalLines: make(map[string]plotter.XYs),
		currentLevelTimeLines:  make(map[string]plotter.XYs),
	}
}

func (s *SlicePlot) addSimpleTarget(bps int) {
	s.targetLine = append(s.targetLine, plotter.XY{s.elapsed(), float64(bps) / 1000})
}

func (s *SlicePlot) addSimpleWidth(width int) {
	s.widthLine = append(s.widthLine, plotter.XY{s.elapsed(), float64(width)})
}

func (s *SlicePlot) repeatLastTarget() {
	lenTarget := s.targetLine.Len()
	if lenTarget > 0 {
		_, lastTargetY := s.targetLine.XY(lenTarget - 1)
		s.addSimpleTarget(int(lastTargetY * 1000)) // repeat last
	}
}

func (s *SlicePlot) repeatLastWidth() {
	lenWidth := s.widthLine.Len()
	if lenWidth > 0 {
		_, lastWidthY := s.widthLine.XY(lenWidth - 1)
		s.addSimpleWidth(int(lastWidthY)) // repeat last
	}
}

func (s *SlicePlot) Loop() {
	s.started = true
	s.startedAt = time.Now()
	// initial data
	s.AddInput(0)
	s.AddOutput(0)
	defaultBitrate := config.SFU.Audio.DefaultBitrate
	if s.kind == "video" {
		defaultBitrate = config.SFU.Video.DefaultBitrate
	}
	s.AddTarget(defaultBitrate)
	// wait till ms is done
	<-s.controller.Done()
	// final data
	s.repeatLastTarget()
	s.repeatLastWidth()
	s.save()
}

func (s *SlicePlot) AddResolution(resolution string) {
	widthString := strings.Split(resolution, "x")[0]
	width, err := strconv.Atoi(widthString)
	if err != nil {
		return
	}
	// add two points to display as constant bps
	s.repeatLastWidth()
	s.addSimpleWidth(width)
}

func (s *SlicePlot) AddFramerate(framerate string) {
	f, err := strconv.Atoi(framerate)
	if err != nil {
		return
	}
	s.framerateLine = append(s.framerateLine, plotter.XY{s.elapsed(), float64(f) * 30})
	s.framerateLabels = append(s.framerateLabels, framerate)
}

func (s *SlicePlot) AddKeyFrame() {
	s.keyframeLine = append(s.keyframeLine, plotter.XY{s.elapsed(), 1500})
}

func (s *SlicePlot) AddInput(bps int) {
	s.inputLine = append(s.inputLine, plotter.XY{s.elapsed(), float64(bps) / 1000})
}

func (s *SlicePlot) AddOutput(bps int) {
	s.outputLine = append(s.outputLine, plotter.XY{s.elapsed(), float64(bps) / 1000})
}

func (s *SlicePlot) AddTarget(bps int) {
	// add two points to display as constant bps
	s.repeatLastTarget()
	s.addSimpleTarget(bps)
}

// func (s *SlicePlot) AddRtpDiffIn(diff int64) {
// 	s.rtpDiffIn = append(s.rtpDiffIn, plotter.XY{s.elapsed(), float64(diff)})
// }

func (s *SlicePlot) AddSenderCCOptimal(toUserId string, bps int) {
	line := s.senderCCOptimalLines[toUserId]
	s.senderCCOptimalLines[toUserId] = append(line, plotter.XY{s.elapsed(), float64(bps) / 1000})
}

func (s *SlicePlot) AddSenderLossOptimal(toUserId string, bps int) {
	line := s.senderLossOptimalLines[toUserId]
	s.senderLossOptimalLines[toUserId] = append(line, plotter.XY{s.elapsed(), float64(bps) / 1000})
}

func (s *SlicePlot) AddCurrentLevelTime(element string, level uint64) {
	l := float64(level)
	if l > 2000 { // don't pollute plot scale
		l = 2000
	}
	line := s.currentLevelTimeLines[element]
	s.currentLevelTimeLines[element] = append(line, plotter.XY{s.elapsed(), l})
}
