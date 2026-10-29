// Command play is a terminal client for an aidnd server.
//
//	play new "a haunted lake town"      create a campaign, prints its id
//	play join <campaign> <name>         join as this player
//	play char <campaign> "<concept>"    draft a character, confirm, save
//	play start <campaign>               start the game
//	play <campaign>                     play: type actions, watch events
//
// Flags (or env): -server (AIDND_SERVER), -player (AIDND_PLAYER), -key
// (AIDND_API_KEY, service mode only). Without -player a random id is made
// once and kept in ~/.aidnd-player.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func main() {
	fs := flag.NewFlagSet("play", flag.ExitOnError)
	server := fs.String("server", envOr("AIDND_SERVER", "http://localhost:8080"), "server base URL")
	player := fs.String("player", os.Getenv("AIDND_PLAYER"), "player id (X-Player-ID)")
	key := fs.String("key", os.Getenv("AIDND_API_KEY"), "API key for service mode")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: play [flags] new <premise> | join <id> <name> | char <id> <concept> | start <id> | <id>")
		fs.PrintDefaults()
	}
	_ = fs.Parse(os.Args[1:])
	if *player == "" {
		*player = savedPlayerID()
	}
	c := &Client{Base: strings.TrimRight(*server, "/"), Player: *player, Key: *key, HTTP: http.DefaultClient}
	if err := run(context.Background(), c, fs.Args(), os.Stdin, os.Stdout); err != nil {
		if errors.Is(err, errUsage) {
			fs.Usage()
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

var errUsage = errors.New("usage")

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func savedPlayerID() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return randomID()
	}
	path := filepath.Join(home, ".aidnd-player")
	if b, err := os.ReadFile(path); err == nil && len(bytes.TrimSpace(b)) > 0 {
		return string(bytes.TrimSpace(b))
	}
	id := randomID()
	_ = os.WriteFile(path, []byte(id+"\n"), 0o600)
	return id
}

func randomID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "cli-" + hex.EncodeToString(b)
}

// Client talks to the server as one player.
type Client struct {
	Base, Player, Key string
	HTTP              *http.Client
}

// APIError is a non-2xx answer.
type APIError struct {
	Status   int
	Message  string
	Problems []string
}

func (e *APIError) Error() string {
	if len(e.Problems) > 0 {
		return fmt.Sprintf("%s: %s", e.Message, strings.Join(e.Problems, "; "))
	}
	return fmt.Sprintf("%s (%d)", e.Message, e.Status)
}

func (c *Client) request(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Player != "" {
		req.Header.Set("X-Player-ID", c.Player)
	}
	if c.Key != "" {
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}
	return req, nil
}

// Do sends a JSON request and decodes the JSON answer into out.
func (c *Client) Do(ctx context.Context, method, path string, body, out any) error {
	req, err := c.request(ctx, method, path, body)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		var e struct {
			Error    string   `json:"error"`
			Problems []string `json:"problems"`
		}
		_ = json.Unmarshal(data, &e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return &APIError{Status: resp.StatusCode, Message: e.Error, Problems: e.Problems}
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// Event is one SSE event from the server.
type Event struct {
	ID   int64           `json:"id"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// Stream reads events after the given id until ctx ends or the stream
// closes, sending each on ch.
func (c *Client) Stream(ctx context.Context, campaign string, after int64, ch chan<- Event) error {
	req, err := c.request(ctx, "GET", fmt.Sprintf("/campaigns/%s/events?after=%d", campaign, after), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return &APIError{Status: resp.StatusCode, Message: "event stream refused"}
	}
	return readSSE(resp.Body, func(data string) {
		var e Event
		if json.Unmarshal([]byte(data), &e) == nil {
			select {
			case ch <- e:
			case <-ctx.Done():
			}
		}
	})
}

// readSSE calls fn with the data of every event in r.
func readSSE(r io.Reader, fn func(data string)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	var data []string
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if len(data) > 0 {
				fn(strings.Join(data, "\n"))
				data = data[:0]
			}
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	return sc.Err()
}

// --- views (only the fields the client shows)

type campaignView struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Pitch    string `json:"pitch"`
	Status   string `json:"status"`
	Location *struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Zones       []struct {
			ID, Name string
			Cover    bool     `json:"cover"`
			Adjacent []string `json:"adjacent"`
		} `json:"zones"`
		Exits []struct {
			Label  string `json:"label"`
			ToName string `json:"to_name"`
			State  string `json:"state"`
		} `json:"exits"`
		NPCs []struct {
			Name   string `json:"name"`
			Role   string `json:"role"`
			ZoneID string `json:"zone_id"`
		} `json:"npcs"`
	} `json:"location"`
	Party     []character `json:"party"`
	Encounter *struct {
		Round   int `json:"round"`
		Enemies []struct {
			Display string `json:"display"`
			HP      int    `json:"hp"`
			MaxHP   int    `json:"max_hp"`
			ZoneID  string `json:"zone_id"`
		} `json:"enemies"`
	} `json:"encounter"`
	Quests []struct {
		Title      string `json:"title"`
		Status     string `json:"status"`
		Objectives []struct {
			Text string `json:"text"`
			Done bool   `json:"done"`
		} `json:"objectives"`
	} `json:"quests"`
	Active *struct {
		Name     string `json:"name"`
		PlayerID string `json:"player_id"`
	} `json:"active"`
	Budget *budget `json:"budget"`
	You    struct {
		CharacterID string `json:"character_id"`
		Joined      bool   `json:"joined"`
		YourTurn    bool   `json:"your_turn"`
	} `json:"you"`
}

type budget struct {
	Action int `json:"action"`
	Move   int `json:"move"`
	Free   int `json:"free"`
}

func (b budget) String() string {
	free := fmt.Sprint(b.Free)
	if b.Free < 0 {
		free = "∞"
	}
	return fmt.Sprintf("action %d · move %d · free %s", b.Action, b.Move, free)
}

type character struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Class     string         `json:"class"`
	Kit       string         `json:"kit"`
	Level     int            `json:"level"`
	XP        int            `json:"xp"`
	HP        int            `json:"hp"`
	MaxHP     int            `json:"max_hp"`
	AC        int            `json:"ac"`
	Gold      int            `json:"gold"`
	ZoneID    string         `json:"zone_id"`
	Scores    map[string]int `json:"scores"`
	Modifiers map[string]int `json:"modifiers"`
	Backstory string         `json:"backstory"`
	Weapon    *struct {
		Name, Damage, Reach string
	} `json:"weapon"`
	Inventory []struct {
		Name     string `json:"name"`
		Equipped bool   `json:"equipped"`
		Effect   string `json:"effect"`
	} `json:"inventory"`
}

func (v *campaignView) me() *character {
	for i := range v.Party {
		if v.Party[i].ID == v.You.CharacterID && v.You.CharacterID != "" {
			return &v.Party[i]
		}
	}
	return nil
}

func (v *campaignView) zoneName(id string) string {
	if v.Location != nil {
		for _, z := range v.Location.Zones {
			if z.ID == id {
				return z.Name
			}
		}
	}
	return id
}

// --- commands

func run(ctx context.Context, c *Client, args []string, in io.Reader, out io.Writer) error {
	if len(args) == 0 {
		return errUsage
	}
	switch args[0] {
	case "new":
		var v campaignView
		fmt.Fprintln(out, "Authoring the world (this can take a minute)...")
		if err := c.Do(ctx, "POST", "/campaigns", map[string]string{"premise": strings.Join(args[1:], " ")}, &v); err != nil {
			return err
		}
		fmt.Fprintf(out, "%s\n%s\ncampaign id: %s\n", v.Title, v.Pitch, v.ID)
		return nil
	case "join":
		if len(args) < 2 {
			return errUsage
		}
		name := strings.Join(args[2:], " ")
		if name == "" {
			name = "Player"
		}
		var r struct {
			Player struct{ ID, Name string } `json:"player"`
		}
		if err := c.Do(ctx, "POST", "/campaigns/"+args[1]+"/players", map[string]string{"name": name}, &r); err != nil {
			return err
		}
		fmt.Fprintf(out, "joined as %s (player %s)\n", r.Player.Name, r.Player.ID)
		return nil
	case "char":
		if len(args) < 3 {
			return errUsage
		}
		return makeCharacter(ctx, c, args[1], strings.Join(args[2:], " "), bufio.NewReader(in), out)
	case "start":
		if len(args) < 2 {
			return errUsage
		}
		if err := c.Do(ctx, "POST", "/campaigns/"+args[1]+"/start", nil, nil); err != nil {
			return err
		}
		fmt.Fprintln(out, "started")
		return nil
	case "play":
		if len(args) < 2 {
			return errUsage
		}
		return play(ctx, c, args[1], in, out)
	}
	if len(args) == 1 {
		return play(ctx, c, args[0], in, out)
	}
	return errUsage
}

type charSpec struct {
	Name       string         `json:"name"`
	Concept    string         `json:"concept,omitempty"`
	Class      string         `json:"class,omitempty"`
	Kit        string         `json:"kit"`
	Backstory  string         `json:"backstory,omitempty"`
	Pronouns   string         `json:"pronouns,omitempty"`
	Scores     map[string]int `json:"scores"`
	Proficient []string       `json:"proficient"`
}

var abilities = []string{"STR", "DEX", "CON", "INT", "WIS", "CHA"}

func makeCharacter(ctx context.Context, c *Client, campaign, concept string, in *bufio.Reader, out io.Writer) error {
	fmt.Fprintln(out, "Drafting...")
	var d struct {
		Character charSpec `json:"character"`
	}
	if err := c.Do(ctx, "POST", "/campaigns/"+campaign+"/characters/draft", map[string]string{"concept": concept}, &d); err != nil {
		return err
	}
	s := d.Character
	for {
		fmt.Fprintf(out, "\n%s (%s), %s (%s kit)\n", s.Name, s.Pronouns, s.Class, s.Kit)
		var sc []string
		for _, a := range abilities {
			sc = append(sc, fmt.Sprintf("%s %d", a, s.Scores[a]))
		}
		fmt.Fprintf(out, "%s · proficient: %s\n%s\n", strings.Join(sc, " "), strings.Join(s.Proficient, ", "), s.Backstory)
		fmt.Fprint(out, "[s]ave, rename with 'n <name>', pronouns with 'p she/her', or 'q' to quit: ")
		line, err := in.ReadString('\n')
		line = strings.TrimSpace(line)
		switch {
		case line == "s" || line == "y" || (line == "" && err == nil):
			var saved character
			if err := c.Do(ctx, "POST", "/campaigns/"+campaign+"/characters", map[string]any{"character": s}, &saved); err != nil {
				return err
			}
			fmt.Fprintf(out, "saved %s (L%d %s, HP %d, AC %d)\n", saved.Name, saved.Level, saved.Class, saved.MaxHP, saved.AC)
			return nil
		case strings.HasPrefix(line, "n "):
			s.Name = strings.TrimSpace(line[2:])
		case strings.HasPrefix(line, "p "):
			s.Pronouns = strings.TrimSpace(line[2:])
		case line == "q" || err != nil:
			return errors.New("not saved")
		}
	}
}

func play(ctx context.Context, c *Client, campaign string, in io.Reader, out io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var v campaignView
	if err := c.Do(ctx, "GET", "/campaigns/"+campaign, nil, &v); err != nil {
		return err
	}
	if !v.You.Joined {
		return fmt.Errorf("player %s has not joined %s; run: play join %s <name>", c.Player, campaign, campaign)
	}
	fmt.Fprintf(out, "== %s ==  (%s)\ncommands: :look :sheet :map :quests :end :quit\n\n", v.Title, v.Status)
	evs := make(chan Event, 64)
	go func() {
		var last int64
		for ctx.Err() == nil {
			inner := make(chan Event)
			done := make(chan error, 1)
			go func() { done <- c.Stream(ctx, campaign, last, inner); close(inner) }()
			for e := range inner {
				last = e.ID
				select {
				case evs <- e:
				case <-ctx.Done():
				}
			}
			<-done
			select { // reconnect after a short pause
			case <-time.After(time.Second):
			case <-ctx.Done():
			}
		}
	}()
	lines := make(chan string)
	go func() {
		sc := bufio.NewScanner(in)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	me := ""
	if m := v.me(); m != nil {
		me = m.ID
	}
	for {
		select {
		case e := <-evs:
			fmt.Fprint(out, formatEvent(e, me))
			if e.Type == "turn_started" || e.Type == "turn_budget" {
				if err := c.Do(ctx, "GET", "/campaigns/"+campaign, nil, &v); err == nil && v.You.YourTurn && v.Budget != nil {
					fmt.Fprintf(out, "> your turn (%s)\n", v.Budget)
				}
			}
			if e.Type == "campaign_ended" {
				return nil
			}
		case line, ok := <-lines:
			if !ok {
				return nil
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if err := command(ctx, c, campaign, line, out); err != nil {
				if errors.Is(err, io.EOF) {
					return nil
				}
				fmt.Fprintln(out, "!", err)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func command(ctx context.Context, c *Client, campaign, line string, out io.Writer) error {
	var v campaignView
	get := func() error { return c.Do(ctx, "GET", "/campaigns/"+campaign, nil, &v) }
	switch line {
	case ":quit", ":q":
		return io.EOF
	case ":end":
		return c.Do(ctx, "POST", "/campaigns/"+campaign+"/skip", map[string]any{}, nil)
	case ":look":
		if err := get(); err != nil {
			return err
		}
		fmt.Fprint(out, formatLook(&v))
		return nil
	case ":sheet":
		if err := get(); err != nil {
			return err
		}
		fmt.Fprint(out, formatSheet(v.me()))
		return nil
	case ":map":
		var m struct {
			CurrentID string `json:"current_id"`
			Locations []struct {
				ID, Name string
				Visited  bool `json:"visited"`
			} `json:"locations"`
			Edges []struct{ A, B, State string } `json:"edges"`
		}
		if err := c.Do(ctx, "GET", "/campaigns/"+campaign+"/map", nil, &m); err != nil {
			return err
		}
		names := map[string]string{}
		for _, l := range m.Locations {
			names[l.ID] = l.Name
		}
		for _, l := range m.Locations {
			mark := "  "
			if l.ID == m.CurrentID {
				mark = "* "
			}
			var links []string
			for _, e := range m.Edges {
				if e.A == l.ID && names[e.B] != "" {
					links = append(links, names[e.B]+stateNote(e.State))
				} else if e.B == l.ID && names[e.A] != "" {
					links = append(links, names[e.A]+stateNote(e.State))
				}
			}
			sort.Strings(links)
			fmt.Fprintf(out, "%s%s -> %s\n", mark, l.Name, strings.Join(links, ", "))
		}
		return nil
	case ":quests":
		if err := get(); err != nil {
			return err
		}
		for _, q := range v.Quests {
			fmt.Fprintf(out, "%s [%s]\n", q.Title, q.Status)
			for _, o := range q.Objectives {
				box := "[ ]"
				if o.Done {
					box = "[x]"
				}
				fmt.Fprintf(out, "   %s %s\n", box, o.Text)
			}
		}
		return nil
	}
	if strings.HasPrefix(line, ":") {
		return fmt.Errorf("unknown command %s", line)
	}
	return c.Do(ctx, "POST", "/campaigns/"+campaign+"/actions", map[string]string{"text": line}, nil)
}

func stateNote(s string) string {
	if s == "open" || s == "" {
		return ""
	}
	return " (" + s + ")"
}

func formatLook(v *campaignView) string {
	var b strings.Builder
	if v.Location == nil {
		return "(the game has not started)\n"
	}
	l := v.Location
	fmt.Fprintf(&b, "%s\n%s\n", l.Name, l.Description)
	for _, z := range l.Zones {
		var who []string
		for _, c := range v.Party {
			if c.ZoneID == z.ID {
				who = append(who, c.Name)
			}
		}
		for _, n := range l.NPCs {
			if n.ZoneID == z.ID {
				who = append(who, n.Name+" ("+n.Role+")")
			}
		}
		if v.Encounter != nil {
			for _, e := range v.Encounter.Enemies {
				if e.ZoneID == z.ID {
					who = append(who, fmt.Sprintf("%s %d/%d", e.Display, e.HP, e.MaxHP))
				}
			}
		}
		cover := ""
		if z.Cover {
			cover = " [cover]"
		}
		fmt.Fprintf(&b, "  %s%s: %s\n", z.Name, cover, strings.Join(who, ", "))
	}
	for _, x := range l.Exits {
		fmt.Fprintf(&b, "  exit: %s -> %s%s\n", x.Label, x.ToName, stateNote(x.State))
	}
	return b.String()
}

func formatSheet(c *character) string {
	if c == nil {
		return "(no character)\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s, L%d %s (%s) · XP %d\nHP %d/%d · AC %d · %d gold\n", c.Name, c.Level, c.Class, c.Kit, c.XP, c.HP, c.MaxHP, c.AC, c.Gold)
	var sc []string
	for _, a := range abilities {
		sc = append(sc, fmt.Sprintf("%s %d (%+d)", a, c.Scores[a], c.Modifiers[a]))
	}
	fmt.Fprintln(&b, strings.Join(sc, " · "))
	if c.Weapon != nil {
		fmt.Fprintf(&b, "weapon: %s %s (%s)\n", c.Weapon.Name, c.Weapon.Damage, c.Weapon.Reach)
	}
	for _, it := range c.Inventory {
		eq := ""
		if it.Equipped {
			eq = " (equipped)"
		}
		fmt.Fprintf(&b, "  - %s%s\n", it.Name, eq)
	}
	return b.String()
}

// formatEvent renders one event for the terminal; me is the reader's
// character id, if any.
func formatEvent(e Event, me string) string {
	var d map[string]any
	_ = json.Unmarshal(e.Data, &d)
	s := func(k string) string { v, _ := d[k].(string); return v }
	switch e.Type {
	case "campaign_created":
		return fmt.Sprintf("~ campaign: %s\n", s("title"))
	case "player_joined":
		return fmt.Sprintf("~ %s joined\n", s("name"))
	case "character_created":
		return fmt.Sprintf("~ %s the %s is ready\n", s("name"), s("class"))
	case "location_changed":
		return fmt.Sprintf("\n## %s\n", s("name"))
	case "turn_started":
		who := s("name")
		if s("character_id") == me {
			who += " (you)"
		}
		return fmt.Sprintf("\n-- %s's turn --\n", who)
	case "player_action":
		return fmt.Sprintf("%s: %s\n", s("character"), s("text"))
	case "narration":
		return wrap(s("text"), 78) + "\n"
	case "quest_updated":
		return fmt.Sprintf("~ quest: %s\n", s("text"))
	case "campaign_ended":
		if s("status") == "completed" {
			return "\n*** The campaign is won. ***\n"
		}
		return "\n*** The campaign has failed. ***\n"
	case "action_resolved":
		var r struct {
			Steps []struct {
				Action  struct{ Kind string } `json:"action"`
				Error   string                `json:"error"`
				Rolls   []rollView            `json:"rolls"`
				Changes []string              `json:"changes"`
			} `json:"steps"`
			Skipped []struct {
				Action struct{ Kind string } `json:"action"`
				Error  string                `json:"error"`
			} `json:"skipped"`
			WorldChanges []string `json:"world_changes"`
		}
		_ = json.Unmarshal(e.Data, &r)
		var b strings.Builder
		for _, st := range r.Steps {
			if st.Error != "" {
				fmt.Fprintf(&b, "  x %s: %s\n", st.Action.Kind, st.Error)
			}
			for _, roll := range st.Rolls {
				fmt.Fprintf(&b, "  %s\n", roll)
			}
			for _, ch := range st.Changes {
				fmt.Fprintf(&b, "  * %s\n", ch)
			}
		}
		for _, sk := range r.Skipped {
			fmt.Fprintf(&b, "  (skipped %s: %s)\n", sk.Action.Kind, sk.Error)
		}
		for _, ch := range r.WorldChanges {
			fmt.Fprintf(&b, "  + %s\n", ch)
		}
		return b.String()
	case "enemy_turns":
		var r struct {
			Turns []struct {
				Text   string `json:"text"`
				Attack *struct {
					Roll   rollView `json:"roll"`
					Damage *struct {
						Total int `json:"total"`
					} `json:"damage"`
				} `json:"attack"`
			} `json:"turns"`
			Narration string `json:"narration"`
		}
		_ = json.Unmarshal(e.Data, &r)
		var b strings.Builder
		for _, t := range r.Turns {
			if t.Attack != nil {
				dmg := ""
				if t.Attack.Damage != nil {
					dmg = fmt.Sprintf(", %d damage", t.Attack.Damage.Total)
				}
				fmt.Fprintf(&b, "  %s%s\n", t.Attack.Roll, dmg)
			} else if t.Text != "" {
				fmt.Fprintf(&b, "  * %s\n", t.Text)
			}
		}
		if r.Narration != "" {
			b.WriteString(wrap(r.Narration, 78) + "\n")
		}
		return b.String()
	}
	return ""
}

type rollView struct {
	Label   string `json:"label"`
	Kind    string `json:"kind"`
	Natural int    `json:"natural"`
	Dice    []int  `json:"dice"`
	Expr    string `json:"expr"`
	Mod     int    `json:"mod"`
	Total   int    `json:"total"`
	Target  int    `json:"target"`
	Tier    string `json:"tier"`
}

func (r rollView) String() string {
	switch r.Kind {
	case "damage", "heal":
		return fmt.Sprintf("[%s] %s %v = %d", r.Label, r.Expr, r.Dice, r.Total)
	case "encounter":
		return fmt.Sprintf("[%s] d100 %d (<= %d)", r.Label, r.Total, r.Target)
	}
	return fmt.Sprintf("[%s] d20 %d%+d = %d vs %d: %s", r.Label, r.Natural, r.Mod, r.Total, r.Target, strings.ReplaceAll(r.Tier, "_", " "))
}

func wrap(s string, width int) string {
	var b strings.Builder
	for i, para := range strings.Split(s, "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		n := 0
		for _, w := range strings.Fields(para) {
			if n > 0 && n+1+len(w) > width {
				b.WriteByte('\n')
				n = 0
			} else if n > 0 {
				b.WriteByte(' ')
				n++
			}
			b.WriteString(w)
			n += len(w)
		}
	}
	return b.String()
}
