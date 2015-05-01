package irc

import (
	"code.google.com/p/go-uuid/uuid"
	//"my"
	"strings"
)

type Channel struct {
	Name     string
	server   *Server
	Topic    string
	Modes    []byte
	users    map[string]*User
	inMsgs   MChannel
	Msgs     <-chan *Message
	handlers []HandlerFunc
	Handlers map[string]HandlerFunc
	old      bool
}

func (s *Server) NewChannel(name string) *Channel {
	c := &Channel{
		Name:     name,
		server:   s,
		Modes:    []byte{},
		users:    make(map[string]*User),
		inMsgs:   MChannel(make(chan *Message)),
		Handlers: make(map[string]HandlerFunc),
	}

	joins := func(m *Message) {
		if m.Type == JOIN {
			u := m.Source.(*User)
			c.users[u.Name] = u
		}
	}
	parts := func(m *Message) {
		if m.Type == PART {
			u := m.Source.(*User)
			delete(c.users, u.Name)
		}
	}
	topics := func(m *Message) {
		if m.Type == TOPIC {
			c.Topic = m.Text
		}
	}
	rpl_namereplies := func(m *Message) {
		if m.Type == RPL_NAMREPLY {
			for _, name := range strings.Fields(m.Text) {
				m2 := s.NewMessage(name + " JOIN " + c.Name)
				m2.Extra = []byte{1}
				joins(m2)
			}
		}
	}

	c.handlers = []HandlerFunc{joins, parts, topics, rpl_namereplies}
	c.Msgs = c.inMsgs.Queue()

	go func() {
		for m := range c.Msgs {
			for _, f := range c.handlers {
				f(m)
			}
			for _, f := range c.Handlers {
				f(m)
			}
		}
	}()
	if c2, ok := s.Channels[name]; ok {
		*c2 = *c
	} else {
		s.Channels[name] = c
	}
	return c
}

func (c *Channel) addHandler(f HandlerFunc) {
	c.handlers = append(c.handlers, f)
}

func (this *Channel) AddHandler(name string, f HandlerFunc) string {
	if name == "" {
		name = uuid.New()
	}
	this.Handlers[name] = f
	return name
}

func (this *Channel) RemHandler(name string) {
	delete(this.Handlers, name)
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

func (c *Channel) Read(p []byte) (int, error) {
	return copy(p, []byte((<-c.Msgs).String())), nil
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

func (c *Channel) Users() map[string]*User {
	return c.users
}

func (c *Channel) Old() bool {
	return c.old
}
