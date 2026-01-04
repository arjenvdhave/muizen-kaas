package main

import (
	"embed"
	"image"
	"image/color"
	"log"
	"math/rand"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

//go:embed sprites/*.png
var images embed.FS

// TileType defines the type of a floor tile.
type TileType int

const (
	TILE_NORMAL TileType = iota
	TILE_LEAF
)

// FloorTile represents a single tile in the level's floor.
type FloorTile struct {
	Type   TileType
	Height int
	Width  int
}

// Camera represents the view into the game world.
type Camera struct {
	X float64
	Y float64
}

type Game struct {
	playerX       float32
	playerY       float32
	playerDy      float32 // Vertical speed (Delta Y)
	width         int
	height        int
	spriteSheet   *ebiten.Image
	groundSprite  *ebiten.Image
	groundSprite2 *ebiten.Image
	level         []FloorTile // The pre-generated level data
	camera        Camera
}

func (g *Game) generateLevel() {
	g.level = make([]FloorTile, 5000)
	groundW := g.groundSprite.Bounds().Dx()
	groundH := g.groundSprite.Bounds().Dy()
	groundH2 := g.groundSprite2.Bounds().Dy()

	for i := 0; i < 5000; i++ {
		tileType := TILE_NORMAL
		height := groundH
		if rand.Float64() < 0.2 {
			tileType = TILE_LEAF
			height = groundH2
		}
		g.level[i] = FloorTile{
			Type:   tileType,
			Width:  groundW,
			Height: height,
		}
	}
}

func (g *Game) Update() error {
	// --- Constants for physics ---
	const (
		speed     = 4.0
		gravity   = 0.6
		jumpPower = 10.0
	)
	// The collision height is always based on the normal tile height (53px)
	groundLevel := float32(g.height - g.groundSprite.Bounds().Dy())

	// 1. Handle Input
	// =================
	if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) {
		g.playerX -= speed
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowRight) {
		g.playerX += speed
	}
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		if g.playerDy == 0 {
			g.playerDy = -jumpPower
		}
	}

	// 2. Apply Physics
	// =================
	g.playerDy += gravity
	g.playerY += g.playerDy

	// 3. Resolve Collisions
	// =====================
	if g.playerY+40 > groundLevel { // +40 is player height
		g.playerY = groundLevel - 40 // -40 is player height
		g.playerDy = 0
	}

	if g.playerX < 0 {
		g.playerX = 0
	}
	// For now, prevent player from going past the visible screen area
	if g.playerX > float32(g.width-40) {
		g.playerX = float32(g.width - 40)
	}

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{R: 0x5C, G: 0xD3, B: 0xFB, A: 0xFF})

	g.width = screen.Bounds().Dx()
	g.height = screen.Bounds().Dy()

	// Draw the player first, so they appear behind the ground
	playerColor := color.RGBA{R: 255, G: 0, B: 0, A: 255}
	vector.DrawFilledRect(screen, g.playerX, g.playerY, 40, 40, playerColor, false)

	// Draw the visible part of the level
	groundW := g.groundSprite.Bounds().Dx()
	tileCount := g.width/groundW + 1 // how many tiles fit on screen

	for i := 0; i < tileCount; i++ {
		// Later, we will add a camera offset to 'i' for scrolling
		tileData := g.level[i]

		var tileImage *ebiten.Image
		if tileData.Type == TILE_LEAF {
			tileImage = g.groundSprite2
		} else {
			tileImage = g.groundSprite
		}

		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(i*groundW), float64(g.height-tileData.Height))
		screen.DrawImage(tileImage, op)
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func main() {
	rand.Seed(time.Now().UnixNano())

	spriteSheet, _, err := ebitenutil.NewImageFromFileSystem(images, "sprites/sprite1.png")
	if err != nil {
		log.Fatal(err)
	}

	groundSprite := spriteSheet.SubImage(image.Rect(70, 380, 70+53, 380+53)).(*ebiten.Image)
	groundSprite2 := spriteSheet.SubImage(image.Rect(131, 364, 131+53, 364+69)).(*ebiten.Image)

	ebiten.SetWindowSize(640, 480)
	ebiten.SetWindowTitle("Muizen Kaas")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	game := &Game{
		playerX:       640 / 2,
		playerY:       float32(480 - groundSprite.Bounds().Dy()),
		spriteSheet:   spriteSheet,
		groundSprite:  groundSprite,
		groundSprite2: groundSprite2,
	}

	game.generateLevel() // Generate the 5000 tiles once

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}
