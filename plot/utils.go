package plot

import (
	"image/color"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
)

func newPlot(title, x, y string) *plot.Plot {
	p := plot.New()
	p.Title.Text = title
	p.X.Label.Text = x
	p.Y.Label.Text = y
	grid := plotter.NewGrid()
	grid.Horizontal.Dashes = []vg.Length{vg.Points(3), vg.Points(3)}
	grid.Vertical.Dashes = []vg.Length{vg.Points(3), vg.Points(3)}
	p.Add(grid)
	return p
}

func createLinePoints(p *plot.Plot, label string, xys plotter.XYer, width, dashes float64, color color.Color, shape draw.GlyphDrawer, shapeSize float64) {
	line, points, _ := plotter.NewLinePoints(xys)
	line.LineStyle.Width = vg.Points(width)
	line.LineStyle.Color = color
	if dashes > 0 {
		line.LineStyle.Dashes = []vg.Length{vg.Points(dashes), vg.Points(dashes)}
	}
	points.Shape = shape
	points.Color = color
	points.Radius = vg.Points(shapeSize)
	p.Add(line, points)
	p.Legend.Add(label, line, points)
}

func getColorFromInt(counter int) color.RGBA {
	return color.RGBA{R: uint8((10 + 100*counter) % 255), G: uint8((240 - 50*counter) % 255), B: uint8((200 + 50*counter) % 255), A: 255}
}
