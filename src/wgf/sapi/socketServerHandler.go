package sapi

import (
	"net"
	"os"
	"time"
)

type SocketHandlerInfo struct {
	KeepAlived bool
	FirstRequest bool
	Ext map[string]interface{}
}

func NewSocketHandlerInfo() *SocketHandlerInfo {
	return &SocketHandlerInfo {
		KeepAlived : true,
		FirstRequest : true,
		Ext : map[string]interface{}{},
	}
}

type SocketServerHandler struct {
	disabled bool

	pServer *Server

	//listener
	Ln net.Listener

	maxChildren int64
	currentChildren int64
}

func (p *SocketServerHandler) Shutdown() chan bool{
	c := make(chan bool)
	go p.shutdownWorker(c)
	return c
}

func (p *SocketServerHandler) shutdownWorker(c chan bool){
	p.disabled = true
	for p.currentChildren > 0 {
		p.pServer.Logger.Infof(
			"wait for currentChildren stop, remains %d. use [ kill -9 %d ] if you want to kill it at once.",
			p.currentChildren,
			os.Getpid(),
		)
		time.Sleep(1*time.Second)
	}
	p.Ln.Close()
	c<-true
}


func (p *SocketServerHandler) Serve(pServer *Server) {

	//notifyHandlerFinished
	defer pServer.NotifyHandlerFinished()

	p.pServer = pServer
	p.maxChildren = pServer.Conf.Int64("wgf.sapi.maxChildren", 1000)

	var err error
	lnet := pServer.Conf.String("wgf.sapi.ListenNet", "")
	laddr := pServer.Conf.String("wgf.sapi.ListenLaddr", "")

	p.Ln, err = net.Listen(lnet, laddr)
	if nil != err {
		pServer.Logger.Fatalf("cannot listen to %s[%s], error: %s", lnet, laddr, err.Error())
		return //exit
	}

	var conn net.Conn
	for true {
		if p.disabled {
			break
		}

		conn, err = p.Ln.Accept()
		if nil != err {
			pServer.Logger.Warningf("accept_error %s", err)
			continue
		}
		go p.serveRequest(conn)
	}
}

func (p *SocketServerHandler) serveRequest(conn net.Conn) {

	//close the conn
	defer conn.Close()
	var err error

	handlerInfo := NewSocketHandlerInfo()

	for true {

		if p.disabled {
			break
		}

		if p.currentChildren >= p.maxChildren {
			p.pServer.Logger.Warningf("currentChildren has reached %d, please raise the wgf.sapi.maxChildren", p.currentChildren)
			return
		}

		p.currentChildren++
		err = p.serveRequestEx(conn, handlerInfo)
		p.currentChildren--

		handlerInfo.FirstRequest = false
		if nil!=err || !handlerInfo.KeepAlived {
			p.pServer.Logger.Debug(err)
			break
		}
	}
}

func (p *SocketServerHandler) serveRequestEx(conn net.Conn, handlerInfo *SocketHandlerInfo) error {
	sapi := NewSocketSapi(p.pServer, conn)
	defer sapi.Close()

	sapi.HandlerInfo = handlerInfo

	var err error
	c := make(chan int)
	go func(){err = sapi.start(c)}()
	<-c
	return err
}
