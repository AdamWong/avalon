package main

import (
	"fmt"

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

	sessionInstances[sessionID] = &Session{ID: sessionID, Admin: p, Players: []*Player{p}}

	p.SendText(fmt.Sprintf("Session %s successfully created.", sessionID))

	updatePlayerList(sessionID)

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

	updatePlayerList(sessionID)

	return session, nil
}
