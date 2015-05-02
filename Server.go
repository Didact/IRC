package irc

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

type HandlerFunc func(*Message)

//go:generate goast write impl $gen/Queue.go
type MChannel chan *Message

type Server struct {
	Name     string
	Users    map[string]*User
	Channels map[string]*Channel
	Msgs     MChannel
	inMsgs   MChannel //Used internally
	readMsgs chan *Message
	ip       string
	conn     net.Conn
	Prof     *Profile
	Me       *User
	handlers []HandlerFunc
	Handlers map[string]HandlerFunc
	c        <-chan time.Time
	old      bool
	All      io.Reader
}

type all struct {
	s *Server
	c chan *Message
}

func NewServer(name, ip string, prof *Profile) *Server {
	s := &Server{
		Name:     name,
		Users:    make(map[string]*User),
		Channels: make(map[string]*Channel),
		Handlers: make(map[string]HandlerFunc),
		inMsgs:   make(chan *Message),
		readMsgs: make(chan *Message),
		ip:       ip,
		conn:     nil,
		Prof:     prof,
		c:        time.Tick(100 * time.Millisecond),
	}

	s.Msgs = s.inMsgs.Queue()
	s.Me = s.NewUser("~me", "~me")
	s.All = &all{s, make(chan *Message)}
	s.handlers = s.defaultHandlers()
	return s
}

func (s *Server) Start() {
	var err error
	s.conn, err = net.Dial("tcp", s.ip)
	if err != nil {
		fmt.Println("***", err)
		defer s.Restart()
		<-time.After(5 * time.Second)
		return
	}
	go func() {
		<-time.After(1 * time.Second)
		s.Nick(s.Prof.nname)
		s.User(s.Prof.uname, s.Prof.rname)
		<-time.After(2 * time.Second)
		s.Join(s.Prof.autos...)
	}()
	go func() {
		var i int
		for {
			<-time.After(10 * time.Second)
			s.conn.Write([]byte(fmt.Sprintf("PING %d\r\n", i)))
			i++
			if i == 2000000000 {
				i = 0
			}
		}
	}()
	buf := make([]byte, 512)
	go func() {
		for {
			n, err := s.conn.Read(buf)
			if err != nil {
				fmt.Println("***", err)
				defer s.Restart()
				<-time.After(5 * time.Second)
				return
			}
			strs := strings.Split(string(buf[:n]), "\r\n")
			strs = strs[:len(strs)-1]
			for _, str := range strs {
				m := s.NewMessage(str)
				if m != nil {
					for _, f := range s.Handlers {
						f(m)
					}
				}
			}
		}
	}()
}

func (s *Server) Restart() {
	s.old = true
	defer func() { s.old = false }()
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
	wg := &sync.WaitGroup{}
	//Empty as much of as many chans as possible
	emptyChan := func(c <-chan *Message) {
		wg.Add(1)
		for {
			select {
			case <-c:
			default:
				wg.Done()
				return
			}
		}
	}
	go emptyChan(s.Msgs)
	go emptyChan(s.inMsgs)
	for _, c := range s.Channels {
		c.old = true
		go emptyChan(c.inMsgs)
		go emptyChan(c.Msgs)
	}
	for _, u := range s.Users {
		u.old = true
		go emptyChan(u.inMsgs)
		go emptyChan(u.Msgs)
	}
	wg.Wait()
	s.Start()
}

func (s *Server) Nick(str string) {
	s.Say("NICK " + str)
}

func (s *Server) User(uname, rname string) {
	s.Say(fmt.Sprintf("USER %s 0 * :%s", uname, rname))
}

func (s *Server) Join(chans ...string) []*Channel {
	wg := &sync.WaitGroup{}
	if len(chans) < 1 {
		return nil
	}
	channels := make([]*Channel, len(chans))
	for _, str := range chans {
		s.Say("JOIN " + str)
		c := s.NewChannel(str)
		c.AddHandler("wait", func(m *Message) {
			if m.Type == RPL_NAMREPLY {
				wg.Done()
				c.RemHandler("wait")
			}
		})
		channels = append(channels, c)
		wg.Add(1)
	}
	wg.Wait()
	return channels
}

func (s *Server) Read(p []byte) (int, error) {
	return copy(p, []byte((<-s.readMsgs).String())), nil
}

func (a *all) Read(p []byte) (int, error) {
	return copy(p, []byte((<-a.c).String())), nil
}

func (s *Server) Write(p []byte) (int, error) {
	if len(p) > 510 {
		p = p[:511]
	}
	<-s.c
	return s.conn.Write(append(p, "\r\n"...))
}

func (s *Server) Say(str string) {
	s.Write([]byte(str))
}

func (s *Server) defaultHandlers() []HandlerFunc {
	pings := func(m *Message) {
		if m.Type == PING {
			s.Say("PONG " + m.Text)
		}
	}
	chans := func(m *Message) {
		if c, ok := m.Destination.(*Channel); ok {
			if c == nil {
				fmt.Println("***NIL CHANNEL")
				return
			}
			c.inMsgs <- m
		}
	}
	server := func(m *Message) {
		if m.Source == s {
			s.inMsgs <- m
		}
	}
	users := func(m *Message) {
		u, ok := m.Source.(*User)
		if ok && m.Destination == s.Me { //Iff the message is going to us
			u.inMsgs <- m
		}
	}
	nicks := func(m *Message) {
		if m.Type == NICK {
			u := m.Source.(*User)
			u.Name = m.Text
		}
	}
	quits := func(m *Message) {
		if m.Type == QUIT {
			for _, c := range s.Channels {
				t := new(Message)
				*t = *m
				t.Type = PART
				t.Destination = c
				c.inMsgs <- t
			}
			delete(s.Users, m.Source.Ident())
		}
	}
	reads := func(m *Message) {
		s.readMsgs <- m
	}
	allReads := func(m *Message) {
		s.All.(*all).c <- m
	}
	return []HandlerFunc{pings, chans, server, users, nicks, quits, reads, allReads}
}

func (s *Server) addHandler(f HandlerFunc) {
	s.handlers = append(s.handlers, f)
}

func (s *Server) AddHandler(name string, f HandlerFunc) string {
	if name == "" {
		name = uuid.New()
	}
	s.Handlers[name] = f
	return name
}

func (s *Server) RemHandler(name string) {
	delete(s.Handlers, name)
}

func (s *Server) PrivMsg(t Target, str string) {
	s.Say(fmt.Sprintf("PRIVMSG %s :%s", t.Ident(), str))
}

func (s *Server) Notice(t Target, str string) {
	s.Say(fmt.Sprintf("NOTICE %s :%s", t.Ident(), str))
}

func (s *Server) Topic(c *Channel, str string) {
	s.Say(fmt.Sprintf("TOPIC %s :%s", c.Name, str))
}

func (s *Server) String() string {
	if s == nil {
		return ""
	}
	return s.Name
}

func (s *Server) Ident() string {
	if s == nil {
		return ""
	}
	return s.Name
}

func (s *Server) Old() bool {
	return s.old
}
