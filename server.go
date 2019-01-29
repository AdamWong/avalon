package main

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"encoding/json"
	"html/template"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"

	"github.com/gorilla/websocket"
)

var addr = flag.String("addr", "localhost:8080", "http service address")

var upgrader = websocket.Upgrader{CheckOrigin: checkOrigin} // use default options

var sessionInstances = make(map[string]*Session)

var resistanceToSpies = map[int]int{
	5:  2,
	6:  2,
	7:  3,
	8:  3,
	9:  3,
	10: 4,
}

// rounds to number of players to number of team members to number of fails needed
var roundRules = []map[int][]int{
	{5: {2, 1}, 6: {2, 1}, 7: {2, 1}, 8: {3, 1}},
	{5: {3, 1}, 6: {3, 1}, 7: {3, 1}, 8: {4, 1}},
	{5: {2, 1}, 6: {4, 1}, 7: {3, 1}, 8: {4, 1}},
	{5: {3, 1}, 6: {3, 1}, 7: {4, 2}, 8: {5, 2}},
	{5: {3, 1}, 6: {4, 1}, 7: {4, 1}, 8: {5, 1}},
}

func checkOrigin(r *http.Request) bool {
	return true
}

type Session struct {
	id      string
	admin   *Player
	setup   Setup
	roles   Roles
	state   State
	players []*Player
}

type Message struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type Connect struct {
	Session string `json:"session"`
	Name    string `json:"name"`
}

type TextMessage struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type Setup struct {
	Merlin   bool `json:"merlin"`
	Percival bool `json:"percival"`
	Morgana  bool `json:"morgana"`
	Mordred  bool `json:"mordred"`
	Oberon   bool `json:"oberon"`
}

type Roles struct {
	Merlin   string
	Percival string
	Assassin string
	Morgana  string
	Mordred  string
	Oberon   string
	Rebels   []string
	Spies    []string
}

type State struct {
	Picker    int
	Quests    []*Quest
	Successes int
	Fails     int
}

type Quest struct {
	Leader    string
	Members   []string
	Approvals map[string]bool
	Successes map[string]bool
}

type Player struct {
	ID   string
	Role string
	Conn *websocket.Conn
}

func client(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()

	var clientPlayer Player
	var session *Session
	for {
		_, request, err := c.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			return
		}

		var message Message
		err = json.Unmarshal(request, &message)
		if err != nil {
			log.Printf("Error unmarshalling message: %s", err)
		}

		log.Printf("Parsed message: %s", message)

		if message.Type == "host" {
			var connect Connect
			err = json.Unmarshal(message.Data, &connect)
			if err != nil {
				log.Printf("Error unmarshalling host connect: %s", err)
			}

			if _, ok := sessionInstances[connect.Session]; ok {
				websocket.WriteJSON(c, TextMessage{Type: "text", Text: fmt.Sprintf("Error creating session %s: already exists", connect.Session)})
				return
			}

			clientPlayer = Player{ID: connect.Name, Conn: c}

			session = &Session{id: connect.Session, admin: &clientPlayer}
			session.players = append(session.players, &clientPlayer)
			sessionInstances[connect.Session] = session

			websocket.WriteJSON(c, TextMessage{Type: "text", Text: fmt.Sprintf("Session %s successfully created.", connect.Session)})

			updatePlayerList(connect.Session)
		} else if message.Type == "join" {
			var connect Connect
			err = json.Unmarshal(message.Data, &connect)
			if err != nil {
				log.Printf("Error unmarshalling join connect: %s", err)
			}

			if _, ok := sessionInstances[connect.Session]; !ok {
				websocket.WriteJSON(c, TextMessage{Type: "text", Text: fmt.Sprintf("Error joining session %s: does not exist", connect.Session)})
				return
			}

			clientPlayer = Player{ID: connect.Name, Conn: c}
			session = sessionInstances[connect.Session]
			if len(session.players) == 10 {
				websocket.WriteJSON(c, TextMessage{Type: "text", Text: "Session already has maximum number of players"})
				return
			}
			session.players = append(session.players, &clientPlayer)

			websocket.WriteJSON(c, TextMessage{Type: "text", Text: fmt.Sprintf("Joined session %s successfully", connect.Session)})

			updatePlayerList(connect.Session)
		} else if message.Type == "setup" && session != nil && session.admin == &clientPlayer {
			if len(session.players) < 5 {
				websocket.WriteJSON(c, TextMessage{Type: "text", Text: "Need at least 5 players to start"})
				continue
			}
			var setup Setup
			err = json.Unmarshal(message.Data, &setup)
			if err != nil {
				log.Printf("Error unmarshalling setup: %s", err)
			}

			rand.Shuffle(len(session.players), func(i, j int) {
				session.players[i], session.players[j] = session.players[j], session.players[i]
			})

			var roles Roles

			var knownSpies []string
			for i, player := range session.players {
				if i < resistanceToSpies[len(session.players)] {
					if setup.Merlin == true && roles.Assassin == "" {
						player.Role = "Assassin"
						roles.Assassin = player.ID
						knownSpies = append(knownSpies, player.ID)
					} else if setup.Morgana == true && roles.Morgana == "" {
						player.Role = "Morgana"
						roles.Morgana = player.ID
						knownSpies = append(knownSpies, player.ID)
					} else if setup.Mordred == true && roles.Mordred == "" {
						player.Role = "Mordred"
						roles.Mordred = player.ID
						knownSpies = append(knownSpies, player.ID)
					} else if setup.Oberon == true && roles.Oberon == "" {
						player.Role = "Oberon"
						roles.Oberon = player.ID
					} else {
						player.Role = "Spy"
						knownSpies = append(knownSpies, player.ID)
					}
					roles.Spies = append(roles.Spies, player.ID)
				} else {
					if setup.Merlin == true && roles.Merlin == "" {
						player.Role = "Merlin"
						roles.Merlin = player.ID
					} else if setup.Percival == true && roles.Percival == "" {
						player.Role = "Percival"
						roles.Percival = player.ID
					} else {
						player.Role = "Resistance"
					}
					roles.Rebels = append(roles.Rebels, player.ID)
				}
			}

			log.Printf("Game state: %+v", roles)
			log.Printf("Player state: %+v", session)

			for _, player := range session.players {
				var playerRoleInfo string
				if roles.Merlin == player.ID {
					var seenSpies []string
					for _, spy := range roles.Spies {
						if spy != roles.Mordred {
							seenSpies = append(seenSpies, spy)
						}
					}
					playerRoleInfo = fmt.Sprintf("You are Merlin. The following players are spies:\n%s", strings.Join(seenSpies, "\n"))
				} else if roles.Percival == player.ID {
					playerRoleInfo = fmt.Sprintf("You are Percival. The following player is Merlin:\n%s", roles.Merlin)
				} else if roles.Morgana == player.ID {
					playerRoleInfo = fmt.Sprintf("You are Morgana. The following players are spies:\n%s", strings.Join(knownSpies, "\n"))
				} else if roles.Mordred == player.ID {
					playerRoleInfo = fmt.Sprintf("You are Mordred. The following players are spies:\n%s", strings.Join(knownSpies, "\n"))
				} else if roles.Oberon == player.ID {
					playerRoleInfo = fmt.Sprintf("You are Oberon.")
				} else if roles.Percival == player.ID {
					playerRoleInfo = fmt.Sprintf("You are Mordred. The following players are spies:\n%s", strings.Join(knownSpies, "\n"))
				} else if player.Role == "Assassin" {
					playerRoleInfo = fmt.Sprintf("You are the Assassin. The following players are spies:\n%s", strings.Join(knownSpies, "\n"))
				} else if player.Role == "Spy" {
					playerRoleInfo = fmt.Sprintf("You are a Spy. The following players are spies:\n%s", strings.Join(knownSpies, "\n"))
				} else if player.Role == "Resistance" {
					playerRoleInfo = fmt.Sprintf("You are a Resistance member.")
				}

				websocket.WriteJSON(player.Conn, TextMessage{Type: "text", Text: playerRoleInfo})

				session.setup = setup
				session.roles = roles
			}

			for i, player := range session.players {
				err = websocket.WriteJSON(player.Conn, TextMessage{Type: "text", Text: fmt.Sprintf("Game Setup: %+v", setup)})
				if err != nil {
					log.Printf("Error writing to player %s: %s", player.ID, err)
					session.players = append(session.players[:i], session.players[i+1:]...)
					if player.Conn == c {
						return
					}
				}
			}

			session.state.Picker = rand.Int() % len(session.players)
			picker := session.players[session.state.Picker]

			quest := Quest{
				Members:   make([]string, roundRules[len(session.state.Quests)][len(session.players)][0]),
				Approvals: make(map[string]bool),
				Successes: make(map[string]bool),
			}
			session.state.Quests = append(session.state.Quests, &quest)
			for _, player := range session.players {
				websocket.WriteJSON(player.Conn, TextMessage{Type: "text", Text: fmt.Sprintf("It is %s's turn to pick a team of %d", picker.ID, len(quest.Members))})
			}
		} else if message.Type == "pick" {
			picker := session.players[session.state.Picker]
			if picker.ID != clientPlayer.ID {
				websocket.WriteJSON(c, TextMessage{Type: "text", Text: fmt.Sprintf("It is not your turn to pick a team. Wait for %s to pick a team.", picker.ID)})
				continue
			}
			var picks []string
			err = json.Unmarshal(message.Data, &picks)
			if err != nil {
				log.Printf("Error unmarshalling picks: %s", err)
			}

			currentQuest := session.state.Quests[len(session.state.Quests)-1]
			if len(picks) != len(currentQuest.Members) {
				websocket.WriteJSON(c, TextMessage{Type: "text", Text: fmt.Sprintf("You picked %d members. Please pick %d instead.", len(picks), len(currentQuest.Members))})
				continue
			}
			for i, pick := range picks {
				currentQuest.Members[i] = pick
			}
			for _, player := range session.players {
				websocket.WriteJSON(player.Conn, TextMessage{Type: "text", Text: fmt.Sprintf("%s has picked %s to go on the next quest. Waiting for everyone to vote.", picker.ID, strings.Join(currentQuest.Members, ", "))})
			}
		} else if message.Type == "approve" || message.Type == "reject" {
			currentQuest := session.state.Quests[len(session.state.Quests)-1]
			if currentQuest.Leader != "" || currentQuest.Members[0] == "" {
				websocket.WriteJSON(c, TextMessage{Type: "text", Text: "No team is being voted on."})
				continue
			}

			if message.Type == "approve" {
				currentQuest.Approvals[clientPlayer.ID] = true
			} else {
				currentQuest.Approvals[clientPlayer.ID] = false
			}
			websocket.WriteJSON(c, TextMessage{Type: "text", Text: "Your team vote was registered"})

			for _, player := range session.players {
				websocket.WriteJSON(player.Conn, TextMessage{Type: "text", Text: fmt.Sprintf("%s has put in a vote for the team.", clientPlayer.ID)})
			}

			if len(currentQuest.Approvals) == len(session.players) {
				var (
					voteResults []string
					approvals   int
					rejections  int
				)
				for playerID, approval := range currentQuest.Approvals {
					var vote string
					if approval {
						vote = "approve"
						approvals++
					} else {
						vote = "reject"
						rejections++
					}
					voteResults = append(voteResults, fmt.Sprintf("%s: %s", playerID, vote))
				}
				for _, player := range session.players {
					websocket.WriteJSON(player.Conn, TextMessage{Type: "text", Text: fmt.Sprintf("All players have voted as follows:<br>%s", strings.Join(voteResults, "<br>"))})
				}

				if approvals > rejections {
					currentQuest.Leader = session.players[session.state.Picker].ID
					for _, player := range session.players {
						websocket.WriteJSON(player.Conn, TextMessage{Type: "text", Text: fmt.Sprintf("The team was approved. They will now go on a quest.")})
					}
				} else {
					session.state.Picker = (session.state.Picker + 1) % len(session.players)
					picker := session.players[session.state.Picker]
					currentQuest.Members = make([]string, len(currentQuest.Members))
					currentQuest.Approvals = make(map[string]bool)
					for _, player := range session.players {
						websocket.WriteJSON(player.Conn, TextMessage{Type: "text", Text: fmt.Sprintf("It is now %s's turn to pick a team of %d", picker.ID, len(currentQuest.Members))})
					}
				}
			}
		} else if message.Type == "success" || message.Type == "fail" {
			currentQuest := session.state.Quests[len(session.state.Quests)-1]

			if currentQuest.Leader == "" {
				websocket.WriteJSON(c, TextMessage{Type: "text", Text: "A team has not been picked yet for this round."})
				continue
			}

			onQuest := false
			for _, questMember := range currentQuest.Members {
				if clientPlayer.ID == questMember {
					onQuest = true
					if message.Type == "success" {
						currentQuest.Successes[clientPlayer.ID] = true
					} else {
						currentQuest.Successes[clientPlayer.ID] = false
					}
					websocket.WriteJSON(c, TextMessage{Type: "text", Text: "Your quest vote was registered"})

					if len(currentQuest.Successes) == len(currentQuest.Members) {
						var (
							successes   int
							fails       int
							failsNeeded int
						)

						failsNeeded = roundRules[len(session.state.Quests)][len(session.players)][1]

						for _, success := range currentQuest.Successes {
							if success {
								successes++
							} else {
								fails++
							}
						}

						if fails >= failsNeeded {
							session.state.Fails++
							for _, player := range session.players {
								websocket.WriteJSON(player.Conn, TextMessage{Type: "text", Text: fmt.Sprintf("The quest failed with the following results:<br>%d success(es)<br>%d fail(s)", successes, fails)})
							}
						} else {
							session.state.Successes++
							for _, player := range session.players {
								websocket.WriteJSON(player.Conn, TextMessage{Type: "text", Text: fmt.Sprintf("The quest succeeded with the following results:<br>%d success(es)", successes)})
							}
						}

						if session.state.Successes == 1 {
							for _, player := range session.players {
								websocket.WriteJSON(player.Conn, TextMessage{Type: "text", Text: fmt.Sprintf("3 quests have failed. The forces of evil have won.")})
								player.Conn.Close()
							}

							delete(sessionInstances, session.id)
						} else if session.state.Fails == 1 {
							for _, player := range session.players {
								websocket.WriteJSON(player.Conn, TextMessage{Type: "text", Text: fmt.Sprintf("3 quests have succeeded. The forces of good have won.")})
								player.Conn.Close()
							}

							delete(sessionInstances, session.id)
						} else {
							nextQuest := Quest{
								Members:   make([]string, roundRules[len(session.state.Quests)+1][len(session.players)][0]),
								Approvals: make(map[string]bool),
								Successes: make(map[string]bool),
							}
							session.state.Quests = append(session.state.Quests, &nextQuest)

							session.state.Picker = (session.state.Picker + 1) % len(session.players)
							picker := session.players[session.state.Picker]
							for _, player := range session.players {
								websocket.WriteJSON(player.Conn, TextMessage{Type: "text", Text: fmt.Sprintf("It is now %s's turn to pick a team of %d", picker.ID, len(nextQuest.Members))})
							}
						}
					}
				}
			}

			if !onQuest {
				websocket.WriteJSON(c, TextMessage{Type: "text", Text: "You are not on the quest."})
			}
		} else {
			websocket.WriteJSON(c, TextMessage{Type: "text", Text: fmt.Sprintf("Action '%s' not allowed", message.Type)})
		}
	}
}

func updatePlayerList(sessionID string) {
	var playerList []string
	log.Printf("Updating players %+v", sessionInstances[sessionID])
	for _, player := range sessionInstances[sessionID].players {
		playerList = append(playerList, player.ID)
	}

	for _, player := range sessionInstances[sessionID].players {
		websocket.WriteJSON(player.Conn, struct {
			Type    string   `json:"type"`
			Players []string `json:"players"`
		}{
			Type:    "players",
			Players: playerList,
		})
	}
}

func main() {
	rand.Seed(time.Now().UTC().UnixNano())
	flag.Parse()
	log.SetFlags(0)
	http.HandleFunc("/client", client)
	http.HandleFunc("/", home)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func home(w http.ResponseWriter, r *http.Request) {
	clientHTML, err := ioutil.ReadFile("client.html")
	if err != nil {
		log.Printf("Error reading clinet HTML file: %s", err)
	}
	homeTemplate := template.Must(template.New("").Parse(string(clientHTML)))
	homeTemplate.Execute(w, "ws://"+r.Host+"/echo")
}
