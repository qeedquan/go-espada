package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/qeedquan/go-media/image/imageutil"
	"github.com/qeedquan/go-media/sdl"
	"github.com/qeedquan/go-media/sdl/sdlgfx"
	"github.com/qeedquan/go-media/sdl/sdlimage/sdlcolor"
	"github.com/qeedquan/go-media/sdl/sdlmixer"
	"github.com/qeedquan/go-media/sdl/sdlttf"
)

const (
	WIDTH  = 640
	HEIGHT = 480
	BOTTOM = HEIGHT - 32
	FPS    = 60

	MAX_LASERS     = 5
	MAX_ENEMIES    = 4
	MAX_EXPLOSIONS = 16
	MAX_HEALTH     = 5
)

const (
	TITLE = iota
	PLAY
	GAMEOVER
)

const (
	KDL = 1 << iota
	KDR
	KDD
	KDU
	KDZ
	KDQ
	KDP
	KDI
	KRP
)

type Menu struct {
	Selection int
	Level     int
}

type Timer struct {
	Animation int
	Spawn     int
}

var (
	window   *sdl.Window
	renderer *sdl.Renderer
	conf     Config
	fps      sdlgfx.FPSManager
	run      bool
	paused   bool
	state    int
	menu     Menu
	canvas   *image.RGBA
	surface  *sdl.Surface
	texture  *sdl.Texture
	font     *sdlttf.Font

	ctls []*sdl.GameController

	background struct {
		Y int
	}

	player          *Player
	enemies         []*Enemy
	totalEnemies    int
	explosions      []*Explosion
	animationTimer  int
	enemySpawnTimer int
	enemyWaves      int
	transitionTimer int
	status          Status

	gfx struct {
		background *Image
		title      *Image
		menu       struct {
			cursor *Image
		}
		player *Image
		health struct {
			full  *Image
			empty *Image
		}
		laser struct {
			player *Image
			enemy  *Image
		}
		enemy     *Image
		enemy2    *Image
		explosion *Image
	}
	sfx struct {
		music *sdlmixer.Music
		fire  struct {
			player *sdlmixer.Chunk
			enemy  *sdlmixer.Chunk
		}
		explosion *sdlmixer.Chunk
	}
)

func main() {
	runtime.LockOSThread()
	rand.Seed(time.Now().UnixNano())
	parseFlags()
	initSDL()
	loadAssets()
	loop()
}

func ck(err error) {
	if err != nil {
		sdl.LogCritical(sdl.LOG_CATEGORY_APPLICATION, "%v", err)
		sdl.ShowSimpleMessageBox(sdl.MESSAGEBOX_ERROR, "Error", err.Error(), window)
		os.Exit(1)
	}
}

func ek(err error) bool {
	if err != nil {
		sdl.LogError(sdl.LOG_CATEGORY_APPLICATION, "%v", err)
		return true
	}
	return false
}

func randn(a, b int) int {
	return rand.Int()%(b-a+1) + a
}

func cyclic(x, a, b int) int {
	if x < a {
		return b
	}
	if x > b {
		return a
	}
	return x
}

func clamp(x, a, b int) int {
	if x < a {
		return a
	}
	if x > b {
		return b
	}
	return x
}

func parseFlags() {
	var flags Config
	flags.Defaults()
	flag.StringVar(&flags.Assets, "assets", flags.Assets, "assets directory")
	flag.StringVar(&flags.Pref, "pref", flags.Pref, "preference directory")
	flag.BoolVar(&flags.Invincible, "invincible", flags.Invincible, "invincible")
	flag.BoolVar(&flags.Fullscreen, "fullscreen", flags.Fullscreen, "fullscreen")
	flag.BoolVar(&flags.Sound, "sound", flags.Sound, "enable sound")
	flag.BoolVar(&flags.Music, "music", flags.Music, "enable music")
	flag.IntVar(&flags.Volume.Sound, "soundvol", flags.Volume.Sound, "sound volume")
	flag.IntVar(&flags.Volume.Music, "musicvol", flags.Volume.Music, "music volume")
	flag.Parse()

	conf.Assets = flags.Assets
	conf.Pref = flags.Pref
	conf.Load()
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "invincible":
			conf.Invincible = flags.Invincible
		case "fullscreen":
			conf.Fullscreen = flags.Fullscreen
		case "sound":
			conf.Sound = flags.Sound
		case "music":
			conf.Music = flags.Music
		case "soundvol":
			conf.Volume.Sound = flags.Volume.Sound
		case "musicvol":
			conf.Volume.Music = flags.Volume.Music
		}
	})
}

func initSDL() {
	err := sdl.Init(sdl.INIT_EVERYTHING &^ sdl.INIT_AUDIO)
	ck(err)

	err = sdlttf.Init()
	ck(err)

	err = sdl.InitSubSystem(sdl.INIT_AUDIO)
	ek(err)

	err = sdlmixer.OpenAudio(44100, sdl.AUDIO_S16, 2, 8192)
	ek(err)

	_, err = sdlmixer.Init(sdlmixer.INIT_OGG)
	ek(err)

	sdlmixer.AllocateChannels(128)

	sdl.SetHint(sdl.HINT_RENDER_SCALE_QUALITY, "best")

	w, h := WIDTH, HEIGHT
	wflag := sdl.WINDOW_RESIZABLE
	if conf.Fullscreen {
		wflag |= sdl.WINDOW_FULLSCREEN_DESKTOP
	}
	window, renderer, err = sdl.CreateWindowAndRenderer(w, h, wflag)
	ck(err)

	texture, err = renderer.CreateTexture(sdl.PIXELFORMAT_ABGR8888, sdl.TEXTUREACCESS_STREAMING, w, h)
	ck(err)

	canvas = image.NewRGBA(image.Rect(0, 0, w, h))

	surface, err = sdl.CreateRGBSurface(sdl.SWSURFACE, w, h, 32, 0x00FF0000, 0x0000FF00, 0x000000FF, 0xFF000000)
	ck(err)

	window.SetTitle("Espada")
	renderer.SetLogicalSize(w, h)
	renderer.Clear()
	renderer.Present()

	mapControllers()

	sdl.ShowCursor(0)

	fps.Init()
	fps.SetRate(FPS)
}

func mapControllers() {
	for _, c := range ctls {
		if c != nil {
			c.Close()
		}
	}

	ctls = make([]*sdl.GameController, sdl.NumJoysticks())
	for i, _ := range ctls {
		if sdl.IsGameController(i) {
			var err error
			ctls[i], err = sdl.GameControllerOpen(i)
			ek(err)
		}
	}
}

func loadAssets() {
	font = loadFont("LCD_Solid.ttf", 20)
	gfx.background = loadImage("background.png")
	gfx.title = loadImage("title.png")
	gfx.menu.cursor = loadImage("menu_cursor.png")
	gfx.player = loadImage("player_ship.png")
	gfx.health.full = loadImage("health_full.png")
	gfx.health.empty = loadImage("health_empty.png")
	gfx.laser.player = loadImage("laser.png")
	gfx.laser.enemy = loadImage("laser_enemy.png")
	gfx.enemy = loadImage("enemy_ship.png")
	gfx.enemy2 = loadImage("enemy_ship2.png")
	gfx.explosion = loadImage("explosion.png")
	sfx.music = loadMusic("music1.ogg")
	sfx.fire.player = loadSound("player_fire.wav")
	sfx.fire.enemy = loadSound("enemy_fire.wav")
	sfx.explosion = loadSound("explosion.wav")
}

func loadFont(name string, ptSize int) *sdlttf.Font {
	name = filepath.Join(conf.Assets, name)
	sdl.Log("loading font %v", name)
	font, err := sdlttf.OpenFont(name, ptSize)
	ck(err)
	return font
}

func loadImage(name string) *Image {
	name = filepath.Join(conf.Assets, name)
	sdl.Log("loading image %v", name)
	rgba, err := imageutil.LoadFile(name)
	ck(err)
	rgba = imageutil.ColorKey(rgba, color.RGBA{0xff, 0, 0xff, 0xff})
	return &Image{
		RGBA: rgba,
	}
}

func loadMusic(name string) *sdlmixer.Music {
	name = filepath.Join(conf.Assets, name)
	sdl.Log("loading music %v", name)
	mus, err := sdlmixer.LoadMUS(name)
	if ek(err) {
		return nil
	}
	return mus
}

func loadSound(name string) *sdlmixer.Chunk {
	name = filepath.Join(conf.Assets, name)
	sdl.Log("loading sound %v", name)
	snd, err := sdlmixer.LoadWAV(name)
	if ek(err) {
		return nil
	}
	return snd
}

func playMusic(mus *sdlmixer.Music) {
	if mus != nil && conf.Music {
		sdlmixer.VolumeMusic(conf.Volume.Music * 10)
		sdlmixer.FadeInMusic(mus, -1, 500)
	}
}

func playSFX(snd *sdlmixer.Chunk) {
	if snd != nil && conf.Sound {
		snd.Volume(conf.Volume.Sound * 10)
		snd.PlayChannel(0, 0)
	}
}

func collide(a, b sdl.Rect) bool {
	if a.Y+a.H <= b.Y {
		return false
	}
	if a.Y >= b.Y+b.H {
		return false
	}
	if a.X+a.W <= b.X {
		return false
	}
	if a.X >= b.X+b.W {
		return false
	}
	return true
}

func reset() {
	menu = Menu{}
	status = Status{}
	paused = false
	run = true
	state = TITLE
	sdlmixer.FadeOutMusic(500)
}

func newGame() {
	state = PLAY
	playMusic(sfx.music)
	player = newPlayer()
	enemies = make([]*Enemy, MAX_ENEMIES)
	for i := range enemies {
		enemies[i] = newEnemy()
	}
	explosions = make([]*Explosion, MAX_EXPLOSIONS)
	for i := range explosions {
		explosions[i] = newExplosion()
	}
	enemySpawnTimer = 180
	enemyWaves = 0
	totalEnemies = 0
	transitionTimer = 10
}

func subImage(m *Image, x, y, w, h int) *Image {
	return &Image{m.SubImage(image.Rect(x, y, x+w, y+h)).(*image.RGBA)}
}

func frameAdvance(frame *int, total int) {
	if animationTimer == 0 {
		if *frame++; *frame > total-1 {
			*frame = 0
		}
	}
}

func loop() {
	reset()
	for run {
		event()
		update()
		blit()
		fps.Delay()
	}
	conf.Save()
}

func event() {
	for {
		ev := sdl.PollEvent()
		if ev == nil {
			break
		}

		switch ev := ev.(type) {
		case sdl.QuitEvent:
			run = false
		case sdl.KeyDownEvent:
			if ev.Sym == sdl.K_ESCAPE {
				run = false
			}
		case sdl.ControllerDeviceAddedEvent:
			mapControllers()
			continue
		}

		key := keyState(ev)
		switch state {
		case TITLE:
			evTitle(key)
		case PLAY:
			action := actionState()
			evPlay(key, action)
		case GAMEOVER:
			evGameOver(key)
		}
	}
}

func keyState(ev sdl.Event) uint64 {
	var key uint64
	switch ev := ev.(type) {
	case sdl.KeyDownEvent:
		switch ev.Sym {
		case sdl.K_a, sdl.K_LEFT:
			key |= KDL
		case sdl.K_d, sdl.K_RIGHT:
			key |= KDR
		case sdl.K_w, sdl.K_UP:
			key |= KDU
		case sdl.K_s, sdl.K_DOWN:
			key |= KDD
		case sdl.K_z, sdl.K_SPACE:
			key |= KDZ
		case sdl.K_p, sdl.K_BACKSPACE:
			key |= KDP
		case sdl.K_q:
			key |= KDQ
		case sdl.K_i:
			key |= KDI
		}
		if ev.Repeat {
			key |= KRP
		}
	case sdl.ControllerButtonDownEvent:
		button := sdl.GameControllerButton(ev.Button)
		switch button {
		case sdl.CONTROLLER_BUTTON_DPAD_LEFT:
			key |= KDL
		case sdl.CONTROLLER_BUTTON_DPAD_RIGHT:
			key |= KDR
		case sdl.CONTROLLER_BUTTON_DPAD_UP:
			key |= KDU
		case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
			key |= KDD
		case sdl.CONTROLLER_BUTTON_A, sdl.CONTROLLER_BUTTON_B:
			key |= KDZ
		case sdl.CONTROLLER_BUTTON_X:
			key |= KDI
		case sdl.CONTROLLER_BUTTON_START:
			key |= KDP
		case sdl.CONTROLLER_BUTTON_BACK:
			key |= KDQ
		}
	}

	return key
}

func actionState() uint64 {
	var action uint64
	keys := sdl.GetKeyboardState()
	if keys[sdl.SCANCODE_LEFT] != 0 || keys[sdl.SCANCODE_A] != 0 {
		action |= KDL
	}
	if keys[sdl.SCANCODE_RIGHT] != 0 || keys[sdl.SCANCODE_D] != 0 {
		action |= KDR
	}
	if keys[sdl.SCANCODE_UP] != 0 || keys[sdl.SCANCODE_W] != 0 {
		action |= KDU
	}
	if keys[sdl.SCANCODE_DOWN] != 0 || keys[sdl.SCANCODE_S] != 0 {
		action |= KDD
	}
	if keys[sdl.SCANCODE_SPACE] != 0 || keys[sdl.SCANCODE_Z] != 0 {
		action |= KDZ
	}
	if keys[sdl.SCANCODE_I] != 0 {
		action |= KDI
	}
	for _, ctl := range ctls {
		if ctl.Button(sdl.CONTROLLER_BUTTON_DPAD_UP) != 0 {
			action |= KDU
		}
		if ctl.Button(sdl.CONTROLLER_BUTTON_DPAD_DOWN) != 0 {
			action |= KDD
		}
		if ctl.Button(sdl.CONTROLLER_BUTTON_DPAD_LEFT) != 0 {
			action |= KDL
		}
		if ctl.Button(sdl.CONTROLLER_BUTTON_DPAD_RIGHT) != 0 {
			action |= KDR
		}
		if ctl.Button(sdl.CONTROLLER_BUTTON_A) != 0 || ctl.Button(sdl.CONTROLLER_BUTTON_B) != 0 {
			action |= KDZ
		}
		if ctl.Button(sdl.CONTROLLER_BUTTON_X) != 0 {
			action |= KDI
		}
		if ctl.Button(sdl.CONTROLLER_BUTTON_START) != 0 {
			action |= KDP
		}
		if ctl.Button(sdl.CONTROLLER_BUTTON_BACK) != 0 {
			action |= KDQ
		}
	}
	return action
}

func moveMenuSelector(key uint64, max int) {
	if key&KDD != 0 {
		menu.Selection = cyclic(menu.Selection+1, 0, max)
	}
	if key&KDU != 0 {
		menu.Selection = cyclic(menu.Selection-1, 0, max)
	}
}

func evTitle(key uint64) {
	switch menu.Level {
	case 0: // main menu
		moveMenuSelector(key, 2)

		if key&KDZ == 0 {
			break
		}
		switch menu.Selection {
		case 0: // start
			newGame()
		case 1: // options
			menu.Level = 1
			menu.Selection = 0
		case 2: // quit
			run = false
		}
	case 1: // options
		moveMenuSelector(key, 5)
		if key&(KDZ|KDL|KDR) == 0 {
			break
		}

		switch menu.Selection {
		case 0: // fullscreen
			if key&KRP == 0 {
				conf.Fullscreen = !conf.Fullscreen
				if conf.Fullscreen {
					window.SetFullscreen(sdl.WINDOW_FULLSCREEN_DESKTOP)
				} else {
					window.SetFullscreen(0)
				}
			}
		case 1: // sound
			if key&KRP == 0 {
				conf.Sound = !conf.Sound
			}
		case 2: // music
			if key&KRP == 0 {
				conf.Music = !conf.Music
			}
		case 3: // sound volume
			if key&KDL != 0 {
				conf.Volume.Sound--
			}
			if key&KDR != 0 {
				conf.Volume.Sound++
			}
			conf.Volume.Sound = clamp(conf.Volume.Sound, 0, 12)
			sdlmixer.Volume(0, conf.Volume.Sound*10)
		case 4: // music volume
			if key&KDL != 0 {
				conf.Volume.Music--
			}
			if key&KDR != 0 {
				conf.Volume.Music++
			}
			conf.Volume.Music = clamp(conf.Volume.Music, 0, 12)
			sdlmixer.VolumeMusic(conf.Volume.Music * 10)
		case 5: // back
			if key&KDZ != 0 {
				menu.Level = 0
				menu.Selection = 0
			}
		}
	}
}

func evPlay(key, action uint64) {
	if key&KDI != 0 && key&KRP == 0 {
		conf.Invincible = !conf.Invincible
		sdl.Log("invincible: %v", toggle(conf.Invincible))
	}

	if key&KDP != 0 {
		paused = !paused

		if paused {
			status.Set("Game Paused | Press 'q' to quit", -1)
			sdlmixer.VolumeMusic(conf.Volume.Music / 2 * 10)
		} else {
			status.Set("", 0)
			sdlmixer.VolumeMusic(conf.Volume.Music * 10)
		}
	}

	if key&KDQ != 0 && paused {
		reset()
		return
	}

	if transitionTimer > 0 {
		transitionTimer--
	}

	player.Action = action
	if transitionTimer > 0 {
		player.Action &^= KDZ
	}
}

func evGameOver(key uint64) {
	if key&KDQ != 0 {
		reset()
		return
	}
}

func update() {
	if paused || state == TITLE {
		return
	}

	if state != GAMEOVER {
		player.InvulnTick()
		player.Move()
		player.Fire()
		testCollisions()
	}
	spawnEnemies()
	moveEnemies()
	enemiesFire()

	moveLasers()

	if state == GAMEOVER {
		status.Set("Game Over | Press 'q' to continue", -1)
	}

	animationTimer = cyclic(animationTimer-1, 0, 2)
}

func testCollisions() {
	for _, l := range player.Lasers {
		for _, e := range enemies {
			if !(player.Alive && l.Alive && e.Alive && e.Y+e.H >= 0 && collide(l.Rect, e.Rect)) {
				continue
			}
			e.Alive = false
			totalEnemies--
			l.Alive = false
			if e.Kind == 0 {
				player.Score += 50
			} else if e.Kind == 1 {
				player.Score += 100
			}
			spawnExplosion(int(e.X), int(e.Y))
			playSFX(sfx.explosion)
			break
		}
	}

	for _, e := range enemies {
		for _, l := range e.Lasers {
			if !(player.Alive && l.Alive && collide(player.Rect, l.Rect)) {
				continue
			}
			if !player.Invuln {
				l.Alive = false
				player.Damage(1)
			}
			break
		}
	}

	for _, e := range enemies {
		if !(e.Alive && player.Alive && collide(e.Rect, player.Rect)) {
			continue
		}
		if !player.Invuln {
			e.Alive = false
			totalEnemies--
			player.Damage(2)
		}
		break
	}
}

func blit() {
	draw.Draw(canvas, canvas.Bounds(), image.Black, image.ZP, draw.Src)
	blitBackground()
	switch state {
	case TITLE:
		blitTitle()
	case PLAY:
		player.Blit()
		fallthrough
	case GAMEOVER:
		blitEnemies()
		blitExplosions()
		blitLasers()
		blitInfo()
		status.Blit()
	}

	renderer.SetDrawColor(sdlcolor.Black)
	renderer.Clear()
	texture.Update(nil, canvas.Pix, canvas.Stride)
	renderer.Copy(texture, nil, nil)
	renderer.Present()
}

func blitBackground() {
	const scrollSpeed = 10
	if !paused {
		if background.Y < 640 {
			background.Y += scrollSpeed
		} else {
			background.Y = 0
		}
	}
	gfx.background.Blit(0, background.Y)
	gfx.background.Blit(0, background.Y-640)
}

func blitText(x, y int, text string) {
	r, err := font.RenderUTF8BlendedEx(surface, text, sdlcolor.White)
	ck(err)
	draw.Draw(canvas, image.Rect(x, y, x+int(r.W), y+int(r.H)), surface, image.ZP, draw.Over)
}

type toggle bool

func (t toggle) String() string {
	if t {
		return "on"
	}
	return "off"
}

func blitTitle() {
	options := [][]string{
		{"Start", "Options", "Quit"},
		{
			fmt.Sprintf("Fullscreen:   %v", toggle(conf.Fullscreen)),
			fmt.Sprintf("SFX:          %v", toggle(conf.Sound)),
			fmt.Sprintf("Music:        %v", toggle(conf.Music)),
			fmt.Sprintf("SFX Volume:   %v", conf.Volume.Sound),
			fmt.Sprintf("Music Volume: %v", conf.Volume.Music),
			"Back",
		},
	}
	xoff := 0
	if menu.Level == 1 {
		xoff = -30
	}
	for i, opt := range options[menu.Level] {
		blitText(280+xoff, 300+i*20, opt)
	}
	gfx.menu.cursor.Blit(260+xoff, 300+menu.Selection*20)
	gfx.title.Blit((WIDTH-486)/2, 50)
}

func blitInfo() {
	text := fmt.Sprintf("Score: %d", player.Score)
	blitText(5, 5+BOTTOM, text)

	blitText(WIDTH-200, 5+BOTTOM, "Health")

	for i := 0; i < MAX_HEALTH; i++ {
		if i < player.Health {
			gfx.health.full.Blit(WIDTH-120+i*18, 3+BOTTOM)
		} else {
			gfx.health.empty.Blit(WIDTH-120+i*18, 3+BOTTOM)
		}
	}
}

func blitEnemies() {
	for _, e := range enemies {
		e.Blit()
	}
}

func blitLasers() {
	for _, l := range player.Lasers {
		if l.Alive {
			l.Blit(int(l.X), int(l.Y))
		}
	}

	for _, e := range enemies {
		for _, l := range e.Lasers {
			if l.Alive {
				l.Blit(int(l.X), int(l.Y))
			}
		}
	}
}

func blitExplosions() {
	for _, e := range explosions {
		if e.Alive {
			e.Sheet[e.Frame].Blit(int(e.X), int(e.Y))
			if e.Frame++; e.Frame >= len(e.Sheet) {
				e.Alive = false
			}
		}
	}
}

type Config struct {
	Assets     string `json:"-"`
	Pref       string `json:"-"`
	Invincible bool   `json:"-"`
	Sound      bool
	Music      bool
	Fullscreen bool
	Volume     struct {
		Sound int
		Music int
	}
}

func (c *Config) Defaults() {
	c.Assets = filepath.Join(sdl.GetBasePath(), "assets")
	c.Pref = sdl.GetPrefPath("", "espada")
	c.Sound = true
	c.Music = true
	c.Fullscreen = false
	c.Volume.Sound = 6
	c.Volume.Music = 8
}

func (c *Config) Load() {
	var err error
	defer func() {
		if err != nil {
			c.Defaults()
		}
	}()

	name := filepath.Join(c.Pref, "espada.json")
	buf, err := ioutil.ReadFile(name)
	if err != nil {
		return
	}
	err = json.Unmarshal(buf, c)
}

func (c *Config) Save() {
	name := filepath.Join(c.Pref, "espada.json")
	sdl.Log("saving config to %v", name)
	buf, err := json.MarshalIndent(c, "", "\t")
	if ek(err) {
		return
	}
	ek(ioutil.WriteFile(name, buf, 0644))
}

type Image struct {
	*image.RGBA
}

func (m *Image) Blit(x, y int) {
	r := m.Bounds()
	draw.Draw(canvas, image.Rect(int(x), int(y), int(x)+r.Dx(), int(y)+r.Dy()), m.RGBA, r.Min, draw.Over)
}

type Entity struct {
	sdl.Rect
	Alive      bool
	Sheet      [2][2]*Image
	Frame      int
	Lasers     []*Laser
	LaserTimer int
}

type Player struct {
	Entity
	Health      int
	Score       int64
	Action      uint64
	Vx, Vy      int
	Invuln      bool
	InvulnTimer uint32
}

func newPlayer() *Player {
	p := &Player{
		Entity: Entity{
			Rect: sdl.Rect{
				X: 295,
				Y: BOTTOM - 64,
				W: 64,
				H: 64,
			},
			Alive:  true,
			Lasers: make([]*Laser, MAX_LASERS),
			Sheet: [2][2]*Image{
				{
					subImage(gfx.player, 0, 0, 64, 64),
					subImage(gfx.player, 64, 0, 64, 64),
				},
				{
					subImage(gfx.player, 0, 0, 64, 64),
					subImage(gfx.player, 64, 64, 64, 64),
				},
			},
		},
		Health: MAX_HEALTH,
	}
	for i := range p.Lasers {
		p.Lasers[i] = newLaser(gfx.laser.player)
	}
	return p
}

func (p *Player) InvulnTick() {
	if p.InvulnTimer != 0 {
		p.InvulnTimer--
	} else {
		p.Invuln = false
	}
}

func (p *Player) Move() {
	const maxSpeed = 8

	if p.Action&KDL != 0 {
		if p.Vx > -maxSpeed {
			p.Vx--
		}
	} else if p.Action&KDR != 0 {
		if p.Vx < maxSpeed {
			p.Vx++
		}
	}

	if p.Action&KDU != 0 {
		if p.Vy > -maxSpeed/2 {
			p.Vy--
		}
	} else if p.Action&KDD != 0 {
		if p.Vy < maxSpeed/2 {
			p.Vy++
		}
	}

	if p.Action&KDL == 0 {
		if p.Vx < 0 {
			p.Vx++
		}
	}

	if p.Action&KDR == 0 {
		if p.Vx > 0 {
			p.Vx--
		}
	}
	if p.Action&KDU == 0 {
		if p.Vy < 0 {
			p.Vy++
		}
	}
	if p.Action&KDD == 0 {
		if p.Vy > 0 {
			p.Vy--
		}
	}

	p.X += int32(p.Vx)
	p.Y += int32(p.Vy)

	if p.X < 0 {
		p.X = 0
	}
	if p.Y < 0 {
		p.Y = 0
	}
	if p.X+p.W > WIDTH {
		p.X = WIDTH - p.W
	}
	if p.Y+p.H > BOTTOM {
		p.Y = BOTTOM - p.H
	}
}

func (p *Player) Fire() {
	if p.Action&KDZ != 0 && p.LaserTimer == 0 {
		for _, l := range p.Lasers {
			if !l.Alive {
				l.Alive = true
				l.X = p.X + p.W/2
				l.Y = p.Y - l.H
				p.LaserTimer = 15
				playSFX(sfx.fire.player)
				break
			}
		}
	}

	if p.LaserTimer > 0 {
		p.LaserTimer--
	}
}

func (p *Player) Blit() {
	if !p.Alive {
		return
	}

	x, y := int(p.X), int(p.Y)
	if !p.Invuln {
		p.Sheet[0][p.Frame].Blit(x, y)
	} else {
		p.Sheet[1][p.Frame].Blit(x, y)

		r := p.Sheet[1][p.Frame].Bounds()
		alpha := image.NewUniform(color.RGBA{0, 0, 0, 127})
		draw.Draw(canvas, image.Rect(x, y, x+r.Dx(), y+r.Dy()), alpha, image.ZP, draw.Over)
	}

	frameAdvance(&p.Frame, len(p.Sheet[0]))
}

func (p *Player) Damage(d int) {
	if conf.Invincible {
		return
	}

	player.Invuln = true
	player.InvulnTimer = 100
	player.Health -= d
	playSFX(sfx.explosion)

	if player.Health <= 0 {
		player.Health = 0
		player.Alive = false
		spawnExplosion(int(player.X), int(player.Y))
		state = GAMEOVER
	}
}

type Enemy struct {
	Entity
	Kind       int
	PathLength int
	Dir        int
}

func newEnemy() *Enemy {
	e := &Enemy{
		Entity: Entity{
			Sheet: [2][2]*Image{
				{
					subImage(gfx.player, 0, 0, 64, 32),
					subImage(gfx.player, 64, 0, 64, 32),
				},
				{
					subImage(gfx.player, 0, 0, 64, 64),
					subImage(gfx.player, 64, 0, 64, 64),
				},
			},
			Lasers: make([]*Laser, MAX_LASERS),
		},
	}
	for i := range e.Lasers {
		e.Lasers[i] = newLaser(gfx.laser.enemy)
	}
	return e
}

func (e *Enemy) Blit() {
	if !e.Alive {
		return
	}

	x, y := int(e.X), int(e.Y)
	e.Sheet[e.Kind][e.Frame].Blit(x, y)
	frameAdvance(&e.Frame, len(e.Sheet[e.Kind]))
}

func (e *Enemy) Fire() {
	if e.LaserTimer == 0 && e.Alive && e.Y >= 0 {
		for _, l := range e.Lasers {
			if !l.Alive {
				l.Alive = true
				l.X = e.X + e.W/2
				l.Y = e.Y + e.H
				if e.Kind == 0 {
					e.LaserTimer = randn(100, 250)
				} else if e.Kind == 1 {
					e.LaserTimer = randn(50, 100)
				}
				playSFX(sfx.fire.enemy)
				break
			}
		}
	}

	if e.LaserTimer > 0 {
		e.LaserTimer--
	}
}

func (e *Enemy) Move() bool {
	if e.Alive {
		moveSpeed := int32(2)
		if e.Kind == 1 {
			moveSpeed = 3
		}

		if e.PathLength == 0 {
			e.PathLength = randn(10, WIDTH/2)
		}

		if e.PathLength != 0 {
			if e.Dir == 0 {
				if e.X+e.W < WIDTH {
					e.X += moveSpeed
					e.PathLength--
				}
				if e.X+e.W >= WIDTH || e.PathLength == 0 {
					e.Dir = 1
					e.PathLength = 0
				}
			} else if e.Dir == 1 {
				if e.X > 0 {
					e.X -= moveSpeed
					e.PathLength--
				}
				if e.X <= 0 || e.PathLength == 0 {
					e.Dir = 0
					e.PathLength = 0
				}
			}
		}

		e.Y++
	}

	if e.Y > BOTTOM+e.H {
		e.Alive = false
		totalEnemies--
		e.X = 0
		e.Y = 0
		if state != GAMEOVER {
			if e.Kind == 0 {
				player.Score -= 100
			} else if e.Kind == 1 {
				player.Score -= 200
			}
			if player.Score < 0 {
				player.Score = 0
			}
			return true
		}
	}

	return false
}

func spawnEnemies() {
	if totalEnemies == 0 {
		if enemySpawnTimer == 0 {
			for _, e := range enemies {
				if !e.Alive {
					if enemyWaves < 5 {
						e.W = 64
						e.H = 32
						e.Kind = 0
					} else {
						e.W = 64
						e.H = 64
						e.Kind = 1
					}
					e.Alive = true
					e.Frame = 0
					e.PathLength = 0
					e.LaserTimer = 0
					e.Dir = randn(0, 1)
					e.X = int32(randn(0, WIDTH-int(e.W)))
					e.Y = int32(randn(-192, -64))
					totalEnemies++
				}
			}
		}

		if enemySpawnTimer > 0 {
			enemySpawnTimer--
		}
	} else {
		enemySpawnTimer = 180
	}

	if enemySpawnTimer == 179 && totalEnemies == 0 {
		if enemyWaves < 1e9 {
			enemyWaves++
		}
		status.Set(fmt.Sprintf("Wave: %d", enemyWaves), 120)
	}
}

func enemiesFire() {
	for _, e := range enemies {
		e.Fire()
	}
}

func moveEnemies() {
	for _, e := range enemies {
		if e.Move() {
			break
		}
	}
}

type Laser struct {
	*Image
	sdl.Rect
	Alive bool
}

func newLaser(m *Image) *Laser {
	return &Laser{
		Image: m,
		Rect:  sdl.Rect{W: 8, H: 16},
	}
}

func moveLasers() {
	const moveSpeed = 10

	for _, l := range player.Lasers {
		if l.Alive {
			l.Y -= moveSpeed
		}
		if l.Y < 0 {
			l.Alive = false
		}
	}

	for _, e := range enemies {
		for _, l := range e.Lasers {
			if l.Alive {
				l.Y += moveSpeed / 2
			}
			if l.Y > HEIGHT {
				l.Alive = false
			}
		}
	}
}

type Explosion struct {
	sdl.Rect
	Sheet [8]*Image
	Frame int
	Alive bool
}

func newExplosion() *Explosion {
	return &Explosion{
		Sheet: [8]*Image{
			subImage(gfx.explosion, 0, 0, 64, 64),
			subImage(gfx.explosion, 64, 0, 64, 64),
			subImage(gfx.explosion, 128, 0, 64, 64),
			subImage(gfx.explosion, 192, 0, 64, 64),
			subImage(gfx.explosion, 64, 0, 64, 64),
			subImage(gfx.explosion, 128, 0, 64, 64),
			subImage(gfx.explosion, 192, 0, 64, 64),
			subImage(gfx.explosion, 192, 0, 64, 64),
		},
	}
}

func spawnExplosion(x, y int) {
	for _, e := range explosions {
		if !e.Alive {
			e.Alive = true
			e.Rect = sdl.Rect{int32(x), int32(y), 64, 64}
			e.Frame = 0
			break
		}
	}
}

type Status struct {
	Text    string
	Timeout int
}

func (s *Status) Set(text string, timeout int) {
	s.Text = text
	s.Timeout = timeout
}

func (s *Status) Blit() {
	if s.Timeout != 0 {
		x := (WIDTH - len(s.Text)*12) / 2
		blitText(x, 200, s.Text)
	}

	if s.Timeout > 0 {
		s.Timeout--
	}
}
