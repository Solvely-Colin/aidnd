package game

import (
	"strings"
	"testing"
)

// Within an era, two different rules never share a name, so a player can
// tell from the sheet which one they have.
func TestTermNamesUniquePerEra(t *testing.T) {
	schools := map[string]bool{}
	for _, s := range Spells {
		schools[s.School] = true
	}
	for setting := range eraIndex {
		seen := map[string]string{}
		claim := func(name, key string) {
			if prev, ok := seen[name]; ok && prev != key {
				t.Errorf("%s: %q is both %s and %s", setting, name, prev, key)
			}
			seen[name] = key
		}
		for k := range TermEra {
			if !schools[k] {
				claim(TermName(k, k, setting), k)
			}
		}
		for k, s := range Spells {
			claim(SpellName(s, setting), "spell "+k)
		}
	}
}

func TestReskinPulp(t *testing.T) {
	mi := Reskin(Feats["magic_initiate"].Desc, SettingPulp)
	if strings.Contains(mi, "wizard") || strings.Contains(mi, "cleric") {
		t.Errorf("magic initiate still names D&D classes in pulp: %q", mi)
	}
	if got := Reskin("Lay on Hands: 5 left", SettingPulp); got != "Laying On of Hands: 5 left" {
		t.Errorf("resource name: %q", got)
	}
	if got := Reskin("Channel Divinity: Turn Undead", SettingPulp); got != TermName("turn_undead", "", SettingPulp) {
		t.Errorf("longest name should win: %q", got)
	}
	if got := Reskin("Lay on Hands", SettingFantasy); got != "Lay on Hands" {
		t.Errorf("fantasy changed: %q", got)
	}
	for k, f := range Feats {
		if TermName(k, f.Name, SettingPulp) == f.Name && TermEra[k] == nil {
			t.Errorf("feat %s has no era names", k)
		}
	}
}

func TestEveryFeatureHasEraNames(t *testing.T) {
	for _, k := range ClassKeys() {
		for _, f := range Classes[k].Features {
			if TermEra[f.Key] == nil {
				t.Errorf("%s feature %s has no era names", k, f.Key)
			}
		}
	}
	for _, k := range SpeciesKeys() {
		for _, f := range Species[k].Traits {
			if TermEra[f.Key] == nil {
				t.Errorf("%s trait %s has no era names", k, f.Key)
			}
		}
	}
	for k := range Invocations {
		if TermEra[k] == nil {
			t.Errorf("invocation %s has no era names", k)
		}
	}
}
