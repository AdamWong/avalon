package main

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/gorilla/websocket"
)

var sessionInstances = make(map[string]*Session)

var goodToEvil = map[int]int{
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
	Good     []*Player
	Evil     []*Player
}

func (s *Session) Start(setup Setup) error {
	rand.Shuffle(len(s.Players), func(i, j int) {
		s.Players[i], s.Players[j] = s.Players[j], s.Players[i]
	})

	var (
		roles     Roles
		knownEvil []string
		seenEvil  []string
		merlin    []string
	)
	for i, player := range s.Players {
		if i < goodToEvil[len(s.Players)] {
			if setup.Merlin == true && roles.Assassin == nil {
				roles.Assassin = player
				knownEvil = append(knownEvil, player.ID)
				seenEvil = append(seenEvil, player.ID)
			} else if setup.Morgana == true && roles.Morgana == nil {
				roles.Morgana = player
				knownEvil = append(knownEvil, player.ID)
				seenEvil = append(seenEvil, player.ID)
				merlin = append(merlin, player.ID)
			} else if setup.Mordred == true && roles.Mordred == nil {
				roles.Mordred = player
				knownEvil = append(knownEvil, player.ID)
			} else if setup.Oberon == true && roles.Oberon == nil {
				roles.Oberon = player
				seenEvil = append(seenEvil, player.ID)
			} else {
				knownEvil = append(knownEvil, player.ID)
				seenEvil = append(seenEvil, player.ID)
			}
			player.Role = "Evil"
			roles.Evil = append(roles.Evil, player)
		} else {
			if setup.Merlin == true && roles.Merlin == nil {
				player.Role = "Merlin"
				roles.Merlin = player
				merlin = append(merlin, player.ID)
			} else if setup.Percival == true && roles.Percival == nil {
				player.Role = "Percival"
				roles.Percival = player
			}
			player.Role = "Good"
			roles.Good = append(roles.Good, player)
		}
	}
	s.Roles = roles

	for _, player := range s.Players {
		var roleInfo string
		if roles.Merlin == player {
			roleInfo = fmt.Sprintf("You are Merlin. The following players are Evil:\n%s", strings.Join(seenEvil, "\n"))
		} else if roles.Percival == player {
			rand.Shuffle(len(merlin), func(i, j int) {
				merlin[i], merlin[j] = merlin[j], merlin[i]
			})
			roleInfo = fmt.Sprintf("You are Percival. Merlin is among the following players:\n%s", strings.Join(merlin, "\n"))
		} else if roles.Morgana == player {
			roleInfo = fmt.Sprintf("You are Morgana. The following players are Evil:\n%s", strings.Join(knownEvil, "\n"))
		} else if roles.Mordred == player {
			roleInfo = fmt.Sprintf("You are Mordred. The following players are Evil:\n%s", strings.Join(knownEvil, "\n"))
		} else if roles.Oberon == player {
			roleInfo = fmt.Sprintf("You are Oberon.")
		} else if roles.Percival == player {
			roleInfo = fmt.Sprintf("You are Mordred. The following players are Evil:\n%s", strings.Join(knownEvil, "\n"))
		} else if roles.Assassin == player {
			roleInfo = fmt.Sprintf("You are the Assassin. The following players are Evil:\n%s", strings.Join(knownEvil, "\n"))
		} else if player.Role == "Evil" {
			roleInfo = fmt.Sprintf("You are on the side of Evil. The following players are Evil:\n%s", strings.Join(knownEvil, "\n"))
		} else if player.Role == "Good" {
			roleInfo = fmt.Sprintf("You are on the side of Good.")
		}

		player.SendText(roleInfo)
	}

	s.SendGlobalText(fmt.Sprintf("The following roles are enabled: %+v", setup))

	s.State.Picker = rand.Int() % len(s.Players)
	picker := s.Players[s.State.Picker]

	quest := Quest{
		Members: make([]string, roundRules[len(s.State.Quests)][len(s.Players)][0]),
	}
	s.State.Quests = append(s.State.Quests, &quest)
	s.SendGlobalText(fmt.Sprintf("It is %s's turn to pick a team of %d", picker.ID, len(quest.Members)))

	return nil
}

func (s *Session) SendGlobalText(message string) {
	for _, player := range s.Players {
		player.SendText(message)
	}
}

func (s *Session) UpdatePlayerList() {
	var playerList []string
	for _, player := range s.Players {
		playerList = append(playerList, player.ID)
	}

	for _, player := range s.Players {
		websocket.WriteJSON(player.Conn, struct {
			Type    string   `json:"type"`
			Players []string `json:"players"`
		}{
			Type:    "players",
			Players: playerList,
		})
	}
}
