package main

import (
    "log"
    "strings"
    "errors"
    "fmt"
    "time"

    "code.google.com/p/go.net/websocket"
)

var socketIds chan string
type Socket struct {
    SID    string  // socket ID, randomly generated
    UID    string  // User ID, passed in via client
    Page   string  // Current page, if set.
    
    ws        *websocket.Conn
    buff      chan *Message
    heartbeat <-chan time.Time
    done      chan bool
    Server    *Server
}

func init() {
    socketIds = make(chan string)
    
    go func() {
        var i = 1
        for {
            i++
            socketIds <- fmt.Sprintf("%v", i)
        }
    }()
}

func newSocket(ws *websocket.Conn, server *Server, UID string) *Socket {
    return &Socket{<-socketIds, UID, "", ws, make(chan *Message, 1000), time.After(20 * time.Second), make(chan bool), server}
}

func (this *Socket) Close() error {
    if this.done == nil {
        return nil
    }
    
    if this.Page != "" {
        this.Server.Store.UnsetPage(this)
        this.Page = ""
    }
    
    this.Server.Store.Remove(this)
    close(this.done)
    this.done = nil
    
    return nil
}

func (this *Socket) Authenticate() error {
    var message CommandMsg
    err := websocket.JSON.Receive(this.ws, &message)

    if DEBUG { log.Println(message.Command) }
    if err != nil {
        return err
    }
    
    command := message.Command["command"]
    if strings.ToLower(command) != "authenticate" {
        return errors.New("Error: Authenticate Expected.\n")
    }
    
    UID, ok := message.Command["user"]
    if !ok {
        return errors.New("Error on Authenticate: Bad Input.\n")
    }
    
    if DEBUG { log.Printf("saving UID as %s", UID) }
    
    this.UID = UID
    this.Server.Store.Save(this)
        
    return nil
}

func (this *Socket) listenForMessages() {
    for {
        
        select {
            case <- this.done:
                return
            
            default:
                var command CommandMsg
                err := websocket.JSON.Receive(this.ws, &command)
                if err != nil {
                    if DEBUG { log.Printf("Error: %s\n", err.Error()) }
                    
                    go this.Close()
                    return 
                }
                
                if DEBUG { log.Println(command) }
                go command.FromSocket(this)
        }
    }
}

func (this *Socket) listenForWrites() {
    for {
        select {
            case <-this.heartbeat:
                this.heartbeat = time.After(20 * time.Second)
                this.buff <- newHeartbeat(this.SID)
            
            case message := <-this.buff:
                if DEBUG { log.Println("Sending:", message) }
                if err := websocket.JSON.Send(this.ws, message); err != nil {
                    if DEBUG { log.Printf("Error: %s\n", err.Error()) }
                    go this.Close()
                    return
                }
                
            case <-this.done:
                return
        }
    }
}
