package disambig

import (
	. "chukuparser/alg/transition"
	. "chukuparser/nlp/types"
	"chukuparser/util"
	"fmt"
	"log"
)

type MDWBTrans struct {
	ParamFunc MDParam
	POP       Transition

	Transitions *util.EnumSet
	oracle      Oracle

	Log bool
}

var _ TransitionSystem = &MDWBTrans{}

func (t *MDWBTrans) Transition(from Configuration, transition Transition) Configuration {
	c := from.Copy().(*MDConfig)

	if transition == t.POP {
		c.Pop()
		c.SetLastTransition(transition)
		if TSAllOut || t.Log {
			log.Println("POPing")
		}
		return c
	}
	if transition == Transition(0) {
		c.SetLastTransition(transition)
		if TSAllOut || t.Log {
			log.Println("Idling")
		}
		return c
	}
	paramStr := t.Transitions.ValueOf(int(transition))
	qTop, _ := c.LatticeQueue.Peek()
	// if !qExists {
	// 	panic("Lattice queue is empty! Whatcha doin'?!")
	// }

	if TSAllOut || t.Log {
		log.Println("Qtop:", qTop, "currentNode", c.CurrentLatNode)
	}
	lattice := c.Lattices[qTop]
	if TSAllOut || t.Log {
		log.Println("At lattice", qTop, "-", lattice.Token)
		log.Println("Current lat node", c.CurrentLatNode)
	}
	nexts, _ := lattice.Next[c.CurrentLatNode]
	if TSAllOut || t.Log {
		log.Println("Nexts are", nexts)
		log.Println("Morphemes are", lattice.Morphemes)
	}
	for _, next := range nexts {
		morph := lattice.Morphemes[next]
		if TSAllOut || t.Log {
			log.Println("Comparing morpheme param val", t.ParamFunc(morph), "to", paramStr)
		}
		if t.ParamFunc(morph) == paramStr {
			if TSAllOut || t.Log {
				log.Println("Adding morph", morph)
			}
			c.AddMapping(morph)
			c.SetLastTransition(transition)
			return c
		}
	}
	var panicStr string
	panicStr = "transition did not match a given morpheme :`( -- "
	panicStr += fmt.Sprintf("failed to transition to %v", paramStr)
	panic(panicStr)
}

func (t *MDWBTrans) TransitionTypes() []string {
	return []string{"MD-*"}
}

func (t *MDWBTrans) possibleTransitions(from Configuration, transitions chan Transition) {
	var transition int

	conf, ok := from.(*MDConfig)
	if !ok {
		panic("Got wrong configuration type")
	}
	qTop, qExists := conf.LatticeQueue.Peek()
	if (!qExists && len(conf.Mappings) != conf.popped) ||
		(qExists && qTop != conf.popped) {
		transitions <- t.POP
	} else {
		if qExists {
			lat := conf.Lattices[qTop]
			if conf.CurrentLatNode < lat.Top() {
				nextList, _ := lat.Next[conf.CurrentLatNode]
				if t.Log {
					log.Println("\t\tpossible transitions", nextList)
				}
				for _, next := range nextList {
					transition, _ = t.Transitions.Add(t.ParamFunc(lat.Morphemes[next]))
					transitions <- Transition(transition)
				}
			}
		} else {
			// if t.Log {
			// 	log.Println("\t\tpossible transitions IDLE")
			// }
			// transitions <- Transition(0)
		}
	}
	close(transitions)
}

func (a *MDWBTrans) GetTransitions(from Configuration) []int {
	retval := make([]int, 0, 10)
	transitions := a.YieldTransitions(from)
	for transition := range transitions {
		retval = append(retval, int(transition))
	}
	return retval
}

func (t *MDWBTrans) YieldTransitions(conf Configuration) chan Transition {
	transitions := make(chan Transition)
	go t.possibleTransitions(conf, transitions)
	return transitions
}

func (t *MDWBTrans) Oracle() Oracle {
	return t.oracle
}

func (t *MDWBTrans) AddDefaultOracle() {
	t.oracle = &MDWBOracle{
		Transitions: t.Transitions,
		ParamFunc:   t.ParamFunc,
	}
}

func (t *MDWBTrans) Name() string {
	return "Morphological Disambiguator"
}

type MDWBOracle struct {
	Transitions *util.EnumSet
	gold        Mappings
	ParamFunc   MDParam
}

var _ Decision = &MDWBOracle{}

func (o *MDWBOracle) SetGold(g interface{}) {
	mappings, ok := g.(Mappings)
	if !ok {
		panic("Gold is not an array of mappings")
	}
	o.gold = mappings
}

func (o *MDWBOracle) Transition(conf Configuration) Transition {
	c := conf.(*MDConfig)

	if o.gold == nil {
		panic("Oracle needs gold reference, use SetGold")
	}

	qTop, qExists := c.LatticeQueue.Peek()
	if (!qExists && len(c.Mappings) != c.popped) ||
		(qExists && qTop != c.popped) {
		return c.POP
	}
	if !qExists {
		// oracle forces a single final idle
		// if c.Last != Transition(0) {
		// 	return Transition(0)
		// }
		panic("No lattices in given configuration to disambiguate")
	}
	if len(o.gold) <= qTop {
		panic("Gold has less mappings than given configuration")
	}
	goldSpellout := o.gold[qTop].Spellout

	confSpellout := c.Mappings[len(c.Mappings)-1].Spellout
	// log.Println("Confspellout")
	// log.Println(confSpellout)
	// log.Println("At lattice", qTop, "mapping", len(confSpellout))
	// log.Println("GoldSpellout", goldSpellout)
	// log.Println("len(confSpellout)", len(confSpellout))
	// currentMorph := goldSpellout[len(confSpellout)]
	// log.Println("Gold morpheme", currentMorph.Form)
	paramVal := o.ParamFunc(goldSpellout[len(confSpellout)])
	// log.Println("Gold transition", paramVal)
	transition, _ := o.Transitions.Add(paramVal)
	return Transition(transition)
}

func (o *MDWBOracle) Name() string {
	return "MD Exact Match"
}