package search

import (
	"chukuparser/util"
	"fmt"
	"log"
	"sync"
)

const (
	MAX_TRANSITIONS = 800
)

var AllOut bool = false

type Agenda interface {
	AddCandidates([]Candidate, Candidate, int) (Candidate, int)
	Contains(Candidate) bool
	Len() int
	Clear()
}

type Problem interface{}

type Candidate interface {
	Copy() Candidate
	Equal(Candidate) bool
	Score() int64
	Len() int
}

type Aligned interface {
	Alignment() int
}

type Candidates interface {
	Get(int) Candidate
	Len() int
}

type Interface interface {
	StartItem(p Problem) []Candidate
	Clear(Agenda) Agenda
	Insert(cs chan Candidate, a Agenda) []Candidate //Agenda
	Expand(c Candidate, p Problem, candidateNum int) chan Candidate
	Top(a Agenda) Candidate
	Best(a Agenda) Candidate
	GoalTest(p Problem, c Candidate, rounds int) bool
	TopB(a Agenda, B int) []Candidate
	Concurrent() bool
	SetEarlyUpdate(int)

	Name() string
	Aligned() bool
}

func Search(b Interface, problem Problem, B int) Candidate {
	candidate, _ := search(b, problem, B, 1, false, nil)
	return candidate
}

func SearchEarlyUpdate(b Interface, problem Problem, B int, goldSequence Candidates) (Candidate, Candidate) {
	return search(b, problem, B, 1, true, goldSequence)
}

func search(b Interface, problem Problem, B, topK int, earlyUpdate bool, goldSequence Candidates) (Candidate, Candidate) {
	var (
		goldValue Candidate
		best      Candidate
		agenda    Agenda

		// for early update
		i                 int
		goldIndex         int
		goldExists        bool
		bestBeamCandidate Candidate
		resultsReady      chan chan int

		// for alignment
		minAgendaAlignment    int
		minCandidateAlignment int
	)
	tempAgendas := make([][]Candidate, 0, B)

	// candidates <- {STARTITEM(problem)}
	candidates := b.StartItem(problem)
	bestBeamCandidate = candidates[0]

	// verify alignment support
	if _, aligned := bestBeamCandidate.(Aligned); b.Aligned() {
		if !aligned {
			panic("Beam is aligned but candidate does not support alignment")
		} else {
			minAgendaAlignment = bestBeamCandidate.(Aligned).Alignment()
		}
	}

	// agenda <- CLEAR(agenda)
	agenda = b.Clear(agenda)
	if earlyUpdate {
		goldValue = goldSequence.Get(0)
		goldIndex = 0
	}
	// loop do
	for {
		// log.Println()
		// log.Println()
		// log.Println("At gold sequence", i)

		// early update
		if earlyUpdate {
			goldExists, bestBeamCandidate = false, nil
			if AllOut {
				log.Println("Gold:", goldValue)
			}
		}

		best = nil
		tempAgendas = tempAgendas[0:0]
		resultsReady = make(chan chan int, B)
		var wg sync.WaitGroup
		if len(candidates) > cap(tempAgendas) {
			panic(fmt.Sprintf("Should not have more candidates than the capacity of the tempAgenda: (%d,%d)\n", len(candidates), cap(tempAgendas)))
		}
		// for each candidate in candidates
		go func() {
			if b.Aligned() {
				minCandidateAlignment = candidates[0].(Aligned).Alignment()
				for _, candidate := range candidates[1:] {
					if candAlign := candidate.(Aligned).Alignment(); candAlign < minCandidateAlignment {
						minCandidateAlignment = candAlign
					}
					if earlyUpdate && candidate.Equal(goldValue) {
						goldExists = true
					}
				}
				minAgendaAlignment = -1
			}
			for i, candidate := range candidates {
				tempAgendas = append(tempAgendas, nil)
				readyChan := make(chan int, 1)
				resultsReady <- readyChan
				if b.Aligned() && candidate.(Aligned).Alignment() > minCandidateAlignment {
					if AllOut {
						log.Println("Skipping candidate", i, "due to misalignment", candidate.(Aligned).Alignment(), minCandidateAlignment)
						log.Println("Skipped candidate", candidate)
					}
					tempAgendas[i] = []Candidate{candidate}
					if !b.Concurrent() {
						best, minAgendaAlignment = agenda.AddCandidates(tempAgendas[i], best, minAgendaAlignment)
					}
					readyChan <- i
					close(readyChan)
					continue
				}
				wg.Add(1)
				go func(ag Agenda, cand Candidate, j int, doneChan chan int) {
					defer wg.Done()

					// agenda <- INSERT(EXPAND(candidate,problem),agenda)
					// tempAgendas[i] = b.Insert(b.Expand(candidate, problem, i), agenda)
					tempAgendas[j] = b.Insert(b.Expand(cand, problem, j), ag)

					doneChan <- j
					close(doneChan)
					// readyChan <- i
					// close(readyChan)
					if !b.Concurrent() {
						best, minAgendaAlignment = agenda.AddCandidates(tempAgendas[j], best, minAgendaAlignment)
					}
				}(agenda, candidate, i, readyChan)
				if !b.Concurrent() {
					wg.Wait()
					// best = agenda.AddCandidates(tempAgendas[i], best)
				}

				if earlyUpdate {
					if bestBeamCandidate == nil || candidate.Score() > bestBeamCandidate.Score() {
						// bestScore = candidate.Score()
						bestBeamCandidate = candidate
						// log.Println("Candidate is best")
					} else {
						// log.Println("Candidate is not best")
					}
					if candidate.Equal(goldValue) {
						goldExists = true
						// log.Println("Candidate is gold")
					}
				}
				// *** <POSSIBLY REDUNDANT>
				// if !b.Concurrent() {
				// 	wg.Wait()
				// }
				// *** </POSSIBLY REDUNDANT>
			}
			close(resultsReady)
		}()
		// wg.Wait()

		for readyChan := range resultsReady {
			if b.Concurrent() {
				for tempAgendaId := range readyChan {
					best, minAgendaAlignment = agenda.AddCandidates(tempAgendas[tempAgendaId], best, minAgendaAlignment)
				}
			} else {
				for _ = range readyChan {
				}
			}
		}

		// for _, tempCandidates := range tempAgendas {
		// 	agenda.AddCandidates(tempCandidates)
		// }
		i++

		// early update
		if earlyUpdate {
			if !goldExists || goldIndex+1 >= goldSequence.Len() {
				if AllOut {
					log.Println("EARLY UPDATE")
				}
				if bestBeamCandidate == nil {
					panic("Best Beam Candidate is nil")
				}
				b.SetEarlyUpdate(util.Min(goldIndex, bestBeamCandidate.Len()-1))
				best = bestBeamCandidate
				break
			} else {
				if b.Aligned() {
					if AllOut {
						log.Println("  Early Update continues, testing alignment")
						log.Println("\tMin Alignment:", minAgendaAlignment)
						log.Println("\tNext Gold Alignment:", goldSequence.Get(goldIndex).(Aligned).Alignment())
					}
					if goldSequence.Get(goldIndex).(Aligned).Alignment() == minAgendaAlignment {
						goldIndex++
						goldValue = goldSequence.Get(goldIndex)
					} else {
						if AllOut {
							log.Println("Not aligned, leaving gold as is")
						}
					}
				} else {
					goldIndex++
					goldValue = goldSequence.Get(goldIndex)
				}
			}
			// best <- TOP(AGENDA)
			best = b.Top(agenda)
		}

		// if GOALTEST(problem,best)
		if b.GoalTest(problem, best, i) || i > MAX_TRANSITIONS {
			if AllOut {
				log.Println("Next Round", i-1)
			}

			// return best
			break
		}

		// candidates <- TOP-B(agenda, B)
		candidates = b.TopB(agenda, B)

		// agenda <- CLEAR(agenda)
		agenda = b.Clear(agenda)

		if AllOut {
			log.Println("Next Round", i-1)
		}
	}
	if !earlyUpdate {
		best = b.Best(agenda)
	}
	best = best.Copy()
	agenda = b.Clear(agenda)
	return best, goldValue
}