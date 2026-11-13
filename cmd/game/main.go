package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"soundcheck/internal/core"
	"soundcheck/internal/net"
	"soundcheck/internal/render"
)

type GameApp struct {
	mode      string
	port      int
	addr      string
	tickCount uint64

	// Состояние хоста
	world  *core.World
	server *net.Server

	// Состояние клиента
	client *net.Client

	// Стартовое меню выбора режима (если не задан -mode)
	inMenu    bool
	addrInput string
	menuErr   string

	// Общий рендерер и последний снапшот
	renderer   *render.Renderer
	latestSnap core.WorldSnapshot
	hasSnap    bool
	netStatus  string
}

func NewGameApp(mode string, port int, addr string, showMenu bool) (*GameApp, error) {
	app := &GameApp{
		mode:      mode,
		port:      port,
		addr:      addr,
		addrInput: addr,
		inMenu:    showMenu,
		renderer:  render.NewRenderer(),
	}

	if showMenu {
		return app, nil
	}

	if mode == "host" {
		if err := app.startHost(); err != nil {
			return nil, err
		}
	} else {
		app.startClient(addr)
	}

	return app, nil
}

func (g *GameApp) startHost() error {
	g.mode = "host"
	g.world = core.NewWorld()
	g.server = net.NewServer(g.port, g.world)
	if err := g.server.Start(); err != nil {
		return fmt.Errorf("не удалось запустить сервер на порту %d: %w", g.port, err)
	}
	g.netStatus = g.formatHostStatus(false)
	g.inMenu = false
	g.menuErr = ""
	g.updateWindowTitle()
	return nil
}

func (g *GameApp) startClient(addr string) {
	g.mode = "join"
	g.addr = addr
	g.client = net.NewClient(addr)
	g.client.Start()
	g.netStatus = g.formatClientStatus(false, "")
	g.inMenu = false
	g.menuErr = ""
	g.updateWindowTitle()
}

func (g *GameApp) formatHostStatus(p2Connected bool) string {
	if g.renderer.Lang() == core.LangEN {
		if p2Connected {
			return fmt.Sprintf("HOST (:%d) — PLAYER 2 CONNECTED", g.port)
		}
		return fmt.Sprintf("HOST (:%d) — SOLO / WAITING FOR PLAYER 2", g.port)
	}
	if p2Connected {
		return fmt.Sprintf("ХОСТ (:%d) — ИГРОК 2 ПОДКЛЮЧЁН", g.port)
	}
	return fmt.Sprintf("ХОСТ (:%d) — СОЛО / ОЖИДАНИЕ ИГРОКА 2", g.port)
}

func (g *GameApp) formatClientStatus(connected bool, errStr string) string {
	if g.renderer.Lang() == core.LangEN {
		if connected {
			return fmt.Sprintf("CLIENT CONNECTED TO %s", g.addr)
		}
		if errStr != "" {
			return fmt.Sprintf("RECONNECTING TO %s (%s)", g.addr, errStr)
		}
		return fmt.Sprintf("CONNECTING TO %s...", g.addr)
	}
	if connected {
		return fmt.Sprintf("КЛИЕНТ ПОДКЛЮЧЁН К %s", g.addr)
	}
	if errStr != "" {
		return fmt.Sprintf("ПЕРЕПОДКЛЮЧЕНИЕ К %s (%s)", g.addr, errStr)
	}
	return fmt.Sprintf("ПОДКЛЮЧЕНИЕ К %s...", g.addr)
}

func (g *GameApp) updateWindowTitle() {
	if g.renderer.Lang() == core.LangEN {
		modeTitle := "Host"
		if g.mode != "host" {
			modeTitle = "Client"
		}
		ebiten.SetWindowTitle(fmt.Sprintf("Soundcheck at 19:59 (%s)", modeTitle))
		return
	}
	modeTitle := "Хост"
	if g.mode != "host" {
		modeTitle = "Клиент"
	}
	ebiten.SetWindowTitle(fmt.Sprintf("Саундчек в 19:59 (%s)", modeTitle))
}

func captureLocalInput() core.PlayerInput {
	var inp core.PlayerInput

	if ebiten.IsKeyPressed(ebiten.KeyW) || ebiten.IsKeyPressed(ebiten.KeyUp) {
		inp.MoveY -= 1.0
	}
	if ebiten.IsKeyPressed(ebiten.KeyS) || ebiten.IsKeyPressed(ebiten.KeyDown) {
		inp.MoveY += 1.0
	}
	if ebiten.IsKeyPressed(ebiten.KeyA) || ebiten.IsKeyPressed(ebiten.KeyLeft) {
		inp.MoveX -= 1.0
	}
	if ebiten.IsKeyPressed(ebiten.KeyD) || ebiten.IsKeyPressed(ebiten.KeyRight) {
		inp.MoveX += 1.0
	}

	cx, cy := ebiten.CursorPosition()
	inp.CursorX = float64(cx)
	inp.CursorY = float64(cy)

	inp.Primary = ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	inp.Secondary = ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) || ebiten.IsKeyPressed(ebiten.KeySpace)
	inp.Interact = ebiten.IsKeyPressed(ebiten.KeyE)

	if ebiten.IsKeyPressed(ebiten.Key1) || ebiten.IsKeyPressed(ebiten.KeyNumpad1) {
		inp.SelectClass = 1
		inp.SelectItem = 1
	} else if ebiten.IsKeyPressed(ebiten.Key2) || ebiten.IsKeyPressed(ebiten.KeyNumpad2) {
		inp.SelectClass = 2
		inp.SelectItem = 2
	} else if ebiten.IsKeyPressed(ebiten.Key3) || ebiten.IsKeyPressed(ebiten.KeyNumpad3) {
		inp.SelectClass = 3
		inp.SelectItem = 3
	} else if ebiten.IsKeyPressed(ebiten.Key4) || ebiten.IsKeyPressed(ebiten.KeyNumpad4) {
		inp.SelectClass = 4
	}

	if ebiten.IsKeyPressed(ebiten.KeyEnter) || ebiten.IsKeyPressed(ebiten.KeySpace) {
		inp.ReadyToggle = true
	}

	return inp
}

func (g *GameApp) Update() error {
	g.tickCount++
	localInput := captureLocalInput()

	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		if g.renderer.HandleClick(localInput.CursorX, localInput.CursorY) {
			g.updateWindowTitle()
			localInput.Primary = false
		}
	} else if g.renderer.IsLangButtonHovered(localInput.CursorX, localInput.CursorY) {
		localInput.Primary = false
	}

	if g.inMenu {
		for _, r := range ebiten.AppendInputChars(nil) {
			if r >= 32 && r != 127 && len(g.addrInput) < 48 {
				g.addrInput += string(r)
			}
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && len(g.addrInput) > 0 {
			runes := []rune(g.addrInput)
			g.addrInput = string(runes[:len(runes)-1])
		}
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			switch g.renderer.HandleModeSelectClick(localInput.CursorX, localInput.CursorY) {
			case render.ModeActionHost:
				if err := g.startHost(); err != nil {
					g.menuErr = err.Error()
				}
			case render.ModeActionJoin:
				target := g.addrInput
				if target == "" {
					target = g.addr
				}
				g.startClient(target)
			}
		}
		return nil
	}

	if g.mode == "host" {
		// Хост обновляет ввод Игрока 0 и крутит авторитетную симуляцию на 60 TPS
		g.world.SetInput(0, localInput)
		g.world.Update()

		snap := g.world.DrainSnapshot()
		g.renderer.PlaySounds(snap.Sounds)

		// Рассылка снапшота клиенту 30 раз в секунду (каждые 2 тика)
		if g.tickCount%2 == 0 {
			g.server.BroadcastSnapshot(snap)
		}
		g.latestSnap = snap
		g.hasSnap = true
		g.netStatus = g.formatHostStatus(snap.Players[1].Connected)
	} else {
		// Клиент отправляет ввод хосту 30 раз в секунду (каждые 2 тика)
		if g.tickCount%2 == 0 {
			g.client.SendInput(localInput)
		}
		snap, has, connected, errStr := g.client.GetSnapshot()
		if has {
			g.renderer.PlaySounds(snap.Sounds)
			g.latestSnap = snap
			g.hasSnap = true
		}
		g.netStatus = g.formatClientStatus(connected, errStr)
	}

	return nil
}

func (g *GameApp) Draw(screen *ebiten.Image) {
	if g.inMenu {
		g.renderer.DrawModeSelect(screen, g.addrInput, g.port, g.menuErr)
		return
	}
	if !g.hasSnap {
		g.renderer.DrawConnecting(screen, g.netStatus)
		return
	}

	localID := 0
	if g.mode != "host" {
		localID = 1
	}
	g.renderer.Draw(screen, g.latestSnap, localID, g.netStatus)
}

func (g *GameApp) Layout(outsideWidth, outsideHeight int) (int, int) {
	return int(core.ArenaWidth), int(core.ArenaHeight)
}

func main() {
	mode := flag.String("mode", "host", "Сетевой режим: 'host' (хост) или 'join' (клиент)")
	port := flag.Int("port", 7777, "Порт для прослушивания в режиме хоста")
	addr := flag.String("addr", "127.0.0.1:7777", "Адрес хоста для подключения в режиме клиента")
	flag.Parse()

	modeExplicit := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "mode" {
			modeExplicit = true
		}
	})

	app, err := NewGameApp(*mode, *port, *addr, !modeExplicit)
	if err != nil {
		log.Fatalf("Ошибка инициализации: %v", err)
	}

	ebiten.SetWindowSize(int(core.ArenaWidth), int(core.ArenaHeight))
	app.updateWindowTitle()
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetTPS(int(core.TPS))

	if err := ebiten.RunGame(app); err != nil {
		log.Fatal(err)
	}
}
