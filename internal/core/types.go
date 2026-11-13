package core

import "math"

const (
	ArenaWidth  = 1280.0
	ArenaHeight = 720.0
	TPS         = 60.0
	DT          = 1.0 / TPS
)

type Lang string

const (
	LangRU Lang = "ru"
	LangEN Lang = "en"
)

type GameStage int

const (
	StageLobby GameStage = iota
	StageCombat
	StageUpgrade
	StageGameOver
)

type ClassType int

const (
	ClassBassist ClassType = iota
	ClassVocalist
	ClassGuitarist
	ClassDrummer
)

func (c ClassType) Name() string {
	return c.NameLang(LangRU)
}

func (c ClassType) NameLang(lang Lang) string {
	if lang == LangEN {
		switch c {
		case ClassBassist:
			return "BASSIST"
		case ClassVocalist:
			return "VOCALIST"
		case ClassGuitarist:
			return "GUITARIST"
		case ClassDrummer:
			return "DRUMMER"
		default:
			return "UNKNOWN"
		}
	}
	switch c {
	case ClassBassist:
		return "БАСИСТ"
	case ClassVocalist:
		return "ВОКАЛИСТ"
	case ClassGuitarist:
		return "ГИТАРИСТ"
	case ClassDrummer:
		return "БАРАБАНЩИК"
	default:
		return "НЕИЗВЕСТНО"
	}
}

func (c ClassType) Subtitle() string {
	return c.SubtitleLang(LangRU)
}

func (c ClassType) SubtitleLang(lang Lang) string {
	if lang == LangEN {
		switch c {
		case ClassBassist:
			return "Heavy Artillery & Wall-Slam Knockback"
		case ClassVocalist:
			return "Frontman Berserker & Crowd Spotlight"
		case ClassGuitarist:
			return "Ricochet Riffs & Chain Feedback"
		case ClassDrummer:
			return "Hyper-Tempo Melee & Cymbal Saw"
		default:
			return ""
		}
	}
	switch c {
	case ClassBassist:
		return "Тяжёлая артиллерия и удары о стены"
	case ClassVocalist:
		return "Фронтмен-берсерк и пассивка Софиты"
	case ClassGuitarist:
		return "Рикошеты риффов и цепной фидбэк"
	case ClassDrummer:
		return "Разгон темпа и тарелка-бумеранг"
	default:
		return ""
	}
}

type ItemType int

const (
	ItemBassCabinet ItemType = iota
	ItemOverheatedTubes
	ItemDigitalDelay
	ItemBigMuffFuzz
	ItemDoubleKickPedal
	ItemVampirePickup
	TotalItems
)

type ItemDef struct {
	Type    ItemType
	Name    string
	Desc    string
	Short   string
	NameEN  string
	DescEN  string
	ShortEN string
}

func (d ItemDef) GetName(lang Lang) string {
	if lang == LangEN && d.NameEN != "" {
		return d.NameEN
	}
	return d.Name
}

func (d ItemDef) GetDesc(lang Lang) string {
	if lang == LangEN && d.DescEN != "" {
		return d.DescEN
	}
	return d.Desc
}

func (d ItemDef) GetShort(lang Lang) string {
	if lang == LangEN && d.ShortEN != "" {
		return d.ShortEN
	}
	return d.Short
}

var AllItems = []ItemDef{
	{
		Type:    ItemBassCabinet,
		Name:    "Басовый кабинет 8x10",
		Desc:    "+50 макс. HP, +25% к силе отбрасывания, -15% входящего урона",
		Short:   "Кабинет",
		NameEN:  "8x10 Bass Cabinet",
		DescEN:  "+50 Max HP, +25% Knockback, -15% Damage Taken",
		ShortEN: "Cab",
	},
	{
		Type:    ItemOverheatedTubes,
		Name:    "Перегретые лампы",
		Desc:    "+45% ко всему исходящему урону, -25 макс. HP",
		Short:   "Лампы",
		NameEN:  "Overheated Tubes",
		DescEN:  "+45% All Damage Dealt, -25 Max HP",
		ShortEN: "Tubes",
	},
	{
		Type:    ItemDigitalDelay,
		Name:    "Цифровой дилей",
		Desc:    "35% шанс продублировать любую атаку через 0.25 сек",
		Short:   "Дилей",
		NameEN:  "Digital Delay",
		DescEN:  "35% chance to echo-duplicate any attack after 0.25s",
		ShortEN: "Delay",
	},
	{
		Type:    ItemBigMuffFuzz,
		Name:    "Примочка Big Muff Fuzz",
		Desc:    "+25% к размеру снарядов/зон и поджог врагов (урон от огня)",
		Short:   "Фузз",
		NameEN:  "Big Muff Fuzz",
		DescEN:  "+25% Attack Size & Ignites enemies (burn DoT)",
		ShortEN: "Fuzz",
	},
	{
		Type:    ItemDoubleKickPedal,
		Name:    "Двойная педаль (Кардан)",
		Desc:    "+25% к скорости бега, -30% к перезарядке способности (ПКМ/Пробел)",
		Short:   "Кардан",
		NameEN:  "Double Kick Pedal",
		DescEN:  "+25% Move Speed, -30% Ability Cooldown (RMB/Space)",
		ShortEN: "Kick",
	},
	{
		Type:    ItemVampirePickup,
		Name:    "Вампирский звукосниматель",
		Desc:    "Восстанавливает 2 HP за каждого убитого врага",
		Short:   "Звукосн.",
		NameEN:  "Vampire Pickup",
		DescEN:  "Restore 2 HP on every enemy kill",
		ShortEN: "Vamp",
	},
}

type EnemyType int

const (
	EnemyHum EnemyType = iota
	EnemyDrunkFan
	EnemySoundGuy
	EnemyFeedbackAmp
)

type ProjType int

const (
	ProjBassWave ProjType = iota
	ProjVocalScream
	ProjGuitarRiff
	ProjCymbalBoomerang
	ProjEnemyOrb
	ProjEnemyWave
)

type SoundType int

const (
	SoundNone SoundType = iota
	SoundBassWave
	SoundSlap
	SoundVocalHit
	SoundStageDive
	SoundGuitarRiff
	SoundDrumHit
	SoundCymbalThrow
	SoundWallSlam
	SoundEnemyDeath
	SoundPlayerHit
	SoundJamStart
	SoundUpgradePick
	SoundRevive
)

type EffectType int

const (
	EffectRing EffectType = iota
	EffectCone
	EffectLine
	EffectArc
)

type PlayerInput struct {
	MoveX       float64
	MoveY       float64
	CursorX     float64
	CursorY     float64
	Primary     bool
	Secondary   bool
	Interact    bool
	SelectClass int // 1..4, 0 if none
	SelectItem  int // 1..3, 0 if none
	ReadyToggle bool
}

type DelayedAttack struct {
	Timer       float64
	IsSecondary bool
	DirX        float64
	DirY        float64
}

type Player struct {
	ID             int
	Connected      bool
	Ready          bool
	Class          ClassType
	X, Y           float64
	VX, VY         float64
	Radius         float64
	HP             float64
	MaxHP          float64
	BaseSpeed      float64
	Downed         bool
	ReviveProgress float64 // 0..1 (2 seconds to revive)
	RevivingTarget int     // ID of player being revived (-1 if none)

	AimX, AimY     float64
	PrimaryCD      float64
	SecondaryCD    float64
	SecondaryMaxCD float64
	InvulnTimer    float64
	DashTimer      float64
	DashVX, DashVY float64
	HitFlash       float64

	// Class passives & mechanics
	SpotlightCount int     // Vocalist: nearby enemies count
	GainStacks     int     // Guitarist: consecutive hit stacks (each +8% crit up to 80%)
	ComboCount     int     // Drummer: hit combo count
	ComboTimer     float64 // Drummer: 2.5s decay timer
	SwingDir       int     // Alternates 1 / -1 for melee visual

	// Gear / Upgrade items (count per ItemType)
	Items          map[ItemType]int
	UpgradeChoices []ItemType
	UpgradePicked  bool

	// Internal timers
	DelayedQueue    []DelayedAttack `json:"-"`
	PrevSecondary   bool            `json:"-"`
	PrevSelectClass int             `json:"-"`
	PrevSelectItem  int             `json:"-"`
	PrevReady       bool            `json:"-"`
}

type Enemy struct {
	ID     int
	Type   EnemyType
	X, Y   float64
	VX, VY float64
	Radius float64
	HP     float64
	MaxHP  float64
	Speed  float64
	Damage float64

	// Knockback & Stun physics (Bassist specialty)
	KBVX, KBVY  float64
	KnockedBack bool
	KBSourceID  int
	StunTimer   float64
	WallSlamCD  float64

	// Status effects
	BurnTimer    float64
	BurnTick     float64
	BurnSourceID int
	HitFlash     float64

	// AI state
	StateTimer   float64
	AttackCD     float64
	Charging     bool
	Telegraphing bool
	ChargeDirX   float64
	ChargeDirY   float64
}

type Projectile struct {
	ID          int
	Type        ProjType
	OwnerID     int // Player ID (0 or 1), or -1 for enemy
	X, Y        float64
	VX, VY      float64
	Radius      float64
	Damage      float64
	Knockback   float64
	Life        float64
	MaxLife     float64
	Piercing    bool
	BouncesLeft int
	Returning   bool
	Ignite      bool
	IsCrit      bool
	HitResetCD  float64
	HitEntities map[int]bool `json:"-"`
}

type SlowZone struct {
	ID      int
	X, Y    float64
	Radius  float64
	Life    float64
	MaxLife float64
	IsEnemy bool
}

type Obstacle struct {
	X, Y    float64
	W, H    float64
	Label   string
	LabelEN string
}

func (o Obstacle) GetLabel(lang Lang) string {
	if lang == LangEN && o.LabelEN != "" {
		return o.LabelEN
	}
	return o.Label
}

type FloatingText struct {
	X, Y    float64
	VY      float64
	Text    string
	TextEN  string
	Life    float64
	MaxLife float64
	ColorID int // 0: White, 1: Yellow (Crit), 2: Orange (Wall Slam/Burn), 3: Green (Heal), 4: Cyan (Status/Info), 5: Red (Player Hit)
}

func (ft FloatingText) GetText(lang Lang) string {
	if lang == LangEN && ft.TextEN != "" {
		return ft.TextEN
	}
	return ft.Text
}

type Particle struct {
	X, Y    float64
	VX, VY  float64
	Size    float64
	Life    float64
	MaxLife float64
	ColorID int
}

type VisualEffect struct {
	Type    EffectType
	X, Y    float64
	X2, Y2  float64 // End point for EffectLine
	Radius  float64
	Angle   float64 // Center angle for Cone/Arc
	Spread  float64 // Half-angle spread for Cone/Arc
	Life    float64
	MaxLife float64
	ColorID int
}

type SoundEvent struct {
	Type SoundType
	X, Y float64
}

type WorldSnapshot struct {
	Tick        uint64
	Stage       GameStage
	Wave        int
	EnemiesLeft int
	Synergy     float64 // 0..100
	JamActive   bool
	JamTimer    float64
	ScreenShake float64

	Players       []Player
	Enemies       []Enemy
	Projectiles   []Projectile
	Zones         []SlowZone
	Obstacles     []Obstacle
	FloatingTexts []FloatingText
	Particles     []Particle
	Effects       []VisualEffect
	Sounds        []SoundEvent
}

// Helper math functions
func DistSq(x1, y1, x2, y2 float64) float64 {
	dx := x2 - x1
	dy := y2 - y1
	return dx*dx + dy*dy
}

func Dist(x1, y1, x2, y2 float64) float64 {
	return math.Hypot(x2-x1, y2-y1)
}

func Normalize(x, y float64) (float64, float64) {
	l := math.Hypot(x, y)
	if l < 1e-6 {
		return 1, 0
	}
	return x / l, y / l
}

func Clamp(v, minV, maxV float64) float64 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

// CircleRectResolve resolves collision between a circle (cx, cy, r) and axis-aligned rectangle (rx, ry, rw, rh).
// Returns pushed (cx, cy), normal (nx, ny), and whether collision occurred.
func CircleRectResolve(cx, cy, r, rx, ry, rw, rh float64) (float64, float64, float64, float64, bool) {
	closestX := Clamp(cx, rx, rx+rw)
	closestY := Clamp(cy, ry, ry+rh)
	dx := cx - closestX
	dy := cy - closestY
	distSq := dx*dx + dy*dy
	if distSq >= r*r {
		return cx, cy, 0, 0, false
	}
	dist := math.Sqrt(distSq)
	if dist < 1e-5 {
		left := cx - rx
		right := (rx + rw) - cx
		top := cy - ry
		bottom := (ry + rh) - cy
		minDist := left
		nx, ny := -1.0, 0.0
		if right < minDist {
			minDist = right
			nx, ny = 1.0, 0.0
		}
		if top < minDist {
			minDist = top
			nx, ny = 0.0, -1.0
		}
		if bottom < minDist {
			minDist = bottom
			nx, ny = 0.0, 1.0
		}
		return cx + nx*(minDist+r), cy + ny*(minDist+r), nx, ny, true
	}
	nx := dx / dist
	ny := dy / dist
	overlap := r - dist
	return cx + nx*overlap, cy + ny*overlap, nx, ny, true
}
