package sfu

import (
	"image/color"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
)

type mixerSlicePlot struct {
	filepath              string
	p                     *plot.Plot
	inputLine             plotter.XYs
	outputLine            plotter.XYs
	targetLine            plotter.XYs
	senderCCOptimalLine   map[string]plotter.XYs
	senderLossOptimalLine map[string]plotter.XYs
}

func newMixerSlicePlot(filepath string) *mixerSlicePlot {
	p := plot.New()
	p.Title.Text = "Bitrates"
	p.X.Label.Text = "seconds"
	p.Y.Label.Text = "kbit/s"
	msp := &mixerSlicePlot{filepath, p, nil, nil, nil, make(map[string]plotter.XYs), make(map[string]plotter.XYs)}
	msp.addInput(0, 0)
	msp.addOutput(0, 0)
	return msp
}

func (msp *mixerSlicePlot) addInput(ms, bps float64) {
	msp.inputLine = append(msp.inputLine, plotter.XY{ms / 1000, bps / 1000})
}

func (msp *mixerSlicePlot) addOutput(ms, bps float64) {
	msp.outputLine = append(msp.outputLine, plotter.XY{ms / 1000, bps / 1000})
}

func (msp *mixerSlicePlot) addTarget(ms, bps float64) {
	msp.targetLine = append(msp.targetLine, plotter.XY{ms / 1000, bps / 1000})
}

func (msp *mixerSlicePlot) addSenderCCOptimal(toUserId string, ms, bps float64) {
	line := msp.senderCCOptimalLine[toUserId]
	msp.senderCCOptimalLine[toUserId] = append(line, plotter.XY{ms / 1000, bps / 1000})
}

func (msp *mixerSlicePlot) addSenderLossOptimal(toUserId string, ms, bps float64) {
	line := msp.senderLossOptimalLine[toUserId]
	msp.senderLossOptimalLine[toUserId] = append(line, plotter.XY{ms / 1000, bps / 1000})
}

func (msp *mixerSlicePlot) createLinePoints(label string, xys plotter.XYer, dashes float64, color color.Color, shape draw.GlyphDrawer) {
	line, points, _ := plotter.NewLinePoints(xys)
	line.LineStyle.Width = vg.Points(1)
	line.LineStyle.Color = color
	if dashes > 0 {
		line.LineStyle.Dashes = []vg.Length{vg.Points(dashes), vg.Points(dashes)}
	}
	points.Shape = shape
	points.Color = color
	msp.p.Add(line, points)
	msp.p.Legend.Add(label, line, points)
}

func (msp *mixerSlicePlot) save() {
	msp.createLinePoints(
		"input",
		msp.inputLine,
		0,
		color.RGBA{R: 0, G: 223, B: 162, A: 255},
		draw.RingGlyph{},
	)
	msp.createLinePoints(
		"output",
		msp.outputLine,
		0,
		color.RGBA{R: 255, G: 0, B: 96, A: 255},
		draw.RingGlyph{},
	)
	msp.createLinePoints(
		"target",
		msp.targetLine,
		5,
		color.RGBA{R: 0, G: 121, B: 255, A: 255},
		draw.BoxGlyph{},
	)

	for toUserId, line := range msp.senderCCOptimalLine {
		msp.createLinePoints(
			"cc-optimal-output-"+toUserId,
			line,
			5,
			color.RGBA{R: 160, G: 160, B: 160, A: 255},
			draw.BoxGlyph{},
		)
	}
	for toUserId, line := range msp.senderLossOptimalLine {
		msp.createLinePoints(
			"loss-optimal-output-"+toUserId,
			line,
			5,
			color.RGBA{R: 160, G: 160, B: 160, A: 255},
			draw.TriangleGlyph{},
		)
	}

	// Save the plot to a PNG file.
	if err := msp.p.Save(6*vg.Inch, 4*vg.Inch, msp.filepath+".pdf"); err != nil {
		panic(err)
	}
}
