// Package dice provides an injectable random source, "NdS+M" notation and
// d20 checks that resolve to one of four outcome tiers.
package dice

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
)

// Source returns a value in [0, n). math/rand's *Rand satisfies it.
type Source interface {
	Intn(n int) int
}

// Roller rolls dice from a Source. It is safe for concurrent use.
type Roller struct {
	mu  sync.Mutex
	src Source
}

// New returns a Roller seeded deterministically.
func New(seed int64) *Roller {
	return &Roller{src: rand.New(rand.NewSource(seed))}
}

// FromSource wraps any Source (for example a Scripted one in tests).
func FromSource(s Source) *Roller { return &Roller{src: s} }

// Die rolls one die with the given number of sides (1..sides).
func (r *Roller) Die(sides int) int {
	if sides < 1 {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.src.Intn(sides) + 1
}

// Intn exposes the raw source for non-dice choices (target picking etc.).
func (r *Roller) Intn(n int) int {
	if n <= 1 {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.src.Intn(n)
}

// Scripted is a Source that returns preset die faces in order. Each value is
// the face wanted (1-based); it is clamped into range for the die rolled.
// When exhausted it repeats the last value (or 1 if empty).
type Scripted struct {
	mu    sync.Mutex
	faces []int
	i     int
}

// NewScripted returns a Scripted source for the given faces.
func NewScripted(faces ...int) *Scripted { return &Scripted{faces: faces} }

// Push appends more faces.
func (s *Scripted) Push(faces ...int) {
	s.mu.Lock()
	s.faces = append(s.faces, faces...)
	s.mu.Unlock()
}

func (s *Scripted) Intn(n int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := 1
	if s.i < len(s.faces) {
		v = s.faces[s.i]
		s.i++
	} else if len(s.faces) > 0 {
		v = s.faces[len(s.faces)-1]
	}
	if v < 1 {
		v = 1
	}
	if v > n {
		v = n
	}
	return v - 1
}

// Expr is parsed dice notation: Count d Sides + Mod.
type Expr struct {
	Count int `json:"count"`
	Sides int `json:"sides"`
	Mod   int `json:"mod"`
}

func (e Expr) String() string {
	s := fmt.Sprintf("%dd%d", e.Count, e.Sides)
	switch {
	case e.Mod > 0:
		s += fmt.Sprintf("+%d", e.Mod)
	case e.Mod < 0:
		s += fmt.Sprintf("%d", e.Mod)
	}
	return s
}

// Max is the highest possible total.
func (e Expr) Max() int { return e.Count*e.Sides + e.Mod }

// Average is the mean total, rounded down.
func (e Expr) Average() int { return (e.Count*(e.Sides+1))/2 + e.Mod }

// Parse reads notation such as "2d6+3", "d8", "1d4-1" or a flat "5".
func Parse(s string) (Expr, error) {
	t := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	if t == "" {
		return Expr{}, fmt.Errorf("empty dice expression")
	}
	d := strings.IndexByte(t, 'd')
	if d < 0 {
		n, err := strconv.Atoi(t)
		if err != nil {
			return Expr{}, fmt.Errorf("bad dice expression %q", s)
		}
		return Expr{Mod: n}, nil
	}
	e := Expr{Count: 1}
	if d > 0 {
		n, err := strconv.Atoi(t[:d])
		if err != nil || n < 1 {
			return Expr{}, fmt.Errorf("bad dice count in %q", s)
		}
		e.Count = n
	}
	rest := t[d+1:]
	cut := strings.IndexAny(rest, "+-")
	sides := rest
	if cut >= 0 {
		sides = rest[:cut]
		m, err := strconv.Atoi(rest[cut:])
		if err != nil {
			return Expr{}, fmt.Errorf("bad modifier in %q", s)
		}
		e.Mod = m
	}
	n, err := strconv.Atoi(sides)
	if err != nil || n < 2 {
		return Expr{}, fmt.Errorf("bad die size in %q", s)
	}
	e.Sides = n
	if e.Count > 100 || e.Sides > 1000 {
		return Expr{}, fmt.Errorf("dice expression %q too large", s)
	}
	return e, nil
}

// MustParse panics on bad notation; for catalog literals only.
func MustParse(s string) Expr {
	e, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return e
}

// Result is a rolled expression.
type Result struct {
	Expr  string `json:"expr"`
	Dice  []int  `json:"dice"`
	Mod   int    `json:"mod"`
	Total int    `json:"total"`
}

// Roll rolls e. If crit is true the dice are doubled (not the modifier).
// Totals never go below zero.
func (r *Roller) Roll(e Expr, crit bool) Result {
	n := e.Count
	if crit {
		n *= 2
	}
	res := Result{Expr: e.String(), Mod: e.Mod}
	sum := 0
	for i := 0; i < n; i++ {
		v := r.Die(e.Sides)
		res.Dice = append(res.Dice, v)
		sum += v
	}
	res.Total = sum + e.Mod
	if res.Total < 0 {
		res.Total = 0
	}
	return res
}

// RollString parses and rolls notation in one step.
func (r *Roller) RollString(s string) (Result, error) {
	e, err := Parse(s)
	if err != nil {
		return Result{}, err
	}
	return r.Roll(e, false), nil
}

// Tier is the outcome band of a d20 check.
type Tier string

const (
	CritSuccess Tier = "critical_success"
	Success     Tier = "success"
	Failure     Tier = "failure"
	CritFailure Tier = "critical_failure"
)

// Succeeded reports whether the tier counts as a success.
func (t Tier) Succeeded() bool { return t == CritSuccess || t == Success }

// DCs are the only difficulty classes the engine uses.
var DCs = []int{5, 10, 15, 20, 25}

// SnapDC rounds any number to the nearest allowed DC.
func SnapDC(dc int) int {
	best := DCs[0]
	for _, v := range DCs {
		if abs(dc-v) < abs(dc-best) {
			best = v
		}
	}
	return best
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// TierFor classifies a d20 roll: natural 20/1 are criticals, otherwise
// total >= target succeeds.
func TierFor(natural, total, target int) Tier {
	switch {
	case natural == 20:
		return CritSuccess
	case natural == 1:
		return CritFailure
	case total >= target:
		return Success
	default:
		return Failure
	}
}

// Check is a resolved d20 roll against a target number (DC or AC).
type Check struct {
	Natural int  `json:"natural"`
	Mod     int  `json:"mod"`
	Total   int  `json:"total"`
	Target  int  `json:"target"`
	Tier    Tier `json:"tier"`
}

// D20 rolls d20+mod against target.
func (r *Roller) D20(mod, target int) Check {
	n := r.Die(20)
	t := n + mod
	return Check{Natural: n, Mod: mod, Total: t, Target: target, Tier: TierFor(n, t, target)}
}
