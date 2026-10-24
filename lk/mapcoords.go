package lk

// PixelToPos converts a pixel of a map image, counted in pixels from its
// top-left corner, to a feature Pos ([lat, lng]) in the map's coordinate
// frame:
//
//	lat = north + (y / height) × (south − north)
//	lng = west  + (x / width)  × (east − west)
//
// with img.Bounds = [[south, west], [north, east]] (DECISIONS.md D-014,
// A-37; the spec's MapImage gives the same formula). Pixels outside the
// image map linearly outside the bounds.
//
// An image with zero (or negative) width or height has no frame; PixelToPos
// then returns [0, 0], never NaN or an infinity.
func PixelToPos(img MapImage, x, y float64) [2]float64 {
	if img.Width <= 0 || img.Height <= 0 {
		return [2]float64{}
	}
	south, west := img.Bounds[0][0], img.Bounds[0][1]
	north, east := img.Bounds[1][0], img.Bounds[1][1]
	lat := north + y/float64(img.Height)*(south-north)
	lng := west + x/float64(img.Width)*(east-west)
	return [2]float64{lat, lng}
}

// PosToPixel converts a feature Pos ([lat, lng]) back to a pixel of the map
// image, counted from its top-left corner; it is the inverse of PixelToPos:
//
//	x = (lng − west)  × width  / (east − west)
//	y = (north − lat) × height / (north − south)
//
// An image with zero (or negative) width or height, or bounds of zero
// extent, has no frame; PosToPixel then returns 0 for the affected
// coordinate, never NaN or an infinity.
func PosToPixel(img MapImage, pos [2]float64) (x, y float64) {
	if img.Width <= 0 || img.Height <= 0 {
		return 0, 0
	}
	south, west := img.Bounds[0][0], img.Bounds[0][1]
	north, east := img.Bounds[1][0], img.Bounds[1][1]
	if east != west {
		x = (pos[1] - west) * float64(img.Width) / (east - west)
	}
	if north != south {
		y = (north - pos[0]) * float64(img.Height) / (north - south)
	}
	return x, y
}
