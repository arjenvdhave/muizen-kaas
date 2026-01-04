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
	"github.com/hajimehoshi/ebiten/v2/vector"
)

//go:embed sprites/*.png
var images embed.FS

//go:embed sounds/*.mp3
var sounds embed.FS

const (
	playerWidth       = 172
	playerHeight      = 210
	levelLength       = 5000 // Number of tiles in the level
	cloudWidth        = 14   // How many ground tiles a cloud block takes
	bushWidth         = 14   // How many ground tiles a bush block takes
	obstacleSlotWidth = 3    // How many ground tiles one obstacle takes
	obstacleMaxLength = 10   // Max number of obstacle slots in a sequence
	obstacleMaxHeight = 4
)

// TileType defines the type of a floor tile.
type TileType int

const (
	TILE_NORMAL TileType = iota
	TILE_LEAF
)

// CloudType defines the type of a sky tile. 0 is empty.
type CloudType int

const (
	CLOUD_EMPTY CloudType = iota
	CLOUD_1
	CLOUD_2
	CLOUD_3
)

// BushType defines the type of a bush tile. 0 is empty.
type BushType int

const (
	BUSH_EMPTY BushType = iota
	BUSH_1
	BUSH_2
)

// ObstacleType defines the type of an obstacle block.
type ObstacleType int

const (
	OBSTACLE_EMPTY ObstacleType = iota
	OBSTACLE_BUILD
	OBSTACLE_TOP
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

// ObstacleBlock represents a single block in an obstacle formation.
type ObstacleBlock struct {
	Type    ObstacleType
	OffsetX float64
}

type Game struct {
	playerX             float32
	playerY             float32
	playerDy            float32 // Vertical speed (Delta Y)
	width               int
	height              int
	spriteSheet         *ebiten.Image
	groundSprite        *ebiten.Image
	groundSprite2       *ebiten.Image
	level               []FloorTile // The pre-generated level data
	camera              Camera
	playerSprites       map[string]*ebiten.Image
	playerFacingRight   bool
	audioContext        *audio.Context
	jumpSound           *audio.Player
	walkSound           *audio.Player
	cloudSprites        []*ebiten.Image
	skyLayer1           []CloudType
	skyLayer2           []CloudType
	bushSprites         []*ebiten.Image
	bushLayer           []BushType
	obstacleBuildSprite *ebiten.Image
	obstacleTopSprite   *ebiten.Image
	obstacles           [][]ObstacleBlock // 2D grid for obstacles
}

func (g *Game) generateLevel() {
	g.level = make([]FloorTile, levelLength)
	groundW := g.groundSprite.Bounds().Dx()
	groundH := g.groundSprite.Bounds().Dy()
	groundH2 := g.groundSprite2.Bounds().Dy()

	for i := 0; i < levelLength; i++ {
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

func (g *Game) populateSkyLayer(layer []CloudType, cloudBlockWidth, minEmptyBlocks int) {
	i := 0
	for i < len(layer) {
		if rand.Float64() < 0.1 {
			if i+cloudBlockWidth+minEmptyBlocks < len(layer) {
				cloudChoice := CloudType(rand.Intn(len(g.cloudSprites)) + 1)
				layer[i] = cloudChoice
				i += cloudBlockWidth + minEmptyBlocks
			} else {
				i++
			}
		} else {
			i++
		}
	}
}

func (g *Game) generateSky() {
	g.skyLayer1 = make([]CloudType, levelLength)
	g.skyLayer2 = make([]CloudType, levelLength)
	g.populateSkyLayer(g.skyLayer1, cloudWidth, 5)
	g.populateSkyLayer(g.skyLayer2, cloudWidth, 25)
}

func (g *Game) generateBushes() {
	g.bushLayer = make([]BushType, levelLength)
	i := 0
	for i < len(g.bushLayer) {
		if rand.Float64() < 0.15 {
			if i+bushWidth+20 < len(g.bushLayer) {
				bushChoice := BushType(rand.Intn(len(g.bushSprites)) + 1)
				g.bushLayer[i] = bushChoice
				i += bushWidth + 20
			} else {
				i++
			}
		} else {
			i++
		}
	}
}

func (g *Game) generateObstacles() {
	// Using a 2D slice for obstacles: [x][y]
	// x is in ground tile units
	g.obstacles = make([][]ObstacleBlock, levelLength)
	for i := range g.obstacles {
		g.obstacles[i] = make([]ObstacleBlock, obstacleMaxHeight)
	}

	// Work in slots of obstacleSlotWidth ground tiles
	numSlots := levelLength / obstacleSlotWidth
	slot := 0

	for slot < numSlots {
		// Chance to start an obstacle sequence
		if rand.Float64() < 0.2 {
			sequenceLength := rand.Intn(obstacleMaxLength) + 1

			if slot+sequenceLength >= numSlots {
				break // Not enough space
			}

			// First obstacle must be height 1
			previousHeight := 1

			// Place obstacles in each slot of the sequence
			for seqIdx := 0; seqIdx < sequenceLength; seqIdx++ {
				currentSlot := slot + seqIdx
				tileIndex := currentSlot * obstacleSlotWidth

				// Determine height for this obstacle
				var currentHeight int

				if seqIdx == 0 {
					// First obstacle is always height 1
					currentHeight = 1
				} else {
					// Check how many times the same height appears by looking back in obstacles array
					sameHeightCount := 0
					checkIndex := tileIndex - obstacleSlotWidth

					// Look back max 2 slots (to check for 2 consecutive same heights)
					for i := 0; i < 2 && checkIndex >= 0; i++ {
						// Count how many blocks in this column (that's the height)
						prevHeight := 0
						for y := 0; y < obstacleMaxHeight; y++ {
							if g.obstacles[checkIndex][y].Type != OBSTACLE_EMPTY {
								prevHeight = y + 1
							}
						}

						if prevHeight == previousHeight && prevHeight > 0 {
							sameHeightCount++
							checkIndex -= obstacleSlotWidth
						} else {
							break
						}
					}

					// If we already had same height twice, force change
					if sameHeightCount >= 2 {
						// Must go up or down
						if previousHeight < obstacleMaxHeight && rand.Float64() < 0.5 {
							currentHeight = previousHeight + 1
						} else if previousHeight > 1 {
							currentHeight = rand.Intn(previousHeight-1) + 1
						} else {
							currentHeight = previousHeight + 1
						}
					} else {
						// Random choice: go up (+1), stay same, or go down
						choice := rand.Float64()
						if choice < 0.3 && previousHeight < obstacleMaxHeight {
							// Go up by 1
							currentHeight = previousHeight + 1
						} else if choice < 0.6 {
							// Stay same height
							currentHeight = previousHeight
						} else {
							// Go down (can be any amount, but min 1)
							if previousHeight > 1 {
								currentHeight = rand.Intn(previousHeight) + 1
							} else {
								currentHeight = 1
							}
						}
					}
				}

				// Build the obstacle column at this tile position
				for y := 0; y < currentHeight; y++ {
					if y == currentHeight-1 {
						g.obstacles[tileIndex][y] = ObstacleBlock{Type: OBSTACLE_TOP}
					} else {
						g.obstacles[tileIndex][y] = ObstacleBlock{Type: OBSTACLE_BUILD, OffsetX: 10}
					}
				}

				previousHeight = currentHeight
			}

			// Move past this sequence and add a gap
			slot += sequenceLength + 3 // 3 slot gap between sequences
		} else {
			slot++
		}
	}
}

func (g *Game) Update() error {
	const (
		speed     = 4.0
		gravity   = 0.6
		jumpPower = 15.0
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
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) || inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
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
	groundW := g.groundSprite.Bounds().Dx()
	blockH := float32(g.obstacleTopSprite.Bounds().Dy())
	blockW := float32(g.obstacleTopSprite.Bounds().Dx())

	// Check collision with obstacles
	playerLeft := g.playerX
	playerRight := g.playerX + playerWidth
	playerTop := g.playerY
	playerBottom := g.playerY + playerHeight

	onObstacle := false

	// Check tiles the player overlaps with
	startTile := int(playerLeft) / groundW
	endTile := int(playerRight) / groundW

	for tileIdx := startTile; tileIdx <= endTile && tileIdx < len(g.obstacles); tileIdx++ {
		if tileIdx < 0 {
			continue
		}

		for y := 0; y < obstacleMaxHeight; y++ {
			if g.obstacles[tileIdx][y].Type != OBSTACLE_EMPTY {
				// Calculate obstacle bounds
				obstacleLeft := float32(tileIdx*groundW) + float32(g.obstacles[tileIdx][y].OffsetX)
				obstacleRight := obstacleLeft + blockW
				obstacleTop := groundLevel - float32((y+1)*int(blockH))
				obstacleBottom := obstacleTop + blockH

				// Check if player overlaps with this obstacle block
				if playerRight > obstacleLeft && playerLeft < obstacleRight &&
					playerBottom > obstacleTop && playerTop < obstacleBottom {

					// Determine collision direction and resolve
					overlapLeft := playerRight - obstacleLeft
					overlapRight := obstacleRight - playerLeft
					overlapTop := playerBottom - obstacleTop
					overlapBottom := obstacleBottom - playerTop

					// Find minimum overlap to determine collision side
					minOverlap := overlapLeft
					collisionSide := "left"

					if overlapRight < minOverlap {
						minOverlap = overlapRight
						collisionSide = "right"
					}
					if overlapTop < minOverlap {
						minOverlap = overlapTop
						collisionSide = "top"
					}
					if overlapBottom < minOverlap {
						minOverlap = overlapBottom
						collisionSide = "bottom"
					}

					// Resolve collision based on side
					switch collisionSide {
					case "top":
						// Player landed on top of obstacle
						g.playerY = obstacleTop - playerHeight
						g.playerDy = 0
						onObstacle = true
					case "bottom":
						// Player hit bottom of obstacle (head bump)
						g.playerY = obstacleBottom
						g.playerDy = 0
					case "left":
						// Player hit left side
						g.playerX = obstacleLeft - playerWidth
					case "right":
						// Player hit right side
						g.playerX = obstacleRight
					}
				}
			}
		}
	}

	// Ground collision (only if not on obstacle)
	if !onObstacle && g.playerY+playerHeight > groundLevel {
		g.playerY = groundLevel - playerHeight
		g.playerDy = 0
	}

	// Screen boundaries
	if g.playerX < 0 {
		g.playerX = 0
	}
	if g.playerX > float32(levelLength*groundW-playerWidth) {
		g.playerX = float32(levelLength*groundW - playerWidth)
	}

	// 4. Update Camera
	g.camera.X = float64(g.playerX) - float64(g.width)/2
	if g.camera.X < 0 {
		g.camera.X = 0
	}
	levelWidth := float64(len(g.level) * groundW)
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

	drawSkyLayer(screen, g, g.skyLayer2, 120)
	drawSkyLayer(screen, g, g.skyLayer1, 90)

	drawBushes(screen, g)
	drawObstacles(screen, g) // Draw obstacles behind the player/ground

	drawPlayer(screen, g)
	drawGround(screen, g)

	barColor := color.RGBA{R: 0xDA, G: 0x9E, B: 0x61, A: 0xFF}
	vector.DrawFilledRect(screen, 0, 0, float32(g.width), 80, barColor, false)
}

func drawSkyLayer(screen *ebiten.Image, g *Game, layer []CloudType, yPos float64) {
	groundW := g.groundSprite.Bounds().Dx()
	const preRenderBuffer = 640
	startPixel := g.camera.X - preRenderBuffer
	startIndex := int(startPixel) / groundW
	if startIndex < 0 {
		startIndex = 0
	}
	tileCount := (g.width+preRenderBuffer)/groundW + 2
	for i := 0; i < tileCount; i++ {
		tileIndex := startIndex + i
		if tileIndex >= len(layer) {
			break
		}
		cloudType := layer[tileIndex]
		if cloudType != CLOUD_EMPTY {
			cloudSprite := g.cloudSprites[cloudType-1]
			op := &ebiten.DrawImageOptions{}
			worldX := float64(tileIndex * groundW)
			screenX := worldX - g.camera.X
			op.GeoM.Translate(screenX, yPos)
			screen.DrawImage(cloudSprite, op)
		}
	}
}

func drawBushes(screen *ebiten.Image, g *Game) {
	groundW := g.groundSprite.Bounds().Dx()
	groundLevel := float64(g.height - g.groundSprite.Bounds().Dy())
	const preRenderBuffer = 640
	startPixel := g.camera.X - preRenderBuffer
	startIndex := int(startPixel) / groundW
	if startIndex < 0 {
		startIndex = 0
	}
	tileCount := (g.width+preRenderBuffer)/groundW + 2
	for i := 0; i < tileCount; i++ {
		tileIndex := startIndex + i
		if tileIndex >= len(g.bushLayer) {
			break
		}
		bushType := g.bushLayer[tileIndex]
		if bushType != BUSH_EMPTY {
			bushSprite := g.bushSprites[bushType-1]
			op := &ebiten.DrawImageOptions{}
			worldX := float64(tileIndex * groundW)
			screenX := worldX - g.camera.X
			yPos := groundLevel - float64(bushSprite.Bounds().Dy())
			op.GeoM.Translate(screenX, yPos)
			screen.DrawImage(bushSprite, op)
		}
	}
}

func drawObstacles(screen *ebiten.Image, g *Game) {
	groundW := g.groundSprite.Bounds().Dx()
	blockH := g.obstacleTopSprite.Bounds().Dy()
	groundLevel := float64(g.height - g.groundSprite.Bounds().Dy())

	const preRenderBuffer = 640
	startPixel := g.camera.X - preRenderBuffer
	startIndex := int(startPixel) / groundW
	if startIndex < 0 {
		startIndex = 0
	}
	tileCount := (g.width+preRenderBuffer)/groundW + 2

	for i := 0; i < tileCount; i++ {
		tileIndex := startIndex + i
		if tileIndex >= len(g.obstacles) {
			break
		}

		for y, block := range g.obstacles[tileIndex] {
			if block.Type != OBSTACLE_EMPTY {
				var sprite *ebiten.Image
				if block.Type == OBSTACLE_TOP {
					sprite = g.obstacleTopSprite
				} else {
					sprite = g.obstacleBuildSprite
				}

				op := &ebiten.DrawImageOptions{}
				worldX := float64(tileIndex * groundW)
				screenX := worldX - g.camera.X
				yPos := groundLevel - float64((y+1)*blockH)

				op.GeoM.Translate(screenX+block.OffsetX, yPos)
				screen.DrawImage(sprite, op)
			}
		}
	}
}

func drawPlayer(screen *ebiten.Image, g *Game) {
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
	screen.DrawImage(playerSprite, playerOp)
}

func drawGround(screen *ebiten.Image, g *Game) {
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
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func main() {
	rand.Seed(time.Now().UnixNano())

	audioContext := audio.NewContext(44100)
	// ... (Audio loading remains the same)
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

	cloudSprites := make([]*ebiten.Image, 3)
	cloudSprites[0] = spriteSheet.SubImage(image.Rect(85, 776, 85+608, 776+324)).(*ebiten.Image)
	cloudSprites[1] = spriteSheet.SubImage(image.Rect(739, 836, 739+412, 836+212)).(*ebiten.Image)
	cloudSprites[2] = spriteSheet.SubImage(image.Rect(1208, 848, 1208+383, 848+180)).(*ebiten.Image)

	bushSprites := make([]*ebiten.Image, 2)
	bushSprites[0] = spriteSheet.SubImage(image.Rect(939, 1190, 939+608, 1190+239)).(*ebiten.Image)
	bushSprites[1] = spriteSheet.SubImage(image.Rect(248, 1172, 248+612, 1172+257)).(*ebiten.Image)

	obstacleBuildSprite := spriteSheet.SubImage(image.Rect(379, 380, 379+120, 380+130)).(*ebiten.Image)
	obstacleTopSprite := spriteSheet.SubImage(image.Rect(246, 380, 246+130, 380+130)).(*ebiten.Image)

	ebiten.SetWindowSize(640, 480)
	ebiten.SetWindowTitle("Muizen Kaas")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetFullscreen(true)

	game := &Game{
		playerX:             0,
		playerY:             float32(480 - groundSprite.Bounds().Dy() - playerHeight),
		spriteSheet:         spriteSheet,
		groundSprite:        groundSprite,
		groundSprite2:       groundSprite2,
		playerSprites:       playerSprites,
		playerFacingRight:   true,
		audioContext:        audioContext,
		jumpSound:           jumpPlayer,
		walkSound:           walkPlayer,
		cloudSprites:        cloudSprites,
		bushSprites:         bushSprites,
		obstacleBuildSprite: obstacleBuildSprite,
		obstacleTopSprite:   obstacleTopSprite,
	}

	game.generateLevel()
	game.generateSky()
	game.generateBushes()
	game.generateObstacles()

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}
