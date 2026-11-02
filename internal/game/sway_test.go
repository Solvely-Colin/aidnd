package game

import (
	"strings"
	"testing"
)

func swayRun(f *fixture, ctx *Ctx, c *Character, a Action) Step {
	r := tendRun(f, ctx, c, []Action{a})
	return r.Steps[0]
}

func changes(st Step) string { return strings.Join(st.Changes, "; ") }

// A persuasion check sent as "other" (the way the interpreter sent the tend
// check) now changes the NPC's attitude, once per day.
func TestPersuadeFromOtherCheckWarmsNPC(t *testing.T) {
	f := newFixture(t)
	f.elder.Disposition = 0
	st := swayRun(f, f.ctx(18), f.aria, Action{Kind: ActCheck, Purpose: "other", Skill: "persuasion", Text: "convince Maren to trust us"})
	if !st.OK {
		t.Fatalf("failed: %s", st.Error)
	}
	if f.elder.Disposition != 1 {
		t.Fatalf("disposition = %d, want 1; %s", f.elder.Disposition, changes(st))
	}
	if st.Rolls[0].Target != 15 {
		t.Fatalf("neutral persuade DC = %d, want 15", st.Rolls[0].Target)
	}
	st = swayRun(f, f.ctx(18), f.aria, Action{Kind: ActCheck, Purpose: "persuade", Target: f.elder.ID})
	if st.OK || f.elder.Disposition != 1 {
		t.Fatalf("second try the same day should be refused: ok %v disp %d", st.OK, f.elder.Disposition)
	}
	// another character may try, but talk alone tops out at +2
	swayRun(f, f.ctx(18), f.bram, Action{Kind: ActCheck, Purpose: "persuade", Target: f.elder.ID})
	f.w.Campaign.Scene.Day++
	swayRun(f, f.ctx(18), f.aria, Action{Kind: ActCheck, Purpose: "persuade", Target: f.elder.ID})
	if f.elder.Disposition != MaxTalkedAttitude {
		t.Fatalf("disposition = %d, want cap %d", f.elder.Disposition, MaxTalkedAttitude)
	}
}

func TestDeceiveFailureCools(t *testing.T) {
	f := newFixture(t)
	f.elder.Disposition = 1
	st := swayRun(f, f.ctx(2), f.aria, Action{Kind: ActCheck, Skill: "deception", Text: "lie to Elder Maren about the money"})
	if f.elder.AttitudeTo(f.aria) >= 1 {
		t.Fatalf("a caught lie should cool her: %d; %s", f.elder.AttitudeTo(f.aria), changes(st))
	}
}

func ledgerQuest(f *fixture, it *Item) *Quest {
	q := &Quest{ID: f.w.NewID("quest"), Title: "The Ledger", Status: QuestActive,
		Objectives: []Objective{{ID: f.w.NewID("obj"), Kind: ObjObtainItem, Text: "get the ledger", Target: it.ID}}}
	f.w.Quests = append(f.w.Quests, q)
	return q
}

func TestIntimidateHandsOverQuestItem(t *testing.T) {
	f := newFixture(t)
	f.elder.Disposition = -2
	it := NewItem(f.w, "healing_potion", Owner{Kind: OwnerNPC, ID: f.elder.ID})
	ledgerQuest(f, it)
	// hostile: just talking gets nothing
	tendRun(f, f.ctx(), f.aria, []Action{{Kind: ActTalkTo, Target: f.elder.ID}})
	if it.Owner.Kind != OwnerNPC {
		t.Fatal("a hostile NPC handed the item over for a chat")
	}
	st := swayRun(f, f.ctx(19), f.aria, Action{Kind: ActCheck, Skill: "intimidation", Target: f.elder.ID, Text: "threaten her"})
	if it.Owner.Kind != OwnerCharacter || it.Owner.ID != f.aria.ID {
		t.Fatalf("cowed NPC kept the item: %s %s", changes(st), st.Error)
	}
}

func TestIntimidateFailureLeavesGrudge(t *testing.T) {
	f := newFixture(t)
	swayRun(f, f.ctx(2), f.aria, Action{Kind: ActCheck, Purpose: "intimidate", Target: "Elder Maren"})
	if len(f.elder.TiesTo(f.aria.ID)) == 0 || f.elder.AttitudeTo(f.aria) != 0 {
		t.Fatalf("want a grudge (attitude 1-1=0), got %d ties %v", f.elder.AttitudeTo(f.aria), f.elder.Ties)
	}
}

func TestStealMovesItem(t *testing.T) {
	f := newFixture(t)
	key := NewItem(f.w, "healing_potion", Owner{Kind: OwnerNPC, ID: f.elder.ID})
	st := swayRun(f, f.ctx(18), f.bram, Action{Kind: ActCheck, Purpose: "other", Skill: "sleight_of_hand", Text: "pickpocket Maren"})
	if !st.OK || key.Owner.ID != f.bram.ID {
		t.Fatalf("item not lifted: %s %s", changes(st), st.Error)
	}
	// caught: attitude drops and she holds a grudge
	ring := NewItem(f.w, "healing_potion", Owner{Kind: OwnerNPC, ID: f.elder.ID})
	st = swayRun(f, f.ctx(2), f.aria, Action{Kind: ActCheck, Purpose: "steal", Target: f.elder.ID, Item: ring.ID})
	if ring.Owner.Kind != OwnerNPC || f.elder.AttitudeTo(f.aria) >= 1 {
		t.Fatalf("caught thief kept item or no grudge: %s", changes(st))
	}
}

func TestStealRefusesMerchantStockWithoutName(t *testing.T) {
	f := newFixture(t)
	st := swayRun(f, f.ctx(18), f.bram, Action{Kind: ActCheck, Purpose: "steal", Target: f.smith.ID})
	if st.OK {
		t.Fatalf("stealing an unnamed thing from a merchant's stock should ask what: %s", changes(st))
	}
}

func TestSneakBeatsAmbush(t *testing.T) {
	f := newFixture(t)
	f.goTo(f.wood, 0)
	st := swayRun(f, f.ctx(18), f.bram, Action{Kind: ActCheck, Purpose: "other", Skill: "stealth", Text: "creep through the trees"})
	if !f.bram.Conditions.Has(CondHidden) {
		t.Fatalf("sneak success should hide: %s %s", changes(st), st.Error)
	}
	if st.Rolls[0].Target != 12 {
		t.Fatalf("sneak DC in danger 2 = %d, want 12", st.Rolls[0].Target)
	}
	enc, _, err := StartEncounter(f.w, []EnemySpec{{Name: "Wolf", HP: 5, AC: 10, AttackBonus: 30, Damage: "1d6"}})
	must(t, err)
	var amb Step
	hp := f.aria.HP
	surprise(f.ctx(20, 20, 6), enc, &amb)
	if f.aria.HP != hp || !strings.Contains(changes(amb), "no surprise") {
		t.Fatalf("a sneaking party should not be surprised: %s", changes(amb))
	}
}

func TestSearchInferredFromClues(t *testing.T) {
	f := newFixture(t)
	f.goTo(f.wood, 2)
	st := swayRun(f, f.ctx(16), f.bram, Action{Kind: ActCheck, Purpose: "other", Skill: "perception", DC: 15, Text: "search the clearing for clues"})
	if !strings.Contains(changes(st), "hidden way") {
		t.Fatalf("searching for clues should find the hidden trail: %s", changes(st))
	}
}

func TestCharmPersonWarmsThenFades(t *testing.T) {
	f := newFixture(t)
	f.elder.Disposition = -1
	var st Step
	ctx := f.ctx(1)
	mindSpell(ctx, f.aria, Spells["charm_person"], f.elder, Action{}, &st)
	if f.elder.Disposition != 2 || f.elder.Charm == nil {
		t.Fatalf("charm failed: %d %s", f.elder.Disposition, changes(st))
	}
	f.w.Campaign.Turn += CharmTurns
	ExpireCharms(f.w)
	if f.elder.Disposition != -2 || f.elder.Charm != nil {
		t.Fatalf("after the charm she should be one worse than before: %d", f.elder.Disposition)
	}
}

func TestSuggestionHandsOver(t *testing.T) {
	f := newFixture(t)
	f.elder.Disposition = -3
	it := NewItem(f.w, "healing_potion", Owner{Kind: OwnerNPC, ID: f.elder.ID})
	ledgerQuest(f, it)
	var st Step
	mindSpell(f.ctx(1), f.aria, Spells["suggestion"], f.elder, Action{Text: "give us the ledger"}, &st)
	if it.Owner.ID != f.aria.ID {
		t.Fatalf("suggestion should get the quest item: %s", changes(st))
	}
}

// Review follow-ups: the daily guard covers stealing, speaking or a failed
// repeat sneak gives a sneak away, an omitted ability uses the purpose's,
// and mind spells respect range.
func TestSwayReviewFollowUps(t *testing.T) {
	f := newFixture(t)
	NewItem(f.w, "healing_potion", Owner{Kind: OwnerNPC, ID: f.elder.ID})
	swayRun(f, f.ctx(2), f.bram, Action{Kind: ActCheck, Purpose: "steal", Target: f.elder.ID})
	if st := swayRun(f, f.ctx(20), f.bram, Action{Kind: ActCheck, Purpose: "steal", Target: f.elder.ID}); st.OK {
		t.Fatal("a second theft attempt the same day should be refused")
	}

	swayRun(f, f.ctx(18), f.aria, Action{Kind: ActCheck, Purpose: "sneak"})
	if !f.aria.Conditions.Has(CondHidden) {
		t.Fatal("sneak should hide")
	}
	tendRun(f, f.ctx(), f.aria, []Action{{Kind: ActSay, Text: "Over here!"}})
	if f.aria.Conditions.Has(CondHidden) {
		t.Fatal("shouting should give the sneak away")
	}
	swayRun(f, f.ctx(18), f.aria, Action{Kind: ActCheck, Purpose: "sneak"})
	swayRun(f, f.ctx(1), f.aria, Action{Kind: ActCheck, Purpose: "sneak"})
	if f.aria.Conditions.Has(CondHidden) {
		t.Fatal("a failed sneak should leave them seen")
	}

	st := swayRun(f, f.ctx(15), f.bram, Action{Kind: ActCheck, Purpose: "persuade", Target: f.elder.ID})
	if !strings.Contains(st.Rolls[0].Label, "Persuasion") {
		t.Fatalf("persuade with no ability should roll Persuasion: %s", st.Rolls[0].Label)
	}

	f.elder.ZoneID = f.hub.Zones[2].ID // two zones from the square
	if _, err := spellNPC(f.w, f.aria, Action{Target: f.elder.ID}, "Suggestion", 1); err == nil {
		t.Fatal("suggestion two zones away should be out of range")
	}
}
