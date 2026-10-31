package game

import "testing"

func TestNewcomerJoinsTheBiggestParty(t *testing.T) {
	w := &World{Campaign: Campaign{LocationID: "l1"}}
	w.Characters = []*Character{{ID: "c1", Name: "A", HP: 5, XP: 300}, {ID: "c2", Name: "B", HP: 5, XP: 900}, {ID: "c3", Name: "C", HP: 5, XP: 600}}
	w.Campaign.Turns.Order = []string{"c1", "c2", "c3"}
	g, err := SplitOff(w, []string{"c2", "c3"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := JoinGroupFor(w); got != g.ID {
		t.Fatalf("joined %s, want the pair %s", got, g.ID)
	}
	if xp := JoinXP(w, g.ID); xp != 750 {
		t.Fatalf("xp %d", xp)
	}
	// a party that is all down loses to one on its feet
	w.Characters[1].HP, w.Characters[2].HP = 0, 0
	if got := JoinGroupFor(w); got != MainGroup {
		t.Fatalf("joined %s, want main", got)
	}
}
