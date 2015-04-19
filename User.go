package irc

import (
	"code.google.com/p/go-uuid/uuid"
)

type User struct {
	Name     string
	Uname    string
	Rname    string
	Msgs     <-chan *Message
	inMsgs   MChannel
	server   *Server
	handlers map[string]HandlerFunc
}

func (s *Server) NewUser(nname, uname string) *User {
	this := new(User)
	this.Name = nname
	this.Uname = uname
	this.Rname = ""
	this.inMsgs = make(chan *Message)
	this.Msgs = this.inMsgs.Queue()
	this.server = s
	this.handlers = make(map[string]HandlerFunc)
	s.Users[nname] = this
	this.AddHandler("nicks", func(m *Message) {
		if m.Type == NICK {
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

func (this *User) AddHandler(name string, f HandlerFunc) string {
	if name == "" {
		name = uuid.New()
	}
	this.handlers[name] = f
	return name
}

func (this *User) RemHandler(name string) {
	delete(this.handlers, name)
}

func (this *User) Say(s string) {
	this.server.PrivMsg(this, s)
}

func (this *User) Write(p []byte) (int, error) {
	return this.server.Write(append([]byte("PRIVMSG "+this.Name+" :"), p...))
}

func (this *User) String() string {
	if this == nil {
		return ""
	}
	return this.Name
}

func (this *User) Ident() string {
	if this == nil {
		return ""
	}
	return this.Name
}
