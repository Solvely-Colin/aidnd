package game

import (
	"strings"
	"testing"
)

func TestTrade(t *testing.T) {
	f := newFixture(t)
	f.goTo(f.hub, 1)
	f.bram.Gold = 10
	r := Resolve(f.ctx(), f.bram, []Action{{Kind: ActBuy, Target: "Dunna", Item: "chain mail"}})
	if !strings.Contains(stepErr(r, 0), "has only 10") || f.bram.Gold != 10 {
		t.Fatalf("insufficient gold: %q gold %d", stepErr(r, 0), f.bram.Gold)
	}
	if r.TurnEnded {
		t.Fatal("failed buy spent the action")
	}
	f.bram.Gold = 100
	r = Resolve(f.ctx(), f.bram, []Action{{Kind: ActBuy, Target: "Dunna", Item: "Potion of Healing"}})
	if stepErr(r, 0) != "" || f.bram.Gold != 50 || f.w.FindItemOf(OwnerCharacter, f.bram.ID, "healing_potion") == nil {
		t.Fatalf("buy: %q gold %d", stepErr(r, 0), f.bram.Gold)
	}
	f.w.Campaign.Turns.Budget = FullBudget(false)
	r = Resolve(f.ctx(), f.bram, []Action{{Kind: ActSell, Target: "Dunna", Item: "longbow"}})
	if stepErr(r, 0) != "" || f.bram.Gold != 75 || f.bram.Weapon != "" {
		t.Fatalf("sell: %q gold %d weapon %q", stepErr(r, 0), f.bram.Gold, f.bram.Weapon)
	}
	// goodwill discounts
	f.smith.Disposition = 2
	if p := BuyPrice(&Item{Value: 50}, f.smith); p != 40 {
		t.Fatalf("price %d", p)
	}
	// non-merchants don't trade
	if _, err := Buy(f.w, f.bram, "Elder Maren", "anything"); err == nil {
		t.Fatal("elder sold something")
	}
}

func TestMultiStepMessage(t *testing.T) {
	f := newFixture(t)
	e := f.fight(t, 1, 1, wolf)
	// "I look around, draw my dagger and stab the wolf"
	r := Resolve(f.ctx(15, 3), f.bram, []Action{
		{Kind: ActLook},
		{Kind: ActEquip, Item: "handaxe"},
		{Kind: ActAttack, Target: "wolf"},
	})
	for i := range r.Steps {
		if stepErr(r, i) != "" {
			t.Fatalf("step %d: %s", i, stepErr(r, i))
		}
	}
	if r.Steps[0].Slot != SlotFree || r.Steps[1].Slot != SlotFree || r.Steps[2].Slot != SlotAction {
		t.Fatal("slots wrong")
	}
	if !r.TurnEnded || e.HP != 7 || r.Budget.Free != 1 {
		t.Fatalf("ended %v hp %d free %d", r.TurnEnded, e.HP, r.Budget.Free)
	}
}

func TestStepsBeyondBudgetSkippedAndReported(t *testing.T) {
	f := newFixture(t)
	f.fight(t, 0, 3, wolf)
	r := Resolve(f.ctx(), f.aria, []Action{{Kind: ActLook}, {Kind: ActLook}, {Kind: ActLook}, {Kind: ActLook}, {Kind: ActLook}})
	if len(r.Steps) != 3 || len(r.Skipped) != 2 {
		t.Fatalf("steps %d skipped %d", len(r.Steps), len(r.Skipped))
	}
	var reasons []string
	for _, s := range r.Skipped {
		reasons = append(reasons, s.Error)
	}
	joined := strings.Join(reasons, "|")
	if !strings.Contains(joined, "only 4 steps") || !strings.Contains(joined, "no free actions") {
		t.Fatalf("reasons %v", reasons)
	}
	if r.TurnEnded || !r.AllFree {
		t.Fatal("free-only message ended the turn")
	}
}

func TestFreeOnlyMessageKeepsTurnOpen(t *testing.T) {
	f := newFixture(t)
	r := Resolve(f.ctx(), f.aria, []Action{{Kind: ActLook}, {Kind: ActSay, Text: "Morning, all."}})
	if !r.AllFree || r.TurnEnded || r.Budget.Action != 1 || r.Budget.Free != Unlimited {
		t.Fatalf("%+v", r)
	}
	if !strings.Contains(strings.Join(r.Steps[0].Info, "|"), "Oakvale") {
		t.Fatalf("look info %v", r.Steps[0].Info)
	}
}

func TestEndTurnAction(t *testing.T) {
	f := newFixture(t)
	r := Resolve(f.ctx(), f.aria, []Action{{Kind: ActEndTurn}, {Kind: ActLook}})
	if !r.TurnEnded || r.AllFree || len(r.Skipped) != 1 {
		t.Fatalf("%+v", r)
	}
}

func TestMoveThenActionInOneTurn(t *testing.T) {
	f := newFixture(t)
	r := Resolve(f.ctx(100), f.aria, []Action{
		{Kind: ActMove, Target: "Whisperwood"},
		{Kind: ActMove, Target: "Oakvale"}, // second move is paid with the action (dash)
		{Kind: ActLook},
	})
	if r.Steps[1].Slot != SlotDash || !r.TurnEnded || len(r.Skipped) != 0 || len(r.Steps) != 3 {
		t.Fatalf("slots %v %v skipped %d", r.Steps[0].Slot, r.Steps[1].Slot, len(r.Skipped))
	}
}

func TestStepsAfterActionRunWhenBudgetAllows(t *testing.T) {
	f := newFixture(t)
	e := f.fight(t, 1, 1, wolf)
	// "I stab the wolf, then step back to the road" (Station Halcyon play-test)
	back := f.wood.Zones[0].Name
	r := Resolve(f.ctx(15, 3), f.bram, []Action{
		{Kind: ActAttack, Target: "wolf"},
		{Kind: ActReposition, Zone: back},
		{Kind: ActAttack, Target: "wolf"},
	})
	if stepErr(r, 0) != "" || stepErr(r, 1) != "" || e.HP != 7 {
		t.Fatalf("steps %q %q hp %d", stepErr(r, 0), stepErr(r, 1), e.HP)
	}
	if r.Steps[1].Slot != SlotMove || f.bram.ZoneID != f.wood.Zones[0].ID {
		t.Fatalf("step back slot %s zone %s", r.Steps[1].Slot, f.bram.ZoneID)
	}
	if !r.TurnEnded || len(r.Skipped) != 1 || !strings.Contains(r.Skipped[0].Error, "no action left") {
		t.Fatalf("ended %v skipped %+v", r.TurnEnded, r.Skipped)
	}
}

func TestPickUpIsFree(t *testing.T) {
	f := newFixture(t)
	f.goTo(f.hub, 1)
	it := NewItem(f.w, "torch", Owner{Kind: OwnerLocation, ID: f.hub.ID})
	r := Resolve(f.ctx(), f.bram, []Action{{Kind: ActPickUp, Item: it.ID}, {Kind: ActLook}})
	if stepErr(r, 0) != "" || r.Steps[0].Slot != SlotFree || r.TurnEnded || len(r.Steps) != 2 {
		t.Fatalf("pick up: %q slot %s ended %v", stepErr(r, 0), r.Steps[0].Slot, r.TurnEnded)
	}
}

func TestRepositionToOwnZoneIsDropped(t *testing.T) {
	f := newFixture(t)
	f.goTo(f.hub, 1)
	r := Resolve(f.ctx(), f.bram, []Action{{Kind: ActReposition, Zone: f.bram.ZoneID}, {Kind: ActLook}})
	if len(r.Steps) != 1 || r.Steps[0].Action.Kind != ActLook || stepErr(r, 0) != "" {
		t.Fatalf("steps %+v", r.Steps)
	}
}

func TestSeveralBuysFromOneMerchantCostOneAction(t *testing.T) {
	f := newFixture(t)
	f.goTo(f.hub, 1)
	// trades are free outside combat; in a fight the first one costs the action
	_, _, err := StartEncounter(f.w, []EnemySpec{wolf})
	must(t, err)
	f.w.Campaign.Turns.Budget = FullBudget(true)
	f.bram.Gold = 200
	// "I buy a potion and a longsword from Dunna"
	r := Resolve(f.ctx(), f.bram, []Action{
		{Kind: ActBuy, Target: "Dunna", Item: "Potion of Healing"},
		{Kind: ActBuy, Target: "Dunna", Item: "longsword"},
	})
	if stepErr(r, 0) != "" || stepErr(r, 1) != "" || len(r.Skipped) != 0 {
		t.Fatalf("buys %q %q skipped %+v", stepErr(r, 0), stepErr(r, 1), r.Skipped)
	}
	if r.Steps[0].Slot != SlotAction || r.Steps[1].Slot != SlotFree || !r.TurnEnded {
		t.Fatalf("slots %s %s ended %v", r.Steps[0].Slot, r.Steps[1].Slot, r.TurnEnded)
	}
}

func TestUnknownIDsRejected(t *testing.T) {
	f := newFixture(t)
	cases := []struct {
		a    Action
		want string
	}{
		{Action{Kind: ActAttack, Target: "dragon"}, "no enemy"},
		{Action{Kind: ActMove, Target: "the moon"}, "no exit"},
		{Action{Kind: ActTalkTo, Target: "Gandalf"}, "nobody called"},
		{Action{Kind: ActBuy, Target: "Dunna", Item: "vorpal sword"}, "no \"vorpal sword\""},
		{Action{Kind: ActUseItem, Item: "wand of wishes"}, "has no"},
		{Action{Kind: ActReposition, Zone: "Narnia"}, "unknown zone"},
		{Action{Kind: ActTravel, Target: "Atlantis"}, "unknown destination"},
		{Action{Kind: ActAcceptQuest, Target: "quest999"}, "unknown quest"},
		{Action{Kind: "fly"}, "unknown action"},
	}
	for _, c := range cases {
		r := Resolve(f.ctx(), f.aria, []Action{c.a})
		if !strings.Contains(stepErr(r, 0), c.want) {
			t.Errorf("%s: got %q want %q", c.a.Kind, stepErr(r, 0), c.want)
		}
		if r.TurnEnded {
			t.Errorf("%s: invalid action ended the turn", c.a.Kind)
		}
	}
}

func TestBudgetPay(t *testing.T) {
	b := FullBudget(true)
	if s, _ := b.Pay(SlotMove); s != SlotMove {
		t.Fatal("move")
	}
	if s, _ := b.Pay(SlotMove); s != SlotDash || b.Action != 0 {
		t.Fatal("second move should dash")
	}
	if _, err := b.Pay(SlotMove); err == nil {
		t.Fatal("third move allowed")
	}
	if !strings.Contains(b.Left(), "free 3") {
		t.Fatal(b.Left())
	}
}

func TestTalkAndTradeFreeOutsideCombat(t *testing.T) {
	f := newFixture(t)
	f.goTo(f.hub, 1)
	f.bram.Gold = 100
	r := Resolve(f.ctx(), f.bram, []Action{
		{Kind: ActTalkTo, Target: "Dunna"},
		{Kind: ActBuy, Target: "Dunna", Item: "Potion of Healing"},
		{Kind: ActSell, Target: "Dunna", Item: "longbow"},
	})
	for i := range r.Steps {
		if stepErr(r, i) != "" || r.Steps[i].Slot != SlotFree {
			t.Fatalf("step %d: %q slot %v", i, stepErr(r, i), r.Steps[i].Slot)
		}
	}
	if r.TurnEnded {
		t.Fatal("talk/trade outside combat ended the turn")
	}
	if r.AllFree {
		t.Fatal("talk/trade marked minor; it should get full narration")
	}
	f.fight(t, 1, 1, wolf)
	if s := SlotFor(f.w, Action{Kind: ActTalkTo, Target: "Dunna"}); s != SlotAction {
		t.Fatalf("talk in combat slot %v", s)
	}
	if s := SlotFor(f.w, Action{Kind: ActBuy, Target: "Dunna"}); s != SlotAction {
		t.Fatalf("buy in combat slot %v", s)
	}
}
