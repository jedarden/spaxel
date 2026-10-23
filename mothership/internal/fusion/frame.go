package fusion

// The engine works in an internal frame with Y up:
//
//	engine: X = width, Y = height above floor, Z = depth
//
// The fleet frame — what nodes report in their hello (pos_x/pos_y/pos_z),
// what the simulator's ground truth records, and what the /api/blobs REST
// surface serves — has Z up:
//
//	fleet:  X = width, Y = depth, Z = height
//
// The two frames differ by a transposition in the vertical plane. Every
// boundary crossing must convert: cmd/mothership feeds fleet-frame node
// positions through NodeFromFleet when seeding the engine, and converts
// engine-frame blobs back through BlobsToFleet when serving them. Feeding
// either side through unconverted distorts the reconstruction (node heights
// read as depths and vice versa) and systematically offsets every blob.
// These helpers are the single authority for that contract.

// NodeFromFleet converts a node position from the fleet frame to the engine
// frame. The conversion is its own inverse.
func NodeFromFleet(x, y, z float64) (ex, ey, ez float64) {
	return x, z, y
}

// BlobToFleet converts one engine-frame blob to the fleet frame. It returns
// a new Blob: Result.Blobs aliases the engine's cached last result and must
// never be mutated in place.
func BlobToFleet(b Blob) Blob {
	return Blob{X: b.X, Y: b.Z, Z: b.Y, Confidence: b.Confidence}
}

// BlobsToFleet converts engine-frame blobs to the fleet frame, returning a
// fresh slice. A nil input yields nil.
func BlobsToFleet(blobs []Blob) []Blob {
	if blobs == nil {
		return nil
	}
	out := make([]Blob, len(blobs))
	for i, b := range blobs {
		out[i] = BlobToFleet(b)
	}
	return out
}
