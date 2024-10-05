package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

var addr = flag.String("addr", "localhost", "http service address")
var token = "FIND ME ON ai-arena.de"

func main() {
	var identified = false

	flag.Parse()
	log.SetFlags(0)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	u := url.URL{Scheme: "ws", Host: *addr, Path: "/api/live"}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			log.Printf("<<< %s", message)
			var parsed map[string]any
			json.Unmarshal([]byte(message), &parsed)

			log.Printf("recv: %s", parsed["type"])

			//Successful identification message ( Step 1 )
			if parsed["type"] == "identified" {
				log.Printf("Identifcation successful")
				identified = true
			}
			//Search for game ( Step 2 )
			if parsed["type"] == "open-lobbies" {
				log.Printf("Game list response")
				var gamelist = parsed["matches"]
				var castedArray = gamelist.([]interface{})
				if(len(castedArray) > 0){
					log.Printf("Game found, starting!")
					gameid := castedArray[0].(map[string]any)["match"]

					//Join game (Step 3, no answer)
					err := c.WriteMessage(websocket.TextMessage, []byte("{\"action\":\"join\", \"match\":\""+gameid.(string)+"\"}"))

					if err != nil {
						log.Println("write:", err)
						return
					}

				}
			}
			//Game update message ( Step 4 - #1.INF )
			if(parsed["type"] == "game-update"){
				if(parsed["activePlayer"] != nil){
					ap := int8(parsed["activePlayer"].(float64))
					slot := int8(parsed["slot"].(float64))
					if(ap == slot){
						state := parsed["state"].(map[string]any)
						board := state["board"].([]interface{})
						gi := parsed["gi"].(map[string]any)
						matchId := gi["match"].(string)
						done := false
						for col := 0; col < 7; col++{
							column := board[col].([]interface{})
							if(len(column) < 6 && !done){
								err := c.WriteMessage(websocket.TextMessage, []byte("{\"action\":\"action\",\"match\":\""+matchId+"\", \"payload\":{\"col\":"+strconv.Itoa(col)+"}}"))

								if err != nil {
									log.Println("write:", err)
									return
								}

								done = true
							}
						}
					}
				}
			}
		}
	}()

	ticker := time.NewTicker(time.Second)
	gameSearchTicker := time.NewTicker(5 * time.Second)
	pingTicker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
			//Step 1: Try identification after 1s
		case <-ticker.C:
			err := c.WriteMessage(websocket.TextMessage, []byte("{\"action\":\"identify\", \"token\":\""+token+"\"}"))
			ticker.Stop()

			if err != nil {
				log.Println("write:", err)
				return
			}
			//Keep WS alive
		case <-pingTicker.C:
			err := c.WriteMessage(websocket.TextMessage, []byte("PING"))

			if err != nil {
				log.Println("write:", err)
				return
			}
			//Step 2: Search for game
		case <-gameSearchTicker.C:
			if(identified){
				log.Println("Search for game")
				err := c.WriteMessage(websocket.TextMessage, []byte("{\"action\":\"match-list\", \"game\":\"Connect Four\"}"))

				if err != nil {
					log.Println("write:", err)
					return
				}
			} else {
				log.Println("Skipping game search because not identified")
			}
		case <-interrupt:
			log.Println("interrupt")

			// Cleanly close the connection by sending a close message and then
			// waiting (with timeout) for the server to close the connection.
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("write close:", err)
				return
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		}
	}
}

