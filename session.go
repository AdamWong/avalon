package main

import (
	"fmt"
	"math/rand"
	"strings"
)

var sessionInstances = make(map[string]*Session)

var resistanceToSpies = map[int]int{
	5:  2,
	6:  2,
	7:  3,
	8:  3,
	9:  3,
	10: 4,
}

type Session struct {
	ID      string
	Admin   *Player
	Roles   Roles
	State   State
	Players []*Player
}

type Roles struct {
	Merlin   *Player
	Percival *Player
	Assassin *Player
	Morgana  *Player
	Mordred  *Player
	Oberon   *Player
	Rebels   []*Player
	Spies    []*Player
}

func (s *Session) Start(setup Setup) error {
	rand.Shuffle(len(s.Players), func(i, j int) {
		s.Players[i], s.Players[j] = s.Players[j], s.Players[i]
	})

	var (
		roles      Roles
		knownSpies []string
		seenSpies  []string
		merlin     []string
	)
	for i, player := range s.Players {
		if i < resistanceToSpies[len(s.Players)] {
			if setup.Merlin == true && roles.Assassin == nil {
				roles.Assassin = player
				knownSpies = append(knownSpies, player.ID)
				seenSpies = append(seenSpies, player.ID)
			} else if setup.Morgana == true && roles.Morgana == nil {
				roles.Morgana = player
				knownSpies = append(knownSpies, player.ID)
				seenSpies = append(seenSpies, player.ID)
				merlin = append(merlin, player.ID)
			} else if setup.Mordred == true && roles.Mordred == nil {
				roles.Mordred = player
				knownSpies = append(knownSpies, player.ID)
			} else if setup.Oberon == true && roles.Oberon == nil {
				roles.Oberon = player
				seenSpies = append(seenSpies, player.ID)
			} else {
				knownSpies = append(knownSpies, player.ID)
				seenSpies = append(seenSpies, player.ID)
			}
			player.Role = "Spy"
			roles.Spies = append(roles.Spies, player)
		} else {
			if setup.Merlin == true && roles.Merlin == nil {
				player.Role = "Merlin"
				roles.Merlin = player
				merlin = append(merlin, player.ID)
			} else if setup.Percival == true && roles.Percival == nil {
				player.Role = "Percival"
				roles.Percival = player
			}
			player.Role = "Resistance"
			roles.Rebels = append(roles.Rebels, player)
		}
	}
	s.Roles = roles

	for _, player := range s.Players {
		var roleInfo string
		if roles.Merlin == player {
			roleInfo = fmt.Sprintf("You are Merlin. The following players are spies:\n%s", strings.Join(seenSpies, "\n"))
		} else if roles.Percival == player {
			rand.Shuffle(len(merlin), func(i, j int) {
				merlin[i], merlin[j] = merlin[j], merlin[i]
			})
			roleInfo = fmt.Sprintf("You are Percival. Merlin is among the following players:\n%s", strings.Join(merlin, "\n"))
		} else if roles.Morgana == player {
			roleInfo = fmt.Sprintf("You are Morgana. The following players are spies:\n%s", strings.Join(knownSpies, "\n"))
		} else if roles.Mordred == player {
			roleInfo = fmt.Sprintf("You are Mordred. The following players are spies:\n%s", strings.Join(knownSpies, "\n"))
		} else if roles.Oberon == player {
			roleInfo = fmt.Sprintf("You are Oberon.")
		} else if roles.Percival == player {
			roleInfo = fmt.Sprintf("You are Mordred. The following players are spies:\n%s", strings.Join(knownSpies, "\n"))
		} else if roles.Assassin == player {
			roleInfo = fmt.Sprintf("You are the Assassin. The following players are spies:\n%s", strings.Join(knownSpies, "\n"))
		} else if player.Role == "Spy" {
			roleInfo = fmt.Sprintf("You are a Spy. The following players are spies:\n%s", strings.Join(knownSpies, "\n"))
		} else if player.Role == "Resistance" {
			roleInfo = fmt.Sprintf("You are a Resistance member.")
		}

		player.SendText(roleInfo)
	}

	for _, player := range s.Players {
		player.SendText(fmt.Sprintf("The following roles are enabled: %+v", setup))
	}

	s.State.Picker = rand.Int() % len(s.Players)
	picker := s.Players[s.State.Picker]

	quest := Quest{
		Members:   make([]string, roundRules[len(s.State.Quests)][len(s.Players)][0]),
		Approvals: make(map[string]bool),
		Successes: make(map[string]bool),
	}
	s.State.Quests = append(s.State.Quests, &quest)
	for _, player := range s.Players {
		player.SendText(fmt.Sprintf("It is %s's turn to pick a team of %d", picker.ID, len(quest.Members)))
	}

	return nil
}
