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

// rounds to number of players to number of team members to number of fails needed
var roundRules = []map[int][]int{
	{5: {2, 1}, 6: {2, 1}, 7: {2, 1}, 8: {3, 1}},
	{5: {3, 1}, 6: {3, 1}, 7: {3, 1}, 8: {4, 1}},
	{5: {2, 1}, 6: {4, 1}, 7: {3, 1}, 8: {4, 1}},
	{5: {3, 1}, 6: {3, 1}, 7: {4, 2}, 8: {5, 2}},
	{5: {3, 1}, 6: {4, 1}, 7: {4, 1}, 8: {5, 1}},
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

/* TODO: Handle player disconnects gracefully
for i, player := range session.Players {
	err = websocket.WriteJSON(player.Conn, TextMessage{Type: "text", Text: fmt.Sprintf("Game Setup: %+v", setup)})
	if err != nil {
		log.Printf("Error writing to player %s: %s", player.ID, err)
		session.Players = append(session.players[:i], session.players[i+1:]...)
		if player.Conn == c {
			return
		}
	}
}
*/

func main() {
	rand.Seed(time.Now().UTC().UnixNano())
	addr := flag.String("addr", "localhost:8080", "http service address")
	flag.Parse()
	log.SetFlags(log.LstdFlags)
	http.HandleFunc("/", home)
	http.HandleFunc("/client", client)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func home(w http.ResponseWriter, r *http.Request) {
	clientHTML, err := ioutil.ReadFile("client.html")
	if err != nil {
		log.Printf("Error reading client HTML file: %s", err)
	}
	homeTemplate := template.Must(template.New("").Parse(string(clientHTML)))
	homeTemplate.Execute(w, "")
}

func client(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()

	clientPlayer := &Player{Conn: c}
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
			continue
		}

		log.Printf("Parsed message: %s", message)

		if message.Type == "host" {
			var connect Connect
			err = json.Unmarshal(message.Data, &connect)
			if err != nil {
				log.Printf("Error unmarshalling host connect: %s", err)
				return
			}

			session, err = clientPlayer.Host(connect.Session, connect.Name)
			if err != nil {
				log.Printf("Error hosting session by Player %s: %s", clientPlayer.ID, err)
				return
			}
		} else if message.Type == "join" {
			var connect Connect
			err = json.Unmarshal(message.Data, &connect)
			if err != nil {
				log.Printf("Error unmarshalling join connect: %s", err)
				return
			}

			session, err = clientPlayer.Join(connect.Session, connect.Name)
			if err != nil {
				log.Printf("Error joining session by Player %s: %s", clientPlayer.ID, err)
				return
			}
		} else if message.Type == "start" && session != nil && session.Admin == clientPlayer {
			if len(session.Players) < 5 {
				clientPlayer.SendText("Need at least 5 players to start")
				continue
			}

			var setup Setup
			err = json.Unmarshal(message.Data, &setup)
			if err != nil {
				log.Printf("Error unmarshalling setup: %s", err)
				continue
			}

			err = session.Start(setup)
			if err != nil {
				log.Printf("Error starting session: %s", err)
				continue
			}

			log.Printf("Session %s started with roles: %+v", session.ID, session.Roles)
		} else if message.Type == "pick" {
			var picks []string
			err = json.Unmarshal(message.Data, &picks)
			if err != nil {
				log.Printf("Error unmarshalling picks: %s", err)
				continue
			}

			currentQuest := session.State.Quests[len(session.State.Quests)-1]
			err = clientPlayer.Pick(session, currentQuest, picks)
			if err != nil {
				log.Printf("Error picking team by Player %s: %s", clientPlayer.ID, err)
				continue
			}

			session.SendGlobalText(fmt.Sprintf("%s has picked %s to go on the next quest. Waiting for everyone to vote on new team.", clientPlayer.ID, strings.Join(currentQuest.Members, ", ")))
		} else if message.Type == "approve" || message.Type == "reject" {
			currentQuest := session.State.Quests[len(session.State.Quests)-1]

			if message.Type == "approve" {
				clientPlayer.VoteForTeam(session, currentQuest, true)
			} else {
				clientPlayer.VoteForTeam(session, currentQuest, false)
			}
		} else if message.Type == "success" || message.Type == "fail" {
			currentQuest := session.State.Quests[len(session.State.Quests)-1]

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

						failsNeeded = roundRules[len(session.State.Quests)][len(session.Players)][1]

						for _, success := range currentQuest.Successes {
							if success {
								successes++
							} else {
								fails++
							}
						}

						if fails >= failsNeeded {
							session.State.Fails++
							for _, player := range session.Players {
								websocket.WriteJSON(player.Conn, TextMessage{Type: "text", Text: fmt.Sprintf("The quest failed with the following results:<br>%d success(es)<br>%d fail(s)", successes, fails)})
							}
						} else {
							session.State.Successes++
							for _, player := range session.Players {
								websocket.WriteJSON(player.Conn, TextMessage{Type: "text", Text: fmt.Sprintf("The quest succeeded with the following results:<br>%d success(es)", successes)})
							}
						}

						if session.State.Successes == 1 {
							for _, player := range session.Players {
								websocket.WriteJSON(player.Conn, TextMessage{Type: "text", Text: fmt.Sprintf("3 quests have failed. The forces of evil have won.")})
								player.Conn.Close()
							}

							delete(sessionInstances, session.ID)
						} else if session.State.Fails == 1 {
							for _, player := range session.Players {
								websocket.WriteJSON(player.Conn, TextMessage{Type: "text", Text: fmt.Sprintf("3 quests have succeeded. The forces of good have won.")})
								player.Conn.Close()
							}

							delete(sessionInstances, session.ID)
						} else {
							nextQuest := Quest{
								Members:   make([]string, roundRules[len(session.State.Quests)+1][len(session.Players)][0]),
								Approvals: make(map[string]bool),
								Successes: make(map[string]bool),
							}
							session.State.Quests = append(session.State.Quests, &nextQuest)

							session.State.Picker = (session.State.Picker + 1) % len(session.Players)
							picker := session.Players[session.State.Picker]
							for _, player := range session.Players {
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
