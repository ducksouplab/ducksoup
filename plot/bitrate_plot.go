package plot

import (
	"image/color"
	"time"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"

	"github.com/ducksouplab/ducksoup/config"
	"github.com/ducksouplab/ducksoup/types"
)

type BitratePlot struct {
	controller            types.Terminable
	kind                  string
	id                    string
	folder                string
	plot                  *plot.Plot
	inputLine             plotter.XYs
	outputLine            plotter.XYs
	targetLine            plotter.XYs
	senderCCOptimalLine   map[string]plotter.XYs
	senderLossOptimalLine map[string]plotter.XYs
	startedAt             time.Time
}

// private

// seconds (float64)
func (p *BitratePlot) elapsed() float64 {
	return float64(time.Since(p.startedAt).Milliseconds()) / 1000
}

func (p *BitratePlot) save() {
	p.createLinePoints(
		"input",
		p.inputLine,
		0,
		color.RGBA{R: 0, G: 223, B: 162, A: 255},
		draw.RingGlyph{},
	)
	p.createLinePoints(
		"output",
		p.outputLine,
		0,
		color.RGBA{R: 255, G: 0, B: 96, A: 255},
		draw.RingGlyph{},
	)
	p.createLinePoints(
		"target",
		p.targetLine,
		5,
		color.RGBA{R: 0, G: 121, B: 255, A: 255},
		draw.BoxGlyph{},
	)

	for toUserId, line := range p.senderCCOptimalLine {
		p.createLinePoints(
			"cc-optimal-output-"+toUserId,
			line,
			5,
			color.RGBA{R: 160, G: 160, B: 160, A: 255},
			draw.BoxGlyph{},
		)
	}
	for toUserId, line := range p.senderLossOptimalLine {
		p.createLinePoints(
			"loss-optimal-output-"+toUserId,
			line,
			5,
			color.RGBA{R: 160, G: 160, B: 160, A: 255},
			draw.TriangleGlyph{},
		)
	}

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

func (p *BitratePlot) Loop() {
	p.startedAt = time.Now()
	// initial data
	p.AddInput(0)
	p.AddOutput(0)
	defaultBitrate := config.SFU.Video.DefaultBitrate
	if p.kind == "audio" {
		defaultBitrate = config.SFU.Audio.DefaultBitrate
	}
	p.AddTarget(defaultBitrate)
	// wait till ms is done
	<-p.controller.Done()
	// final data
	lenTarget := p.targetLine.Len()
	_, lastTargetY := p.targetLine.XY(lenTarget - 1)
	p.AddTarget(int(lastTargetY * 1000))
	p.save()
}

func (p *BitratePlot) AddInput(bps int) {
	p.inputLine = append(p.inputLine, plotter.XY{p.elapsed(), float64(bps) / 1000})
}

func (p *BitratePlot) AddOutput(bps int) {
	p.outputLine = append(p.outputLine, plotter.XY{p.elapsed(), float64(bps) / 1000})
}

func (p *BitratePlot) AddTarget(bps int) {
	p.targetLine = append(p.targetLine, plotter.XY{p.elapsed(), float64(bps) / 1000})
}

func (p *BitratePlot) AddSenderCCOptimal(toUserId string, bps int) {
	line := p.senderCCOptimalLine[toUserId]
	p.senderCCOptimalLine[toUserId] = append(line, plotter.XY{p.elapsed(), float64(bps) / 1000})
}

func (p *BitratePlot) AddSenderLossOptimal(toUserId string, bps int) {
	line := p.senderLossOptimalLine[toUserId]
	p.senderLossOptimalLine[toUserId] = append(line, plotter.XY{p.elapsed(), float64(bps) / 1000})
}

func (p *BitratePlot) createLinePoints(label string, xys plotter.XYer, dashes float64, color color.Color, shape draw.GlyphDrawer) {
	line, points, _ := plotter.NewLinePoints(xys)
	line.LineStyle.Width = vg.Points(1)
	line.LineStyle.Color = color
	if dashes > 0 {
		line.LineStyle.Dashes = []vg.Length{vg.Points(dashes), vg.Points(dashes)}
	}
	points.Shape = shape
	points.Color = color
	p.plot.Add(line, points)
	p.plot.Legend.Add(label, line, points)
}
