package game

import "fmt"

// BuyPrice is the catalog value adjusted by disposition: 10% off per point
// of goodwill, 10% more per point of dislike. Minimum 1.
func BuyPrice(it *Item, n *NPC) int {
	p := it.Value * (10 - n.Disposition) / 10
	if p < 1 {
		p = 1
	}
	return p
}

// SellPrice is half value, at least 1 for anything with value.
func SellPrice(it *Item) int {
	if it.Value <= 0 {
		return 0
	}
	return max(1, it.Value/2)
}

func merchantHere(w *World, npcRef string) (*NPC, error) {
	n := w.NPC(npcRef)
	if n == nil || !n.Alive || n.LocationID != w.Campaign.LocationID {
		return nil, fmt.Errorf("no merchant %q here", npcRef)
	}
	if !n.Merchant {
		return nil, fmt.Errorf("%s does not trade", n.Name)
	}
	if n.Disposition <= -3 {
		return nil, fmt.Errorf("%s refuses to deal with you", n.Name)
	}
	return n, nil
}

// Buy moves an item from a merchant's stock to the character for gold.
// Quest reward items are not for sale.
func Buy(w *World, c *Character, npcRef, itemRef string) (string, error) {
	n, err := merchantHere(w, npcRef)
	if err != nil {
		return "", err
	}
	it := w.FindItemOf(OwnerNPC, n.ID, itemRef)
	if it == nil || isReward(w, it.ID) || it.Kind == ItemQuest {
		return "", fmt.Errorf("%s has no %q for sale", n.Name, itemRef)
	}
	price := BuyPrice(it, n)
	if c.Gold < price {
		return "", fmt.Errorf("%s costs %d gold; %s has only %d", it.Name, price, c.Name, c.Gold)
	}
	c.Gold -= price
	it.Owner = Owner{Kind: OwnerCharacter, ID: c.ID}
	n.Remember(fmt.Sprintf("sold %s to %s", it.Name, c.Name))
	return fmt.Sprintf("%s buys %s from %s for %d gold (%d left)", c.Name, it.Name, n.Name, price, c.Gold), nil
}

// Sell moves an item from the character to a merchant for gold.
func Sell(w *World, c *Character, npcRef, itemRef string) (string, error) {
	n, err := merchantHere(w, npcRef)
	if err != nil {
		return "", err
	}
	it := w.FindItemOf(OwnerCharacter, c.ID, itemRef)
	if it == nil {
		return "", fmt.Errorf("%s has no %q", c.Name, itemRef)
	}
	if it.Kind == ItemQuest {
		return "", fmt.Errorf("%s won't buy %s", n.Name, it.Name)
	}
	price := SellPrice(it)
	unequip(c, it.ID)
	c.Gold += price
	it.Owner = Owner{Kind: OwnerNPC, ID: n.ID}
	n.Remember(fmt.Sprintf("bought %s from %s", it.Name, c.Name))
	return fmt.Sprintf("%s sells %s to %s for %d gold (%d now)", c.Name, it.Name, n.Name, price, c.Gold), nil
}

func isReward(w *World, itemID string) bool {
	for _, q := range w.Quests {
		if q.Status == QuestTurnedIn {
			continue
		}
		for _, id := range q.RewardItems {
			if id == itemID {
				return true
			}
		}
	}
	return false
}

func unequip(c *Character, itemID string) {
	if c.Weapon == itemID {
		c.Weapon = ""
	}
	if c.Armor == itemID {
		c.Armor = ""
	}
}

// PickUp moves an item from the ground to the character.
func PickUp(w *World, c *Character, itemRef string) (string, error) {
	it := w.FindItemOf(OwnerLocation, w.Campaign.LocationID, itemRef)
	if it == nil {
		return "", fmt.Errorf("there is no %q here", itemRef)
	}
	it.Owner = Owner{Kind: OwnerCharacter, ID: c.ID}
	return fmt.Sprintf("%s picks up %s", c.Name, it.Name), nil
}

// Drop moves an item from the character to the ground.
func Drop(w *World, c *Character, itemRef string) (string, error) {
	it := w.FindItemOf(OwnerCharacter, c.ID, itemRef)
	if it == nil {
		return "", fmt.Errorf("%s has no %q", c.Name, itemRef)
	}
	unequip(c, it.ID)
	it.Owner = Owner{Kind: OwnerLocation, ID: w.Campaign.LocationID}
	return fmt.Sprintf("%s drops %s", c.Name, it.Name), nil
}

// Give hands an item to a party member or an NPC present.
func Give(w *World, c *Character, itemRef, toRef string) (string, error) {
	it := w.FindItemOf(OwnerCharacter, c.ID, itemRef)
	if it == nil {
		return "", fmt.Errorf("%s has no %q", c.Name, itemRef)
	}
	if to := w.Character(toRef); to != nil {
		if to.ID == c.ID {
			return "", fmt.Errorf("you already hold %s", it.Name)
		}
		unequip(c, it.ID)
		it.Owner = Owner{Kind: OwnerCharacter, ID: to.ID}
		return fmt.Sprintf("%s gives %s to %s", c.Name, it.Name, to.Name), nil
	}
	n := w.NPC(toRef)
	if n == nil || !n.Alive || n.LocationID != w.Campaign.LocationID {
		return "", fmt.Errorf("nobody called %q is here", toRef)
	}
	unequip(c, it.ID)
	it.Owner = Owner{Kind: OwnerNPC, ID: n.ID}
	n.Remember(fmt.Sprintf("%s gave me %s", c.Name, it.Name))
	return fmt.Sprintf("%s gives %s to %s", c.Name, it.Name, n.Name), nil
}

// Equip readies a weapon, armor or shield the character owns.
func Equip(w *World, c *Character, itemRef string) (string, error) {
	it := w.FindItemOf(OwnerCharacter, c.ID, itemRef)
	if it == nil {
		return "", fmt.Errorf("%s has no %q", c.Name, itemRef)
	}
	switch it.Kind {
	case ItemWeapon:
		c.Weapon = it.ID
		return fmt.Sprintf("%s readies %s", c.Name, it.Name), nil
	case ItemArmor:
		c.Armor = it.ID
		return fmt.Sprintf("%s dons %s (AC %d)", c.Name, it.Name, ArmorClass(w, c)), nil
	case ItemShield:
		return fmt.Sprintf("%s straps on %s", c.Name, it.Name), nil
	}
	return "", fmt.Errorf("%s can't be equipped", it.Name)
}
