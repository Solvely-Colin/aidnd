package dice

import "testing"

func TestParse(t *testing.T) {
	cases := map[string]Expr{
		"2d6+3":  {2, 6, 3},
		"d8":     {1, 8, 0},
		"1d4-1":  {1, 4, -1},
		" 3D10 ": {3, 10, 0},
		"5":      {0, 0, 5},
	}
	for in, want := range cases {
		got, err := Parse(in)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "xd6", "2d", "2d1", "d6+x", "0d6", "500d6"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) accepted", bad)
		}
	}
	if s := (Expr{2, 6, -1}).String(); s != "2d6-1" {
		t.Errorf("String = %q", s)
	}
}

func TestRollDeterministic(t *testing.T) {
	a, b := New(42), New(42)
	for i := 0; i < 50; i++ {
		x, y := a.Roll(MustParse("3d6+2"), false), b.Roll(MustParse("3d6+2"), false)
		if x.Total != y.Total {
			t.Fatal("same seed diverged")
		}
		if x.Total < 5 || x.Total > 20 {
			t.Fatalf("total %d out of range", x.Total)
		}
	}
}

func TestRollCritDoublesDice(t *testing.T) {
	r := FromSource(NewScripted(3, 4, 5, 6))
	res := r.Roll(MustParse("2d6+1"), true)
	if len(res.Dice) != 4 || res.Total != 19 {
		t.Fatalf("crit roll = %+v", res)
	}
}

func TestRollNeverNegative(t *testing.T) {
	r := FromSource(NewScripted(1))
	if res := r.Roll(MustParse("1d4-3"), false); res.Total != 0 {
		t.Fatalf("total = %d", res.Total)
	}
}

func TestTiers(t *testing.T) {
	cases := []struct {
		nat, total, target int
		want               Tier
	}{
		{20, 5, 25, CritSuccess},
		{1, 30, 5, CritFailure},
		{10, 15, 15, Success},
		{10, 14, 15, Failure},
	}
	for _, c := range cases {
		if got := TierFor(c.nat, c.total, c.target); got != c.want {
			t.Errorf("TierFor(%d,%d,%d) = %s want %s", c.nat, c.total, c.target, got, c.want)
		}
	}
	r := FromSource(NewScripted(12))
	ch := r.D20(3, 15)
	if ch.Total != 15 || ch.Tier != Success {
		t.Fatalf("D20 = %+v", ch)
	}
}

func TestSnapDC(t *testing.T) {
	for in, want := range map[int]int{0: 5, 7: 5, 8: 10, 13: 15, 17: 15, 18: 20, 99: 25} {
		if got := SnapDC(in); got != want {
			t.Errorf("SnapDC(%d) = %d want %d", in, got, want)
		}
	}
}

func TestScriptedClampsAndRepeats(t *testing.T) {
	r := FromSource(NewScripted(25, 2))
	if v := r.Die(20); v != 20 {
		t.Fatalf("clamp: %d", v)
	}
	if v := r.Die(20); v != 2 {
		t.Fatalf("second: %d", v)
	}
	if v := r.Die(20); v != 2 {
		t.Fatalf("repeat: %d", v)
	}
}

func TestD20Edge(t *testing.T) {
	r := FromSource(NewScripted(4, 17, 4, 17, 9))
	if c := r.D20Edge(2, 15, 1); c.Natural != 17 || c.Total != 19 || c.Edge != Advantage || len(c.Naturals) != 2 || !c.Tier.Succeeded() {
		t.Fatalf("advantage %+v", c)
	}
	if c := r.D20Edge(2, 15, -1); c.Natural != 4 || c.Edge != Disadvantage || c.Tier.Succeeded() {
		t.Fatalf("disadvantage %+v", c)
	}
	if c := r.D20Edge(0, 10, 0); c.Natural != 9 || c.Edge != "" || c.Naturals != nil {
		t.Fatalf("straight %+v", c)
	}
}
