package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"soundcheck/internal/core"
	"soundcheck/internal/net"
	"soundcheck/internal/relay"
	"soundcheck/internal/render"
)

type roomsFetchResult struct {
	rooms []relay.RoomInfo
	err   error
}

type GameApp struct {
	mode      string
	port      int
	addr      string
	useRelay  bool
	roomCode  string
	tickCount uint64

	// Состояние хоста
	world  *core.World
	server *net.Server

	// Состояние клиента
	client *net.Client

	// Стартовое меню выбора режима (если не задан -mode)
	inMenu    bool
	menu      render.MenuState
	roomsChan chan roomsFetchResult

	// Общий рендерер и последний снапшот
	renderer   *render.Renderer
	latestSnap core.WorldSnapshot
	hasSnap    bool
	netStatus  string
}

func NewGameApp(mode string, port int, addr string, relayAddr string, code string, public bool, showMenu bool) (*GameApp, error) {
	if relayAddr == "" {
		relayAddr = net.DefaultRelayAddr
	}
	app := &GameApp{
		mode:      mode,
		port:      port,
		addr:      addr,
		inMenu:    showMenu,
		roomsChan: make(chan roomsFetchResult, 4),
		renderer:  render.NewRenderer(),
		menu: render.MenuState{
			RelayAddr:   relayAddr,
			RoomCode:    strings.ToUpper(strings.TrimSpace(code)),
			DirectAddr:  addr,
			Port:        port,
			PublicRoom:  public,
			ActiveField: render.MenuFieldRoomCode,
		},
	}

	if showMenu {
		app.refreshRooms()
		return app, nil
	}

	if mode == "host" {
		if code != "" || relayAddr != net.DefaultRelayAddr {
			if err := app.startRelayHost(); err != nil {
				return nil, err
			}
		} else {
			if err := app.startDirectHost(); err != nil {
				return nil, err
			}
		}
	} else {
		if code != "" {
			app.startRelayClient(relayAddr, code)
		} else {
			app.startDirectClient(addr)
		}
	}

	return app, nil
}

func (g *GameApp) refreshRooms() {
	if g.menu.LoadingList {
		return
	}
	target := strings.TrimSpace(g.menu.RelayAddr)
	if target == "" {
		return
	}
	g.menu.LoadingList = true
	go func() {
		rooms, err := net.FetchPublicRooms(target)
		g.roomsChan <- roomsFetchResult{rooms: rooms, err: err}
	}()
}

func (g *GameApp) startDirectHost() error {
	g.mode = "host"
	g.useRelay = false
	g.world = core.NewWorld()
	g.server = net.NewServer(g.port, g.world)
	if err := g.server.Start(); err != nil {
		return fmt.Errorf("не удалось запустить сервер на порту %d: %w", g.port, err)
	}
	g.netStatus = g.formatHostStatus(false)
	g.inMenu = false
	g.menu.ErrMsg = ""
	g.updateWindowTitle()
	return nil
}

func (g *GameApp) startRelayHost() error {
	relayAddr := strings.TrimSpace(g.menu.RelayAddr)
	if relayAddr == "" {
		relayAddr = net.DefaultRelayAddr
	}
	g.mode = "host"
	g.useRelay = true
	g.world = core.NewWorld()
	g.server = net.NewServer(g.port, g.world)
	roomName := "Soundcheck 19:59"
	code, err := g.server.StartRelay(relayAddr, g.menu.PublicRoom, roomName)
	if err != nil {
		return err
	}
	g.roomCode = code
	g.netStatus = g.formatHostStatus(false)
	g.inMenu = false
	g.menu.ErrMsg = ""
	g.updateWindowTitle()
	return nil
}

func (g *GameApp) startDirectClient(addr string) {
	g.mode = "join"
	g.useRelay = false
	g.addr = addr
	g.client = net.NewClient(addr)
	g.client.Start()
	g.netStatus = g.formatClientStatus(false, "")
	g.inMenu = false
	g.menu.ErrMsg = ""
	g.updateWindowTitle()
}

func (g *GameApp) startRelayClient(relayAddr, code string) {
	g.mode = "join"
	g.useRelay = true
	g.roomCode = strings.ToUpper(strings.TrimSpace(code))
	g.addr = relayAddr
	g.client = net.NewRelayClient(relayAddr, g.roomCode)
	g.client.Start()
	g.netStatus = g.formatClientStatus(false, "")
	g.inMenu = false
	g.menu.ErrMsg = ""
	g.updateWindowTitle()
}

func (g *GameApp) formatHostStatus(p2Connected bool) string {
	if g.useRelay {
		code := g.server.RelayCode()
		if code == "" {
			code = g.roomCode
		}
		vis := "ЗАКР"
		if g.menu.PublicRoom {
			vis = "ОТКР"
		}
		if g.renderer.Lang() == core.LangEN {
			vis = "PRIVATE"
			if g.menu.PublicRoom {
				vis = "PUBLIC"
			}
			if p2Connected {
				return fmt.Sprintf("RELAY CODE: %s — PLAYER 2 CONNECTED", code)
			}
			return fmt.Sprintf("RELAY CODE: %s (%s) — WAITING FOR PLAYER 2", code, vis)
		}
		if p2Connected {
			return fmt.Sprintf("RELAY КОД: %s — ИГРОК 2 ПОДКЛЮЧЁН", code)
		}
		return fmt.Sprintf("RELAY КОД: %s (%s) — ОЖИДАНИЕ ИГРОКА 2", code, vis)
	}
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
	target := g.addr
	if g.useRelay {
		target = "RELAY [" + g.roomCode + "]"
	}
	if g.renderer.Lang() == core.LangEN {
		if connected {
			return fmt.Sprintf("CLIENT CONNECTED TO %s", target)
		}
		if errStr != "" {
			return fmt.Sprintf("RECONNECTING TO %s (%s)", target, errStr)
		}
		return fmt.Sprintf("CONNECTING TO %s...", target)
	}
	if connected {
		return fmt.Sprintf("КЛИЕНТ ПОДКЛЮЧЁН К %s", target)
	}
	if errStr != "" {
		return fmt.Sprintf("ПЕРЕПОДКЛЮЧЕНИЕ К %s (%s)", target, errStr)
	}
	return fmt.Sprintf("ПОДКЛЮЧЕНИЕ К %s...", target)
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
		select {
		case res := <-g.roomsChan:
			g.menu.LoadingList = false
			if res.err != nil {
				g.menu.Rooms = nil
				g.menu.ErrMsg = res.err.Error()
			} else {
				g.menu.Rooms = res.rooms
				g.menu.ErrMsg = ""
			}
		default:
		}

		// Ввод текста в активное поле
		var target *string
		maxLen := 64
		upper := false
		switch g.menu.ActiveField {
		case render.MenuFieldRelayAddr:
			target = &g.menu.RelayAddr
		case render.MenuFieldRoomCode:
			target = &g.menu.RoomCode
			maxLen = 10
			upper = true
		case render.MenuFieldDirectAddr:
			target = &g.menu.DirectAddr
		}
		if target != nil {
			for _, r := range ebiten.AppendInputChars(nil) {
				if r >= 32 && r != 127 && len(*target) < maxLen {
					ch := string(r)
					if upper {
						ch = strings.ToUpper(ch)
					}
					*target += ch
				}
			}
			bsDur := inpututil.KeyPressDuration(ebiten.KeyBackspace)
			if (bsDur == 1 || (bsDur > 18 && bsDur%3 == 0)) && len(*target) > 0 {
				runes := []rune(*target)
				*target = string(runes[:len(runes)-1])
			}
		}

		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			action, roomCode := g.renderer.HandleModeSelectClick(localInput.CursorX, localInput.CursorY, g.menu.Rooms)
			switch action {
			case render.ModeActionFocusRelay:
				g.menu.ActiveField = render.MenuFieldRelayAddr
			case render.ModeActionFocusCode:
				g.menu.ActiveField = render.MenuFieldRoomCode
			case render.ModeActionFocusDirect:
				g.menu.ActiveField = render.MenuFieldDirectAddr
			case render.ModeActionToggleVisibility:
				g.menu.PublicRoom = !g.menu.PublicRoom
			case render.ModeActionRefreshRooms:
				g.refreshRooms()
			case render.ModeActionHostRelay:
				if err := g.startRelayHost(); err != nil {
					g.menu.ErrMsg = err.Error()
				}
			case render.ModeActionHostDirect:
				if err := g.startDirectHost(); err != nil {
					g.menu.ErrMsg = err.Error()
				}
			case render.ModeActionJoinCode:
				code := strings.TrimSpace(g.menu.RoomCode)
				if code == "" {
					if g.renderer.Lang() == core.LangEN {
						g.menu.ErrMsg = "Enter a room code first"
					} else {
						g.menu.ErrMsg = "Сначала введите код комнаты"
					}
				} else {
					g.startRelayClient(g.menu.RelayAddr, code)
				}
			case render.ModeActionJoinPublicRoom:
				g.startRelayClient(g.menu.RelayAddr, roomCode)
			case render.ModeActionJoinDirect:
				targetAddr := strings.TrimSpace(g.menu.DirectAddr)
				if targetAddr == "" {
					targetAddr = g.addr
				}
				g.startDirectClient(targetAddr)
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
		g.renderer.DrawModeSelect(screen, g.menu)
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
	addr := flag.String("addr", "127.0.0.1:7777", "Адрес хоста для прямого подключения в режиме клиента")
	relayAddr := flag.String("relay", net.DefaultRelayAddr, "Адрес relay-сервера (host:port)")
	code := flag.String("code", "", "Код комнаты relay для быстрого подключения")
	public := flag.Bool("public", true, "Публиковать ли комнату в списке открытых комнат relay")
	flag.Parse()

	modeExplicit := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "mode" {
			modeExplicit = true
		}
	})

	app, err := NewGameApp(*mode, *port, *addr, *relayAddr, *code, *public, !modeExplicit)
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
