package core

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
)

type World struct {
	mu sync.Mutex

	Tick        uint64
	Stage       GameStage
	Wave        int
	Synergy     float64
	JamActive   bool
	JamTimer    float64
	ScreenShake float64

	Players       [2]Player
	Inputs        [2]PlayerInput
	Enemies       []Enemy
	Projectiles   []Projectile
	Zones         []SlowZone
	Obstacles     []Obstacle
	FloatingTexts []FloatingText
	Particles     []Particle
	Effects       []VisualEffect
	PendingSounds []SoundEvent

	nextEntityID int
	rng          *rand.Rand

	// Wave spawning state
	enemiesToSpawn int
	spawnTimer     float64

	// Co-op synergy tracking (simultaneous combat damage window)
	lastHitTimer [2]float64
}

func NewWorld() *World {
	w := &World{
		Stage: StageLobby,
		Wave:  1,
		rng:   rand.New(rand.NewSource(1959)),
	}
	w.initObstacles()
	w.initPlayer(0, true, ClassBassist)
	w.initPlayer(1, false, ClassGuitarist)
	return w
}

func (w *World) initObstacles() {
	// Club arena obstacles (Amp stacks & stage pillars) that Bassist can slam enemies into!
	w.Obstacles = []Obstacle{
		{X: 290, Y: 170, W: 95, H: 95, Label: "СТЕК Л", LabelEN: "AMP STACK L"},
		{X: 895, Y: 170, W: 95, H: 95, Label: "СТЕК П", LabelEN: "AMP STACK R"},
		{X: 290, Y: 455, W: 95, H: 95, Label: "КОЛОННА Л", LabelEN: "PA COLUMN L"},
		{X: 895, Y: 455, W: 95, H: 95, Label: "КОЛОННА П", LabelEN: "PA COLUMN R"},
		{X: 590, Y: 315, W: 100, H: 90, Label: "ПУЛЬТ", LabelEN: "MIXER BOOTH"},
	}
}

func (w *World) initPlayer(id int, connected bool, class ClassType) {
	startX := 480.0
	if id == 1 {
		startX = 800.0
	}
	w.Players[id] = Player{
		ID:             id,
		Connected:      connected,
		Ready:          false,
		Class:          class,
		X:              startX,
		Y:              360.0,
		Radius:         18.0,
		HP:             100.0,
		MaxHP:          100.0,
		BaseSpeed:      215.0,
		AimX:           startX,
		AimY:           250.0,
		RevivingTarget: -1,
		SwingDir:       1,
		Items:          make(map[ItemType]int),
	}
}

func (w *World) SetClientConnected(connected bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	wasConnected := w.Players[1].Connected
	w.Players[1].Connected = connected
	if connected && !wasConnected {
		w.Players[1].HP = w.Players[1].MaxHP
		w.Players[1].Downed = false
		w.Players[1].X = 780
		w.Players[1].Y = 360
		if w.Stage == StageUpgrade {
			w.rollUpgradesForPlayer(1)
		}
		w.addFloatingTextBilingual(w.Players[1].X, w.Players[1].Y-30, "ИГРОК 2 ПОДКЛЮЧИЛСЯ!", "PLAYER 2 JOINED!", 4)
	}
}

func (w *World) SetInput(playerID int, input PlayerInput) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if playerID < 0 || playerID > 1 {
		return
	}
	w.Inputs[playerID] = input
}

func (w *World) addSound(t SoundType, x, y float64) {
	w.PendingSounds = append(w.PendingSounds, SoundEvent{Type: t, X: x, Y: y})
}

func (w *World) addShake(amount float64) {
	if amount > w.ScreenShake {
		w.ScreenShake = amount
	} else {
		w.ScreenShake = math.Min(25.0, w.ScreenShake+amount*0.4)
	}
}

func (w *World) addFloatingText(x, y float64, text string, colorID int) {
	w.addFloatingTextBilingual(x, y, text, text, colorID)
}

func (w *World) addFloatingTextBilingual(x, y float64, textRU, textEN string, colorID int) {
	jitterX := (w.rng.Float64() - 0.5) * 16.0
	w.FloatingTexts = append(w.FloatingTexts, FloatingText{
		X:       x + jitterX,
		Y:       y,
		VY:      -48.0,
		Text:    textRU,
		TextEN:  textEN,
		Life:    0.75,
		MaxLife: 0.75,
		ColorID: colorID,
	})
}

func (w *World) spawnParticles(x, y float64, count int, speed float64, colorID int) {
	for i := 0; i < count; i++ {
		ang := w.rng.Float64() * math.Pi * 2
		spd := speed * (0.35 + w.rng.Float64()*0.85)
		life := 0.25 + w.rng.Float64()*0.35
		w.Particles = append(w.Particles, Particle{
			X:       x,
			Y:       y,
			VX:      math.Cos(ang) * spd,
			VY:      math.Sin(ang) * spd,
			Size:    2.5 + w.rng.Float64()*3.5,
			Life:    life,
			MaxLife: life,
			ColorID: colorID,
		})
	}
}

// Player stat helpers with items & passives
func (w *World) playerDamageMult(p *Player) float64 {
	mult := 1.0
	// Overheated Tubes: +45% all damage per stack
	mult += 0.45 * float64(p.Items[ItemOverheatedTubes])
	// Vocalist Spotlight passive: +8% damage per nearby enemy (up to +80%)
	if p.Class == ClassVocalist {
		bonus := math.Min(float64(p.SpotlightCount)*0.08, 0.80)
		mult += bonus
	}
	// JAM MODE: +35% elevated damage
	if w.JamActive {
		mult *= 1.35
	}
	return mult
}

func (w *World) playerSpeed(p *Player) float64 {
	mult := 1.0
	// Double Kick Pedal: +25% move speed
	mult += 0.25 * float64(p.Items[ItemDoubleKickPedal])
	// Vocalist Spotlight passive: +5% move speed per nearby enemy (up to +50%)
	if p.Class == ClassVocalist {
		mult += math.Min(float64(p.SpotlightCount)*0.05, 0.50)
	}
	if w.JamActive {
		mult += 0.15
	}
	// Check if inside an enemy SlowZone
	for _, z := range w.Zones {
		if z.IsEnemy && DistSq(p.X, p.Y, z.X, z.Y) <= (z.Radius+p.Radius)*(z.Radius+p.Radius) {
			mult *= 0.55
			break
		}
	}
	return p.BaseSpeed * mult
}

func (w *World) playerSizeMult(p *Player) float64 {
	return 1.0 + 0.25*float64(p.Items[ItemBigMuffFuzz])
}

func (w *World) playerKnockbackMult(p *Player) float64 {
	return 1.0 + 0.25*float64(p.Items[ItemBassCabinet])
}

func (w *World) playerDamageTakenMult(p *Player) float64 {
	stacks := p.Items[ItemBassCabinet]
	if stacks <= 0 {
		return 1.0
	}
	return math.Pow(0.85, float64(stacks))
}

func (w *World) playerAbilityCDMult(p *Player) float64 {
	stacks := p.Items[ItemDoubleKickPedal]
	mult := math.Pow(0.70, float64(stacks))
	return mult
}

func (w *World) applyItemToPlayer(p *Player, item ItemType) {
	p.Items[item]++
	switch item {
	case ItemBassCabinet:
		p.MaxHP += 50.0
		p.HP += 50.0
	case ItemOverheatedTubes:
		p.MaxHP = math.Max(25.0, p.MaxHP-25.0)
		if p.HP > p.MaxHP {
			p.HP = p.MaxHP
		}
	}
	w.addSound(SoundUpgradePick, p.X, p.Y)
	w.addFloatingTextBilingual(p.X, p.Y-28, "+"+AllItems[item].Name, "+"+AllItems[item].NameEN, 3)
}

func (w *World) Update() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.Tick++
	if w.ScreenShake > 0 {
		w.ScreenShake = math.Max(0, w.ScreenShake-DT*28.0)
	}

	switch w.Stage {
	case StageLobby:
		w.updateLobby()
	case StageCombat:
		w.updateCombat()
	case StageUpgrade:
		w.updateUpgrade()
	case StageGameOver:
		w.updateGameOver()
	}

	w.updateParticlesAndTexts()
}

func (w *World) updateLobby() {
	activeCount := 0
	readyCount := 0
	for i := 0; i < 2; i++ {
		p := &w.Players[i]
		if !p.Connected {
			continue
		}
		activeCount++
		inp := w.Inputs[i]

		if inp.SelectClass >= 1 && inp.SelectClass <= 4 && inp.SelectClass != p.PrevSelectClass {
			p.Class = ClassType(inp.SelectClass - 1)
			w.addSound(SoundUpgradePick, p.X, p.Y)
		}
		p.PrevSelectClass = inp.SelectClass

		if inp.ReadyToggle && !p.PrevReady {
			p.Ready = !p.Ready
			w.addSound(SoundDrumHit, p.X, p.Y)
		}
		p.PrevReady = inp.ReadyToggle

		if p.Ready {
			readyCount++
		}
	}

	if activeCount > 0 && readyCount == activeCount {
		w.startNewRun()
	}
}

func (w *World) startNewRun() {
	w.Stage = StageCombat
	w.Wave = 1
	w.Synergy = 0
	w.JamActive = false
	w.JamTimer = 0
	w.Enemies = nil
	w.Projectiles = nil
	w.Zones = nil

	for i := 0; i < 2; i++ {
		p := &w.Players[i]
		p.Items = make(map[ItemType]int)
		p.MaxHP = 100.0
		p.HP = 100.0
		p.Downed = false
		p.ReviveProgress = 0
		p.RevivingTarget = -1
		p.PrimaryCD = 0
		p.SecondaryCD = 0
		p.GainStacks = 0
		p.ComboCount = 0
		p.ComboTimer = 0
		p.SpotlightCount = 0
		p.DelayedQueue = nil
		if i == 0 {
			p.X, p.Y = 480, 360
		} else {
			p.X, p.Y = 800, 360
		}
	}
	w.startWave(1)
}

func (w *World) startWave(waveNum int) {
	w.Wave = waveNum
	w.Stage = StageCombat
	w.Projectiles = nil
	w.Zones = nil

	// Revive or heal slightly on wave start
	for i := 0; i < 2; i++ {
		p := &w.Players[i]
		if !p.Connected {
			continue
		}
		if p.Downed {
			p.Downed = false
			p.HP = p.MaxHP * 0.5
		} else {
			p.HP = math.Min(p.MaxHP, p.HP+15.0)
		}
	}

	activePlayers := 1
	if w.Players[1].Connected {
		activePlayers = 2
	}
	w.enemiesToSpawn = (6 + waveNum*4) + (activePlayers-1)*(3+waveNum*2)
	w.spawnTimer = 0.5
	w.addFloatingTextBilingual(
		ArenaWidth/2, 140,
		fmt.Sprintf("ВОЛНА %d - САУНДЧЕК!", waveNum),
		fmt.Sprintf("WAVE %d - SOUNDCHECK!", waveNum),
		1,
	)
	w.addSound(SoundJamStart, ArenaWidth/2, ArenaHeight/2)
}

func (w *World) updateUpgrade() {
	allPicked := true
	for i := 0; i < 2; i++ {
		p := &w.Players[i]
		if !p.Connected {
			continue
		}
		inp := w.Inputs[i]
		if !p.UpgradePicked {
			if inp.SelectItem >= 1 && inp.SelectItem <= 3 && inp.SelectItem != p.PrevSelectItem {
				idx := inp.SelectItem - 1
				if idx < len(p.UpgradeChoices) {
					chosen := p.UpgradeChoices[idx]
					w.applyItemToPlayer(p, chosen)
					p.UpgradePicked = true
				}
			}
		}
		p.PrevSelectItem = inp.SelectItem
		if !p.UpgradePicked {
			allPicked = false
		}
	}

	if allPicked {
		w.startWave(w.Wave + 1)
	}
}

func (w *World) updateGameOver() {
	for i := 0; i < 2; i++ {
		p := &w.Players[i]
		if !p.Connected {
			continue
		}
		inp := w.Inputs[i]
		if inp.ReadyToggle && !p.PrevReady {
			// Return to lobby
			w.Stage = StageLobby
			w.Players[0].Ready = false
			w.Players[1].Ready = false
			return
		}
		p.PrevReady = inp.ReadyToggle
	}
}

func (w *World) rollUpgradesForPlayer(id int) {
	p := &w.Players[id]
	p.UpgradePicked = false
	perm := w.rng.Perm(int(TotalItems))
	p.UpgradeChoices = []ItemType{
		ItemType(perm[0]),
		ItemType(perm[1]),
		ItemType(perm[2]),
	}
}

func (w *World) updateCombat() {
	// Update Jam Mode & Synergy timers
	for i := 0; i < 2; i++ {
		if w.lastHitTimer[i] > 0 {
			w.lastHitTimer[i] = math.Max(0, w.lastHitTimer[i]-DT)
		}
	}
	if w.JamActive {
		w.JamTimer -= DT
		w.Synergy = (w.JamTimer / 5.0) * 100.0
		if w.JamTimer <= 0 {
			w.JamActive = false
			w.JamTimer = 0
			w.Synergy = 0
		}
	}

	// Wave enemy spawner
	if w.enemiesToSpawn > 0 {
		w.spawnTimer -= DT
		if w.spawnTimer <= 0 {
			w.spawnWaveEnemy()
			w.enemiesToSpawn--
			baseInterval := math.Max(0.35, 1.35-float64(w.Wave)*0.08)
			w.spawnTimer = baseInterval * (0.7 + w.rng.Float64()*0.6)
		}
	}

	// Update Players
	w.updatePlayers()

	// Update Projectiles
	w.updateProjectiles()

	// Update Enemies
	w.updateEnemies()

	// Update Zones
	for i := len(w.Zones) - 1; i >= 0; i-- {
		w.Zones[i].Life -= DT
		if w.Zones[i].Life <= 0 {
			w.Zones = append(w.Zones[:i], w.Zones[i+1:]...)
		}
	}

	// Check All Downed -> Game Over
	anyAlive := false
	for i := 0; i < 2; i++ {
		if w.Players[i].Connected && !w.Players[i].Downed {
			anyAlive = true
		}
	}
	if !anyAlive {
		w.Stage = StageGameOver
		w.addFloatingTextBilingual(ArenaWidth/2, ArenaHeight/2-40, "КОНЦЕРТ ОТМЕНЁН - ПОРАЖЕНИЕ", "SHOW CANCELLED - GAME OVER", 5)
		return
	}

	// Check Wave Cleared -> Gear Upgrade Stage
	if w.enemiesToSpawn == 0 && len(w.Enemies) == 0 {
		w.Stage = StageUpgrade
		for i := 0; i < 2; i++ {
			if w.Players[i].Connected {
				w.rollUpgradesForPlayer(i)
			}
		}
		w.addFloatingTextBilingual(ArenaWidth/2, 160, "ВОЛНА ПРОЙДЕНА! ВЫБЕРИ ОБОРУДОВАНИЕ (1, 2, 3)", "WAVE CLEARED! CHOOSE GEAR (1, 2, 3)", 3)
	}
}

func (w *World) spawnWaveEnemy() {
	// Pick spawn point along arena edges
	margin := 36.0
	var x, y float64
	edge := w.rng.Intn(4)
	switch edge {
	case 0: // Top
		x = margin + w.rng.Float64()*(ArenaWidth-2*margin)
		y = margin
	case 1: // Bottom
		x = margin + w.rng.Float64()*(ArenaWidth-2*margin)
		y = ArenaHeight - margin
	case 2: // Left
		x = margin
		y = margin + w.rng.Float64()*(ArenaHeight-2*margin)
	case 3: // Right
		x = ArenaWidth - margin
		y = margin + w.rng.Float64()*(ArenaHeight-2*margin)
	}

	// Choose enemy type based on wave & randomness
	roll := w.rng.Float64()
	var etype EnemyType
	switch {
	case roll < 0.45:
		etype = EnemyHum
	case roll < 0.72:
		etype = EnemyDrunkFan
	case roll < 0.88 && w.Wave >= 1:
		etype = EnemySoundGuy
	default:
		etype = EnemyFeedbackAmp
	}

	hpScale := 1.0 + float64(w.Wave-1)*0.18
	w.nextEntityID++
	e := Enemy{
		ID:   w.nextEntityID,
		Type: etype,
		X:    x,
		Y:    y,
	}

	switch etype {
	case EnemyHum:
		// 50Hz Hum: fast small triangles rushing melee in swarms
		e.Radius = 12.0
		e.MaxHP = 28.0 * hpScale
		e.Speed = 165.0 + w.rng.Float64()*25.0
		e.Damage = 9.0
	case EnemyDrunkFan:
		// Drunk Fan: large square with telegraphed bull-charge
		e.Radius = 21.0
		e.MaxHP = 85.0 * hpScale
		e.Speed = 88.0
		e.Damage = 18.0
		e.AttackCD = 1.5 + w.rng.Float64()*1.5
	case EnemySoundGuy:
		// Annoying Sound Guy: keeps distance, shoots orbs, drops slow zones
		e.Radius = 16.0
		e.MaxHP = 52.0 * hpScale
		e.Speed = 105.0
		e.Damage = 12.0
		e.AttackCD = 1.2 + w.rng.Float64()
	case EnemyFeedbackAmp:
		// Feedback Amp: stationary turret firing fan of waves; spawns inside arena away from obstacles
		e.Radius = 22.0
		e.MaxHP = 95.0 * hpScale
		e.Speed = 0.0
		e.Damage = 14.0
		e.AttackCD = 1.6 + w.rng.Float64()
		for attempt := 0; attempt < 10; attempt++ {
			tx := 140.0 + w.rng.Float64()*(ArenaWidth-280.0)
			ty := 120.0 + w.rng.Float64()*(ArenaHeight-240.0)
			valid := true
			for _, obs := range w.Obstacles {
				if _, _, _, _, hit := CircleRectResolve(tx, ty, e.Radius+20, obs.X, obs.Y, obs.W, obs.H); hit {
					valid = false
					break
				}
			}
			if valid {
				e.X, e.Y = tx, ty
				break
			}
		}
	}
	e.HP = e.MaxHP
	w.Enemies = append(w.Enemies, e)
	w.spawnParticles(e.X, e.Y, 6, 90, 4)
}

func (w *World) updatePlayers() {
	cdRate := 1.0
	if w.JamActive {
		cdRate = 2.0 // JAM MODE doubles attack & ability recharge speed!
	}

	for i := 0; i < 2; i++ {
		p := &w.Players[i]
		if !p.Connected {
			continue
		}
		inp := w.Inputs[i]
		p.AimX = inp.CursorX
		p.AimY = inp.CursorY

		if p.HitFlash > 0 {
			p.HitFlash = math.Max(0, p.HitFlash-DT)
		}
		if p.InvulnTimer > 0 {
			p.InvulnTimer = math.Max(0, p.InvulnTimer-DT)
		}

		// Update Vocalist Spotlight passive count (radius 195 px)
		if p.Class == ClassVocalist {
			cnt := 0
			for _, e := range w.Enemies {
				if DistSq(p.X, p.Y, e.X, e.Y) <= 195.0*195.0 {
					cnt++
				}
			}
			p.SpotlightCount = cnt
		}

		// Update Drummer Combo Tempo decay (2.5 seconds without dealing damage resets combo)
		if p.Class == ClassDrummer && p.ComboCount > 0 {
			p.ComboTimer -= DT
			if p.ComboTimer <= 0 {
				p.ComboCount = 0
				p.ComboTimer = 0
				w.addFloatingTextBilingual(p.X, p.Y-24, "КОМБО СБРОШЕНО", "COMBO LOST", 4)
			}
		}

		// Handle Downed state & Co-op Revive
		if p.Downed {
			p.VX, p.VY = 0, 0
			p.RevivingTarget = -1
			continue
		}

		// Co-op Revive: Hold E within 60px of a downed partner for 2.0 seconds
		otherID := 1 - i
		other := &w.Players[otherID]
		if other.Connected && other.Downed && inp.Interact && Dist(p.X, p.Y, other.X, other.Y) <= 68.0 {
			p.RevivingTarget = otherID
			other.ReviveProgress += DT / 2.0 // 2 seconds to revive
			if other.ReviveProgress >= 1.0 {
				other.Downed = false
				other.HP = other.MaxHP * 0.50
				other.ReviveProgress = 0
				other.InvulnTimer = 1.5
				p.RevivingTarget = -1
				w.addSound(SoundRevive, other.X, other.Y)
				w.addFloatingTextBilingual(other.X, other.Y-28, "СНОВА НА СЦЕНЕ! (50% HP)", "BACK ON STAGE! (50% HP)", 3)
				w.spawnParticles(other.X, other.Y, 18, 160, 3)
			}
		} else {
			if p.RevivingTarget == otherID && other.Downed {
				other.ReviveProgress = math.Max(0, other.ReviveProgress-DT*0.8)
			}
			p.RevivingTarget = -1
		}

		// Movement & Stage Dive / Dash
		if p.DashTimer > 0 {
			p.DashTimer -= DT
			p.X += p.DashVX * DT
			p.Y += p.DashVY * DT
			if p.DashTimer <= 0 && p.Class == ClassVocalist {
				// Stage Dive landing shockwave!
				w.triggerStageDiveLanding(p)
			}
		} else {
			mx, my := inp.MoveX, inp.MoveY
			if mx != 0 || my != 0 {
				mx, my = Normalize(mx, my)
			}
			spd := w.playerSpeed(p)
			if p.RevivingTarget != -1 {
				spd *= 0.35 // Slow while reviving teammate
			}
			p.VX = mx * spd
			p.VY = my * spd
			p.X += p.VX * DT
			p.Y += p.VY * DT
		}

		// Arena boundary & obstacle collision for player
		p.X = Clamp(p.X, p.Radius+16, ArenaWidth-p.Radius-16)
		p.Y = Clamp(p.Y, p.Radius+16, ArenaHeight-p.Radius-16)
		for _, obs := range w.Obstacles {
			if nx, ny, _, _, hit := CircleRectResolve(p.X, p.Y, p.Radius, obs.X, obs.Y, obs.W, obs.H); hit {
				p.X, p.Y = nx, ny
			}
		}

		// Cooldowns
		if p.PrimaryCD > 0 {
			p.PrimaryCD = math.Max(0, p.PrimaryCD-DT*cdRate)
		}
		if p.SecondaryCD > 0 {
			p.SecondaryCD = math.Max(0, p.SecondaryCD-DT*cdRate)
		}

		// Process Digital Delay echoed attacks
		for q := len(p.DelayedQueue) - 1; q >= 0; q-- {
			p.DelayedQueue[q].Timer -= DT
			if p.DelayedQueue[q].Timer <= 0 {
				da := p.DelayedQueue[q]
				p.DelayedQueue = append(p.DelayedQueue[:q], p.DelayedQueue[q+1:]...)
				w.addFloatingTextBilingual(p.X, p.Y-22, "ЭХО ДИЛЕЯ!", "DELAY ECHO!", 4)
				if da.IsSecondary {
					w.executeSecondaryAttack(p, da.DirX, da.DirY, true)
				} else {
					w.executePrimaryAttack(p, da.DirX, da.DirY, true)
				}
			}
		}

		dirX, dirY := Normalize(p.AimX-p.X, p.AimY-p.Y)

		// Primary Attack (LMB)
		if inp.Primary && p.PrimaryCD <= 0 {
			w.executePrimaryAttack(p, dirX, dirY, false)
			w.maybeQueueDigitalDelay(p, dirX, dirY, false)
		}

		// Secondary Ability (RMB / Space)
		if inp.Secondary && p.SecondaryCD <= 0 {
			w.executeSecondaryAttack(p, dirX, dirY, false)
			w.maybeQueueDigitalDelay(p, dirX, dirY, true)
		}
		p.PrevSecondary = inp.Secondary
	}
}

func (w *World) maybeQueueDigitalDelay(p *Player, dirX, dirY float64, isSecondary bool) {
	stacks := p.Items[ItemDigitalDelay]
	if stacks <= 0 {
		return
	}
	// 35% chance per stack (diminishing or additive up to 85%)
	chance := math.Min(0.85, 0.35*float64(stacks))
	if w.rng.Float64() < chance {
		p.DelayedQueue = append(p.DelayedQueue, DelayedAttack{
			Timer:       0.25,
			IsSecondary: isSecondary,
			DirX:        dirX,
			DirY:        dirY,
		})
	}
}

func (w *World) executePrimaryAttack(p *Player, dirX, dirY float64, isEcho bool) {
	sizeMult := w.playerSizeMult(p)
	dmgMult := w.playerDamageMult(p)
	kbMult := w.playerKnockbackMult(p)
	ignite := p.Items[ItemBigMuffFuzz] > 0

	switch p.Class {
	case ClassBassist:
		// Heavy low-frequency soundwaves that pierce through enemies and impart heavy knockback impulse
		if !isEcho {
			p.PrimaryCD = 0.44
		}
		w.nextEntityID++
		w.Projectiles = append(w.Projectiles, Projectile{
			ID:          w.nextEntityID,
			Type:        ProjBassWave,
			OwnerID:     p.ID,
			X:           p.X + dirX*20,
			Y:           p.Y + dirY*20,
			VX:          dirX * 430.0,
			VY:          dirY * 430.0,
			Radius:      22.0 * sizeMult,
			Damage:      24.0 * dmgMult,
			Knockback:   580.0 * kbMult,
			Life:        0.95,
			MaxLife:     0.95,
			Piercing:    true,
			Ignite:      ignite,
			HitEntities: make(map[int]bool),
		})
		w.addSound(SoundBassWave, p.X, p.Y)
		w.addShake(2.2)

	case ClassVocalist:
		// Combined attack: Close-range mic-stand sweep + Mid-range piercing "Scream" cone wave
		if !isEcho {
			p.PrimaryCD = 0.38
		}
		p.SwingDir = -p.SwingDir
		aimAngle := math.Atan2(dirY, dirX)
		meleeRadius := 82.0 * sizeMult
		meleeSpread := 0.85 // ~48 deg half-angle

		// Visual melee arc
		w.Effects = append(w.Effects, VisualEffect{
			Type:    EffectArc,
			X:       p.X,
			Y:       p.Y,
			Radius:  meleeRadius,
			Angle:   aimAngle,
			Spread:  meleeSpread,
			Life:    0.16,
			MaxLife: 0.16,
			ColorID: 5,
		})

		// Hit enemies in melee mic-stand arc
		for idx := range w.Enemies {
			e := &w.Enemies[idx]
			if w.inCone(p.X, p.Y, aimAngle, meleeRadius+e.Radius, meleeSpread, e.X, e.Y) {
				nx, ny := Normalize(e.X-p.X, e.Y-p.Y)
				w.damageEnemy(idx, 20.0*dmgMult, p.ID, false, ignite, nx*240.0*kbMult, ny*240.0*kbMult)
			}
		}

		// Mid-range piercing Scream wave projectile
		w.nextEntityID++
		w.Projectiles = append(w.Projectiles, Projectile{
			ID:          w.nextEntityID,
			Type:        ProjVocalScream,
			OwnerID:     p.ID,
			X:           p.X + dirX*24,
			Y:           p.Y + dirY*24,
			VX:          dirX * 520.0,
			VY:          dirY * 520.0,
			Radius:      25.0 * sizeMult,
			Damage:      17.0 * dmgMult,
			Knockback:   190.0 * kbMult,
			Life:        0.48,
			MaxLife:     0.48,
			Piercing:    true,
			Ignite:      ignite,
			HitEntities: make(map[int]bool),
		})
		w.addSound(SoundVocalHit, p.X, p.Y)

	case ClassGuitarist:
		// Fast narrow riff projectiles that bounce up to 2 times off walls/obstacles and arc lightning feedback on hit
		if !isEcho {
			p.PrimaryCD = 0.25
		}
		// Gain passive: consecutive hits ramp up critical hit chance (+8% per stack, up to 80%)
		critChance := math.Min(0.80, 0.05+float64(p.GainStacks)*0.08)
		isCrit := w.rng.Float64() < critChance
		dmg := 19.0 * dmgMult
		if isCrit {
			dmg *= 1.85
		}
		w.nextEntityID++
		w.Projectiles = append(w.Projectiles, Projectile{
			ID:          w.nextEntityID,
			Type:        ProjGuitarRiff,
			OwnerID:     p.ID,
			X:           p.X + dirX*18,
			Y:           p.Y + dirY*18,
			VX:          dirX * 690.0,
			VY:          dirY * 690.0,
			Radius:      8.5 * sizeMult,
			Damage:      dmg,
			Knockback:   140.0 * kbMult,
			Life:        1.65,
			MaxLife:     1.65,
			Piercing:    false,
			BouncesLeft: 2,
			Ignite:      ignite,
			IsCrit:      isCrit,
			HitEntities: make(map[int]bool),
		})
		w.addSound(SoundGuitarRiff, p.X, p.Y)

	case ClassDrummer:
		// Fast wide-arc drumstick slashes; Combo Tempo dramatically boosts attack speed up to machine-gun tempo
		if !isEcho {
			// Base cooldown 0.30s, scales down to 0.075s at high combo!
			speedFactor := 1.0 + math.Min(float64(p.ComboCount)*0.11, 2.8)
			p.PrimaryCD = 0.30 / speedFactor
		}
		p.SwingDir = -p.SwingDir
		aimAngle := math.Atan2(dirY, dirX)
		slashRadius := 86.0 * sizeMult
		slashSpread := 1.05 // ~120 deg total arc

		w.Effects = append(w.Effects, VisualEffect{
			Type:    EffectArc,
			X:       p.X,
			Y:       p.Y,
			Radius:  slashRadius,
			Angle:   aimAngle,
			Spread:  slashSpread,
			Life:    0.13,
			MaxLife: 0.13,
			ColorID: 1,
		})

		hitAny := false
		for idx := range w.Enemies {
			e := &w.Enemies[idx]
			if w.inCone(p.X, p.Y, aimAngle, slashRadius+e.Radius, slashSpread, e.X, e.Y) {
				nx, ny := Normalize(e.X-p.X, e.Y-p.Y)
				w.damageEnemy(idx, 18.0*dmgMult, p.ID, false, ignite, nx*175.0*kbMult, ny*175.0*kbMult)
				hitAny = true
			}
		}
		if hitAny {
			p.ComboCount++
			p.ComboTimer = 2.5
		}
		w.addSound(SoundDrumHit, p.X, p.Y)
	}
}

func (w *World) executeSecondaryAttack(p *Player, dirX, dirY float64, isEcho bool) {
	sizeMult := w.playerSizeMult(p)
	dmgMult := w.playerDamageMult(p)
	kbMult := w.playerKnockbackMult(p)
	cdMult := w.playerAbilityCDMult(p)
	ignite := p.Items[ItemBigMuffFuzz] > 0

	switch p.Class {
	case ClassBassist:
		// "SLAP": Massive AoE pulse around character that destroys enemy projectiles and blasts enemies into walls
		if !isEcho {
			p.SecondaryMaxCD = 4.5 * cdMult
			p.SecondaryCD = p.SecondaryMaxCD
		}
		slapRadius := 165.0 * sizeMult
		w.Effects = append(w.Effects, VisualEffect{
			Type:    EffectRing,
			X:       p.X,
			Y:       p.Y,
			Radius:  slapRadius,
			Life:    0.32,
			MaxLife: 0.32,
			ColorID: 4,
		})
		// Destroy all enemy projectiles within slap radius
		destroyedProj := 0
		for i := len(w.Projectiles) - 1; i >= 0; i-- {
			pr := w.Projectiles[i]
			if pr.OwnerID == -1 && Dist(p.X, p.Y, pr.X, pr.Y) <= slapRadius+pr.Radius {
				w.spawnParticles(pr.X, pr.Y, 5, 110, 4)
				w.Projectiles = append(w.Projectiles[:i], w.Projectiles[i+1:]...)
				destroyedProj++
			}
		}
		if destroyedProj > 0 {
			w.addFloatingTextBilingual(
				p.X, p.Y-32,
				fmt.Sprintf("СЛЭП-ПАРИРОВАНИЕ x%d!", destroyedProj),
				fmt.Sprintf("SLAP PARRY x%d!", destroyedProj),
				4,
			)
		}
		// Blast enemies away with enormous velocity
		for idx := range w.Enemies {
			e := &w.Enemies[idx]
			if Dist(p.X, p.Y, e.X, e.Y) <= slapRadius+e.Radius {
				nx, ny := Normalize(e.X-p.X, e.Y-p.Y)
				w.damageEnemy(idx, 32.0*dmgMult, p.ID, false, ignite, nx*820.0*kbMult, ny*820.0*kbMult)
			}
		}
		w.addSound(SoundSlap, p.X, p.Y)
		w.addShake(9.5)

	case ClassVocalist:
		// "STAGE DIVE": Aggressive forward leap with invulnerability and shockwave on landing
		if !isEcho {
			p.SecondaryMaxCD = 4.0 * cdMult
			p.SecondaryCD = p.SecondaryMaxCD
		}
		p.DashTimer = 0.26
		p.InvulnTimer = 0.45
		p.DashVX = dirX * 720.0
		p.DashVY = dirY * 720.0
		w.addSound(SoundStageDive, p.X, p.Y)
		w.addFloatingTextBilingual(p.X, p.Y-25, "ПРЫЖОК СО СЦЕНЫ!", "STAGE DIVE!", 5)
		w.spawnParticles(p.X, p.Y, 10, 140, 5)

	case ClassGuitarist:
		// "FEEDBACK BURST": Fan volley of 5 ricocheting riffs with simultaneous backward recoil leap
		if !isEcho {
			p.SecondaryMaxCD = 4.2 * cdMult
			p.SecondaryCD = p.SecondaryMaxCD
		}
		baseAngle := math.Atan2(dirY, dirX)
		for k := -2; k <= 2; k++ {
			ang := baseAngle + float64(k)*0.16
			vx := math.Cos(ang) * 680.0
			vy := math.Sin(ang) * 680.0
			w.nextEntityID++
			w.Projectiles = append(w.Projectiles, Projectile{
				ID:          w.nextEntityID,
				Type:        ProjGuitarRiff,
				OwnerID:     p.ID,
				X:           p.X + math.Cos(ang)*18,
				Y:           p.Y + math.Sin(ang)*18,
				VX:          vx,
				VY:          vy,
				Radius:      9.0 * sizeMult,
				Damage:      22.0 * dmgMult,
				Knockback:   180.0 * kbMult,
				Life:        1.5,
				MaxLife:     1.5,
				BouncesLeft: 2,
				Ignite:      ignite,
				IsCrit:      p.GainStacks >= 4,
				HitEntities: make(map[int]bool),
			})
		}
		// Recoil hop backward
		p.DashTimer = 0.14
		p.InvulnTimer = 0.22
		p.DashVX = -dirX * 520.0
		p.DashVY = -dirY * 520.0
		w.addSound(SoundGuitarRiff, p.X, p.Y)
		w.addShake(4.5)

	case ClassDrummer:
		// "CYMBAL BOOMERANG": Throws a spinning saw-blade cymbal that slices through crowds and returns to the drummer
		if !isEcho {
			p.SecondaryMaxCD = 3.8 * cdMult
			p.SecondaryCD = p.SecondaryMaxCD
		}
		w.nextEntityID++
		w.Projectiles = append(w.Projectiles, Projectile{
			ID:          w.nextEntityID,
			Type:        ProjCymbalBoomerang,
			OwnerID:     p.ID,
			X:           p.X + dirX*20,
			Y:           p.Y + dirY*20,
			VX:          dirX * 540.0,
			VY:          dirY * 540.0,
			Radius:      24.0 * sizeMult,
			Damage:      21.0 * dmgMult,
			Knockback:   220.0 * kbMult,
			Life:        1.55,
			MaxLife:     1.55,
			Piercing:    true,
			Ignite:      ignite,
			HitEntities: make(map[int]bool),
		})
		w.addSound(SoundCymbalThrow, p.X, p.Y)
	}
}

func (w *World) triggerStageDiveLanding(p *Player) {
	sizeMult := w.playerSizeMult(p)
	dmgMult := w.playerDamageMult(p)
	kbMult := w.playerKnockbackMult(p)
	ignite := p.Items[ItemBigMuffFuzz] > 0
	radius := 135.0 * sizeMult

	w.Effects = append(w.Effects, VisualEffect{
		Type:    EffectRing,
		X:       p.X,
		Y:       p.Y,
		Radius:  radius,
		Life:    0.28,
		MaxLife: 0.28,
		ColorID: 5,
	})
	for idx := range w.Enemies {
		e := &w.Enemies[idx]
		if Dist(p.X, p.Y, e.X, e.Y) <= radius+e.Radius {
			nx, ny := Normalize(e.X-p.X, e.Y-p.Y)
			w.damageEnemy(idx, 42.0*dmgMult, p.ID, true, ignite, nx*460.0*kbMult, ny*460.0*kbMult)
		}
	}
	w.addSound(SoundSlap, p.X, p.Y)
	w.addShake(8.0)
	w.spawnParticles(p.X, p.Y, 18, 200, 5)
}

func (w *World) inCone(ox, oy, aimAngle, radius, halfSpread, tx, ty float64) bool {
	if DistSq(ox, oy, tx, ty) > radius*radius {
		return false
	}
	ang := math.Atan2(ty-oy, tx-ox)
	diff := math.Mod(ang-aimAngle+math.Pi*3, math.Pi*2) - math.Pi
	return math.Abs(diff) <= halfSpread
}

func (w *World) updateProjectiles() {
	for i := len(w.Projectiles) - 1; i >= 0; i-- {
		pr := &w.Projectiles[i]
		pr.Life -= DT
		if pr.Life <= 0 {
			// If Guitarist riff expired without hitting anything, reset Gain stacks
			if pr.Type == ProjGuitarRiff && pr.OwnerID >= 0 && len(pr.HitEntities) == 0 {
				w.Players[pr.OwnerID].GainStacks = 0
			}
			w.Projectiles = append(w.Projectiles[:i], w.Projectiles[i+1:]...)
			continue
		}

		// Boomerang cymbal return physics
		if pr.Type == ProjCymbalBoomerang && pr.OwnerID >= 0 {
			owner := &w.Players[pr.OwnerID]
			pr.HitResetCD -= DT
			if pr.HitResetCD <= 0 {
				pr.HitEntities = make(map[int]bool)
				pr.HitResetCD = 0.22
			}
			if pr.Life < pr.MaxLife*0.58 {
				pr.Returning = true
			}
			if pr.Returning {
				dx, dy := Normalize(owner.X-pr.X, owner.Y-pr.Y)
				pr.VX = dx * 600.0
				pr.VY = dy * 600.0
				if Dist(pr.X, pr.Y, owner.X, owner.Y) <= owner.Radius+pr.Radius {
					w.Projectiles = append(w.Projectiles[:i], w.Projectiles[i+1:]...)
					continue
				}
			}
		}

		pr.X += pr.VX * DT
		pr.Y += pr.VY * DT

		// Wall & Obstacle collisions
		bouncedOrRemoved := false
		// Arena walls
		if pr.X-pr.Radius < 16 || pr.X+pr.Radius > ArenaWidth-16 {
			if pr.BouncesLeft > 0 {
				pr.VX = -pr.VX
				pr.BouncesLeft--
				pr.X = Clamp(pr.X, pr.Radius+17, ArenaWidth-pr.Radius-17)
				w.spawnParticles(pr.X, pr.Y, 4, 90, 4)
			} else if !pr.Returning && pr.Type != ProjCymbalBoomerang {
				if pr.Type == ProjGuitarRiff && pr.OwnerID >= 0 && len(pr.HitEntities) == 0 {
					w.Players[pr.OwnerID].GainStacks = 0
				}
				w.Projectiles = append(w.Projectiles[:i], w.Projectiles[i+1:]...)
				continue
			}
		}
		if pr.Y-pr.Radius < 16 || pr.Y+pr.Radius > ArenaHeight-16 {
			if pr.BouncesLeft > 0 {
				pr.VY = -pr.VY
				pr.BouncesLeft--
				pr.Y = Clamp(pr.Y, pr.Radius+17, ArenaHeight-pr.Radius-17)
				w.spawnParticles(pr.X, pr.Y, 4, 90, 4)
			} else if !pr.Returning && pr.Type != ProjCymbalBoomerang {
				if pr.Type == ProjGuitarRiff && pr.OwnerID >= 0 && len(pr.HitEntities) == 0 {
					w.Players[pr.OwnerID].GainStacks = 0
				}
				w.Projectiles = append(w.Projectiles[:i], w.Projectiles[i+1:]...)
				continue
			}
		}

		// Obstacles
		if pr.Type != ProjCymbalBoomerang {
			for _, obs := range w.Obstacles {
				if px, py, nx, ny, hit := CircleRectResolve(pr.X, pr.Y, pr.Radius, obs.X, obs.Y, obs.W, obs.H); hit {
					if pr.BouncesLeft > 0 {
						pr.X, pr.Y = px, py
						if math.Abs(nx) > math.Abs(ny) {
							pr.VX = -pr.VX
						} else {
							pr.VY = -pr.VY
						}
						pr.BouncesLeft--
						w.spawnParticles(pr.X, pr.Y, 4, 90, 4)
					} else if pr.Type != ProjBassWave && pr.Type != ProjVocalScream {
						if pr.Type == ProjGuitarRiff && pr.OwnerID >= 0 && len(pr.HitEntities) == 0 {
							w.Players[pr.OwnerID].GainStacks = 0
						}
						w.Projectiles = append(w.Projectiles[:i], w.Projectiles[i+1:]...)
						bouncedOrRemoved = true
						break
					}
				}
			}
			if bouncedOrRemoved {
				continue
			}
		}

		// Check hits
		if pr.OwnerID >= 0 {
			// Player projectile vs Enemies
			projDestroyed := false
			for eIdx := range w.Enemies {
				e := &w.Enemies[eIdx]
				if e.HP <= 0 || pr.HitEntities[e.ID] {
					continue
				}
				if DistSq(pr.X, pr.Y, e.X, e.Y) <= (pr.Radius+e.Radius)*(pr.Radius+e.Radius) {
					pr.HitEntities[e.ID] = true
					nx, ny := Normalize(pr.VX, pr.VY)
					w.damageEnemy(eIdx, pr.Damage, pr.OwnerID, pr.IsCrit, pr.Ignite, nx*pr.Knockback, ny*pr.Knockback)

					// Class on-hit effects
					owner := &w.Players[pr.OwnerID]
					if pr.Type == ProjGuitarRiff {
						owner.GainStacks = int(math.Min(10, float64(owner.GainStacks+1)))
						// Chain electric feedback arc to nearest neighbor enemy!
						w.triggerGuitarChainArc(e, pr.Damage*0.55, pr.OwnerID, pr.Ignite)
					}
					if owner.Class == ClassDrummer {
						owner.ComboCount++
						owner.ComboTimer = 2.5
					}

					if !pr.Piercing {
						if pr.BouncesLeft > 0 {
							// Ricochet towards another nearby enemy or reflect!
							pr.BouncesLeft--
							rebounded := false
							for otherIdx := range w.Enemies {
								other := &w.Enemies[otherIdx]
								if other.ID != e.ID && other.HP > 0 && !pr.HitEntities[other.ID] && Dist(e.X, e.Y, other.X, other.Y) < 250.0 {
									rx, ry := Normalize(other.X-pr.X, other.Y-pr.Y)
									spd := math.Hypot(pr.VX, pr.VY)
									pr.VX = rx * spd
									pr.VY = ry * spd
									rebounded = true
									break
								}
							}
							if !rebounded {
								pr.VX = -pr.VX
								pr.VY = -pr.VY
							}
						} else {
							projDestroyed = true
							break
						}
					}
				}
			}
			if projDestroyed {
				w.Projectiles = append(w.Projectiles[:i], w.Projectiles[i+1:]...)
				continue
			}
		} else {
			// Enemy projectile vs Players
			for pIdx := 0; pIdx < 2; pIdx++ {
				p := &w.Players[pIdx]
				if !p.Connected || p.Downed || p.InvulnTimer > 0 {
					continue
				}
				if DistSq(pr.X, pr.Y, p.X, p.Y) <= (pr.Radius+p.Radius)*(pr.Radius+p.Radius) {
					w.damagePlayer(p, pr.Damage)
					w.Projectiles = append(w.Projectiles[:i], w.Projectiles[i+1:]...)
					break
				}
			}
		}
	}
}

func (w *World) triggerGuitarChainArc(sourceEnemy *Enemy, arcDmg float64, ownerID int, ignite bool) {
	bestIdx := -1
	bestDist := 210.0
	for idx := range w.Enemies {
		other := &w.Enemies[idx]
		if other.ID == sourceEnemy.ID || other.HP <= 0 {
			continue
		}
		d := Dist(sourceEnemy.X, sourceEnemy.Y, other.X, other.Y)
		if d < bestDist {
			bestDist = d
			bestIdx = idx
		}
	}
	if bestIdx >= 0 {
		target := &w.Enemies[bestIdx]
		w.Effects = append(w.Effects, VisualEffect{
			Type:    EffectLine,
			X:       sourceEnemy.X,
			Y:       sourceEnemy.Y,
			X2:      target.X,
			Y2:      target.Y,
			Life:    0.18,
			MaxLife: 0.18,
			ColorID: 4,
		})
		w.damageEnemy(bestIdx, arcDmg, ownerID, false, ignite, 0, 0)
	}
}

func (w *World) damageEnemy(enemyIdx int, dmg float64, ownerID int, isCrit bool, ignite bool, kbVX, kbVY float64) {
	if enemyIdx < 0 || enemyIdx >= len(w.Enemies) {
		return
	}
	e := &w.Enemies[enemyIdx]
	if e.HP <= 0 {
		return
	}

	e.HP -= dmg
	e.HitFlash = 0.12

	if ignite {
		e.BurnTimer = 3.0
		e.BurnSourceID = ownerID
	}

	// Knockback impulse (Feedback Amp is heavy, takes 40% knockback; others take full)
	kbFactor := 1.0
	if e.Type == EnemyFeedbackAmp {
		kbFactor = 0.45
	}
	if math.Hypot(kbVX, kbVY) > 10.0 {
		e.KBVX += kbVX * kbFactor
		e.KBVY += kbVY * kbFactor
		if math.Hypot(e.KBVX, e.KBVY) > 260.0 {
			e.KnockedBack = true
			e.KBSourceID = ownerID
		}
	}

	// Floating damage number
	colorID := 0
	text := fmt.Sprintf("%.0f", dmg)
	if isCrit {
		colorID = 1
		text = fmt.Sprintf("%.0f!", dmg)
	}
	w.addFloatingText(e.X, e.Y-e.Radius, text, colorID)
	w.spawnParticles(e.X, e.Y, 4, 110, colorID)

	// Co-op Synergy gauge charging
	if ownerID >= 0 && ownerID < 2 && !w.JamActive {
		w.lastHitTimer[ownerID] = 1.6
		otherID := 1 - ownerID
		if w.Players[1].Connected {
			// Simultaneous damage from both players charges Synergy super fast!
			if w.lastHitTimer[otherID] > 0 {
				w.Synergy += 3.8
			} else {
				w.Synergy += 1.1
			}
		} else {
			// Solo test mode still lets player build Synergy
			w.Synergy += 2.2
		}
		if w.Synergy >= 100.0 {
			w.Synergy = 100.0
			w.JamActive = true
			w.JamTimer = 5.0
			w.addSound(SoundJamStart, ArenaWidth/2, ArenaHeight/2)
			w.addShake(7.0)
			w.addFloatingTextBilingual(
				ArenaWidth/2, 130,
				"РЕЖИМ ДЖЕМА! (2x СКОРОСТЬ И +35% УРОН)",
				"JAM MODE ACTIVATED! (2x SPEED & +35% DMG)",
				1,
			)
		}
	}
}

func (w *World) damagePlayer(p *Player, rawDmg float64) {
	if p.Downed || p.InvulnTimer > 0 {
		return
	}
	dmg := rawDmg * w.playerDamageTakenMult(p)
	p.HP -= dmg
	p.InvulnTimer = 0.55
	p.HitFlash = 0.20
	w.addSound(SoundPlayerHit, p.X, p.Y)
	w.addShake(5.0)
	w.addFloatingText(p.X, p.Y-24, fmt.Sprintf("-%.0f", dmg), 5)
	w.spawnParticles(p.X, p.Y, 10, 140, 5)

	if p.HP <= 0 {
		p.HP = 0
		p.Downed = true
		p.ReviveProgress = 0
		w.addFloatingTextBilingual(p.X, p.Y-32, "СБИТ С НОГ! ЗАЖМИ [E] ДЛЯ ПОДЪЁМА", "DOWNED! HOLD [E] TO REVIVE", 5)
	}
}

func (w *World) updateEnemies() {
	for i := 0; i < len(w.Enemies); i++ {
		e := &w.Enemies[i]
		if e.HP <= 0 {
			continue
		}

		if e.HitFlash > 0 {
			e.HitFlash = math.Max(0, e.HitFlash-DT)
		}
		if e.WallSlamCD > 0 {
			e.WallSlamCD = math.Max(0, e.WallSlamCD-DT)
		}
		if e.StunTimer > 0 {
			e.StunTimer = math.Max(0, e.StunTimer-DT)
		}

		// Burn DoT (Big Muff Fuzz)
		if e.BurnTimer > 0 {
			e.BurnTimer -= DT
			e.BurnTick -= DT
			if e.BurnTick <= 0 {
				e.BurnTick = 0.45
				burnDmg := 7.0
				if e.BurnSourceID >= 0 && e.BurnSourceID < 2 {
					burnDmg *= float64(w.Players[e.BurnSourceID].Items[ItemBigMuffFuzz])
				}
				e.HP -= burnDmg
				w.addFloatingText(e.X, e.Y-e.Radius, fmt.Sprintf("%.0f", burnDmg), 2)
				w.spawnParticles(e.X, e.Y, 2, 60, 2)
			}
		}

		// Handle Knockback physics & Bassist Wall-Slam / Enemy-Slam collisions!
		kbSpeed := math.Hypot(e.KBVX, e.KBVY)
		if kbSpeed > 15.0 {
			e.X += e.KBVX * DT
			e.Y += e.KBVY * DT
			// Friction decay
			decay := math.Exp(-5.5 * DT)
			e.KBVX *= decay
			e.KBVY *= decay

			// Check high-speed Wall Slam or Pillar Slam!
			if e.KnockedBack && kbSpeed > 230.0 && e.WallSlamCD <= 0 {
				slammedWall := false
				if e.X-e.Radius <= 18 || e.X+e.Radius >= ArenaWidth-18 || e.Y-e.Radius <= 18 || e.Y+e.Radius >= ArenaHeight-18 {
					slammedWall = true
				}
				if !slammedWall {
					for _, obs := range w.Obstacles {
						if nx, ny, _, _, hit := CircleRectResolve(e.X, e.Y, e.Radius, obs.X, obs.Y, obs.W, obs.H); hit {
							e.X, e.Y = nx, ny
							slammedWall = true
							break
						}
					}
				}
				if slammedWall {
					w.applyWallSlam(i, kbSpeed)
				}

				// Check collision with another enemy while flying at high speed!
				for j := 0; j < len(w.Enemies); j++ {
					if i == j || w.Enemies[j].HP <= 0 {
						continue
					}
					other := &w.Enemies[j]
					if DistSq(e.X, e.Y, other.X, other.Y) <= (e.Radius+other.Radius)*(e.Radius+other.Radius) {
						// Both enemies take collision bonus damage + micro-stun!
						slamDmg := 28.0 + kbSpeed*0.035
						e.HP -= slamDmg
						other.HP -= slamDmg
						e.StunTimer = 0.85
						other.StunTimer = 0.85
						e.WallSlamCD = 0.35
						other.KBVX = e.KBVX * 0.65
						other.KBVY = e.KBVY * 0.65
						e.KBVX *= -0.35
						e.KBVY *= -0.35
						e.KnockedBack = false
						w.addSound(SoundWallSlam, e.X, e.Y)
						w.addShake(5.5)
						w.addFloatingTextBilingual(
							other.X, other.Y-20,
							fmt.Sprintf("СТОЛКНОВЕНИЕ! %.0f", slamDmg),
							fmt.Sprintf("CRASH! %.0f", slamDmg),
							2,
						)
						w.spawnParticles((e.X+other.X)/2, (e.Y+other.Y)/2, 12, 160, 2)
						break
					}
				}
			}
		} else {
			e.KBVX, e.KBVY = 0, 0
			e.KnockedBack = false
		}

		// Skip AI movement if stunned
		if e.StunTimer <= 0 && e.HP > 0 {
			w.runEnemyAI(e)
		}

		// Keep enemy inside arena bounds and outside obstacles
		e.X = Clamp(e.X, e.Radius+16, ArenaWidth-e.Radius-16)
		e.Y = Clamp(e.Y, e.Radius+16, ArenaHeight-e.Radius-16)
		for _, obs := range w.Obstacles {
			if nx, ny, _, _, hit := CircleRectResolve(e.X, e.Y, e.Radius, obs.X, obs.Y, obs.W, obs.H); hit {
				e.X, e.Y = nx, ny
			}
		}

		// Contact damage with players
		if e.HP > 0 {
			for pIdx := 0; pIdx < 2; pIdx++ {
				p := &w.Players[pIdx]
				if !p.Connected || p.Downed || p.InvulnTimer > 0 {
					continue
				}
				if DistSq(e.X, e.Y, p.X, p.Y) <= (e.Radius+p.Radius)*(e.Radius+p.Radius) {
					w.damagePlayer(p, e.Damage)
					// Push enemy back slightly on contact
					nx, ny := Normalize(e.X-p.X, e.Y-p.Y)
					e.KBVX += nx * 180.0
					e.KBVY += ny * 180.0
				}
			}
		}
	}

	// Remove dead enemies and trigger on-kill effects (Vampire Pickup)
	for i := len(w.Enemies) - 1; i >= 0; i-- {
		e := w.Enemies[i]
		if e.HP <= 0 {
			w.addSound(SoundEnemyDeath, e.X, e.Y)
			w.spawnParticles(e.X, e.Y, 14, 150, 2)
			// Vampire Pickup: restores 2 HP per stack on every enemy kill
			for pIdx := 0; pIdx < 2; pIdx++ {
				p := &w.Players[pIdx]
				if p.Connected && !p.Downed && p.Items[ItemVampirePickup] > 0 {
					heal := 2.0 * float64(p.Items[ItemVampirePickup])
					if p.HP < p.MaxHP {
						p.HP = math.Min(p.MaxHP, p.HP+heal)
						w.addFloatingText(p.X, p.Y-20, fmt.Sprintf("+%.0f HP", heal), 3)
					}
				}
			}
			w.Enemies = append(w.Enemies[:i], w.Enemies[i+1:]...)
		}
	}
}

func (w *World) applyWallSlam(enemyIdx int, speed float64) {
	e := &w.Enemies[enemyIdx]
	slamDmg := 34.0 + speed*0.045
	e.HP -= slamDmg
	e.StunTimer = 1.05
	e.WallSlamCD = 0.35
	e.KnockedBack = false
	e.KBVX *= -0.3
	e.KBVY *= -0.3
	w.addSound(SoundWallSlam, e.X, e.Y)
	w.addShake(6.5)
	w.addFloatingTextBilingual(
		e.X, e.Y-24,
		fmt.Sprintf("О СТЕНУ! %.0f", slamDmg),
		fmt.Sprintf("WALL SLAM! %.0f", slamDmg),
		2,
	)
	w.spawnParticles(e.X, e.Y, 14, 180, 2)
}

func (w *World) findNearestAlivePlayer(x, y float64) *Player {
	var best *Player
	bestDist := 1e9
	for i := 0; i < 2; i++ {
		p := &w.Players[i]
		if !p.Connected || p.Downed {
			continue
		}
		d := Dist(x, y, p.X, p.Y)
		if d < bestDist {
			bestDist = d
			best = p
		}
	}
	return best
}

func (w *World) runEnemyAI(e *Enemy) {
	target := w.findNearestAlivePlayer(e.X, e.Y)
	if target == nil {
		return
	}
	dx, dy := Normalize(target.X-e.X, target.Y-e.Y)
	dist := Dist(e.X, e.Y, target.X, target.Y)

	switch e.Type {
	case EnemyHum:
		// 50Hz Hum: fast serpentine movement toward player
		e.StateTimer += DT * 8.0
		perpX, perpY := -dy, dx
		wiggle := math.Sin(e.StateTimer+float64(e.ID)) * 0.45
		vx, vy := Normalize(dx+perpX*wiggle, dy+perpY*wiggle)
		e.X += vx * e.Speed * DT
		e.Y += vy * e.Speed * DT

	case EnemyDrunkFan:
		// Drunk Fan: walks toward player, periodically telegraphs a line and bull-charges
		if e.Telegraphing {
			e.StateTimer -= DT
			if e.StateTimer <= 0 {
				e.Telegraphing = false
				e.Charging = true
				e.StateTimer = 0.55 // Charge duration
			}
		} else if e.Charging {
			e.StateTimer -= DT
			e.X += e.ChargeDirX * 390.0 * DT
			e.Y += e.ChargeDirY * 390.0 * DT
			if e.StateTimer <= 0 {
				e.Charging = false
				e.AttackCD = 2.4
			}
		} else {
			e.X += dx * e.Speed * DT
			e.Y += dy * e.Speed * DT
			e.AttackCD -= DT
			if e.AttackCD <= 0 && dist < 340.0 {
				e.Telegraphing = true
				e.StateTimer = 0.65 // Telegraph warning time
				e.ChargeDirX = dx
				e.ChargeDirY = dy
			}
		}

	case EnemySoundGuy:
		// Annoying Sound Guy: maintains distance (~260px), shoots orbs and drops slow zones
		if dist < 210.0 {
			e.X -= dx * e.Speed * DT
			e.Y -= dy * e.Speed * DT
		} else if dist > 310.0 {
			e.X += dx * e.Speed * DT
			e.Y += dy * e.Speed * DT
		} else {
			// Strafe
			e.X += -dy * e.Speed * 0.6 * DT
			e.Y += dx * e.Speed * 0.6 * DT
		}

		e.AttackCD -= DT
		if e.AttackCD <= 0 {
			e.AttackCD = 2.1
			// Shoot sound orb
			w.nextEntityID++
			w.Projectiles = append(w.Projectiles, Projectile{
				ID:      w.nextEntityID,
				Type:    ProjEnemyOrb,
				OwnerID: -1,
				X:       e.X + dx*18,
				Y:       e.Y + dy*18,
				VX:      dx * 265.0,
				VY:      dy * 265.0,
				Radius:  9.0,
				Damage:  12.0,
				Life:    2.6,
				MaxLife: 2.6,
			})
			// Every second shot, place a Slow Zone near player
			e.StateTimer += 1.0
			if int(e.StateTimer)%2 == 0 {
				w.nextEntityID++
				w.Zones = append(w.Zones, SlowZone{
					ID:      w.nextEntityID,
					X:       target.X + (w.rng.Float64()-0.5)*70.0,
					Y:       target.Y + (w.rng.Float64()-0.5)*70.0,
					Radius:  62.0,
					Life:    4.5,
					MaxLife: 4.5,
					IsEnemy: true,
				})
			}
		}

	case EnemyFeedbackAmp:
		// Stationary turret firing a fan of 3 waves toward nearest player
		e.AttackCD -= DT
		if e.AttackCD <= 0 {
			e.AttackCD = 2.3
			baseAng := math.Atan2(dy, dx)
			for k := -1; k <= 1; k++ {
				ang := baseAng + float64(k)*0.28
				w.nextEntityID++
				w.Projectiles = append(w.Projectiles, Projectile{
					ID:      w.nextEntityID,
					Type:    ProjEnemyWave,
					OwnerID: -1,
					X:       e.X + math.Cos(ang)*22,
					Y:       e.Y + math.Sin(ang)*22,
					VX:      math.Cos(ang) * 235.0,
					VY:      math.Sin(ang) * 235.0,
					Radius:  11.0,
					Damage:  13.0,
					Life:    2.8,
					MaxLife: 2.8,
				})
			}
		}
	}
}

func (w *World) updateParticlesAndTexts() {
	for i := len(w.FloatingTexts) - 1; i >= 0; i-- {
		ft := &w.FloatingTexts[i]
		ft.Y += ft.VY * DT
		ft.Life -= DT
		if ft.Life <= 0 {
			w.FloatingTexts = append(w.FloatingTexts[:i], w.FloatingTexts[i+1:]...)
		}
	}
	for i := len(w.Particles) - 1; i >= 0; i-- {
		pt := &w.Particles[i]
		pt.X += pt.VX * DT
		pt.Y += pt.VY * DT
		pt.VX *= 0.94
		pt.VY *= 0.94
		pt.Life -= DT
		if pt.Life <= 0 {
			w.Particles = append(w.Particles[:i], w.Particles[i+1:]...)
		}
	}
	for i := len(w.Effects) - 1; i >= 0; i-- {
		ef := &w.Effects[i]
		ef.Life -= DT
		if ef.Life <= 0 {
			w.Effects = append(w.Effects[:i], w.Effects[i+1:]...)
		}
	}
}

// DrainSnapshot creates a deep-copied WorldSnapshot and clears PendingSounds so each sound triggers once.
func (w *World) DrainSnapshot() WorldSnapshot {
	w.mu.Lock()
	defer w.mu.Unlock()

	players := make([]Player, 2)
	for i := 0; i < 2; i++ {
		players[i] = w.Players[i]
		itemsCopy := make(map[ItemType]int, len(w.Players[i].Items))
		for k, v := range w.Players[i].Items {
			itemsCopy[k] = v
		}
		players[i].Items = itemsCopy
		if len(w.Players[i].UpgradeChoices) > 0 {
			choices := make([]ItemType, len(w.Players[i].UpgradeChoices))
			copy(choices, w.Players[i].UpgradeChoices)
			players[i].UpgradeChoices = choices
		}
	}

	enemies := make([]Enemy, len(w.Enemies))
	copy(enemies, w.Enemies)

	projs := make([]Projectile, len(w.Projectiles))
	copy(projs, w.Projectiles)

	zones := make([]SlowZone, len(w.Zones))
	copy(zones, w.Zones)

	obs := make([]Obstacle, len(w.Obstacles))
	copy(obs, w.Obstacles)

	texts := make([]FloatingText, len(w.FloatingTexts))
	copy(texts, w.FloatingTexts)

	parts := make([]Particle, len(w.Particles))
	copy(parts, w.Particles)

	effs := make([]VisualEffect, len(w.Effects))
	copy(effs, w.Effects)

	sounds := make([]SoundEvent, len(w.PendingSounds))
	copy(sounds, w.PendingSounds)
	w.PendingSounds = w.PendingSounds[:0]

	return WorldSnapshot{
		Tick:          w.Tick,
		Stage:         w.Stage,
		Wave:          w.Wave,
		EnemiesLeft:   len(w.Enemies) + w.enemiesToSpawn,
		Synergy:       w.Synergy,
		JamActive:     w.JamActive,
		JamTimer:      w.JamTimer,
		ScreenShake:   w.ScreenShake,
		Players:       players,
		Enemies:       enemies,
		Projectiles:   projs,
		Zones:         zones,
		Obstacles:     obs,
		FloatingTexts: texts,
		Particles:     parts,
		Effects:       effs,
		Sounds:        sounds,
	}
}
