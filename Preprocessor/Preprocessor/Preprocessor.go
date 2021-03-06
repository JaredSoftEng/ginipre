package Preprocessor

import (
	"fmt"
	"log"
)

//

// UTILITY FUNCTIONS AND STRUCTURES

type Status byte

// A Problem is a list of clauses & a number of vars.
type Problem struct {
	NbVars     int        // Total number of vars
	Clauses    []*Clause  // List of non-empty, non-unit clauses
	Status     Status     // Status of the problem. Can be trivially UNSAT (if empty clause was met or inferred by UP) or Indet.
	Units      []Lit      // List of unit literal found in the problem.
	Model      []decLevel // For each var, its inferred binding. 0 means unbound, 1 means bound to true, -1 means bound to false.
	minLits    []Lit      // For an optimisation problem, the list of lits whose sum must be minimized
	minWeights []int      // For an optimisation problem, the weight of each lit.
}

// CNF returns a DIMACS CNF representation of the problem.
func (pb *Problem) CNF() string {
	res := fmt.Sprintf("p cnf %d %d\n", pb.NbVars, len(pb.Clauses)+len(pb.Units))
	for _, unit := range pb.Units {
		res += fmt.Sprintf("%d 0\n", unit.Int())
	}
	for _, clause := range pb.Clauses {
		res += fmt.Sprintf("%s\n", clause.CNF())
	}
	return res
}

///// PROBLEM UTILITY FUNCTIONS FROM GOPHERSAT

func (pb *Problem) updateStatus(nbClauses int) {
	pb.Clauses = pb.Clauses[:nbClauses]
	if pb.Status == Undetermined && nbClauses == 0 {
		pb.Status = Sat
	}
}

func (pb *Problem) addUnit(lit Lit) {
	if lit.IsPositive() {
		if pb.Model[lit.Var()] == -1 {
			pb.Status = Unsat
			return
		}
		pb.Model[lit.Var()] = 1
	} else {
		if pb.Model[lit.Var()] == 1 {
			pb.Status = Unsat
			return
		}
		pb.Model[lit.Var()] = -1
	}
	pb.Units = append(pb.Units, lit)
}

// simplify simplifies the pure SAT problem, i.e runs unit propagation if possible.
func (pb *Problem) Simplify2() {
	nbClauses := len(pb.Clauses)
	restart := true
	for restart {
		restart = false
		i := 0
		for i < nbClauses {
			c := pb.Clauses[i]
			nbLits := c.Len()
			clauseSat := false
			j := 0
			for j < nbLits {
				lit := c.Get(j)
				if pb.Model[lit.Var()] == 0 {
					j++
				} else if (pb.Model[lit.Var()] == 1) == lit.IsPositive() {
					clauseSat = true
					break
				} else {
					nbLits--
					c.Set(j, c.Get(nbLits))
				}
			}
			if clauseSat {
				nbClauses--
				pb.Clauses[i] = pb.Clauses[nbClauses]
			} else if nbLits == 0 {
				pb.Status = Unsat
				return
			} else if nbLits == 1 { // UP
				pb.addUnit(c.First())
				if pb.Status == Unsat {
					return
				}
				nbClauses--
				pb.Clauses[i] = pb.Clauses[nbClauses]
				restart = true // Must restart, since this lit might have made one more clause Unit or SAT.
			} else { // nb lits unbound > cardinality
				if c.Len() != nbLits {
					c.Shrink(nbLits)
				}
				i++
			}
		}
	}
	pb.updateStatus(nbClauses)
}

// Preprocess main function

func (pb *Problem) Preprocess() {
	pb.SelfSub()
	pb.Subsumption()
}

// RUN self-subsuming resolution
func (pb *Problem) SelfSub() {
	log.Printf("Preprocessing... %d clauses currently", len(pb.Clauses))
	occurs := make([][]int, pb.NbVars*2)
	for i, c := range pb.Clauses {
		for j := 0; j < c.Len(); j++ {
			occurs[c.Get(j)] = append(occurs[c.Get(j)], i)
		}
	}
	log.Printf("Occurence list: %s", occurs)
	modified := true
	neverModified := true
	for modified {
		modified = false

		// for each variable
		for i := 0; i < pb.NbVars; i++ {
			if pb.Model[i] != 0 {
				continue
			}
			v := Var(i)
			lit := v.Lit()
			nbLit := len(occurs[lit])
			nbLit2 := len(occurs[lit.Negation()])

			// slow method is only effective with less than 10 literals
			if (nbLit < 10 || nbLit2 < 10) && (nbLit != 0 || nbLit2 != 0) {
				log.Printf("Examining literal: %d", lit.Int())
				// loop through the occurence list and check clauses where literals and their negations exist
				for _, idx1 := range occurs[lit] {
					for _, idx2 := range occurs[lit.Negation()] {
						log.Printf("%d can be removed: %d and %d", lit.Int(), len(occurs[lit]), len(occurs[lit.Negation()]))
						// positive clause
						c1 := pb.Clauses[idx1]
						// negative clause
						c2 := pb.Clauses[idx2]

						// determine whether self-subsuming resolution is possible for clauses (both ways)
						canP := c1.SelfSubsumes(c2)
						canN := c2.SelfSubsumes(c1)

						log.Printf("Can positive clause be self-subsumed? %t",canP)
						log.Printf("Can negative clause be self-subsumed? %t",canN)

						// if both are true then remove negative clause and literal from positive clause
						if(canP && canN){
							// generate new clause with self-subsuming resolution
							newC := c1.Generate(c2, v)
							if !newC.Simplify() {
								switch newC.Len() {
								case 0:
									log.Printf("Inferred UNSAT")
									pb.Status = Unsat
									return
								case 1:
									log.Printf("Unit %d", newC.First().Int())
									lit2 := newC.First()
									if lit2.IsPositive() {
										if pb.Model[lit2.Var()] == -1 {
											pb.Status = Unsat
											log.Printf("Inferred UNSAT")
											return
										}
										pb.Model[lit2.Var()] = 1
									} else {
										if pb.Model[lit2.Var()] == 1 {
											pb.Status = Unsat
											log.Printf("Inferred UNSAT")
											return
										}
										pb.Model[lit2.Var()] = -1
									}

									// Check if unit literal exists so that we don't add duplicates
									unitexists := false
									//log.Printf("Number of unit literals: %d",len(pb.Units))
									for idx3, _ := range pb.Units {
										if pb.Units[idx3] == lit2{
											unitexists = true
										}
									}
									if unitexists{
										// don't add it if it exists
									} else {
										pb.Units = append(pb.Units, lit2)
									}
								default:
									pb.Clauses = append(pb.Clauses, newC)
								}
							}

							// REMOVE THE LITERAL FROM POSITIVE CLAUSE AND DELETE NEGATIVE CLAUSE
							//nbRemoved := 0
							if len(occurs[lit.Negation()])>0{
								pb.Clauses[idx1] = pb.Clauses[len(pb.Clauses)-1]
								pb.Clauses = pb.Clauses[:len(pb.Clauses)-1]
								pb.Clauses[idx2] = pb.Clauses[len(pb.Clauses)-1]
								pb.Clauses = pb.Clauses[:len(pb.Clauses)-1]

								occurs = make([][]int, pb.NbVars*2)
								for i, c := range pb.Clauses {
									for j := 0; j < c.Len(); j++ {
										occurs[c.Get(j)] = append(occurs[c.Get(j)], i)
									}
								}
								modified = true
								neverModified = false
								break
							} else{
								modified = false
							}

						// if positive clause subsumes negative clause, delete literal from negative clause
						} else if(canP){
							// generate new clause with self-subsuming resolution
							newC := c1.Generate(c2, v)
							if !newC.Simplify() {
								switch newC.Len() {
								case 0:
									log.Printf("Inferred UNSAT")
									pb.Status = Unsat
									return
								case 1:
									log.Printf("Unit %d", newC.First().Int())
									lit2 := newC.First()
									if lit2.IsPositive() {
										if pb.Model[lit2.Var()] == -1 {
											pb.Status = Unsat
											log.Printf("Inferred UNSAT")
											return
										}
										pb.Model[lit2.Var()] = 1
									} else {
										if pb.Model[lit2.Var()] == 1 {
											pb.Status = Unsat
											log.Printf("Inferred UNSAT")
											return
										}
										pb.Model[lit2.Var()] = -1
									}

									// Check if unit literal exists so that we don't add duplicates
									unitexists := false
									//log.Printf("Number of unit literals: %d",len(pb.Units))
									for idx3, _ := range pb.Units {
										if pb.Units[idx3] == lit2{
											unitexists = true
										}
									}
									if unitexists{
										// don't add it if it exists
									} else {
										pb.Units = append(pb.Units, lit2)
									}
								default:
									pb.Clauses = append(pb.Clauses, newC)
								}
							}

							// REMOVE THE LITERAL FROM POSITIVE CLAUSE
							if len(occurs[lit.Negation()])>0{
								pb.Clauses[idx2] = pb.Clauses[len(pb.Clauses)-1]
								pb.Clauses = pb.Clauses[:len(pb.Clauses)-1]
								// Redo occurs
								occurs = make([][]int, pb.NbVars*2)
								for i, c := range pb.Clauses {
									for j := 0; j < c.Len(); j++ {
										occurs[c.Get(j)] = append(occurs[c.Get(j)], i)
									}
								}
								modified = true
								neverModified = false
								break
							} else{
								modified = false
							}

							// if negative clause subsumes positive clause, delete literal from positive clause
						} else if(canN){
							// generate new clause with self-subsuming resolution
							newC := c2.Generate(c1, v)
							if !newC.Simplify() {
								switch newC.Len() {
								case 0:
									log.Printf("Inferred UNSAT")
									pb.Status = Unsat
									return
								case 1:
									log.Printf("Unit %d", newC.First().Int())
									lit2 := newC.First()
									if lit2.IsPositive() {
										if pb.Model[lit2.Var()] == -1 {
											pb.Status = Unsat
											log.Printf("Inferred UNSAT")
											return
										}
										pb.Model[lit2.Var()] = 1
									} else {
										if pb.Model[lit2.Var()] == 1 {
											pb.Status = Unsat
											log.Printf("Inferred UNSAT")
											return
										}
										pb.Model[lit2.Var()] = -1
									}

									// Check if unit literal exists so that we don't add duplicates
									unitexists := false
									//log.Printf("Number of unit literals: %d",len(pb.Units))
									for idx3, _ := range pb.Units {
										if pb.Units[idx3] == lit2{
											unitexists = true
										}
									}
									if unitexists{
										// don't add it if it exists
									} else {
										pb.Units = append(pb.Units, lit2)
									}
								default:
									pb.Clauses = append(pb.Clauses, newC)
								}
							}

							// REMOVE THE LITERAL FROM NEGATIVE CLAUSE
							if len(occurs[lit.Negation()])>0{
								pb.Clauses[idx1] = pb.Clauses[len(pb.Clauses)-1]
								pb.Clauses = pb.Clauses[:len(pb.Clauses)-1]
								// Redo occurs
								occurs = make([][]int, pb.NbVars*2)
								for i, c := range pb.Clauses {
									for j := 0; j < c.Len(); j++ {
										occurs[c.Get(j)] = append(occurs[c.Get(j)], i)
									}
								}
								modified = true
								neverModified = false
								break
							} else{
								modified = false
							}
						}

					}
					if modified {
						break
					}
				}
				log.Printf("clauses=%s", pb.CNF())
				continue
			}
		}
	}
	if !neverModified {
		pb.Simplify2()
	}
	log.Printf("Done. %d clauses now", len(pb.Clauses))
}

// Simplify with Subsumption
func (pb *Problem) Subsumption() {
	log.Printf("Preprocessing... %d clauses currently", len(pb.Clauses))
	occurs := make([][]int, pb.NbVars*2)
	for i, c := range pb.Clauses {
		for j := 0; j < c.Len(); j++ {
			occurs[c.Get(j)] = append(occurs[c.Get(j)], i)
		}
	}
	log.Printf("Occurence list: %s", occurs)
	toRemove := make([]int, 0)

	// for each positive variable
	for i := 0; i < pb.NbVars; i++ {
		if pb.Model[i] != 0 {
			continue
		}
		v := Var(i)
		lit := v.Lit()
		//nbLit := len(occurs[lit])
		//nbLit2 := len(occurs[lit.Negation()])
		log.Printf("Examining literal: %d", lit.Int())
		// loop through the occurence list and compare clauses where the literals exist
		for _, idx1 := range occurs[lit] {
			for _, idx2 := range occurs[lit] {
				// clause 1
				c1 := pb.Clauses[idx1]
				// clause 2
				c2 := pb.Clauses[idx2]

				// CHECK IF POSSIBLE
				if idx1 <= idx2 {
					continue
				}
				if c1.Len() > c2.Len(){
					canP := c2.Subsumes(c1)
					log.Printf("Can clause 2 subsume clause 1? %t",canP)
					if canP{
						// Save index of clause to remove for later
						toRemove = append(toRemove, idx1)
					}

				}
				if c2.Len() > c1.Len(){
					canN := c1.Subsumes(c2)
					log.Printf("Can clause 1 subsume clause 2? %t",canN)
					if canN{
						// Save index of clause to remove for later
						toRemove = append(toRemove, idx2)
					}
				}
			}
		}

		// negative literal loop
		for _, idx1 := range occurs[lit.Negation()] {
			for _, idx2 := range occurs[lit.Negation()] {
				// clause 1
				c1 := pb.Clauses[idx1]
				// clause 2
				c2 := pb.Clauses[idx2]

				// CHECK IF POSSIBLE
				if idx1 <= idx2 {
					continue
				}
				if c1.Len() > c2.Len(){
					canP := c2.Subsumes(c1)
					log.Printf("Can clause 2 subsume clause 1? %t",canP)
					if canP{
						// Save index of clause to remove for later
						toRemove = append(toRemove, idx1)
					}

				}
				if c2.Len() > c1.Len(){
					canN := c1.Subsumes(c2)
					log.Printf("Can clause 1 subsume clause 2? %t",canN)
					if canN{
						// Save index of clause to remove for later
						toRemove = append(toRemove, idx2)
					}
				}

			}
		}
	}

	// Generate new clause list by removing all the subsumed clauses
	newClauses := make([]*Clause, 0, 0)
	match := false
	for i := 0; i < len(pb.Clauses); i++ {
		match = false
		for j := 0; j < len(toRemove); j++ {

			// REMOVE THE SUBSUMED CLAUSE
			if toRemove[j] == i{
				match = true
			}
		}
		if match == false{
			newClauses = append(newClauses,pb.Clauses[i])
		}
	}
	pb.Clauses = newClauses

	// Redo occurs
	occurs = make([][]int, pb.NbVars*2)
	for i, c := range pb.Clauses {
		for j := 0; j < c.Len(); j++ {
			occurs[c.Get(j)] = append(occurs[c.Get(j)], i)
		}
	}

	log.Printf("clauses=%s", pb.CNF())
	pb.Simplify2()
	log.Printf("Done. %d clauses now", len(pb.Clauses))
}