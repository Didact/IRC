package irc

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"strings"
)

type Channel struct {
	Name     string
	server   *Server
	Topic    string
	Modes    []byte
	Users    map[string]*User
	Msgs     <-chan *Message
	inMsgs   MChannel
	handlers map[string]HandlerFunc
}

func (s *Server) NewChannel(name string) *Channel {
	this := new(Channel)
	this.Name = name
	this.server = s
	this.Modes = []byte{}
	this.Users = make(map[string]*User)
	this.inMsgs = make(chan *Message)
	this.Msgs = this.inMsgs.Queue()
	this.handlers = make(map[string]HandlerFunc)
	this.AddHandler("joins", func(m *Message) {
		if m.Type == JOIN {
			u := m.Source.(*User)
			this.Users[u.Name] = u
			fmt.Println("User has joined: " + u.Name)
		}
	})
	this.AddHandler("parts", func(m *Message) {
		if m.Type == PART {
			u := m.Source.(*User)
			delete(this.Users, u.Name)
		}
	})
	this.AddHandler("topics", func(m *Message) {
		if m.Type == TOPIC {
			this.Topic = m.Text
		}
	})
	this.AddHandler("rpl_namreplys", func(m *Message) {
		if m.Type == RPL_NAMREPLY {
			for _, name := range strings.Fields(m.Text) {
				m2 := s.NewMessage(name + " JOIN " + this.Name)
				m2.Extra = []byte{1}
				this.inMsgs <- m2
			}
		}
	})
	go func() {
		for m := range this.Msgs {
			for _, f := range this.handlers {
				go f(m)
			}
		}
	}()
	return this
}

func (this *Channel) AddHandler(name string, f HandlerFunc) string {
	if name == "" {
		name = uuid.New()
	}
	this.handlers[name] = f
	return name
}

func (this *Channel) RemHandler(name string) {
	delete(this.handlers, name)
}

func (this *Channel) SetTopic(s string) {
	this.server.Topic(this, s)
}

func (this *Channel) Say(s string) {
	this.server.PrivMsg(this, s)
}

func (this *Channel) Write(p []byte) (int, error) {
	return this.server.Write(append([]byte("PRIVMSG "+this.Name+" :"), p...))
}

func (this *Channel) String() string {
	if this == nil {
		return ""
	}
	return this.Name
}

func (this *Channel) Ident() string {
	if this == nil {
		return ""
	}
	return this.Name
}
