package game

import "testing"

func TestInferSetting(t *testing.T) {
	cases := map[string]string{
		"1930s Lovecraftian horror in a fishing town":      SettingPulp,
		"A quiet frontier town where something old stirs.": SettingFantasy,
		"Outlaws ride through the Wild West":               SettingGaslight,
		"London, 1888. A killer stalks the fog.":           SettingGaslight,
		"A derelict starship drifts into port":             SettingFuture,
		"Seattle in 2026, and the servers are haunted":     SettingModern,
		"a cursed valley and a sleeping dragon":            SettingFantasy,
	}
	for premise, want := range cases {
		if got := InferSetting(premise); got != want {
			t.Errorf("%q: got %s want %s", premise, got, want)
		}
	}
}

func TestChooseSettingPremiseWins(t *testing.T) {
	if s := ChooseSetting("future", "a cursed valley"); s != SettingFuture {
		t.Fatalf("authored pick ignored: %s", s)
	}
	if s := ChooseSetting("fantasy", "Arkham, 1931"); s != SettingPulp {
		t.Fatalf("premise era ignored: %s", s)
	}
	if s := ChooseSetting("steampunk wizards", "a quiet town"); s != SettingFantasy {
		t.Fatalf("unknown pick not clamped: %s", s)
	}
	w := &World{Campaign: Campaign{Premise: "1920s speakeasy"}}
	if w.Setting() != SettingPulp {
		t.Fatal("old campaigns should infer from premise")
	}
	w.Campaign.Setting = SettingModern
	if w.Setting() != SettingModern {
		t.Fatal("stored setting should win")
	}
}

// Every fantasy item has an equivalent in every era with identical
// mechanics, so a setting never changes the game's balance.
func TestEveryEraReskinsTheWholeCatalog(t *testing.T) {
	for _, s := range SettingKeys() {
		for key, base := range Catalog {
			if base.Setting != "" {
				continue
			}
			d, ok := ItemFor(s, key)
			if !ok || !availableIn(d, s) {
				t.Errorf("%s: %s has no %s equivalent", s, key, s)
				continue
			}
			if d.Kind != base.Kind || d.Value != base.Value || d.Damage != base.Damage || d.Ability != base.Ability ||
				d.Finesse != base.Finesse || d.Reach != base.Reach || d.AC != base.AC || d.DexCap != base.DexCap || d.Effect != base.Effect {
				t.Errorf("%s: %s changes mechanics: %+v vs %+v", s, d.Key, d, base)
			}
		}
		if got := len(CatalogKeysFor(s)); got != len(CatalogKeysFor(SettingFantasy)) {
			t.Errorf("%s offers %d keys, fantasy %d", s, got, len(CatalogKeysFor(SettingFantasy)))
		}
	}
}

func TestItemForTranslatesBetweenEras(t *testing.T) {
	for _, c := range []struct{ setting, key, want string }{
		{SettingPulp, "longsword", "Machete"},
		{SettingFantasy, "pulp_crossbow", "Light Crossbow"},
		{SettingModern, "gaslight_crossbow", "9mm Pistol"},
		{SettingFuture, "rations", "Rations"},
		{SettingPulp, "pulp_arcane_focus", "Occult Tome"},
	} {
		d, ok := ItemFor(c.setting, c.key)
		if !ok || d.Name != c.want {
			t.Errorf("%s %s: got %q", c.setting, c.key, d.Name)
		}
	}
	if _, ok := ItemFor(SettingPulp, "laser_cannon"); ok {
		t.Fatal("unknown key should stay unknown")
	}
}

func TestEraKitsAreComplete(t *testing.T) {
	for _, s := range SettingKeys() {
		for _, key := range KitKeys() {
			k := KitFor(s, key)
			if k.Key != key || k.Name == "" || len(k.Suggested) != 2 {
				t.Errorf("%s/%s: %+v", s, key, k)
			}
			has := map[string]bool{}
			for _, it := range k.Items {
				if _, ok := Catalog[it]; !ok {
					t.Errorf("%s/%s: unknown item %s", s, key, it)
				}
				has[it] = true
			}
			if !has[k.Weapon] || !has[k.Armor] {
				t.Errorf("%s/%s: weapon or armor not in items", s, key)
			}
		}
	}
}

func TestCharacterGetsGearForTheSetting(t *testing.T) {
	w := &World{Campaign: Campaign{ID: "c1", Premise: "Arkham, 1931. Something stirs beneath Miskatonic.", Status: StatusLobby, MaxPlayers: 4}}
	w.Players = []*Player{{ID: "u1"}}
	c, err := CreateCharacter(w, "u1", CharacterSpec{Name: "Cal Roarke", Kit: "skirmisher",
		Scores: map[string]int{"STR": 8, "DEX": 15, "CON": 14, "INT": 13, "WIS": 12, "CHA": 10}, Proficient: []string{"DEX", "CHA"}})
	must(t, err)
	if c.Class != "Gumshoe" {
		t.Fatalf("class %q", c.Class)
	}
	wpn := w.Item(c.Weapon)
	if wpn == nil || wpn.Name != "Automatic Pistol" || wpn.Damage != "1d8" || wpn.Reach != ReachRanged || wpn.Ability != DEX {
		t.Fatalf("weapon %+v", wpn)
	}
	if a := w.Item(c.Armor); a == nil || a.Name != "Leather Jacket" || a.AC != 11 {
		t.Fatalf("armor %+v", a)
	}
	for _, it := range w.ItemsOf(OwnerCharacter, c.ID) {
		if d := Catalog[it.Key]; !availableIn(d, SettingPulp) {
			t.Errorf("%s does not belong in a pulp game", it.Name)
		}
	}
	// authored loot is clamped too: a longsword in 1931 is a machete
	if it := NewItem(w, "longsword", Owner{Kind: OwnerLocation, ID: "x"}); it.Name != "Machete" {
		t.Fatalf("loot %q", it.Name)
	}
	if it := CustomItem(w, "Potion of Healing", ItemConsumable, "", 0, Owner{Kind: OwnerLocation, ID: "x"}); it.Name != "Morphine Syrette" {
		t.Fatalf("custom %q", it.Name)
	}
}
