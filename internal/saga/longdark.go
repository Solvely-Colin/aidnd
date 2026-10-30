package saga

import "github.com/Solvely-Colin/aidnd/internal/game"

// The Long Dark Below is the first saga: Lovecraftian New England from
// 1931, one open world that grows region by region as players reach it.
// SPOILERS below: the secrets are gated so the narrator only learns them as
// players uncover them. More regions are added as they are written.
func init() { register(longDark) }

var longDark = &Saga{
	Key:     "long-dark",
	Title:   "The Long Dark Below",
	Setting: game.SettingPulp,
	Pitch: "1931. The fishing schooner Mary Alder drifted into Innsmouth harbor with a full hold and no crew, " +
		"and nobody in town reported it. Something under the Atlantic is dreaming, and the coast is starting to dream with it.",
	Premise: "A Lovecraftian investigation campaign in 1931 Massachusetts, beginning in the decaying fishing town of Innsmouth. " +
		"Investigators, not heroes: horror is felt, the unknown stays partly unknown, and the town keeps its secrets until they are pried loose.",
	Start:   "innsmouth",
	Regions: []Region{innsmouth, arkham},
}

// Deep One hybrids and the Order's men share a stat block.
var orderMan = &game.StatBlock{MaxHP: 11, AC: 12, AttackBonus: 3, Damage: "1d6+1", HorrorRank: 1}

var innsmouth = Region{
	Key:    "innsmouth",
	Name:   "Innsmouth",
	Levels: [2]int{1, 4},
	Brief: "Innsmouth has been dying since the 'plague' of 1846. Most townsfolk have the Innsmouth look and keep away from strangers. " +
		"The Esoteric Order of Dagon replaced every church in town that year. Outsiders who ask too many questions are watched, then warned, then gone.",
	Hub: Place{Name: "Gilman House", Kind: game.KindBuilding,
		Hint: "The only hotel in Innsmouth",
		Desc: "A sagging four-storey hotel on Town Square, the only place in Innsmouth that rents rooms to strangers. " +
			"The lobby smells of damp carpet and fish; the night clerk never quite looks at you. The doors lock, for what that is worth."},
	Places: []Place{
		{Name: "Innsmouth Harbor", Kind: game.KindTown, Danger: 1,
			Hint: "Where the Mary Alder lies at her mooring",
			Desc: "Rotting wharves and a breakwater half fallen into the sea. The schooner Mary Alder rides at her mooring, gulls thick on her rail. " +
				"Out past the breakwater, a long black line of rock breaks the swell: Devil Reef."},
		{Name: "Washington Street", Kind: game.KindTown, Danger: 1,
			Hint: "The old captains' mansions, most of them shuttered",
			Desc: "Wide and once grand, lined with Georgian houses whose widow's walks look out to sea. Most windows are boarded. " +
				"In one, a lamp burns in broad daylight."},
		{Name: "Marsh Refinery", Kind: game.KindBuilding, Danger: 2, From: "Innsmouth Harbor",
			Hint: "The Marsh family's gold refinery, still running somehow",
			Desc: "A brick works by the falls of the Manuxet, its stack the only one in town still smoking. " +
				"The gold it ships is an odd pale alloy, and no one has ever seen ore go in."},
		{Name: "Zadok Allen's Shack", Kind: game.KindBuilding, Danger: 1, From: "Innsmouth Harbor",
			Hint: "Where the town drunk sleeps it off",
			Desc: "A tarpaper shack at the end of Water Street, propped against the old fire station. Empty bottles line the sill, and an old man's voice mutters behind the door."},
		{Name: "Order Hall", Kind: game.KindBuilding, Danger: 2,
			Hint: "The old Masonic hall on New Church Green",
			Desc: "A white-columned hall on New Church Green, its Masonic emblems chiselled away and replaced by a gold sign: Esoteric Order of Dagon. " +
				"A bell hangs in the cupola, green with age. It is the only bell in Innsmouth anyone ever hears."},
		{Name: "The Salt Marsh", Kind: game.KindWilderness, Danger: 2,
			Hint: "Reeds and tidal pools south of town",
			Desc: "Miles of reed and black tidal water south of the town. At low tide, paths appear between the pools; at high tide they vanish. Something has trampled a wide track toward the sea."},
		{Name: "Falcon Point", Kind: game.KindWilderness, Danger: 2, From: "The Salt Marsh",
			Hint: "A spit of rock where lights are seen at low tide",
			Desc: "A finger of granite reaching into the sea, slick with weed. Old iron rings are set into the rock, and the stones near the water are worn smooth by many feet."},
		{Name: "Gull Point Light", Kind: game.KindBuilding, Danger: 2, From: "Falcon Point",
			Hint: "The lighthouse, whose keeper signals the reef at night",
			Desc: "A whitewashed lighthouse on the last rocks of the point. The lamp turns even by day. The keeper's logbooks are stacked to the ceiling of the watch room."},
		{Name: "The Flooded Crypt", Kind: game.KindDungeon, Danger: 3, From: "Order Hall", Lair: true,
			Hint: "Something under the Order Hall, dry only at low tide"},
	},
	People: []Person{
		{Name: "Martha Sargent", Role: "hotel keeper", At: "Gilman House", Disposition: 0,
			Desc: "The widow who runs the Gilman House: thin, pale, wide-eyed, and polite to anyone who pays in advance."},
		{Name: "Ned Pike", Role: "grocery clerk", At: "Gilman House", Disposition: 1, Merchant: true,
			Stock: []string{"healing_potion", "healing_potion", "healers_kit", "rope", "lantern", "torch", "rations", "dagger", "leather"},
			Desc:  "A seventeen-year-old from Arkham who runs the chain grocery across the square and can't wait to be transferred out. Talks to strangers because no one else will talk to him."},
		{Name: "Hannah Coyle", Role: "sister of the Mary Alder's mate", At: "Gilman House", Disposition: 2, Protected: true,
			Desc:  "A schoolteacher from Newburyport in a sensible coat, here because her brother Tom was the Mary Alder's mate and she refuses to believe he drowned.",
			Keeps: []string{"Her brother Tom Coyle sailed as mate on the Mary Alder and has not come home."}},
		{Name: "Mr. Nye", Role: "bell-founder from Providence", At: "Gilman House", Disposition: 1, Protected: true,
			Desc: "A tall, dark, beautifully spoken man in a good grey suit, staying at the hotel on business. He is very interested in old bells."},
		{Name: "Rafe Moretti", Role: "rum-runner for the Marchetti outfit", At: "Innsmouth Harbor", Disposition: 0, Merchant: true,
			Desc: "A Boston bootlegger who runs a motor launch out of Innsmouth because the Coast Guard never comes here. Knows the town pays its way in gold and asks no questions back."},
		{Name: "Mrs. Waite", Role: "captain's widow", At: "Washington Street", Disposition: 1,
			Desc: "An old woman in black in a big shuttered house, who keeps a lamp in her window day and night and asks every stranger to help her tend it."},
		{Name: "Mr. Babson", Role: "refinery clerk", At: "Marsh Refinery", Disposition: 0,
			Desc: "A stooped bookkeeper with ink-stained gloves he never removes. Nervous, underpaid, and three months behind on the refinery's books."},
		{Name: "Zadok Allen", Role: "town drunk, aged ninety-six", At: "Zadok Allen's Shack", Disposition: 0, Protected: true,
			Desc: "The oldest man in Innsmouth, half-mad and mostly drunk, who will tell anyone with a bottle about the old days. Everyone in town says he lies."},
		{Name: "Deacon Joseph Gilman", Role: "deacon of the Esoteric Order of Dagon", At: "Order Hall", Disposition: -2, Stats: orderMan,
			Desc: "A heavy man with a flat, unblinking stare and a tiara-shaped pin on his lapel. He welcomes visitors to the Order's Sunday hall, and remembers every face."},
		{Name: "Eliza Pell", Role: "lighthouse keeper", At: "Gull Point Light", Disposition: 1, Protected: true,
			Desc:  "A weathered woman of fifty who has kept Gull Point Light for twenty years and talks about the sea as if it were listening.",
			Keeps: []string{"Her brother Samuel walked into the sea at Falcon Point in 1927."}},
		{Name: "Tom Coyle", Role: "the Mary Alder's lost mate", Disposition: 0,
			Desc: "Hannah's brother. He stands waist-deep in the black water with the rest of the crew, eyes open, not shivering."},
		{Name: "The Order's men", Role: "Esoteric Order of Dagon enforcers", Disposition: -3, Stats: orderMan,
			Desc: "Townsmen with the Innsmouth look: flat heads, wide wet eyes, skin that seems to flake. They move in silence and they do not tire."},
		{Name: "Agent Ruth Halloway", Role: "Bureau of Special Inquiry agent", Disposition: 0, Protected: true,
			Desc: "A federal agent in a plain coat with a notebook and a revolver she has clearly used. She is here about the gold, she says."},
	},
	Things: []Thing{
		{Name: "Logbook of the Mary Alder", Kind: game.ItemQuest, At: "Innsmouth Harbor",
			Desc: "The schooner's log, swollen with damp. The last entry, in a shaking hand: 'Bell from shore at low water. She called us down.'"},
		{Name: "Marsh refinery ledger", Kind: game.ItemQuest, At: "Marsh Refinery",
			Desc: "The refinery's account book: monthly payments to someone called 'Y.', and an order for repairs to a bell, paid to 'Nye, Providence'."},
		{Name: "Bottle of rye whiskey", Kind: game.ItemGear, Value: 3, At: "Rafe Moretti", Held: true,
			Desc: "Canadian rye, smuggled in on Rafe Moretti's launch."},
	},
	Main: Quest{
		Title:   "Salt and Silence",
		Summary: "Find out what happened to the crew of the Mary Alder, and why Innsmouth doesn't want anyone to ask.",
		Objectives: []game.ObjectiveSpec{
			{Kind: game.ObjObtainItem, Target: "Logbook of the Mary Alder", Text: "Search the Mary Alder at Innsmouth Harbor and recover her logbook"},
			{Kind: game.ObjTalkToNPC, Target: "Zadok Allen", Text: "Get old Zadok Allen to tell what he knows about the town and the sea"},
			{Kind: game.ObjObtainItem, Target: "Marsh refinery ledger", Text: "Get the ledger from the Marsh Refinery's office"},
			{Kind: game.ObjReachLocation, Target: "Falcon Point", Text: "Go to Falcon Point at low tide and see what answers the bell"},
			{Kind: game.ObjTalkToNPC, Target: "Eliza Pell", Text: "Learn from Eliza Pell when the way under the Order Hall runs dry"},
		},
		// a story milestone lifts everyone, so the act carries them from 1 to 4
		Milestones: map[int]int{2: 2, 4: 3, 6: 4},
	},
	Side: []Quest{
		{Title: "The Widow's Lamp", Giver: "Mrs. Waite", Solo: true, XP: 40, Gold: 10,
			Summary: "For one investigator. Mrs. Waite asks a stranger to keep her dead husband's lamp burning through the night.",
			Objectives: []game.ObjectiveSpec{
				{Kind: game.ObjFreeform, Text: "Keep watch with Mrs. Waite's lamp through a night and learn what it keeps away"}}},
		{Title: "Ledger Work", Giver: "Mr. Babson", Solo: true, XP: 40, Gold: 20,
			Summary: "For one investigator. Mr. Babson will pay an outsider to help balance the refinery's books, no questions asked.",
			Objectives: []game.ObjectiveSpec{
				{Kind: game.ObjObtainItem, Target: "Marsh refinery ledger", Text: "Get hold of the refinery ledger for Mr. Babson"}}},
		{Title: "Zadok's Bottle", Giver: "Zadok Allen", Solo: true, XP: 30,
			Summary: "For one investigator. In a dry town run by the Order, find Zadok a bottle without anyone noticing.",
			Objectives: []game.ObjectiveSpec{
				{Kind: game.ObjObtainItem, Target: "Bottle of rye whiskey", Text: "Get a bottle of rye whiskey from the rum-runner at the harbor"},
				{Kind: game.ObjDeliverItem, Target: "Bottle of rye whiskey", DeliverTo: "Zadok Allen", Text: "Bring the bottle to Zadok Allen"}}},
		{Title: "Light on the Reef", Giver: "Eliza Pell", XP: 60, Gold: 20,
			Summary: "For the whole party. Help Eliza Pell keep Gull Point Light burning through a storm night while things climb the rocks.",
			Objectives: []game.ObjectiveSpec{
				{Kind: game.ObjFreeform, Text: "Keep Gull Point Light burning through the storm night and drive off what climbs the rocks"}}},
	},
	Secrets: []Secret{
		{Title: "Mr. Nye", Keywords: []string{"nye", "bell-founder", "providence"},
			Text: "Mr. Nye is charming, helpful and endlessly curious about the Order's bell. He offers to take the bell somewhere safe if anyone gets it. " +
				"He never explains himself and is never caught in a lie. Play him as a gentleman with no fear at all."},
		{Title: "Eliza Pell's signals", Keywords: []string{"lighthouse", "signal", "reef", "eliza"},
			Text: "Eliza Pell signals the reef every night with the lamp, trying to call the drowned back the way her brother never came back. She knows the crypt under the Order Hall floods at high tide and is dry for an hour at low water."},
		{Title: "The bargain of 1846", Gate: "main:2", Keywords: []string{"1846", "bargain", "obed", "marsh", "deep ones", "dagon"},
			Text: "In 1846 Captain Obed Marsh made a bargain with the Deep Ones of Devil Reef: fish and gold for sacrifices, and later for marriages. " +
				"The town that refused was drowned in a single night. The Esoteric Order of Dagon keeps the bargain to this day."},
		{Title: "The ledger's payments", Gate: "main:3", Keywords: []string{"ledger", "gold", "payments", "nye"},
			Text: "The refinery pays 'Y.' (Y'ha-nthlei, the city under the reef) in kind every month. This spring it paid 'Nye, Providence' to recast the Order Hall's bell. The bell has rung at low tide ever since."},
		{Title: "The bell calls them up", Gate: "main:4", Keywords: []string{"bell", "rite", "crew", "ring"},
			Text: "Father Obed Marsh II, great-grandson of the captain, rings the recast bell at low tide. It calls the Deep Ones up out of the sea, " +
				"and it called the Mary Alder's crew down into it. With each ring more come ashore. He means to bring them up in force."},
		{Title: "Father Obed", Gate: "main:5", Keywords: []string{"obed", "priest", "crypt"},
			Text: "Father Obed Marsh II is half-changed under his gloves and collar. He keeps the bell's clapper in the flooded crypt under the Order Hall and holds the rite there at low tide. If he rings it a third time in a fight, something answers from the crypt pool."},
	},
	Beats: []Beat{
		{Title: "The Mary Alder", Trigger: game.TriggerEnterLocation, Target: "Innsmouth Harbor",
			Text: "The Mary Alder sits at her mooring with her hold full of fish packed in salt, and gold coins among the fish. No one on the quay will look at her."},
		{Title: "Watched", Trigger: game.TriggerObjectiveDone, Target: "main:1", Effects: []string{},
			Text: "From now on a man with the Innsmouth look is always somewhere behind the party: on a corner, at a window, on the quay. He never comes closer."},
		{Title: "Low water tonight", Trigger: game.TriggerObjectiveDone, Target: "main:3",
			Text: "Tonight's tide is the lowest of the month. At dusk, every shutter on Washington Street closes, and far off, the Order Hall bell begins to ring."},
		{Title: "What answers the bell", Trigger: game.TriggerEnterLocation, Target: "Falcon Point", Effects: []string{game.EffectBringNPC}, Figure: "Tom Coyle",
			Text: "In the low-tide dark, the Mary Alder's crew stand in the water off Falcon Point, waist-deep and unmoving, facing town. Tom Coyle is nearest. When the bell rings, they take one step further out."},
		{Title: "The night at the Gilman House", Trigger: game.TriggerTension, Value: 7, Effects: []string{game.EffectBringNPC, game.EffectAmbush}, Figure: "The Order's men",
			Text: "The Order has decided. Doors open softly in the night, and the Order's men come for the party: at the Gilman House through the connecting doors, anywhere else out of the fog."},
		{Title: "The federal agent", Trigger: game.TriggerTurn, Value: 45, Effects: []string{game.EffectBringNPC}, Figure: "Agent Ruth Halloway",
			Text: "A federal agent finds the party and asks what they know about Innsmouth gold. She wants it quiet, and she wants it soon."},
		{Title: "A letter from Hannah", Trigger: game.TriggerStalled, Value: 6,
			Text: "Hannah Coyle slips the party a note: an old fisherman in a shack on Water Street, Zadok Allen, talks to anyone who brings him a bottle."},
	},
	Boss: Boss{
		Person: Person{Name: "Father Obed Marsh", Role: "priest of the Esoteric Order of Dagon", At: "The Flooded Crypt",
			Desc:  "The Order's young priest, in a gold tiara and a wet white surplice. His gloved hand rests on the bell's great clapper.",
			Stats: &game.StatBlock{MaxHP: 50, AC: 13, AttackBonus: 5, Damage: "1d8+2", Inflicts: game.CondFrightened, HorrorRank: 1}},
		Move: "The Bell's Third Ring",
		Phases: []game.BossPhase{
			{Name: "The rite", Line: ""},
			{Name: "The bell swings back", Line: "Father Obed strikes the clapper again; the crypt pool boils and he fights harder"},
			{Name: "What answers", Line: "The third ring sounds, and the Deep Ones rise out of the crypt pool to answer it"},
		},
		Minion:       "Deep One",
		MinionHorror: 2,
	},
}
