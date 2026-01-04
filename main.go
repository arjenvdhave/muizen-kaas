package main

import (
	"embed"
	"image"
	"image/color"
	"log"
	"math/rand"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/mp3"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

//go:embed sprites/*.png
var images embed.FS

//go:embed sounds/*.mp3
var sounds embed.FS

const (
	playerWidth  = 172
	playerHeight = 210
)

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
	playerX           float32
	playerY           float32
	playerDy          float32 // Vertical speed (Delta Y)
	width             int
	height            int
	spriteSheet       *ebiten.Image
	groundSprite      *ebiten.Image
	groundSprite2     *ebiten.Image
	level             []FloorTile // The pre-generated level data
	camera            Camera
	playerSprites     map[string]*ebiten.Image
	playerFacingRight bool
	audioContext      *audio.Context
	jumpSound         *audio.Player
	walkSound         *audio.Player
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
	groundLevel := float32(g.height - g.groundSprite.Bounds().Dy())

	// 1. Handle Input
	if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) {
		g.playerX -= speed
		g.playerFacingRight = false
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowRight) {
		g.playerX += speed
		g.playerFacingRight = true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		if g.playerDy == 0 {
			g.playerDy = -jumpPower
			g.jumpSound.Rewind()
			g.jumpSound.Play()
		}
	}

	// Handle walking sound
	isWalking := (ebiten.IsKeyPressed(ebiten.KeyArrowLeft) || ebiten.IsKeyPressed(ebiten.KeyArrowRight)) && g.playerDy == 0
	if isWalking && !g.walkSound.IsPlaying() {
		g.walkSound.Play()
	} else if !isWalking && g.walkSound.IsPlaying() {
		g.walkSound.Pause()
	}

	// 2. Apply Physics
	g.playerDy += gravity
	g.playerY += g.playerDy

	// 3. Resolve Collisions
	if g.playerY+playerHeight > groundLevel {
		g.playerY = groundLevel - playerHeight
		g.playerDy = 0
	}
	if g.playerX < 0 {
		g.playerX = 0
	}
	if g.playerX > float32(5000*53-playerWidth) {
		g.playerX = float32(5000*53 - playerWidth)
	}

	// 4. Update Camera
	g.camera.X = float64(g.playerX) - float64(g.width)/2
	if g.camera.X < 0 {
		g.camera.X = 0
	}
	levelWidth := float64(len(g.level) * g.groundSprite.Bounds().Dx())
	maxCameraX := levelWidth - float64(g.width)
	if g.camera.X > maxCameraX {
		g.camera.X = maxCameraX
	}

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{R: 0x5C, G: 0xD3, B: 0xFB, A: 0xFF})
	g.width = screen.Bounds().Dx()
	g.height = screen.Bounds().Dy()

	playerScreenX := float32(g.playerX) - float32(g.camera.X)

	var playerSprite *ebiten.Image
	if g.playerDy != 0 {
		if g.playerFacingRight {
			playerSprite = g.playerSprites["jump_right"]
		} else {
			playerSprite = g.playerSprites["jump_left"]
		}
	} else if ebiten.IsKeyPressed(ebiten.KeyArrowRight) {
		playerSprite = g.playerSprites["right"]
	} else if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) {
		playerSprite = g.playerSprites["left"]
	} else {
		playerSprite = g.playerSprites["stand"]
	}
	playerOp := &ebiten.DrawImageOptions{}
	playerOp.GeoM.Translate(float64(playerScreenX), float64(g.playerY))

	groundW := g.groundSprite.Bounds().Dx()
	startIndex := int(g.camera.X) / groundW
	if startIndex < 0 {
		startIndex = 0
	}
	tileCount := g.width/groundW + 2

	for i := 0; i < tileCount; i++ {
		tileIndex := startIndex + i
		if tileIndex >= len(g.level) {
			break
		}
		tileData := g.level[tileIndex]
		var tileImage *ebiten.Image
		if tileData.Type == TILE_LEAF {
			tileImage = g.groundSprite2
		} else {
			tileImage = g.groundSprite
		}
		op := &ebiten.DrawImageOptions{}
		worldX := float64(tileIndex * groundW)
		screenX := worldX - g.camera.X
		op.GeoM.Translate(screenX, float64(g.height-tileData.Height))
		screen.DrawImage(tileImage, op)
	}

	screen.DrawImage(playerSprite, playerOp)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func main() {
	rand.Seed(time.Now().UnixNano())

	audioContext := audio.NewContext(44100)
	soundFile, err := sounds.Open("sounds/jump.mp3")
	if err != nil {
		log.Fatal(err)
	}
	decodedStream, err := mp3.DecodeWithoutResampling(soundFile)
	if err != nil {
		log.Fatal(err)
	}
	jumpPlayer, err := audioContext.NewPlayer(decodedStream)
	if err != nil {
		log.Fatal(err)
	}

	// Load walk sound
	walkFile, err := sounds.Open("sounds/walk.mp3")
	if err != nil {
		log.Fatal(err)
	}
	walkStream, err := mp3.DecodeWithoutResampling(walkFile)
	if err != nil {
		log.Fatal(err)
	}
	walkPlayer, err := audioContext.NewPlayer(audio.NewInfiniteLoop(walkStream, walkStream.Length()))
	if err != nil {
		log.Fatal(err)
	}

	spriteSheet, _, err := ebitenutil.NewImageFromFileSystem(images, "sprites/sprite1.png")
	if err != nil {
		log.Fatal(err)
	}

	groundSprite := spriteSheet.SubImage(image.Rect(70, 380, 70+53, 380+53)).(*ebiten.Image)
	groundSprite2 := spriteSheet.SubImage(image.Rect(131, 364, 131+53, 364+69)).(*ebiten.Image)

	playerSprites := make(map[string]*ebiten.Image)
	playerSprites["stand"] = spriteSheet.SubImage(image.Rect(421, 1714, 421+playerWidth, 1714+playerHeight)).(*ebiten.Image)
	playerSprites["right"] = spriteSheet.SubImage(image.Rect(618, 1714, 618+playerWidth, 1714+playerHeight)).(*ebiten.Image)
	playerSprites["left"] = spriteSheet.SubImage(image.Rect(1099, 1714, 1099+playerWidth, 1714+playerHeight)).(*ebiten.Image)
	playerSprites["jump_right"] = spriteSheet.SubImage(image.Rect(861, 1714, 861+playerWidth, 1714+playerHeight)).(*ebiten.Image)
	playerSprites["jump_left"] = spriteSheet.SubImage(image.Rect(1318, 1714, 1318+playerWidth, 1714+playerHeight)).(*ebiten.Image)

	ebiten.SetWindowSize(640, 480)
	ebiten.SetWindowTitle("Muizen Kaas")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetFullscreen(true)

	game := &Game{
		playerX:           0,
		playerY:           float32(480 - groundSprite.Bounds().Dy() - playerHeight),
		spriteSheet:       spriteSheet,
		groundSprite:      groundSprite,
		groundSprite2:     groundSprite2,
		playerSprites:     playerSprites,
		playerFacingRight: true,
		audioContext:      audioContext,
		jumpSound:         jumpPlayer,
		walkSound:         walkPlayer,
	}

	game.generateLevel()

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}
