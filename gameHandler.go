package SecretGopher

import (
	"math/rand"
)

// NewGame creates a game structure and subscribes a goroutine to listen to the events for the game
func NewGame() Game {
	in := make(chan Event)
	out := make(chan Output)
	go handleGame(in, out)
	return Game{
		in:  in,
		out: out,
	}
}

func gameOver(fTracker, lTracker, chancellor int8, killed Set, roles []Role) GameEnding {
	// check the fascist policies
	if fTracker == 6 {
		return FascistPolicyWin
		// check the liberal policies
	} else if lTracker == 5 {
		return LiberalPolicyWin
		// check if hitler is chancellor
	} else if roles[chancellor] == Hitler {
		return FascistElectionWin
		// check if hitler is dead
	} else {
		for hitler, v := range roles { // find the player who is hitler
			if v == Hitler {
				if killed.has(hitler) {
					return LiberalExecutionWin
				}
				break
			}
		}
		// if none of the above succeed then the game is still running
		return StillRunning
	}
}

func enactPolicy(p Policy, players int8, lTracker, fTracker *int8) SpecialPowers {
	s := Nothing // special powers checked only when the policy is fascist
	switch p {
	case LiberalPolicy:
		*lTracker++
	case FascistPolicy:
		*fTracker++
		switch players {
		case 5, 6:
			s = powersTable[0][*fTracker]
		case 7, 8:
			s = powersTable[1][*fTracker]
		case 9, 10:
			s = powersTable[2][*fTracker]
		}
	}
	return s
}

// handleGame handles the game events.
func handleGame(in <-chan Event, out chan<- Output) {
	// information about the game is kept in the handler to ensure thread safety
	var (
		state         state    = waitingPlayers
		players       int8     = 0
		deck          Deck     // initialized with Start
		president     int8     = NotSet
		chancellor    int8     = NotSet
		roles         []Role   // initialized with Start
		nextPresident int8     = NotSet
		oldGov        Set      = make(Set, 2)
		killed        Set      // initialized with Start
		investigated  Set      // initialized with Start
		votes         []Vote   // initialized with Start
		voteCounter   int8     = 0
		policyChoice  []Policy // initialized when entering presidentLegislation
		eTracker      int8     = 0
		fTracker      int8     = 0
		lTracker      int8     = 0
	)
	for {
		event := <-in
		switch event.(type) {
		case AddPlayer:
			// if the game is accepting players
			if state == waitingPlayers {
				if players < 10 {
					players++                                  // adds a player to the game
					out <- Ok{Info: PlayerRegistered(players - 1)} // say the player was registered under the player number
				} else {
					out <- Error{Err: GameFull{}} // send out error
				}
			} else {
				out <- Error{Err: WrongPhase{}} // send out error
			}
		case Start:
			// if the game was accepting players
			if state == waitingPlayers {
				if players >= 5 {
					roles = make([]Role, players) // initialize roles to the proper size
					votes = make([]Vote, players) // initialize votes to the proper size
					oldGov = make(Set, 2)         // initialize oldGov to the proper size
					killed = make(Set, 2)         // initialize killed to the proper size
					deck = newDeck()              // initialize deck and shuffle it

					roles[rand.Intn(int(players))] = Hitler // set one player as Hitler
					var nF int                              // number of fascists based on the lobby size
					switch players {
					case 5, 6:
						investigated = nil
						nF = 1
					case 7, 8:
						investigated = make(Set, 1)
						nF = 2
					case 9, 10:
						investigated = make(Set, 2)
						nF = 3
					}
					// assign nF FascistParty roles randomly
					for i := 0; i < nF; {
						// extract a player
						// if the role for that player is not FascistParty or Hitler, set him as FascistParty
						// and increase the counter
						if r := rand.Intn(int(players)); roles[r] == LiberalParty {
							roles[r] = FascistParty
							i++
						}
					}
					// the first player to be president is random
					president = int8(rand.Intn(int(players)))
					// set the next president in line
					nextPresident = (president + 1) % players

					state = chancellorCandidacy // after a president is selected, a chancellor needs to be selected

					out <- Ok{Info: GameStart(&GameState{
						ElectionTracker: NotSet,
						FascistTracker:  NotSet,
						LiberalTracker:  NotSet,
						President:       president,
						Chancellor:      NotSet,
						Roles:           append([]Role{}, roles...), // clone the roles
					})} // tell the caller the game has started
				}
			} else {
				out <- Error{Err: WrongPhase{}} // send out error
			}
		case MakeChancellor:
			// if the game was accepting players
			if state == chancellorCandidacy {
				e := event.(MakeChancellor)
				if e.Caller == president {
					if !oldGov.has(e.Proposal) {
						chancellor = e.Proposal
						state = governmentElection
						out <- Ok{Info: ElectionStart(&GameState{
							ElectionTracker: eTracker,
							FascistTracker:  fTracker,
							LiberalTracker:  lTracker,
							President:       president,
							Chancellor:      chancellor,
							Roles:           append([]Role{}, roles...), // clone the roles
						})} // say the chancellor registration was successful
					} else {
						out <- Error{Err: Invalid{}} // send out error
					}
				} else {
					out <- Error{Err: Unauthorized{}} // send out error
				}
			} else {
				out <- Error{Err: WrongPhase{}} // send out error
			}
		case GovernmentVote:
			// if the game is waiting for votes on the election
			if state == governmentElection {
				e := event.(GovernmentVote)
				// check that the vote is valid
				if v := e.Vote; v == Ja || v == Nein {
					// if the user hasn't voted yet
					if votes[e.Caller] == NoVote {
						votes[e.Caller] = v // register the vote
						voteCounter++       // increase the vote counter
						// if all players have cast a vote
						if voteCounter == players {
							// add up the votes
							var r int8 = 0
							for _, v := range votes {
								switch v {
								case Ja:
									r++
								case Nein:
									r--
								}
							}
							// if r is greater than 0 the election has passed
							if r > 0 {
								// update the term limits for the next election
								oldGov.clear()
								oldGov.addAll(president, chancellor)

								// checks if the game is over (if hitler is chancellor)
								if o := gameOver(fTracker, lTracker, chancellor, killed, roles); o != StillRunning {
									out <- Ok{Info: GameEnd{
										Why: o,
										State: &GameState{
											ElectionTracker: eTracker,
											FascistTracker:  fTracker,
											LiberalTracker:  lTracker,
											President:       president,
											Chancellor:      chancellor,
											Roles:           append([]Role{}, roles...), // clone the roles
										},
									}}
									return // end the game
								}
								state = presidentLegislation // next step is to let the president choose a card to discard
								policyChoice = deck.draw(3)
								// send a successful election result and notify the cards the president has to choose from
								// in the field 'Hand'
								out <- Ok{Info: LegislationPresident{
									Hand: append([]Policy{}, policyChoice...), // clone the policy choice
									State: &GameState{
										ElectionTracker: eTracker,
										FascistTracker:  fTracker,
										LiberalTracker:  lTracker,
										President:       president,
										Chancellor:      chancellor,
										//Roles:         append([]Role{}, roles...), // clone the roles
									},
								}}
							} else {
								state = chancellorCandidacy // next step is to start a new round
								// set the next president in line
								president = nextPresident
								// calculate the next president in a circular fashion
								nextPresident = (president + 1) % players
								// advance the election tracker
								// if advancing it triggers a forced policy enaction, do that first
								if eTracker == 2 {
									eTracker = 0
									policyChoice = deck.draw(1) // draw the policy to force
									enactPolicy(policyChoice[0], players, &lTracker, &fTracker)

									// checks if the game is over (if the policy limit for a party has been reached)
									if o := gameOver(fTracker, lTracker, chancellor, killed, roles); o != StillRunning {
										out <- Ok{Info: GameEnd{
											Why: o,
											State: &GameState{
												ElectionTracker: eTracker,
												FascistTracker:  fTracker,
												LiberalTracker:  lTracker,
												President:       president,
												Chancellor:      chancellor,
												Roles:           append([]Role{}, roles...), // clone the roles
											},
										}}
										return
									}

									// send a failed election result and notify there was a forced policy enaction
									out <- Ok{Info: PolicyEnaction{
										Enacted:      policyChoice[0], // election failed
										SpecialPower: Nothing,
										State: &GameState{
											ElectionTracker: eTracker,
											FascistTracker:  fTracker,
											LiberalTracker:  lTracker,
											President:       president,
											Chancellor:      chancellor,
											//Roles:           append([]Role{}, roles...), // clone the roles
										},
									}}
								} else {
									eTracker++
									// send a failed election result and notify there was NOT a forced policy enaction by leaving
									// Hand nil
									out <- Ok{Info: NextPresident(
										&GameState{
											ElectionTracker: eTracker,
											FascistTracker:  fTracker,
											LiberalTracker:  lTracker,
											President:       president,
											//Chancellor: chancellor,
											//Hand:       append([]Policy{}, policyChoice...), // clone the policy choice
											//Roles:           append([]Role{}, roles...), // clone the roles
										},
									)}
								}
							}
						} else {
							out <- Ok{Info: General{}} // vote has been registered
						}
					} else {
						// unauthorized vote as user has already voted
						out <- Error{Err: Unauthorized{}} // send out error
					}
				} else {
					out <- Error{Err: Invalid{}} // invalid vote error
				}
			} else {
				out <- Error{Err: WrongPhase{}} // send out error
			}
		case PolicyDiscard:
			e := event.(PolicyDiscard)
			switch state {
			case presidentLegislation:
				if e.Caller == president {
					if s := e.Selection; s < 3 {
						policyChoice = append(policyChoice[:s], policyChoice[s+1:]...)
						// send a successful result and notify the chancellor has to choose from
						// the field 'Hand'
						out <- Ok{Info: LegislationChancellor{
							Hand:            append([]Policy{}, policyChoice...), // clone the policy choice
							State: &GameState{
							ElectionTracker: eTracker,
							FascistTracker:  fTracker,
							LiberalTracker:  lTracker,
							President:       president,
							Chancellor:      chancellor,
						}}}
					}
				} else {
					out <- Error{Err: Unauthorized{}} // send out error
				}
			case chancellorLegislation:
				if e.Caller == chancellor {
					if s := e.Selection; s < 2 {
						policyChoice = append(policyChoice[:s], policyChoice[s+1:]...)
						// todo veto check
						s := enactPolicy(policyChoice[0], players, &lTracker, &fTracker) // s is the special power
						// checks if the game is over (if the policy limit for a party has been reached)
						if o := gameOver(fTracker, lTracker, chancellor, killed, roles); o != StillRunning {
							out <- Ok{Info: GameEnd{
								Why: o,
								State: &GameState{
									ElectionTracker: eTracker,
									FascistTracker:  fTracker,
									LiberalTracker:  lTracker,
									President:       president,
									Chancellor:      chancellor,
									Roles:           append([]Role{}, roles...),          // clone the roles
								},
							}}
							return
						}
						// update the state of the game in accordance to the special power
						switch s {
						case Nothing:
							state = chancellorCandidacy
							// set the next president in line
							president = nextPresident
							// calculate the next president in a circular fashion
							nextPresident = (president + 1) % players
						case Execution: state = specialExecution
						case Election: state = specialElection
						case Investigate: state = specialInvestigate
						case Peek: state = specialPeek
						}
						
						// send a successful result for the enaction of a policy
						out <- Ok{Info: PolicyEnaction{
							Enacted:      policyChoice[0],
							SpecialPower: s,
							State:        &GameState{
								ElectionTracker: eTracker,
								FascistTracker:  fTracker,
								LiberalTracker:  lTracker,
								President:       president,
								Chancellor:      chancellor,
								Roles:           append([]Role{}, roles...),
							},
						}}
					}
				} else {
					out <- Error{Err: Unauthorized{}} // send out error
				}
			default:
				out <- Error{Err: WrongPhase{}} // send out error
			}
		case SpecialPower:
			e := event.(SpecialPower)
			if e.Caller == president {
				switch e.Power {
				case Peek:
					if state == specialPeek {
						out <- Ok{Info: SpecialPowerFeedback{
							Feedback: deck.peek(),
							State:    &GameState{
								ElectionTracker: eTracker,
								FascistTracker:  fTracker,
								LiberalTracker:  lTracker,
								President:       president,
								Chancellor:      chancellor,
								Roles:           append([]Role{}, roles...),
							},
						}} // send out error
					} else {
						out <- Error{Err: WrongPhase{}} // send out error
					}
				case Election:
					if state == specialElection {
						// the president cannot choose himself
						if e.Selection < players && e.Selection != president {
							president = e.Selection
							state = chancellorCandidacy
							out <- Ok{Info: SpecialPowerFeedback{
								Feedback: deck.peek(),
								State: &GameState{
									ElectionTracker: eTracker,
									FascistTracker:  fTracker,
									LiberalTracker:  lTracker,
									President:       president,
									Chancellor:      chancellor,
									Roles:           append([]Role{}, roles...),
								},
							}}
						} else {
							out <- Error{Err: Invalid{}} // send out error
						}
					} else {
						out <- Error{Err: WrongPhase{}} // send out error
					}
				case Execution:
					if state == specialExecution {
						if e.Selection < players && !killed.has(e.Selection) {
							killed.add(e.Selection)
							state = chancellorCandidacy
							out <- Ok{Info: SpecialPowerFeedback{
								State: &GameState{
									ElectionTracker: eTracker,
									FascistTracker:  fTracker,
									LiberalTracker:  lTracker,
									President:       president,
									Chancellor:      chancellor,
									Roles:           append([]Role{}, roles...),
								},
							}}
						} else {
							out <- Error{Err: Invalid{}} // send out error
						}
					} else {
						out <- Error{Err: WrongPhase{}} // send out error
					}
				case Investigate:
					if state == specialInvestigate {
						if e.Selection < players && !investigated.has(e.Selection) {
							investigated.add(e.Selection)
							state = chancellorCandidacy
							out <- Ok{Info: SpecialPowerFeedback{
								Feedback: roles[e.Selection],
								State: &GameState{
									ElectionTracker: eTracker,
									FascistTracker:  fTracker,
									LiberalTracker:  lTracker,
									President:       president,
									Chancellor:      chancellor,
									Roles:           append([]Role{}, roles...),
								},
							}}
						} else {
							out <- Error{Err: Invalid{}} // send out error
						}
					} else {
						out <- Error{Err: WrongPhase{}} // send out error
					}
				default:
					out <- Error{Err: Invalid{}} // send out error
				}
			} else {
				out <- Error{Err: Unauthorized{}} // send out error
			}
		default:
			out <- Error{Err: Invalid{}} // send out error for invalid event
		}
	}
}