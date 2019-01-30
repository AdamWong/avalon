package main

import (
	"fmt"
	"strings"

	"github.com/gorilla/websocket"
)

type Player struct {
	ID   string
	Role string
	Conn *websocket.Conn
}

func (p *Player) SendText(message string) {
	websocket.WriteJSON(p.Conn, TextMessage{Type: "text", Text: message})
}

func (p *Player) Host(sessionID string, name string) (*Session, error) {
	if _, ok := sessionInstances[sessionID]; ok {
		p.SendText(fmt.Sprintf("Error creating session %s: already exists", sessionID))
		return nil, fmt.Errorf("Player %s tried creating a session that already exists", name)
	}

	p.ID = name

	session := Session{ID: sessionID, Admin: p, Players: []*Player{p}}
	sessionInstances[sessionID] = &session

	p.SendText(fmt.Sprintf("Session %s successfully created.", sessionID))

	session.UpdatePlayerList()

	return sessionInstances[sessionID], nil
}

func (p *Player) Join(sessionID string, name string) (*Session, error) {
	if _, ok := sessionInstances[sessionID]; !ok {
		p.SendText(fmt.Sprintf("Error joining session %s: does not exist", sessionID))
		return nil, fmt.Errorf("Player %s tried to join a non-existent session", name)
	}

	p.ID = name
	session := sessionInstances[sessionID]
	if len(session.Players) == 10 {
		p.SendText("Session already has maximum number of players")
		return nil, fmt.Errorf("Player %s tried to join a full session", name)
	}
	session.Players = append(session.Players, p)

	p.SendText(fmt.Sprintf("Joined session %s successfully", sessionID))

	session.UpdatePlayerList()

	return session, nil
}

func (p *Player) Pick(session *Session, quest *Quest, members []string) error {
	// TODO: Check that there is actually a team to pick
	picker := session.Players[session.State.Picker]
	if picker != p {
		p.SendText(fmt.Sprintf("It is not your turn to pick a team. Wait for %s to pick a team.", picker.ID))
		return fmt.Errorf("Not the team picker")
	}

	if len(members) != len(quest.Members) {
		p.SendText(fmt.Sprintf("You picked %d members. Please pick %d instead.", len(members), len(quest.Members)))
		return fmt.Errorf("Incorrect number of team members picked")
	}

	for i, member := range members {
		quest.Members[i] = member
	}

	quest.Approvals = make(map[string]bool)

	return nil
}

func (p *Player) VoteForTeam(session *Session, quest *Quest, approval bool) error {
	if quest == nil || quest.Approvals == nil {
		p.SendText("No team is being voted on.")
		return fmt.Errorf("No team is being voted on.")
	}

	quest.Approvals[p.ID] = approval

	p.SendText("Your team vote was registered")

	session.SendGlobalText(fmt.Sprintf("%s has put in a vote for the team.", p.ID))

	if len(quest.Approvals) == len(session.Players) {
		var (
			voteResults []string
			approvals   int
			rejections  int
		)

		for playerID, approval := range quest.Approvals {
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
		session.SendGlobalText(fmt.Sprintf("All players have voted as follows:<br>%s", strings.Join(voteResults, "<br>")))

		if approvals > rejections {
			quest.Leader = session.Players[session.State.Picker].ID
			session.SendGlobalText(fmt.Sprintf("The team was approved. They will now go on a quest."))
		} else {
			// TODO: Automatically fail if we've gone through 5 pickers for a quest
			session.State.Picker = (session.State.Picker + 1) % len(session.Players)
			picker := session.Players[session.State.Picker]
			for i, _ := range quest.Members {
				quest.Members[i] = ""
			}
			session.SendGlobalText(fmt.Sprintf("The team was rejected. It is now %s's turn to pick a team of %d", picker.ID, len(quest.Members)))
		}
	}
	return nil
}
