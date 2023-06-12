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

type BitratePlot struct {
	controller types.Terminable
	kind       string
	id         string
	folder     string
	plot       *plot.Plot
	// from client/input
	widthLine       plotter.XYs
	framerateLine   plotter.XYs
	framerateLabels []string
	keyframeLine    plotter.XYs
	inputLine       plotter.XYs
	// server-side
	outputLine            plotter.XYs
	targetLine            plotter.XYs
	senderCCOptimalLine   map[string]plotter.XYs
	senderLossOptimalLine map[string]plotter.XYs
	// state
	started   bool
	startedAt time.Time
}

// private

// seconds (float64)
func (p *BitratePlot) elapsed() float64 {
	if !p.started {
		// some initial points may be added before loop is started
		return 0
	}
	return float64(time.Since(p.startedAt).Milliseconds()) / 1000
}

func (p *BitratePlot) createLinePoints(label string, xys plotter.XYer, width, dashes float64, color color.Color, shape draw.GlyphDrawer, shapeSize float64) {
	line, points, _ := plotter.NewLinePoints(xys)
	line.LineStyle.Width = vg.Points(width)
	line.LineStyle.Color = color
	if dashes > 0 {
		line.LineStyle.Dashes = []vg.Length{vg.Points(dashes), vg.Points(dashes)}
	}
	points.Shape = shape
	points.Color = color
	points.Radius = vg.Points(shapeSize)
	p.plot.Add(line, points)
	p.plot.Legend.Add(label, line, points)
}

func (p *BitratePlot) createFramerateLabels() {
	labels, _ := plotter.NewLabels(plotter.XYLabels{
		p.framerateLine,
		p.framerateLabels,
	})
	p.plot.Add(labels)
}

func (p *BitratePlot) save() {
	// latest on foreground
	if p.kind == "video" {
		p.createLinePoints(
			"keyframe (event)",
			p.keyframeLine,
			0,
			0,
			color.RGBA{R: 255, G: 0, B: 0, A: 255},
			draw.TriangleGlyph{},
			bigGlyph,
		)
		p.createLinePoints(
			"input width (pixels)",
			p.widthLine,
			1,
			0,
			color.RGBA{R: 34, G: 153, B: 166, A: 255},
			draw.CrossGlyph{},
			smallGlyph,
		)
		p.createLinePoints(
			"framerate (per second)",
			p.framerateLine,
			0,
			0,
			color.RGBA{R: 0, G: 0, B: 0, A: 255},
			draw.CrossGlyph{},
			smallGlyph,
		)
		p.createFramerateLabels()
	}
	p.createLinePoints(
		"input",
		p.inputLine,
		1,
		0,
		color.RGBA{R: 0, G: 223, B: 162, A: 255},
		draw.CrossGlyph{},
		smallGlyph,
	)
	for toUserId, line := range p.senderCCOptimalLine {
		p.createLinePoints(
			"cc-optimal-output-"+toUserId,
			line,
			1,
			1,
			color.RGBA{R: 160, G: 160, B: 160, A: 255},
			draw.CircleGlyph{},
			smallGlyph,
		)
	}
	for toUserId, line := range p.senderLossOptimalLine {
		p.createLinePoints(
			"loss-optimal-output-"+toUserId,
			line,
			1,
			5,
			color.RGBA{R: 190, G: 190, B: 190, A: 255},
			draw.CircleGlyph{},
			smallGlyph,
		)
	}
	p.createLinePoints(
		"target",
		p.targetLine,
		1,
		5,
		color.RGBA{R: 40, G: 40, B: 40, A: 255},
		draw.CircleGlyph{},
		smallGlyph,
	)
	p.createLinePoints(
		"output",
		p.outputLine,
		1,
		0,
		color.RGBA{R: 242, G: 151, B: 39, A: 255},
		draw.CircleGlyph{},
		smallGlyph,
	)

	// Save the plot to a PNG file.
	if err := p.plot.Save(6*vg.Inch, 4*vg.Inch, p.folder+"/bitrates-"+p.id+"-"+p.kind+".pdf"); err != nil {
		panic(err)
	}
}

// API

func NewBitratePlot(controller types.Terminable, kind, id, folder string) *BitratePlot {
	numplot := plot.New()
	numplot.Title.Text = "Bitrates for the incoming " + kind + " of " + id
	numplot.X.Label.Text = "seconds"
	numplot.Y.Label.Text = "kbit/s"
	return &BitratePlot{
		kind:                  kind,
		id:                    id,
		controller:            controller,
		folder:                folder,
		plot:                  numplot,
		senderCCOptimalLine:   make(map[string]plotter.XYs),
		senderLossOptimalLine: make(map[string]plotter.XYs),
	}
}

func (p *BitratePlot) addSimpleTarget(bps int) {
	p.targetLine = append(p.targetLine, plotter.XY{p.elapsed(), float64(bps) / 1000})
}

func (p *BitratePlot) addSimpleWidth(width int) {
	p.widthLine = append(p.widthLine, plotter.XY{p.elapsed(), float64(width)})
}

func (p *BitratePlot) repeatLastTarget() {
	lenTarget := p.targetLine.Len()
	if lenTarget > 0 {
		_, lastTargetY := p.targetLine.XY(lenTarget - 1)
		p.addSimpleTarget(int(lastTargetY * 1000)) // repeat last
	}
}

func (p *BitratePlot) repeatLastWidth() {
	lenWidth := p.widthLine.Len()
	if lenWidth > 0 {
		_, lastWidthY := p.widthLine.XY(lenWidth - 1)
		p.addSimpleWidth(int(lastWidthY)) // repeat last
	}
}

func (p *BitratePlot) Loop() {
	p.started = true
	p.startedAt = time.Now()
	// initial data
	p.AddInput(0)
	p.AddOutput(0)
	defaultBitrate := config.SFU.Audio.DefaultBitrate
	if p.kind == "video" {
		defaultBitrate = config.SFU.Video.DefaultBitrate
	}
	p.AddTarget(defaultBitrate)
	// wait till ms is done
	<-p.controller.Done()
	// final data
	p.repeatLastTarget()
	p.repeatLastWidth()
	p.save()
}

func (p *BitratePlot) AddResolution(resolution string) {
	widthString := strings.Split(resolution, "x")[0]
	width, err := strconv.Atoi(widthString)
	if err != nil {
		return
	}
	// add two points to display as constant bps
	p.repeatLastWidth()
	p.addSimpleWidth(width)
}

func (p *BitratePlot) AddFramerate(framerate string) {
	f, err := strconv.Atoi(framerate)
	if err != nil {
		return
	}
	p.framerateLine = append(p.framerateLine, plotter.XY{p.elapsed(), float64(f) * 30})
	p.framerateLabels = append(p.framerateLabels, framerate)
}

func (p *BitratePlot) AddKeyFrame() {
	p.keyframeLine = append(p.keyframeLine, plotter.XY{p.elapsed(), 1500})
}

func (p *BitratePlot) AddInput(bps int) {
	p.inputLine = append(p.inputLine, plotter.XY{p.elapsed(), float64(bps) / 1000})
}

func (p *BitratePlot) AddOutput(bps int) {
	p.outputLine = append(p.outputLine, plotter.XY{p.elapsed(), float64(bps) / 1000})
}

func (p *BitratePlot) AddTarget(bps int) {
	// add two points to display as constant bps
	p.repeatLastTarget()
	p.addSimpleTarget(bps)
}

func (p *BitratePlot) AddSenderCCOptimal(toUserId string, bps int) {
	line := p.senderCCOptimalLine[toUserId]
	p.senderCCOptimalLine[toUserId] = append(line, plotter.XY{p.elapsed(), float64(bps) / 1000})
}

func (p *BitratePlot) AddSenderLossOptimal(toUserId string, bps int) {
	line := p.senderLossOptimalLine[toUserId]
	p.senderLossOptimalLine[toUserId] = append(line, plotter.XY{p.elapsed(), float64(bps) / 1000})
}
