package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
)

// generateTrayIcon creates a simple 16x16 icon with "PC" text
func generateTrayIcon() []byte {
	// Create 16x16 image
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))

	// Fill background with blue-ish color
	bgColor := color.RGBA{R: 40, G: 120, B: 200, A: 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)

	// Draw white border
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	for x := 0; x < 16; x++ {
		img.Set(x, 0, white)
		img.Set(x, 15, white)
	}
	for y := 0; y < 16; y++ {
		img.Set(0, y, white)
		img.Set(15, y, white)
	}

	// Simple "PC" text pattern (simplified pixel art)
	// P letter (left)
	for y := 3; y < 13; y++ {
		img.Set(3, y, white)
	}
	img.Set(4, 3, white)
	img.Set(5, 3, white)
	img.Set(6, 3, white)
	img.Set(6, 4, white)
	img.Set(6, 5, white)
	img.Set(6, 6, white)
	img.Set(5, 7, white)
	img.Set(4, 7, white)

	// C letter (right)
	for y := 5; y < 11; y++ {
		img.Set(9, y, white)
	}
	img.Set(10, 4, white)
	img.Set(11, 4, white)
	img.Set(12, 4, white)
	img.Set(10, 11, white)
	img.Set(11, 11, white)
	img.Set(12, 11, white)

	// Encode to PNG
	var buf bytes.Buffer
	png.Encode(&buf, img)

	// Create ICO format with PNG data
	return createICO(buf.Bytes())
}

// createICO wraps PNG data in ICO format
func createICO(pngData []byte) []byte {
	buf := new(bytes.Buffer)

	// ICO header
	binary.Write(buf, binary.LittleEndian, uint16(0)) // Reserved
	binary.Write(buf, binary.LittleEndian, uint16(1)) // Type (1 = ICO)
	binary.Write(buf, binary.LittleEndian, uint16(1)) // Number of images

	// Image directory entry
	buf.WriteByte(16)                                            // Width
	buf.WriteByte(16)                                            // Height
	buf.WriteByte(0)                                             // Color palette
	buf.WriteByte(0)                                             // Reserved
	binary.Write(buf, binary.LittleEndian, uint16(1))            // Color planes
	binary.Write(buf, binary.LittleEndian, uint16(32))           // Bits per pixel
	binary.Write(buf, binary.LittleEndian, uint32(len(pngData))) // Image size
	binary.Write(buf, binary.LittleEndian, uint32(22))           // Image offset (6 + 16)

	// PNG data
	buf.Write(pngData)

	return buf.Bytes()
}
