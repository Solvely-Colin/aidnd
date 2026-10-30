package api

import (
	"net/http"

	"github.com/Solvely-Colin/aidnd/internal/game"
)

// The 5e ruleset as the builder and sheet see it, named for one setting.

type ClassView struct {
	Key         string         `json:"key"`
	Name        string         `json:"name"`
	Rules       string         `json:"rules_name"` // the SRD name
	Desc        string         `json:"desc"`
	HitDie      int            `json:"hit_die"`
	Primary     []game.Ability `json:"primary"`
	Saves       []game.Ability `json:"saves"`
	SkillCount  int            `json:"skill_count"`
	SkillChoice []string       `json:"skill_choice"`
	Caster      string         `json:"caster,omitempty"`
	CastAbility game.Ability   `json:"cast_ability,omitempty"`
	Subclass    string         `json:"subclass"`
	SubclassSRD string         `json:"subclass_rules_name"`
	SubDesc     string         `json:"subclass_desc"`
	Features    []game.Feature `json:"features"`
	Gear        []string       `json:"gear"` // item names in the setting
	Scores      map[string]int `json:"suggested_scores"`
}

type SpeciesView struct {
	Key    string         `json:"key"`
	Name   string         `json:"name"`
	Rules  string         `json:"rules_name"`
	Desc   string         `json:"desc"`
	Traits []game.Feature `json:"traits"`
}

type BackgroundView struct {
	Key       string         `json:"key"`
	Name      string         `json:"name"`
	Rules     string         `json:"rules_name"`
	Desc      string         `json:"desc"`
	Abilities []game.Ability `json:"abilities"`
	Skills    []string       `json:"skills"`
	Feat      string         `json:"feat"`
	FeatList  string         `json:"feat_list,omitempty"`
}

type SpellView struct {
	Key     string    `json:"key"`
	Name    string    `json:"name"`
	Rules   string    `json:"rules_name"`
	Level   int       `json:"level"`
	School  string    `json:"school"`
	Classes []string  `json:"classes"`
	Cast    game.Slot `json:"cast"`
	Range   string    `json:"range"`
	Kind    string    `json:"kind"`
	Area    bool      `json:"area,omitempty"`
	Conc    bool      `json:"conc,omitempty"`
	Targets int       `json:"targets,omitempty"`
	Peace   bool      `json:"peace,omitempty"`
	Desc    string    `json:"desc"`
}

func spellView(s *game.SpellDef, setting string) SpellView {
	return SpellView{Key: s.Key, Name: game.SpellName(s, setting), Rules: s.Name, Level: s.Level, School: game.TermName(s.School, s.School, setting), Classes: s.Classes,
		Cast: s.Cast, Range: s.Range, Kind: s.Kind, Area: s.Area, Conc: s.Conc, Targets: s.Targets, Peace: s.Peace, Desc: game.Reskin(s.Desc, setting)}
}

// rulesetView is every class, species, background, feat and spell, named
// for setting.
func rulesetView(setting string) map[string]any {
	var classes []ClassView
	for _, k := range game.ClassKeys() {
		cd := game.Classes[k]
		v := ClassView{Key: k, Name: game.ClassName(cd, setting), Rules: cd.Name, Desc: cd.Desc, HitDie: cd.HitDie, Primary: cd.Primary, Saves: cd.Saves,
			SkillCount: cd.SkillCount, SkillChoice: cd.SkillChoice, Caster: cd.Caster, CastAbility: cd.CastAbility,
			Subclass: game.SubclassName(cd, setting), SubclassSRD: cd.Subclass.Name, SubDesc: game.Reskin(cd.Subclass.Desc, setting), Features: reskinFeatures(cd.Features, setting), Scores: map[string]int{}}
		for i, a := range game.Abilities {
			v.Scores[string(a)] = cd.Scores[i]
		}
		for _, g := range cd.Gear {
			if d, ok := game.ItemFor(setting, g); ok {
				v.Gear = append(v.Gear, d.Name)
			}
		}
		classes = append(classes, v)
	}
	var species []SpeciesView
	for _, k := range game.SpeciesKeys() {
		sp := game.Species[k]
		species = append(species, SpeciesView{Key: k, Name: game.SpeciesName(sp, setting), Rules: sp.Name, Desc: game.SpeciesDesc(sp, setting), Traits: reskinFeatures(sp.Traits, setting)})
	}
	var bgs []BackgroundView
	for _, k := range game.BackgroundKeys() {
		b := game.Backgrounds[k]
		bgs = append(bgs, BackgroundView{Key: k, Name: game.BackgroundName(b, setting), Rules: b.Name, Desc: b.Desc, Abilities: b.Abilities, Skills: b.Skills, Feat: b.Feat, FeatList: b.FeatList})
	}
	var spells []SpellView
	for _, k := range game.SpellKeys() {
		spells = append(spells, spellView(game.Spells[k], setting))
	}
	var invs []game.Feature
	for _, k := range game.InvocationKeys() {
		invs = append(invs, game.ReskinFeature(game.Invocations[k], setting))
	}
	feats := map[string]game.FeatDef{}
	for k, f := range game.Feats {
		f := *f
		f.Name, f.Desc = game.TermName(k, f.Name, setting), game.Reskin(f.Desc, setting)
		feats[k] = f
	}
	return map[string]any{"setting": setting, "classes": classes, "species": species, "backgrounds": bgs, "feats": feats,
		"invocations": invs, "fighting_styles": game.FightingStyles(), "spells": spells, "skills": game.Skills}
}

// BuildView is the class part of a character sheet.
type BuildView struct {
	ClassKey    string              `json:"class_key"`
	ClassName   string              `json:"class_name"`
	Subclass    string              `json:"subclass,omitempty"`
	Species     string              `json:"species"`
	SpeciesName string              `json:"species_name"`
	Background  string              `json:"background,omitempty"`
	BgName      string              `json:"background_name,omitempty"`
	Saves       map[string]SaveView `json:"saves"`
	Features    []game.Feature      `json:"features"`
	Abilities   []game.FeatureUse   `json:"abilities"`
	Resources   []game.Resource     `json:"resources"`
	Feats       []string            `json:"feats"`
	Picks       map[string][]string `json:"picks,omitempty"`
	Choices     []game.Choice       `json:"pending_choices"`
	Caster      bool                `json:"caster"`
	Slots       []SlotView          `json:"slots,omitempty"`
	SpellDC     int                 `json:"spell_dc,omitempty"`
	SpellAttack int                 `json:"spell_attack,omitempty"`
	Spells      []SpellView         `json:"spells,omitempty"`
	Prepare     *PrepareView        `json:"prepare,omitempty"`
	TempHP      int                 `json:"temp_hp,omitempty"`
	Conc        *game.Concentration `json:"concentration,omitempty"`
	ConcName    string              `json:"concentration_name,omitempty"`
	Rebuild     bool                `json:"made_before_classes,omitempty"`
	Spec        *game.CharacterSpec `json:"spec,omitempty"` // to edit before the story starts
}

type SaveView struct {
	Mod        int  `json:"mod"`
	Proficient bool `json:"proficient"`
}

type SlotView struct {
	Level int `json:"level"`
	Max   int `json:"max"`
	Left  int `json:"left"`
}

// PrepareView is what the character may prepare: counts and the options.
type PrepareView struct {
	Cantrips int      `json:"cantrips"`
	Prepared int      `json:"prepared"`
	Options  []string `json:"options"` // spell keys on the class list they can cast
}

func buildView(w *game.World, c *game.Character) BuildView {
	setting := w.Setting()
	cd := game.ClassOf(c)
	sp := game.Species[c.SpeciesKey()]
	v := BuildView{ClassKey: cd.Key, ClassName: game.ClassName(cd, setting), Species: sp.Key, SpeciesName: game.SpeciesName(sp, setting),
		Saves: map[string]SaveView{}, Features: reskinFeatures(game.FeaturesOf(c), setting), Abilities: reskinUses(game.FeatureUses(w, c), setting),
		Resources: reskinResources(game.Resources(c), setting), Feats: c.FeatList(), Picks: c.Picks, Choices: reskinChoices(game.PendingChoices(c), setting), Caster: c.IsCaster(), TempHP: c.TempHP, Conc: c.Conc,
		Rebuild: c.ClassKey == "", Spec: game.SpecOf(c)}
	if v.Abilities == nil {
		v.Abilities = []game.FeatureUse{}
	}
	if v.Choices == nil {
		v.Choices = []game.Choice{}
	}
	if v.Resources == nil {
		v.Resources = []game.Resource{}
	}
	if c.Level >= 3 {
		v.Subclass = game.SubclassName(cd, setting)
	}
	if b := game.Backgrounds[c.Background]; b != nil {
		v.Background, v.BgName = b.Key, game.BackgroundName(b, setting)
	}
	for _, a := range game.Abilities {
		v.Saves[string(a)] = SaveView{Mod: c.SaveMod(a), Proficient: c.SaveProficient(a)}
	}
	if c.Conc != nil {
		v.ConcName = game.SpellName(game.Spells[c.Conc.Spell], setting)
	}
	if v.Caster {
		mx, left := game.SlotsMax(c), game.SlotsLeft(c)
		for i := range mx {
			if mx[i] > 0 {
				v.Slots = append(v.Slots, SlotView{Level: i + 1, Max: mx[i], Left: left[i]})
			}
		}
		v.SpellDC = game.SpellDC(c)
		for _, p := range game.SpellAttackParts(c) {
			v.SpellAttack += p.Value
		}
		for _, k := range c.SpellList() {
			v.Spells = append(v.Spells, spellView(game.Spells[k], setting))
		}
		if cd.Caster != game.CasterNone {
			v.Prepare = &PrepareView{Cantrips: game.CantripCount(c), Prepared: game.PreparedCount(c), Options: game.ClassSpells(cd.Key, game.MaxSpellLevel(c))}
		}
	}
	return v
}

// handlePreview checks a builder's sheet as it is being made: the final
// scores, HP and AC, the picks it still needs, and what is wrong.
func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Character game.CharacterSpec `json:"character"`
		Fill      bool               `json:"fill"` // complete it with defaults
	}
	if err := decode(r, &body); err != nil {
		s.fail(w, err)
		return
	}
	spec := body.Character
	if game.Classes[spec.ClassKey] == nil && !body.Fill {
		writeError(w, http.StatusBadRequest, "class_key is required")
		return
	}
	if body.Fill {
		spec = game.FillBuild(spec)
	}
	setting := game.SettingFantasy
	if wld, err := s.store.Load(r.Context(), r.PathValue("id")); err == nil {
		setting = wld.Setting()
	}
	scores, problems := game.ValidateBuild(spec)
	for i := range problems {
		problems[i] = game.Reskin(problems[i], setting)
	}
	c := game.PreviewBuild(spec, scores)
	out := map[string]any{"character": spec, "problems": problems, "choices": reskinChoices(game.ChoicesFor(c), setting), "skills": c.SkillList(),
		"max_hp": game.MaxHPFor(c), "scores": c.Scores, "cantrips": game.CantripCount(c), "prepared": game.PreparedCount(c),
		"spell_options": game.ClassSpells(spec.ClassKey, game.MaxSpellLevel(c)), "default_spells": game.DefaultSpells(c)}
	if len(problems) == 0 {
		out["problems"] = []string{}
	}
	writeJSON(w, 200, out)
}

// handleChoose records a level-up pick.
func (s *Server) handleChoose(w http.ResponseWriter, r *http.Request) {
	p, ok := s.needPlayer(w, r)
	if !ok {
		return
	}
	var body struct {
		Key   string   `json:"key"`
		Picks []string `json:"picks"`
	}
	if err := decode(r, &body); err != nil {
		s.fail(w, err)
		return
	}
	wld, c, err := s.gm.Choose(r.Context(), r.PathValue("id"), p, body.Key, body.Picks)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, characterView(wld, c))
}

// handleSpells changes prepared spells.
func (s *Server) handleSpells(w http.ResponseWriter, r *http.Request) {
	p, ok := s.needPlayer(w, r)
	if !ok {
		return
	}
	var body struct {
		Spells []string `json:"spells"`
	}
	if err := decode(r, &body); err != nil {
		s.fail(w, err)
		return
	}
	wld, c, err := s.gm.SetSpells(r.Context(), r.PathValue("id"), p, body.Spells)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, characterView(wld, c))
}

// The engine names rules in SRD words; these put a view in the setting's.

func reskinFeatures(fs []game.Feature, setting string) []game.Feature {
	out := make([]game.Feature, len(fs))
	for i, f := range fs {
		out[i] = game.ReskinFeature(f, setting)
	}
	return out
}

func reskinUses(us []game.FeatureUse, setting string) []game.FeatureUse {
	for i := range us {
		us[i].Name, us[i].Desc, us[i].Why = game.Reskin(us[i].Name, setting), game.Reskin(us[i].Desc, setting), game.Reskin(us[i].Why, setting)
	}
	return us
}

func reskinResources(rs []game.Resource, setting string) []game.Resource {
	for i := range rs {
		rs[i].Name = game.Reskin(rs[i].Name, setting)
	}
	return rs
}

func reskinChoices(cs []game.Choice, setting string) []game.Choice {
	for i := range cs {
		cs[i].Label = game.Reskin(cs[i].Label, setting)
	}
	return cs
}
