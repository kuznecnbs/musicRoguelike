package render

import (
	"bytes"
	"fmt"
	"image/color"
	"math"
	"math/rand"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"soundcheck/internal/core"
)

type Renderer struct {
	SoundBank   *SoundBank
	rng         *rand.Rand
	lang        core.Lang
	regularSrc  *text.GoTextFaceSource
	boldSrc     *text.GoTextFaceSource
	faceSmall   *text.GoTextFace
	faceNormal  *text.GoTextFace
	faceTitle   *text.GoTextFace
	faceHeading *text.GoTextFace
}

const (
	langBtnX = float32(core.ArenaWidth - 108)
	langBtnY = float32(76)
	langBtnW = float32(92)
	langBtnH = float32(24)
)

func NewRenderer() *Renderer {
	regSrc, err := text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	if err != nil {
		panic(fmt.Sprintf("failed to load embedded regular font: %v", err))
	}
	boldSrc, err := text.NewGoTextFaceSource(bytes.NewReader(gobold.TTF))
	if err != nil {
		panic(fmt.Sprintf("failed to load embedded bold font: %v", err))
	}

	return &Renderer{
		SoundBank:   NewSoundBank(),
		rng:         rand.New(rand.NewSource(1959)),
		lang:        core.LangRU,
		regularSrc:  regSrc,
		boldSrc:     boldSrc,
		faceSmall:   &text.GoTextFace{Source: regSrc, Size: 12},
		faceNormal:  &text.GoTextFace{Source: regSrc, Size: 14},
		faceTitle:   &text.GoTextFace{Source: boldSrc, Size: 15},
		faceHeading: &text.GoTextFace{Source: boldSrc, Size: 20},
	}
}

func (r *Renderer) Lang() core.Lang {
	if r.lang == core.LangEN {
		return core.LangEN
	}
	return core.LangRU
}

func (r *Renderer) SetLang(lang core.Lang) {
	if lang == core.LangEN {
		r.lang = core.LangEN
	} else {
		r.lang = core.LangRU
	}
}

func (r *Renderer) ToggleLang() core.Lang {
	if r.Lang() == core.LangRU {
		r.lang = core.LangEN
	} else {
		r.lang = core.LangRU
	}
	return r.lang
}

func (r *Renderer) IsLangButtonHovered(x, y float64) bool {
	return x >= float64(langBtnX) && x <= float64(langBtnX+langBtnW) &&
		y >= float64(langBtnY) && y <= float64(langBtnY+langBtnH)
}

// HandleClick processes clicks on UI controls (currently the RU/EN language toggle button).
// Returns true if the click hit a UI element and should not trigger a combat attack.
func (r *Renderer) HandleClick(x, y float64) bool {
	if r.IsLangButtonHovered(x, y) {
		r.ToggleLang()
		return true
	}
	return false
}

func (r *Renderer) tr(ru, en string) string {
	if r.Lang() == core.LangEN {
		return en
	}
	return ru
}

func (r *Renderer) drawText(screen *ebiten.Image, str string, x, y float64, face text.Face, col color.RGBA) {
	// Subtle dark shadow for contrast over busy combat visuals
	shadowOp := &text.DrawOptions{}
	shadowOp.GeoM.Translate(x+1, y+1)
	shadowOp.ColorScale.ScaleWithColor(color.RGBA{0, 0, 0, 200})
	text.Draw(screen, str, face, shadowOp)

	op := &text.DrawOptions{}
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(col)
	text.Draw(screen, str, face, op)
}

func (r *Renderer) drawTextCentered(screen *ebiten.Image, str string, cx, y float64, face text.Face, col color.RGBA) {
	shadowOp := &text.DrawOptions{}
	shadowOp.GeoM.Translate(cx+1, y+1)
	shadowOp.PrimaryAlign = text.AlignCenter
	shadowOp.ColorScale.ScaleWithColor(color.RGBA{0, 0, 0, 200})
	text.Draw(screen, str, face, shadowOp)

	op := &text.DrawOptions{}
	op.GeoM.Translate(cx, y)
	op.PrimaryAlign = text.AlignCenter
	op.ColorScale.ScaleWithColor(col)
	text.Draw(screen, str, face, op)
}

func classColor(c core.ClassType) color.RGBA {
	switch c {
	case core.ClassBassist:
		return color.RGBA{50, 210, 255, 255} // Голубой суб-бас
	case core.ClassVocalist:
		return color.RGBA{255, 65, 155, 255} // Неоновый маджента фронтмен
	case core.ClassGuitarist:
		return color.RGBA{255, 215, 50, 255} // Электрик-золото рифф
	case core.ClassDrummer:
		return color.RGBA{85, 255, 110, 255} // Кислотно-зелёный темп
	default:
		return color.RGBA{200, 200, 200, 255}
	}
}

func effectColor(id int, alpha uint8) color.RGBA {
	switch id {
	case 1: // Жёлтый (Крит / Темп / Джем)
		return color.RGBA{255, 230, 50, alpha}
	case 2: // Оранжевый (Удар о стену / Ожог)
		return color.RGBA{255, 125, 35, alpha}
	case 3: // Зелёный (Лечение / Апгрейд)
		return color.RGBA{70, 245, 120, alpha}
	case 4: // Голубой (Бас / Цепь / Статус)
		return color.RGBA{75, 225, 255, alpha}
	case 5: // Красный/Маджента (Вокал / Урон по игроку)
		return color.RGBA{255, 75, 120, alpha}
	default: // Белый
		return color.RGBA{240, 240, 255, alpha}
	}
}

func (r *Renderer) PlaySounds(events []core.SoundEvent) {
	for _, ev := range events {
		r.SoundBank.Play(ev.Type)
	}
}

type ModeSelectAction int

const (
	ModeActionNone ModeSelectAction = iota
	ModeActionHost
	ModeActionJoin
)

const (
	menuBtnW = float32(460)
	menuBtnX = float32(core.ArenaWidth/2) - menuBtnW*0.5
	hostBtnY = float32(250)
	hostBtnH = float32(64)
	addrBoxY = float32(365)
	addrBoxH = float32(42)
	joinBtnY = float32(425)
	joinBtnH = float32(64)
)

func pointInRect(x, y float64, rx, ry, rw, rh float32) bool {
	return x >= float64(rx) && x <= float64(rx+rw) && y >= float64(ry) && y <= float64(ry+rh)
}

func (r *Renderer) HandleModeSelectClick(x, y float64) ModeSelectAction {
	if pointInRect(x, y, menuBtnX, hostBtnY, menuBtnW, hostBtnH) {
		return ModeActionHost
	}
	if pointInRect(x, y, menuBtnX, joinBtnY, menuBtnW, joinBtnH) {
		return ModeActionJoin
	}
	return ModeActionNone
}

func (r *Renderer) DrawModeSelect(screen *ebiten.Image, addrInput string, port int, errMsg string) {
	screen.Fill(color.RGBA{16, 16, 24, 255})

	// Декоративная рамка сцены
	vector.StrokeRect(screen, 16, 16, core.ArenaWidth-32, core.ArenaHeight-32, 2.0, color.RGBA{55, 65, 95, 255}, false)

	r.drawTextCentered(
		screen,
		r.tr("САУНДЧЕК В 19:59", "SOUNDCHECK AT 19:59"),
		core.ArenaWidth/2, 115, r.faceHeading, color.RGBA{90, 215, 255, 255},
	)
	r.drawTextCentered(
		screen,
		r.tr("Кооперативный музыкальный рогалик (1–2 игрока)", "Co-op Music Roguelike (1–2 Players)"),
		core.ArenaWidth/2, 152, r.faceNormal, color.RGBA{200, 210, 235, 255},
	)

	cx, cy := ebiten.CursorPosition()

	// Кнопка ХОСТ
	hostFill := color.RGBA{26, 36, 56, 255}
	hostBorder := color.RGBA{75, 195, 255, 255}
	if pointInRect(float64(cx), float64(cy), menuBtnX, hostBtnY, menuBtnW, hostBtnH) {
		hostFill = color.RGBA{38, 56, 86, 255}
		hostBorder = color.RGBA{145, 225, 255, 255}
	}
	vector.DrawFilledRect(screen, menuBtnX, hostBtnY, menuBtnW, hostBtnH, hostFill, false)
	vector.StrokeRect(screen, menuBtnX, hostBtnY, menuBtnW, hostBtnH, 2.5, hostBorder, false)
	r.drawTextCentered(
		screen,
		r.tr("СОЗДАТЬ ИГРУ (ХОСТ)", "HOST GAME"),
		core.ArenaWidth/2, float64(hostBtnY)+12, r.faceTitle, color.RGBA{240, 248, 255, 255},
	)
	r.drawTextCentered(
		screen,
		fmt.Sprintf(r.tr("Соло или кооператив • Порт :%d", "Solo or Co-op • Port :%d"), port),
		core.ArenaWidth/2, float64(hostBtnY)+37, r.faceSmall, color.RGBA{175, 205, 235, 255},
	)

	// Поле ввода адреса хоста
	r.drawTextCentered(
		screen,
		r.tr("Адрес хоста (IP:PORT) для подключения:", "Host address (IP:PORT) to join:"),
		core.ArenaWidth/2, float64(addrBoxY)-22, r.faceSmall, color.RGBA{185, 195, 220, 255},
	)
	vector.DrawFilledRect(screen, menuBtnX, addrBoxY, menuBtnW, addrBoxH, color.RGBA{22, 24, 36, 255}, false)
	vector.StrokeRect(screen, menuBtnX, addrBoxY, menuBtnW, addrBoxH, 1.5, color.RGBA{105, 120, 160, 255}, false)
	r.drawTextCentered(
		screen,
		addrInput+"_",
		core.ArenaWidth/2, float64(addrBoxY)+12, r.faceNormal, color.RGBA{255, 240, 160, 255},
	)

	// Кнопка ПОДКЛЮЧИТЬСЯ
	joinFill := color.RGBA{26, 44, 36, 255}
	joinBorder := color.RGBA{85, 235, 145, 255}
	if pointInRect(float64(cx), float64(cy), menuBtnX, joinBtnY, menuBtnW, joinBtnH) {
		joinFill = color.RGBA{38, 66, 52, 255}
		joinBorder = color.RGBA{145, 255, 190, 255}
	}
	vector.DrawFilledRect(screen, menuBtnX, joinBtnY, menuBtnW, joinBtnH, joinFill, false)
	vector.StrokeRect(screen, menuBtnX, joinBtnY, menuBtnW, joinBtnH, 2.5, joinBorder, false)
	r.drawTextCentered(
		screen,
		r.tr("ПОДКЛЮЧИТЬСЯ К ИГРЕ (КЛИЕНТ)", "JOIN GAME (CLIENT)"),
		core.ArenaWidth/2, float64(joinBtnY)+12, r.faceTitle, color.RGBA{240, 255, 245, 255},
	)
	r.drawTextCentered(
		screen,
		r.tr("Введите IP друга выше и нажмите сюда", "Enter friend's IP above and click here"),
		core.ArenaWidth/2, float64(joinBtnY)+37, r.faceSmall, color.RGBA{175, 225, 195, 255},
	)

	if errMsg != "" {
		r.drawTextCentered(screen, errMsg, core.ArenaWidth/2, 520, r.faceNormal, color.RGBA{255, 95, 110, 255})
	}

	r.drawLangButton(screen)
}

func (r *Renderer) DrawConnecting(screen *ebiten.Image, netStatus string) {
	screen.Fill(color.RGBA{16, 16, 24, 255})
	r.drawTextCentered(
		screen,
		r.tr("САУНДЧЕК В 19:59 — ПОДКЛЮЧЕНИЕ К ХОСТУ...", "SOUNDCHECK AT 19:59 — CONNECTING TO HOST..."),
		core.ArenaWidth/2, 330, r.faceHeading, color.RGBA{90, 210, 255, 255},
	)
	r.drawTextCentered(screen, netStatus, core.ArenaWidth/2, 365, r.faceNormal, color.RGBA{210, 215, 235, 255})
	r.drawLangButton(screen)
}

func (r *Renderer) Draw(screen *ebiten.Image, snap core.WorldSnapshot, localPlayerID int, netStatus string) {
	// Смещение тряски камеры
	var camX, camY float32
	if snap.ScreenShake > 0.2 {
		s := float32(snap.ScreenShake)
		camX = (r.rng.Float32()*2 - 1) * s
		camY = (r.rng.Float32()*2 - 1) * s
	}

	// Фон и пульсация в такт в РЕЖИМЕ ДЖЕМА
	bgCol := color.RGBA{16, 16, 24, 255}
	if snap.JamActive {
		beat := float64(snap.Tick) * 0.28
		pulse := uint8((math.Sin(beat)*0.5 + 0.5) * 28.0)
		bgCol = color.RGBA{18 + pulse, 14, 32 + pulse, 255}
	}
	screen.Fill(bgCol)

	// Сетка арены и границы сцены
	r.drawArena(screen, camX, camY, snap)

	// Зоны замедления (от Звукача)
	for _, z := range snap.Zones {
		zx := float32(z.X) + camX
		zy := float32(z.Y) + camY
		rad := float32(z.Radius)
		vector.DrawFilledCircle(screen, zx, zy, rad, color.RGBA{155, 45, 220, 65}, false)
		vector.StrokeCircle(screen, zx, zy, rad, 2.0, color.RGBA{195, 95, 255, 150}, false)
		r.drawTextCentered(screen, r.tr("ЗОНА ЗАМЕДЛЕНИЯ", "SLOW ZONE"), float64(zx), float64(zy)-7, r.faceSmall, color.RGBA{225, 175, 255, 220})
	}

	// Препятствия (Стеки усилителей, колонны, пульт)
	lang := r.Lang()
	for _, obs := range snap.Obstacles {
		ox := float32(obs.X) + camX
		oy := float32(obs.Y) + camY
		ow := float32(obs.W)
		oh := float32(obs.H)
		vector.DrawFilledRect(screen, ox, oy, ow, oh, color.RGBA{36, 40, 54, 255}, false)
		borderCol := color.RGBA{95, 105, 135, 255}
		if snap.JamActive {
			borderCol = color.RGBA{220, 120, 255, 255}
		}
		vector.StrokeRect(screen, ox, oy, ow, oh, 2.5, borderCol, false)
		vector.StrokeCircle(screen, ox+ow*0.3, oy+oh*0.35, 12, 1.5, color.RGBA{65, 72, 95, 255}, false)
		vector.StrokeCircle(screen, ox+ow*0.7, oy+oh*0.35, 12, 1.5, color.RGBA{65, 72, 95, 255}, false)
		r.drawTextCentered(screen, obs.GetLabel(lang), float64(ox+ow*0.5), float64(oy+oh*0.58), r.faceSmall, color.RGBA{185, 195, 220, 230})
	}

	// Визуальные эффекты (кольца слэпа, дуги ударов, цепные молнии)
	for _, ef := range snap.Effects {
		ratio := ef.Life / math.Max(0.01, ef.MaxLife)
		alpha := uint8(core.Clamp(ratio*220.0, 0, 255))
		col := effectColor(ef.ColorID, alpha)
		switch ef.Type {
		case core.EffectRing:
			curRad := float32(ef.Radius * (1.15 - ratio*0.35))
			vector.StrokeCircle(screen, float32(ef.X)+camX, float32(ef.Y)+camY, curRad, 4.0, col, false)
		case core.EffectLine:
			x1, y1 := float32(ef.X)+camX, float32(ef.Y)+camY
			x2, y2 := float32(ef.X2)+camX, float32(ef.Y2)+camY
			mx := (x1+x2)*0.5 + (r.rng.Float32()*2-1)*10
			my := (y1+y2)*0.5 + (r.rng.Float32()*2-1)*10
			vector.StrokeLine(screen, x1, y1, mx, my, 2.5, col, false)
			vector.StrokeLine(screen, mx, my, x2, y2, 2.5, col, false)
		case core.EffectArc:
			r.drawArcEffect(screen, float32(ef.X)+camX, float32(ef.Y)+camY, float32(ef.Radius), ef.Angle, ef.Spread, col)
		}
	}

	// Снаряды
	for _, pr := range snap.Projectiles {
		r.drawProjectile(screen, pr, camX, camY, snap.Tick)
	}

	// Враги
	for _, e := range snap.Enemies {
		r.drawEnemy(screen, e, camX, camY, snap.Tick)
	}

	// Игроки
	for _, p := range snap.Players {
		if !p.Connected {
			continue
		}
		r.drawPlayer(screen, p, p.ID == localPlayerID, camX, camY, snap.Tick)
	}

	// Частицы
	for _, pt := range snap.Particles {
		alpha := uint8(core.Clamp((pt.Life/math.Max(0.01, pt.MaxLife))*255.0, 0, 255))
		col := effectColor(pt.ColorID, alpha)
		sz := float32(pt.Size)
		vector.DrawFilledRect(screen, float32(pt.X)+camX-sz*0.5, float32(pt.Y)+camY-sz*0.5, sz, sz, col, false)
	}

	// Вылетающие цифры урона и статусы
	for _, ft := range snap.FloatingTexts {
		alpha := uint8(core.Clamp((ft.Life/math.Max(0.01, ft.MaxLife))*255.0, 0, 255))
		col := effectColor(ft.ColorID, alpha)
		r.drawTextCentered(screen, ft.GetText(lang), ft.X+float64(camX), ft.Y+float64(camY), r.faceNormal, col)
	}

	// Верхний HUD
	r.drawHUD(screen, snap, localPlayerID, netStatus)

	// Оверлеи стадий игры
	switch snap.Stage {
	case core.StageLobby:
		r.drawLobbyOverlay(screen, snap, localPlayerID)
	case core.StageUpgrade:
		r.drawUpgradeOverlay(screen, snap, localPlayerID)
	case core.StageGameOver:
		r.drawGameOverOverlay(screen, snap)
	}

	// Кнопка переключения языка (поверх HUD и фона, над оверлеями)
	r.drawLangButton(screen)
}

func (r *Renderer) drawLangButton(screen *ebiten.Image) {
	halfW := langBtnW * 0.5
	vector.DrawFilledRect(screen, langBtnX, langBtnY, langBtnW, langBtnH, color.RGBA{18, 22, 34, 232}, false)

	activeCol := color.RGBA{55, 145, 215, 235}
	if r.Lang() == core.LangRU {
		vector.DrawFilledRect(screen, langBtnX, langBtnY, halfW, langBtnH, activeCol, false)
	} else {
		vector.DrawFilledRect(screen, langBtnX+halfW, langBtnY, halfW, langBtnH, activeCol, false)
	}

	vector.StrokeLine(screen, langBtnX+halfW, langBtnY, langBtnX+halfW, langBtnY+langBtnH, 1.0, color.RGBA{95, 115, 155, 220}, false)

	cx, cy := ebiten.CursorPosition()
	borderCol := color.RGBA{80, 95, 130, 235}
	if r.IsLangButtonHovered(float64(cx), float64(cy)) {
		borderCol = color.RGBA{130, 210, 255, 255}
	}
	vector.StrokeRect(screen, langBtnX, langBtnY, langBtnW, langBtnH, 1.5, borderCol, false)

	ruCol := color.RGBA{170, 180, 205, 230}
	enCol := color.RGBA{170, 180, 205, 230}
	if r.Lang() == core.LangRU {
		ruCol = color.RGBA{255, 255, 255, 255}
	} else {
		enCol = color.RGBA{255, 255, 255, 255}
	}
	textY := float64(langBtnY) + 5
	r.drawTextCentered(screen, "RU", float64(langBtnX+halfW*0.5), textY, r.faceSmall, ruCol)
	r.drawTextCentered(screen, "EN", float64(langBtnX+halfW*1.5), textY, r.faceSmall, enCol)
}

func (r *Renderer) drawArena(screen *ebiten.Image, camX, camY float32, snap core.WorldSnapshot) {
	gridCol := color.RGBA{26, 28, 40, 255}
	for x := float32(64); x < core.ArenaWidth; x += 64 {
		vector.StrokeLine(screen, x+camX, 16+camY, x+camX, core.ArenaHeight-16+camY, 1.0, gridCol, false)
	}
	for y := float32(64); y < core.ArenaHeight; y += 64 {
		vector.StrokeLine(screen, 16+camX, y+camY, core.ArenaWidth-16+camX, y+camY, 1.0, gridCol, false)
	}

	borderCol := color.RGBA{70, 80, 115, 255}
	if snap.JamActive {
		borderCol = color.RGBA{255, 195, 50, 255}
	}
	vector.StrokeRect(screen, 16+camX, 16+camY, core.ArenaWidth-32, core.ArenaHeight-32, 3.0, borderCol, false)
}

func (r *Renderer) drawArcEffect(screen *ebiten.Image, cx, cy, radius float32, angle, spread float64, col color.RGBA) {
	steps := 10
	startAng := angle - spread
	stepAng := (spread * 2.0) / float64(steps)
	prevX := cx + float32(math.Cos(startAng))*radius
	prevY := cy + float32(math.Sin(startAng))*radius

	vector.StrokeLine(screen, cx, cy, prevX, prevY, 2.0, col, false)
	for i := 1; i <= steps; i++ {
		a := startAng + float64(i)*stepAng
		nx := cx + float32(math.Cos(a))*radius
		ny := cy + float32(math.Sin(a))*radius
		vector.StrokeLine(screen, prevX, prevY, nx, ny, 3.5, col, false)
		prevX, prevY = nx, ny
	}
	vector.StrokeLine(screen, cx, cy, prevX, prevY, 2.0, col, false)
}

func (r *Renderer) drawProjectile(screen *ebiten.Image, pr core.Projectile, camX, camY float32, tick uint64) {
	px := float32(pr.X) + camX
	py := float32(pr.Y) + camY
	rad := float32(pr.Radius)

	switch pr.Type {
	case core.ProjBassWave:
		vector.StrokeCircle(screen, px, py, rad, 3.5, color.RGBA{60, 220, 255, 230}, false)
		vector.StrokeCircle(screen, px, py, rad*0.6, 2.0, color.RGBA{150, 245, 255, 190}, false)
	case core.ProjVocalScream:
		vector.StrokeCircle(screen, px, py, rad, 3.0, color.RGBA{255, 80, 180, 220}, false)
		vector.DrawFilledCircle(screen, px, py, rad*0.35, color.RGBA{255, 180, 225, 180}, false)
	case core.ProjGuitarRiff:
		nx, ny := core.Normalize(pr.VX, pr.VY)
		tailX := px - float32(nx)*rad*2.2
		tailY := py - float32(ny)*rad*2.2
		col := color.RGBA{255, 225, 65, 255}
		if pr.IsCrit {
			col = color.RGBA{255, 100, 255, 255}
		}
		vector.StrokeLine(screen, tailX, tailY, px, py, 3.5, col, false)
		vector.DrawFilledCircle(screen, px, py, rad*0.55, color.RGBA{255, 255, 255, 255}, false)
	case core.ProjCymbalBoomerang:
		vector.DrawFilledCircle(screen, px, py, rad, color.RGBA{235, 195, 55, 180}, false)
		vector.StrokeCircle(screen, px, py, rad, 2.5, color.RGBA{255, 245, 130, 255}, false)
		vector.DrawFilledCircle(screen, px, py, 5, color.RGBA{40, 40, 40, 255}, false)
		spin := float64(tick) * 0.45
		for k := 0; k < 4; k++ {
			a := spin + float64(k)*math.Pi*0.5
			sx := px + float32(math.Cos(a))*rad
			sy := py + float32(math.Sin(a))*rad
			vector.StrokeLine(screen, px, py, sx, sy, 2.0, color.RGBA{255, 255, 200, 240}, false)
		}
	case core.ProjEnemyOrb:
		vector.DrawFilledCircle(screen, px, py, rad, color.RGBA{190, 60, 255, 240}, false)
		vector.StrokeCircle(screen, px, py, rad+2, 1.5, color.RGBA{255, 150, 255, 200}, false)
	case core.ProjEnemyWave:
		vector.DrawFilledCircle(screen, px, py, rad, color.RGBA{255, 75, 65, 235}, false)
		vector.StrokeCircle(screen, px, py, rad+2, 1.5, color.RGBA{255, 200, 80, 200}, false)
	}
}

func (r *Renderer) drawEnemy(screen *ebiten.Image, e core.Enemy, camX, camY float32, tick uint64) {
	ex := float32(e.X) + camX
	ey := float32(e.Y) + camY
	rad := float32(e.Radius)

	if e.Type == core.EnemyDrunkFan && e.Telegraphing {
		tx := ex + float32(e.ChargeDirX)*280.0
		ty := ey + float32(e.ChargeDirY)*280.0
		vector.StrokeLine(screen, ex, ey, tx, ty, 3.0, color.RGBA{255, 45, 45, 200}, false)
		r.drawTextCentered(screen, "!", float64(ex), float64(ey-rad-22), r.faceTitle, color.RGBA{255, 80, 80, 255})
	}

	var col color.RGBA
	switch e.Type {
	case core.EnemyHum:
		col = color.RGBA{255, 95, 65, 255}
	case core.EnemyDrunkFan:
		col = color.RGBA{230, 135, 45, 255}
	case core.EnemySoundGuy:
		col = color.RGBA{175, 75, 245, 255}
	case core.EnemyFeedbackAmp:
		col = color.RGBA{235, 45, 90, 255}
	}

	if e.HitFlash > 0 {
		col = color.RGBA{255, 255, 255, 255}
	}

	switch e.Type {
	case core.EnemyHum:
		a1 := float64(tick)*0.1 + float64(e.ID)
		p1x, p1y := ex+float32(math.Cos(a1))*rad, ey+float32(math.Sin(a1))*rad
		p2x, p2y := ex+float32(math.Cos(a1+2.09))*rad, ey+float32(math.Sin(a1+2.09))*rad
		p3x, p3y := ex+float32(math.Cos(a1+4.18))*rad, ey+float32(math.Sin(a1+4.18))*rad
		vector.StrokeLine(screen, p1x, p1y, p2x, p2y, 3.0, col, false)
		vector.StrokeLine(screen, p2x, p2y, p3x, p3y, 3.0, col, false)
		vector.StrokeLine(screen, p3x, p3y, p1x, p1y, 3.0, col, false)
	case core.EnemyDrunkFan:
		vector.DrawFilledRect(screen, ex-rad, ey-rad, rad*2, rad*2, col, false)
		vector.StrokeRect(screen, ex-rad, ey-rad, rad*2, rad*2, 2.0, color.RGBA{255, 220, 160, 255}, false)
	case core.EnemySoundGuy:
		vector.DrawFilledCircle(screen, ex, ey, rad*0.7, col, false)
		vector.StrokeCircle(screen, ex, ey, rad, 2.5, color.RGBA{230, 180, 255, 255}, false)
	case core.EnemyFeedbackAmp:
		vector.DrawFilledRect(screen, ex-rad, ey-rad, rad*2, rad*2, color.RGBA{55, 20, 35, 255}, false)
		vector.StrokeRect(screen, ex-rad, ey-rad, rad*2, rad*2, 3.0, col, false)
		vector.StrokeCircle(screen, ex, ey, rad*0.65, 2.5, col, false)
	}

	if e.BurnTimer > 0 {
		vector.StrokeCircle(screen, ex, ey, rad+4, 1.5, color.RGBA{255, 140, 20, 220}, false)
	}
	if e.StunTimer > 0 {
		r.drawTextCentered(screen, r.tr("ОГЛУШЁН", "STUNNED"), float64(ex), float64(ey-rad-20), r.faceSmall, color.RGBA{255, 230, 90, 255})
	}

	if e.HP < e.MaxHP {
		barW := rad * 2.0
		ratio := float32(core.Clamp(e.HP/e.MaxHP, 0, 1))
		vector.DrawFilledRect(screen, ex-rad, ey-rad-7, barW, 4, color.RGBA{40, 40, 40, 220}, false)
		vector.DrawFilledRect(screen, ex-rad, ey-rad-7, barW*ratio, 4, color.RGBA{255, 75, 75, 255}, false)
	}
}

func (r *Renderer) drawPlayer(screen *ebiten.Image, p core.Player, isLocal bool, camX, camY float32, tick uint64) {
	px := float32(p.X) + camX
	py := float32(p.Y) + camY
	rad := float32(p.Radius)

	if p.Class == core.ClassVocalist && !p.Downed {
		alpha := uint8(25 + math.Min(float64(p.SpotlightCount)*18, 90))
		vector.StrokeCircle(screen, px, py, 195.0, 1.5, color.RGBA{255, 65, 155, alpha}, false)
	}

	col := classColor(p.Class)
	if p.HitFlash > 0 {
		col = color.RGBA{255, 255, 255, 255}
	}
	if p.Downed {
		col = color.RGBA{180, 40, 40, 200}
	}

	if isLocal {
		vector.StrokeCircle(screen, px, py, rad+5, 1.5, color.RGBA{255, 255, 255, 140}, false)
	}

	if p.InvulnTimer > 0 {
		vector.StrokeCircle(screen, px, py, rad+8, 2.5, color.RGBA{255, 245, 120, 220}, false)
	}

	vector.DrawFilledCircle(screen, px, py, rad, col, false)
	vector.StrokeCircle(screen, px, py, rad, 2.5, color.RGBA{255, 255, 255, 230}, false)

	if !p.Downed {
		dx, dy := core.Normalize(p.AimX-p.X, p.AimY-p.Y)
		ax := px + float32(dx)*(rad+10)
		ay := py + float32(dy)*(rad+10)
		vector.StrokeLine(screen, px, py, ax, ay, 3.0, color.RGBA{255, 255, 255, 230}, false)
	}

	if p.Downed {
		r.drawTextCentered(screen, r.tr("СБИТ! [ЗАЖМИ E]", "DOWNED! [HOLD E]"), float64(px), float64(py-rad-28), r.faceSmall, color.RGBA{255, 110, 110, 255})
		if p.ReviveProgress > 0 {
			r.drawCircularProgress(screen, px, py, rad+12, p.ReviveProgress, color.RGBA{80, 255, 120, 255})
		}
	} else {
		pPrefix := r.tr("И", "P")
		tag := fmt.Sprintf("%s%d %s", pPrefix, p.ID+1, p.Class.NameLang(r.Lang()))
		switch p.Class {
		case core.ClassVocalist:
			if p.SpotlightCount > 0 {
				tag = fmt.Sprintf(r.tr("%s%d СОФИТЫ x%d", "%s%d SPOTLIGHT x%d"), pPrefix, p.ID+1, p.SpotlightCount)
			}
		case core.ClassGuitarist:
			if p.GainStacks > 0 {
				tag = fmt.Sprintf(r.tr("%s%d ГЕЙН +%d%%", "%s%d GAIN +%d%%"), pPrefix, p.ID+1, p.GainStacks*8)
			}
		case core.ClassDrummer:
			if p.ComboCount > 0 {
				tag = fmt.Sprintf(r.tr("%s%d КОМБО x%d!", "%s%d COMBO x%d!"), pPrefix, p.ID+1, p.ComboCount)
			}
		}
		r.drawTextCentered(screen, tag, float64(px), float64(py-rad-22), r.faceSmall, color.RGBA{235, 240, 255, 240})
	}

	if isLocal {
		cx := float32(p.AimX) + camX
		cy := float32(p.AimY) + camY
		vector.StrokeCircle(screen, cx, cy, 8, 1.5, col, false)
		vector.StrokeLine(screen, cx-12, cy, cx+12, cy, 1.5, col, false)
		vector.StrokeLine(screen, cx, cy-12, cx, cy+12, 1.5, col, false)
	}
}

func (r *Renderer) drawCircularProgress(screen *ebiten.Image, cx, cy, radius float32, progress float64, col color.RGBA) {
	steps := int(progress * 24.0)
	start := -math.Pi / 2
	for i := 0; i < steps; i++ {
		a1 := start + (float64(i)/24.0)*math.Pi*2
		a2 := start + (float64(i+1)/24.0)*math.Pi*2
		x1, y1 := cx+float32(math.Cos(a1))*radius, cy+float32(math.Sin(a1))*radius
		x2, y2 := cx+float32(math.Cos(a2))*radius, cy+float32(math.Sin(a2))*radius
		vector.StrokeLine(screen, x1, y1, x2, y2, 4.0, col, false)
	}
}

func (r *Renderer) drawHUD(screen *ebiten.Image, snap core.WorldSnapshot, localID int, netStatus string) {
	vector.DrawFilledRect(screen, 0, 0, core.ArenaWidth, 68, color.RGBA{10, 12, 18, 225}, false)
	vector.StrokeLine(screen, 0, 68, core.ArenaWidth, 68, 1.5, color.RGBA{60, 70, 100, 255}, false)

	// Центр: Волна и общая шкала SYNERGY / JAM MODE
	waveTitle := fmt.Sprintf(
		r.tr("САУНДЧЕК В 19:59  |  ВОЛНА %d  (ВРАГИ: %d)", "SOUNDCHECK AT 19:59  |  WAVE %d  (ENEMIES: %d)"),
		snap.Wave, snap.EnemiesLeft,
	)
	r.drawTextCentered(screen, waveTitle, core.ArenaWidth/2, 5, r.faceTitle, color.RGBA{235, 240, 255, 255})

	barX := float32(core.ArenaWidth/2 - 175)
	barY := float32(26)
	barW := float32(350)
	barH := float32(19)
	vector.DrawFilledRect(screen, barX, barY, barW, barH, color.RGBA{30, 32, 45, 255}, false)

	synRatio := float32(core.Clamp(snap.Synergy/100.0, 0, 1))
	synCol := color.RGBA{185, 85, 255, 255}
	synLabel := fmt.Sprintf(r.tr("СЫГРАННОСТЬ: %.0f%%", "SYNERGY: %.0f%%"), snap.Synergy)
	if snap.JamActive {
		synCol = color.RGBA{255, 210, 45, 255}
		synLabel = fmt.Sprintf(r.tr("РЕЖИМ ДЖЕМА (%.1fс) — 2x СКОРОСТЬ И +УРОН!", "JAM MODE (%.1fs) — 2x SPEED & +DMG!"), snap.JamTimer)
	}
	vector.DrawFilledRect(screen, barX, barY, barW*synRatio, barH, synCol, false)
	vector.StrokeRect(screen, barX, barY, barW, barH, 1.5, color.RGBA{200, 200, 220, 255}, false)
	r.drawTextCentered(screen, synLabel, core.ArenaWidth/2, float64(barY)+2, r.faceSmall, color.RGBA{255, 255, 255, 255})

	r.drawTextCentered(screen, netStatus, core.ArenaWidth/2, 49, r.faceSmall, color.RGBA{165, 180, 210, 255})

	// Слева: Игрок 1 | Справа: Игрок 2
	r.drawPlayerHUDPanel(screen, snap.Players[0], 16, 6)
	if snap.Players[1].Connected {
		r.drawPlayerHUDPanel(screen, snap.Players[1], core.ArenaWidth-430, 6)
	} else {
		r.drawText(screen, r.tr("И2: ОЖИДАНИЕ НАПАРНИКА...", "P2: WAITING FOR PARTNER..."), core.ArenaWidth-260, 22, r.faceNormal, color.RGBA{150, 160, 190, 230})
	}
}

func (r *Renderer) drawPlayerHUDPanel(screen *ebiten.Image, p core.Player, x, y float32) {
	lang := r.Lang()
	col := classColor(p.Class)
	hpRatio := float32(core.Clamp(p.HP/math.Max(1, p.MaxHP), 0, 1))
	vector.DrawFilledRect(screen, x, y, 220, 18, color.RGBA{40, 25, 30, 255}, false)
	vector.DrawFilledRect(screen, x, y, 220*hpRatio, 18, col, false)
	vector.StrokeRect(screen, x, y, 220, 18, 1.5, color.RGBA{220, 220, 230, 255}, false)

	pPrefix := r.tr("И", "P")
	hpText := fmt.Sprintf("%s%d [%s] HP: %.0f/%.0f", pPrefix, p.ID+1, p.Class.NameLang(lang), p.HP, p.MaxHP)
	r.drawText(screen, hpText, float64(x)+6, float64(y)+1, r.faceSmall, color.RGBA{255, 255, 255, 255})

	cdText := r.tr("НАВЫК [ПКМ/ПРОБЕЛ]: ГОТОВ", "SKILL [RMB/SPACE]: READY")
	cdCol := color.RGBA{120, 255, 160, 255}
	if p.SecondaryCD > 0 {
		cdText = fmt.Sprintf(r.tr("НАВЫК КД: %.1fс", "SKILL CD: %.1fs"), p.SecondaryCD)
		cdCol = color.RGBA{255, 195, 110, 255}
	}
	r.drawText(screen, cdText, float64(x)+228, float64(y)+1, r.faceSmall, cdCol)

	var gearList []string
	for idx := 0; idx < int(core.TotalItems); idx++ {
		cnt := p.Items[core.ItemType(idx)]
		if cnt > 0 {
			gearList = append(gearList, fmt.Sprintf("%s x%d", core.AllItems[idx].GetShort(lang), cnt))
		}
	}
	gearStr := r.tr("ПЕДАЛБОРД: пусто", "PEDALBOARD: empty")
	if len(gearList) > 0 {
		gearStr = r.tr("ПЕДАЛБОРД: ", "PEDALBOARD: ") + strings.Join(gearList, ", ")
	}
	r.drawText(screen, gearStr, float64(x), float64(y)+26, r.faceSmall, color.RGBA{205, 215, 235, 255})
}

func (r *Renderer) drawLobbyOverlay(screen *ebiten.Image, snap core.WorldSnapshot, localID int) {
	lang := r.Lang()
	px, py := float32(150), float32(105)
	pw, ph := float32(980), float32(525)
	vector.DrawFilledRect(screen, px, py, pw, ph, color.RGBA{12, 14, 24, 244}, false)
	vector.StrokeRect(screen, px, py, pw, ph, 3.0, color.RGBA{90, 200, 255, 255}, false)

	r.drawTextCentered(
		screen,
		r.tr("=== САУНДЧЕК В 19:59 : ЛОББИ ГРУППЫ ===", "=== SOUNDCHECK AT 19:59 : BAND LOBBY ==="),
		float64(px+pw/2), float64(py+14), r.faceHeading, color.RGBA{90, 215, 255, 255},
	)
	r.drawTextCentered(
		screen,
		r.tr(
			"У всех 4 музыкантов на старте 100 HP и равная скорость. Свой билд вы собираете педалями между волнами!",
			"All 4 musicians start with 100 HP and equal speed. Build your loadout with pedals between waves!",
		),
		float64(px+pw/2), float64(py+42), r.faceNormal, color.RGBA{210, 215, 235, 255},
	)

	classes := []struct {
		key    string
		c      core.ClassType
		descRU string
		descEN string
	}{
		{
			"[1]", core.ClassBassist,
			"Пробивающие басовые волны с мощным отбрасыванием + урон при ударе врага о стену/колонну. ПКМ: Слэп (сносит снаряды и толпу).",
			"Piercing bass waves with heavy knockback + wall/amp slam damage. RMB: Slap (clears projectiles & crowd).",
		},
		{
			"[2]", core.ClassVocalist,
			"Удар стойкой вблизи + пробивающий Крик. Пассивка «Софиты»: +урон и +скорость от врагов рядом. ПКМ: Прыжок со сцены.",
			"Mic-stand melee + piercing Scream. Passive 'Spotlight': +damage & +speed per nearby enemy. RMB: Stage Dive.",
		},
		{
			"[3]", core.ClassGuitarist,
			"Быстрые риффы с 2 рикошетами от стен и цепной молнией фидбэка. Пассивка «Гейн»: серия попаданий растит Крит%. ПКМ: Залп с отскоком.",
			"Fast riffs with 2 wall bounces & chain feedback lightning. Passive 'Gain': hit streak builds Crit%. RMB: Ricochet Volley.",
		},
		{
			"[4]", core.ClassDrummer,
			"Широкие быстрые удары палочками. Пассивка «Комбо-темп»: попадания разгоняют скорость атаки. ПКМ: Тарелка-бумеранг.",
			"Wide rapid drumstick strikes. Passive 'Combo Tempo': hits ramp attack speed. RMB: Cymbal Boomerang.",
		},
	}

	localClass := snap.Players[localID].Class
	for idx, info := range classes {
		cy := py + 76 + float32(idx)*68
		boxCol := color.RGBA{24, 28, 42, 255}
		borderCol := classColor(info.c)
		if localClass == info.c {
			boxCol = color.RGBA{42, 52, 78, 255}
		}
		vector.DrawFilledRect(screen, px+26, cy, pw-52, 58, boxCol, false)
		vector.StrokeRect(screen, px+26, cy, pw-52, 58, 2.0, borderCol, false)
		r.drawText(screen, fmt.Sprintf("%s %s — %s", info.key, info.c.NameLang(lang), info.c.SubtitleLang(lang)), float64(px+40), float64(cy+8), r.faceTitle, borderCol)
		r.drawText(screen, r.tr(info.descRU, info.descEN), float64(px+40), float64(cy+31), r.faceSmall, color.RGBA{225, 230, 245, 255})
	}

	sy := float64(py + 362)
	for i := 0; i < 2; i++ {
		p := snap.Players[i]
		status := r.tr(
			"НЕ ПОДКЛЮЧЁН (Хост может начать игру в одиночку или ждать Игрока 2)",
			"NOT CONNECTED (Host can start solo or wait for Player 2)",
		)
		statusCol := color.RGBA{165, 170, 190, 255}
		if p.Connected {
			rStr := r.tr("НЕ ГОТОВ", "NOT READY")
			statusCol = color.RGBA{255, 195, 95, 255}
			if p.Ready {
				rStr = r.tr("ГОТОВ К КОНЦЕРТУ!", "READY TO ROCK!")
				statusCol = color.RGBA{100, 255, 145, 255}
			}
			status = fmt.Sprintf(r.tr("Класс: %s  |  Статус: [%s]", "Class: %s  |  Status: [%s]"), p.Class.NameLang(lang), rStr)
		}
		r.drawText(screen, fmt.Sprintf(r.tr("ИГРОК %d: %s", "PLAYER %d: %s"), i+1, status), float64(px+45), sy+float64(i*28), r.faceNormal, statusCol)
	}

	r.drawTextCentered(
		screen,
		r.tr(
			"ЛОББИ: [1 / 2 / 3 / 4] Выбрать музыканта   |   [ENTER] или [ПРОБЕЛ] — ГОТОВ (READY)",
			"LOBBY: [1 / 2 / 3 / 4] Choose Musician   |   [ENTER] or [SPACE] — READY",
		),
		float64(px+pw/2), float64(py+438), r.faceTitle, color.RGBA{255, 235, 120, 255},
	)
	r.drawTextCentered(
		screen,
		r.tr(
			"В БОЮ: [WASD] Движение  |  [ЛКМ] Атака  |  [ПКМ / ПРОБЕЛ] Способность  |  [УДЕРЖИВАТЬ E] Поднять напарника",
			"COMBAT: [WASD] Move  |  [LMB] Attack  |  [RMB / SPACE] Ability  |  [HOLD E] Revive Partner",
		),
		float64(px+pw/2), float64(py+472), r.faceNormal, color.RGBA{200, 210, 235, 255},
	)
}

func (r *Renderer) drawUpgradeOverlay(screen *ebiten.Image, snap core.WorldSnapshot, localID int) {
	lang := r.Lang()
	px, py := float32(190), float32(135)
	pw, ph := float32(900), float32(440)
	vector.DrawFilledRect(screen, px, py, pw, ph, color.RGBA{14, 16, 28, 244}, false)
	vector.StrokeRect(screen, px, py, pw, ph, 3.0, color.RGBA{85, 255, 130, 255}, false)

	r.drawTextCentered(
		screen,
		fmt.Sprintf(r.tr("=== ВОЛНА %d ЗАЧИЩЕНА! ПЕДАЛБОРД И ОБОРУДОВАНИЕ ===", "=== WAVE %d CLEARED! PEDALBOARD & GEAR ==="), snap.Wave),
		float64(px+pw/2), float64(py+20), r.faceHeading, color.RGBA{95, 255, 145, 255},
	)
	r.drawTextCentered(
		screen,
		r.tr("Нажмите [1], [2] или [3], чтобы выбрать 1 из 3 случайных примочек:", "Press [1], [2], or [3] to pick 1 of 3 random pedals:"),
		float64(px+pw/2), float64(py+50), r.faceNormal, color.RGBA{215, 225, 240, 255},
	)

	p := snap.Players[localID]
	if p.UpgradePicked {
		r.drawTextCentered(
			screen,
			r.tr("ОБОРУДОВАНИЕ УСТАНОВЛЕНО! Ожидание выбора напарника...", "GEAR INSTALLED! Waiting for partner's pick..."),
			float64(px+pw/2), float64(py+210), r.faceHeading, color.RGBA{120, 240, 255, 255},
		)
	} else {
		for idx, itemType := range p.UpgradeChoices {
			def := core.AllItems[itemType]
			cy := py + 92 + float32(idx)*96
			vector.DrawFilledRect(screen, px+36, cy, pw-72, 80, color.RGBA{28, 34, 52, 255}, false)
			vector.StrokeRect(screen, px+36, cy, pw-72, 80, 2.0, color.RGBA{100, 240, 150, 255}, false)
			r.drawText(
				screen,
				fmt.Sprintf(r.tr("[%d] %s  (Собрано: %d)", "[%d] %s  (Owned: %d)"), idx+1, def.GetName(lang), p.Items[itemType]),
				float64(px+54), float64(cy+14), r.faceTitle, color.RGBA{255, 235, 120, 255},
			)
			r.drawText(screen, r.tr("Эффект: ", "Effect: ")+def.GetDesc(lang), float64(px+54), float64(cy+42), r.faceNormal, color.RGBA{225, 235, 250, 255})
		}
	}
}

func (r *Renderer) drawGameOverOverlay(screen *ebiten.Image, snap core.WorldSnapshot) {
	px, py := float32(310), float32(215)
	pw, ph := float32(660), float32(265)
	vector.DrawFilledRect(screen, px, py, pw, ph, color.RGBA{25, 10, 16, 245}, false)
	vector.StrokeRect(screen, px, py, pw, ph, 3.0, color.RGBA{255, 65, 85, 255}, false)

	r.drawTextCentered(
		screen,
		r.tr("=== КОНЦЕРТ ОТМЕНЁН : ПОРАЖЕНИЕ ===", "=== SHOW CANCELLED : GAME OVER ==="),
		float64(px+pw/2), float64(py+45), r.faceHeading, color.RGBA{255, 95, 110, 255},
	)
	r.drawTextCentered(
		screen,
		fmt.Sprintf(r.tr("Ваша группа продержалась до %d-й ВОЛНЫ", "Your band survived until WAVE %d"), snap.Wave),
		float64(px+pw/2), float64(py+105), r.faceTitle, color.RGBA{235, 235, 245, 255},
	)
	r.drawTextCentered(
		screen,
		r.tr("Нажмите [ENTER] или [ПРОБЕЛ], чтобы вернуться в лобби группы", "Press [ENTER] or [SPACE] to return to the band lobby"),
		float64(px+pw/2), float64(py+175), r.faceNormal, color.RGBA{255, 225, 130, 255},
	)
}
