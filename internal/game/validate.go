package game

import (
	"fmt"
	"sort"
	"strings"
)

// Problems is a list of validation errors that the API returns as
// {"error": ..., "problems": [...]}.
type Problems []string

func (p Problems) Error() string { return "validation failed: " + strings.Join(p, "; ") }

// Err returns nil when there are no problems.
func (p Problems) Err() error {
	if len(p) == 0 {
		return nil
	}
	return p
}

// CharacterSpec is a submitted (or AI-drafted) sheet before validation.
type CharacterSpec struct {
	Name       string         `json:"name"`
	Concept    string         `json:"concept,omitempty"`
	Class      string         `json:"class,omitempty"`
	Kit        string         `json:"kit"`
	Backstory  string         `json:"backstory,omitempty"`
	Pronouns   string         `json:"pronouns,omitempty"` // "she/her"; empty means they/them
	Scores     map[string]int `json:"scores"`
	Proficient []string       `json:"proficient"`
}

// ValidateCharacter checks the standard array, two proficiencies and kit.
func ValidateCharacter(spec CharacterSpec) (Scores, []Ability, Problems) {
	var p Problems
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		p = append(p, "name is required")
	} else if len(name) > 40 {
		p = append(p, "name must be at most 40 characters")
	}
	if _, ok := Kits[spec.Kit]; !ok {
		p = append(p, fmt.Sprintf("kit must be one of %s", strings.Join(KitKeys(), ", ")))
	}
	scores := Scores{}
	for k, v := range spec.Scores {
		a, ok := ParseAbility(k)
		if !ok {
			p = append(p, fmt.Sprintf("unknown ability %q", k))
			continue
		}
		if _, dup := scores[a]; dup {
			p = append(p, fmt.Sprintf("ability %s given twice", a))
		}
		scores[a] = v
	}
	var got []int
	for _, a := range Abilities {
		v, ok := scores[a]
		if !ok {
			p = append(p, fmt.Sprintf("missing score for %s", a))
			continue
		}
		got = append(got, v)
	}
	if len(got) == 6 {
		sorted := append([]int(nil), got...)
		sort.Sort(sort.Reverse(sort.IntSlice(sorted)))
		for i, v := range StandardArray {
			if sorted[i] != v {
				p = append(p, "scores must use the standard array 15, 14, 13, 12, 10, 8, each exactly once")
				break
			}
		}
	}
	var prof []Ability
	seen := map[Ability]bool{}
	for _, s := range spec.Proficient {
		a, ok := ParseAbility(s)
		if !ok {
			p = append(p, fmt.Sprintf("unknown proficiency %q", s))
			continue
		}
		if !seen[a] {
			seen[a] = true
			prof = append(prof, a)
		}
	}
	if len(prof) != 2 {
		p = append(p, "pick exactly 2 proficient abilities")
	}
	return scores, prof, p
}

// CreateCharacter validates spec and adds the character with its kit.
func CreateCharacter(w *World, playerID string, spec CharacterSpec) (*Character, error) {
	scores, prof, p := ValidateCharacter(spec)
	if len(p) > 0 {
		return nil, p
	}
	kit := Kits[spec.Kit]
	class := strings.TrimSpace(spec.Class)
	if class == "" {
		class = kit.Name
	}
	c := &Character{
		ID: w.NewID("char"), PlayerID: playerID, Name: strings.TrimSpace(spec.Name),
		Concept: trimTo(spec.Concept, 200), Class: trimTo(class, 40), Kit: kit.Key,
		Backstory: trimTo(spec.Backstory, 600), Pronouns: NormPronouns(spec.Pronouns), Scores: scores, Proficient: prof,
		Level: 1, Gold: kit.Gold,
	}
	c.MaxHP = StartingHP(scores)
	c.HP = c.MaxHP
	owner := Owner{Kind: OwnerCharacter, ID: c.ID}
	for _, key := range kit.Items {
		it := NewItem(w, key, owner)
		if key == kit.Weapon && c.Weapon == "" {
			c.Weapon = it.ID
		}
		if key == kit.Armor && c.Armor == "" {
			c.Armor = it.ID
		}
	}
	w.Characters = append(w.Characters, c)
	if pl := w.Player(playerID); pl != nil {
		pl.CharacterID = c.ID
	}
	return c, nil
}

func trimTo(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}

// RewardCaps bounds what an authored quest may promise.
type RewardCaps struct {
	Gold, XP, Items, ItemValue int
}

// RewardCapsFor scales caps by danger and party level.
func RewardCapsFor(danger, partyLevel int, main bool) RewardCaps {
	d := clamp(danger, 1, 5)
	l := clamp(partyLevel, 1, MaxLevel)
	c := RewardCaps{Gold: 30*d + 20*l, XP: 100 + 25*d, Items: 2, ItemValue: 60 * (d + l)}
	if main {
		c.Gold *= 3
		c.XP *= 2
		c.Items = 3
	}
	return c
}

// QuestSpec is an authored quest offer.
type QuestSpec struct {
	Title       string           `json:"title"`
	Summary     string           `json:"summary"`
	GiverID     string           `json:"giver_id"`
	Objectives  []ObjectiveSpec  `json:"objectives"`
	RewardGold  int              `json:"reward_gold"`
	RewardXP    int              `json:"reward_xp"`
	RewardItems []RewardItemSpec `json:"reward_items,omitempty"`
}

// ObjectiveSpec is an authored objective; Target may be an id or a name.
type ObjectiveSpec struct {
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	Target    string `json:"target,omitempty"`
	DeliverTo string `json:"deliver_to,omitempty"`
}

// RewardItemSpec is a reward: a catalog key or a custom item.
type RewardItemSpec struct {
	Key   string `json:"key,omitempty"`
	Name  string `json:"name,omitempty"`
	Kind  string `json:"kind,omitempty"`
	Value int    `json:"value,omitempty"`
}

// ValidateQuest resolves targets to ids and enforces reward caps. It
// rejects oversized rewards rather than clamping them, so the author
// gets the error back for a retry.
func ValidateQuest(w *World, spec *QuestSpec, main bool) Problems {
	var p Problems
	if strings.TrimSpace(spec.Title) == "" {
		p = append(p, "title is required")
	}
	if !main {
		if n := w.NPC(spec.GiverID); n == nil || !n.Alive {
			p = append(p, fmt.Sprintf("unknown giver %q", spec.GiverID))
		} else {
			spec.GiverID = n.ID
		}
	}
	if len(spec.Objectives) == 0 || len(spec.Objectives) > 6 {
		p = append(p, "a quest needs 1 to 6 objectives")
	}
	for i := range spec.Objectives {
		o := &spec.Objectives[i]
		if err := resolveObjective(w, o); err != "" {
			p = append(p, fmt.Sprintf("objective %d: %s", i+1, err))
		}
	}
	danger := 1
	if l := w.Here(); l != nil {
		danger = l.Danger
	}
	caps := RewardCapsFor(danger, w.PartyLevel(), main)
	if spec.RewardGold < 0 || spec.RewardGold > caps.Gold {
		p = append(p, fmt.Sprintf("reward_gold must be 0..%d", caps.Gold))
	}
	if spec.RewardXP < 0 || spec.RewardXP > caps.XP {
		p = append(p, fmt.Sprintf("reward_xp must be 0..%d", caps.XP))
	}
	if len(spec.RewardItems) > caps.Items {
		p = append(p, fmt.Sprintf("at most %d reward items", caps.Items))
	}
	for _, ri := range spec.RewardItems {
		v := ri.Value
		if d, ok := LookupDef(ri.Key); ok {
			v = d.Value
		} else if d, ok := LookupDef(ri.Name); ok {
			v = d.Value
		} else if strings.TrimSpace(ri.Name) == "" {
			p = append(p, fmt.Sprintf("unknown reward item %q", ri.Key))
			continue
		}
		if v > caps.ItemValue {
			p = append(p, fmt.Sprintf("reward item %q is worth %d, max %d", ri.Name+ri.Key, v, caps.ItemValue))
		}
	}
	return p
}

func resolveObjective(w *World, o *ObjectiveSpec) string {
	switch o.Kind {
	case ObjReachLocation:
		l := w.Location(o.Target)
		if l == nil {
			return fmt.Sprintf("unknown location %q", o.Target)
		}
		o.Target = l.ID
	case ObjTalkToNPC:
		n := w.NPC(o.Target)
		if n == nil {
			return fmt.Sprintf("unknown npc %q", o.Target)
		}
		o.Target = n.ID
	case ObjDeliverItem:
		n := w.NPC(o.DeliverTo)
		if n == nil {
			return fmt.Sprintf("unknown deliver_to npc %q", o.DeliverTo)
		}
		o.DeliverTo = n.ID
		if strings.TrimSpace(o.Target) == "" {
			return "deliver_item needs an item target"
		}
	case ObjDefeatEnemy, ObjObtainItem:
		if strings.TrimSpace(o.Target) == "" {
			return o.Kind + " needs a target"
		}
	case ObjFreeform:
	default:
		return fmt.Sprintf("unknown kind %q", o.Kind)
	}
	if strings.TrimSpace(o.Text) == "" {
		return "text is required"
	}
	return ""
}

// AddQuest validates spec and stores it as offered. Reward items are
// created in the giver's possession.
func AddQuest(w *World, spec QuestSpec, main bool) (*Quest, error) {
	if p := ValidateQuest(w, &spec, main); len(p) > 0 {
		return nil, p
	}
	q := &Quest{ID: w.NewID("quest"), Title: trimTo(spec.Title, 80), Summary: trimTo(spec.Summary, 400),
		GiverID: spec.GiverID, IsMain: main, Status: QuestOffered,
		RewardGold: spec.RewardGold, RewardXP: spec.RewardXP}
	if main {
		q.Status = QuestActive
	}
	for _, o := range spec.Objectives {
		q.Objectives = append(q.Objectives, Objective{ID: w.NewID("obj"), Kind: o.Kind, Text: trimTo(o.Text, 200), Target: o.Target, DeliverTo: o.DeliverTo})
	}
	fixQuestItems(w, q)
	if spec.GiverID != "" {
		owner := Owner{Kind: OwnerNPC, ID: spec.GiverID}
		for _, ri := range spec.RewardItems {
			var it *Item
			if _, ok := Catalog[ri.Key]; ok {
				it = NewItem(w, ri.Key, owner)
			} else {
				it = CustomItem(w, ri.Name, ri.Kind, "", ri.Value, owner)
			}
			q.RewardItems = append(q.RewardItems, it.ID)
		}
	}
	w.Quests = append(w.Quests, q)
	return q, nil
}

// fixQuestItems keeps objectives completable. Authors often name the
// recipient as a deliver_item target, so that becomes the item fetched
// earlier in the quest. Every item target that exists nowhere in the world
// is created as a quest item at the place the quest points to, and a foe
// that is not an NPC gets a place where it waits (see QuestFoes).
func fixQuestItems(w *World, q *Quest) {
	lastItem := ""
	for i := range q.Objectives {
		o := &q.Objectives[i]
		switch o.Kind {
		case ObjObtainItem:
			lastItem = o.Target
		case ObjDeliverItem:
			if n := w.NPC(o.Target); (n != nil || NameMatch(o.Target, o.DeliverTo)) && lastItem != "" {
				o.Target = lastItem
			}
		}
	}
	for i := range q.Objectives {
		o := &q.Objectives[i]
		if o.Kind == ObjDefeatEnemy && w.NPC(o.Target) == nil && o.Where == "" {
			o.Where = questItemPlace(w, q, i).ID
		}
		if (o.Kind != ObjObtainItem && o.Kind != ObjDeliverItem) || itemExists(w, o.Target) {
			continue
		}
		owner := Owner{Kind: OwnerLocation, ID: questItemPlace(w, q, i).ID}
		// "get the salt from Brother Ansel": the named NPC holds it
		for _, n := range w.NPCs {
			if n.Alive && n.ID != w.Campaign.AntagonistID && NameContains(o.Text, n.Name) {
				owner = Owner{Kind: OwnerNPC, ID: n.ID}
				break
			}
		}
		CustomItem(w, o.Target, ItemQuest, "Sought for the quest \""+q.Title+"\".", 0, owner)
	}
}

func itemExists(w *World, ref string) bool {
	for _, it := range w.Items {
		if it.ID == ref || NameMatch(it.Name, ref) || NameContains(it.Name, ref) {
			return true
		}
	}
	return false
}

// questItemPlace picks where a missing quest item lies: a place named in the
// objective or summary, else the last reach_location target before it, else
// an unvisited place away from the hub. Never the hub or the lair.
func questItemPlace(w *World, q *Quest, idx int) *Location {
	ok := func(l *Location) bool { return l != nil && !l.IsHub && !l.IsLair }
	for _, text := range []string{q.Objectives[idx].Text, q.Summary} {
		for _, l := range w.Locations {
			if ok(l) && NameContains(text, l.Name) {
				return l
			}
		}
	}
	for i := idx - 1; i >= 0; i-- {
		if o := q.Objectives[i]; o.Kind == ObjReachLocation {
			if l := w.Location(o.Target); ok(l) {
				return l
			}
		}
	}
	var fallback *Location
	for _, l := range w.Locations {
		if ok(l) {
			if !l.Visited {
				return l
			}
			if fallback == nil {
				fallback = l
			}
		}
	}
	if fallback != nil {
		return fallback
	}
	return w.Location(w.Campaign.LocationID)
}

// NormPronouns tidies a pronoun string ("She / Her" -> "she/her"), keeps it
// short, and defaults to they/them.
func NormPronouns(p string) string {
	var parts []string
	for _, f := range strings.FieldsFunc(strings.ToLower(p), func(r rune) bool { return r == '/' || r == ',' || r == ' ' }) {
		parts = append(parts, f)
	}
	out := trimTo(strings.Join(parts, "/"), 20)
	if out == "" {
		return "they/them"
	}
	return out
}
