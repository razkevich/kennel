// Package blocks is the catalog of building blocks for shaping rooms: shapes,
// floors, walls, decor, and purpose blocks that give a room behavior.
package blocks

// Shape sets a room's minimum footprint on the map, in pup slots.
type Shape struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Icon  string `json:"icon"`
	Cols  int    `json:"cols"`
	Rows  int    `json:"rows"`
	Blurb string `json:"blurb"`
}

var Shapes = []Shape{
	{"closet", "Closet", "🚪", 2, 1, "tiny, for one or two pups"},
	{"office", "Office", "🏢", 4, 2, "the default"},
	{"hall", "Hall", "🏟", 6, 2, "long and wide"},
	{"yard", "Yard", "🌳", 6, 3, "room to roam"},
	{"tower", "Tower", "🗼", 2, 4, "tall and narrow"},
}

type Swatch struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// TUI is the floor/border glyph used in the terminal.
	TUI string `json:"tui"`
}

var Floors = []Swatch{
	{"wood", "Wood", " "}, {"grass", "Grass", "⠂"}, {"tiles", "Tiles", "░"}, {"carpet", "Carpet", "·"},
	{"water", "Water", "~"}, {"stone", "Stone", "∴"}, {"metal", "Metal", "▪"}, {"rug", "Rug", "▒"}, {"checker", "Checker", "⁘"},
}

var Walls = []Swatch{
	{"wood", "Wood panels", "rounded"}, {"brick", "Brick", "thick"}, {"gold", "Gold trim", "double"},
	{"plain", "Plain", "normal"}, {"fence", "Picket fence", "hidden"},
}

// FloorGlyph maps a floor key to the TUI pattern char; ok is false for unknown keys.
func FloorGlyph(key string) (string, bool) {
	for _, f := range Floors {
		if f.Key == key {
			return f.TUI, true
		}
	}
	return "", false
}

func WallBorder(key string) (string, bool) {
	for _, w := range Walls {
		if w.Key == key {
			return w.TUI, true
		}
	}
	return "", false
}

// Decor is cosmetic furniture.
type Decor struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Icon string `json:"icon"`
}

var DecorItems = []Decor{
	{"plant", "Plant", "🪴"}, {"plushie", "Plushie", "🧸"}, {"desk", "Desk", "🖥"}, {"books", "Bookshelf", "📚"},
	{"coffee", "Coffee machine", "☕"}, {"arcade", "Arcade", "👾"}, {"trophy", "Trophy", "🏆"}, {"basket", "Dog basket", "🧺"},
	{"bone", "Bone pile", "🦴"}, {"lamp", "Lamp", "💡"}, {"easel", "Easel", "🎨"}, {"whiteboard", "Whiteboard", "📋"},
	{"fire", "Fire extinguisher", "🧯"}, {"cactus", "Cactus", "🌵"}, {"ball", "Tennis ball", "🎾"}, {"tree", "Tree", "🌳"},
	{"pond", "Pond", "🐟"}, {"bench", "Bench", "🪑"}, {"piano", "Piano", "🎹"}, {"globe", "Globe", "🌍"},
}

// Field is one setting of a purpose block.
type Field struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder"`
}

// Kind is a purpose block: something placed in a room that does something.
type Kind struct {
	Key    string  `json:"key"`
	Name   string  `json:"name"`
	Icon   string  `json:"icon"`
	Blurb  string  `json:"blurb"`
	Fields []Field `json:"fields"`
}

const (
	Door   = "door"
	Magnet = "magnet"
	Bell   = "bell"
	Portal = "portal"
	Board  = "board"
	Chute  = "chute"
)

var Kinds = []Kind{
	{Door, "Spawn door", "🚪", "start a new pup here with preset folder, name, first prompt and model",
		[]Field{{"cwd", "folder", "~/code/infra"}, {"name", "name", "ONCALL"}, {"prompt", "first prompt", "check the alerts channel and summarize"}, {"model", "model", "opus"}}},
	{Magnet, "Magnet", "🧲", "adopts pups into this kennel by folder or by name/branch pattern",
		[]Field{{"cwd", "folder prefix", "~/code/infra"}, {"match", "regex on title/branch", "^INF-|terraform"}}},
	{Bell, "Bell", "🔔", "desktop notification when a pup in this room needs you or finishes",
		[]Field{{"when", "ring on", "waiting,done"}}},
	{Portal, "Portal", "🌀", "pups from this room open in a herdr workspace of this name",
		[]Field{{"workspace", "herdr workspace", "infra"}}},
	{Board, "Pin board", "📌", "notes and links pinned to the room",
		[]Field{{"text", "note", "runbook: docs/oncall.md · escalate to #infra"}}},
	{Chute, "Archive chute", "🧹", "pups asleep in this room for N days get archived (kennel metadata only)",
		[]Field{{"days", "days asleep", "14"}}},
}

func KindByKey(k string) *Kind {
	for i := range Kinds {
		if Kinds[i].Key == k {
			return &Kinds[i]
		}
	}
	return nil
}

func ShapeByKey(k string) *Shape {
	for i := range Shapes {
		if Shapes[i].Key == k {
			return &Shapes[i]
		}
	}
	return nil
}

// Template is a ready-made room: a style plus blocks.
type Template struct {
	Key    string                       `json:"key"`
	Name   string                       `json:"name"`
	Icon   string                       `json:"icon"`
	Shape  string                       `json:"shape"`
	Floor  string                       `json:"floor"`
	Wall   string                       `json:"wall"`
	Decor  []string                     `json:"decor"`
	Blocks map[string]map[string]string `json:"blocks"`
}

var Templates = []Template{
	{"oncall", "On-call room", "🚨", "office", "tiles", "brick", []string{"fire", "coffee", "whiteboard"}, map[string]map[string]string{Door: {"name": "ONCALL"}, Bell: {"when": "waiting"}}},
	{"release", "Release train", "🚂", "hall", "metal", "plain", []string{"desk", "trophy", "plant"}, map[string]map[string]string{Bell: {"when": "waiting,done"}, Board: {"text": "release checklist: tests green · changelog · tag"}}},
	{"lab", "Research lab", "🧪", "office", "checker", "gold", []string{"books", "globe", "lamp"}, map[string]map[string]string{Door: {"name": "SPIKE"}}},
	{"lounge", "Lounge", "🎮", "yard", "grass", "fence", []string{"plushie", "arcade", "pond", "tree"}, map[string]map[string]string{Chute: {"days": "14"}}},
	{"corner", "One-on-one corner", "☕", "closet", "carpet", "wood", []string{"coffee", "plant"}, map[string]map[string]string{Board: {"text": "agenda"}}},
}
