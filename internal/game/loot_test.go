package game

import (
	"strings"
	"testing"
)

func TestMagicGearAppliesItsBonus(t *testing.T) {
	f := newFixture(t)
	f.w.Campaign.Setting = SettingPulp
	owner := Owner{Kind: OwnerCharacter, ID: f.aria.ID}
	wpn := NewMagicGear(f.w, "longsword", "weapon_plus2", owner)
	if wpn == nil || !strings.HasPrefix(wpn.Name, "Blessed ") || wpn.Bonus != 2 || wpn.Rarity != RarityRare {
		t.Fatalf("pulp +2 weapon: %+v", wpn)
	}
	before := sumParts(AttackParts(f.aria, wpn))
	f.aria.Weapon = wpn.ID
	if got := sumParts(AttackParts(f.aria, wpn)); got != before {
		t.Fatalf("parts changed with equipping: %d vs %d", got, before)
	}
	plain := NewItem(f.w, "longsword", owner)
	if sumParts(AttackParts(f.aria, wpn))-sumParts(AttackParts(f.aria, plain)) != 2 {
		t.Fatal("+2 to hit")
	}
	ac := ArmorClass(f.w, f.aria)
	armor := NewMagicGear(f.w, "chain_mail", "armor_plus1", owner)
	f.aria.Armor = armor.ID
	if ArmorClass(f.w, f.aria) != ac+1 {
		t.Fatalf("armor +1: %d -> %d", ac, ArmorClass(f.w, f.aria))
	}
	if NewMagicGear(f.w, "healing_potion", "weapon_plus1", owner) != nil {
		t.Fatal("a potion can't be a +1 weapon")
	}
}

func TestWondrousItemsAndAttunement(t *testing.T) {
	f := newFixture(t)
	owner := Owner{Kind: OwnerCharacter, ID: f.bram.ID}
	ac, save, con := ArmorClass(f.w, f.bram), f.bram.SaveMod(WIS), f.bram.Mod(CON)
	NewWondrous(f.w, "cloak_of_protection", owner)
	NewWondrous(f.w, "amulet_of_health", owner)
	f.w.RefreshMagic()
	if ArmorClass(f.w, f.bram) != ac+1 || f.bram.SaveMod(WIS) != save+1 || f.bram.Mod(CON) != 4 || con == 4 {
		t.Fatalf("AC %d->%d save %d->%d con %d->%d", ac, ArmorClass(f.w, f.bram), save, f.bram.SaveMod(WIS), con, f.bram.Mod(CON))
	}
	// only three work at once
	NewWondrous(f.w, "gauntlets_of_ogre_power", owner)
	ring := NewWondrous(f.w, "ring_of_protection", owner)
	f.w.RefreshMagic()
	if f.bram.Attuned(ring.ID) || ArmorClass(f.w, f.bram) != ac+1 {
		t.Fatal("a fourth wondrous item should not work")
	}
	if f.bram.Mod(STR) != 4 {
		t.Fatal("gauntlets set STR 19")
	}
}

func TestBossLootIsSharedAndDropped(t *testing.T) {
	f := newFixture(t)
	boss, rat := f.bossFight(t)
	gold := f.aria.Gold + f.bram.Gold
	rat.HP = 0
	boss.HP = 0
	changes := EndFight(f.w)
	joined := strings.Join(changes, "|")
	if !strings.Contains(joined, "Loot:") || f.aria.Gold+f.bram.Gold <= gold {
		t.Fatalf("no gold: %v", changes)
	}
	var magic *Item
	for _, it := range f.w.ItemsOf(OwnerLocation, f.wood.ID) {
		if it.Magic() {
			magic = it
		}
	}
	if magic == nil || !strings.Contains(joined, "Left on the ground") {
		t.Fatalf("a boss always leaves a magic item: %v", changes)
	}
}
